---
id: TASK-41
title: 同梱の埋め込みサイドカーで長いチャンクが拒否され、以後の埋め込みが黙って止まる不具合を直す
status: In Review
assignee: []
created_date: '2026-09-23 09:01'
updated_date: '2026-09-23 20:17'
labels: []
dependencies: []
references:
  - internal/embed/sidecar.go
  - internal/embed/manager.go
  - internal/service/embeddingclient.go
  - internal/service/embeddingsync.go
  - app.go
type: bug
ordinal: 41000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-38 の実機確認 (2026-09-23) で、開発 DB の新規ドキュメントに埋め込みが 1 件も作られていないことが分かった。TASK-38 以前に追加した `slides.md` も同じで、TASK-38 の変更とは独立した既存の不具合である。

1. **長い入力の拒否**: 同梱サイドカー (`internal/embed/sidecar.go`) は `llama-server --embedding -c 2048` で起動し、physical batch は既定の 512 のまま。512 トークンを超える入力は 500 (`input (807 tokens) is too large to process. increase the physical batch size (current batch size: 512)`) で拒否される (サイドカーへ直接送って再現済み)。
2. **1 回の失敗でクライアントが無効化される**: `EmbeddingClient.CreateEmbeddings` は失敗すると `disable(err)` で自身を無効化し、次に `RefreshConfiguration` が呼ばれるまで `SyncDocument` / `SyncMemories` / 検索時の埋め込みが黙って飛ばされる。起動時の一括作成で長い Markdown が先に失敗すると、以後に追加したドキュメントもすべて埋め込まれない。設定画面はサイドカーの状態 (準備完了) を出すだけなので、利用者からは見えない。
3. **サイドカーの残留**: 前回起動時の `llama-server` (親プロセス 1) が終了されずに残っていた。アプリ終了時の `embedMgr.Shutdown()` (`app.go`) で子プロセスを止めきれていない経路がある可能性がある。

## 方針 (着手時に調査して確定する)

- サイドカーの起動引数で `-b` / `-ub` をモデルの最大長に合わせる、または送信前に入力をトークン上限内に収める (切り詰め・分割)。どちらにするかは ruri-v3 の最大長と llama-server の制約を確認して決める。
- 1 件の失敗 (入力が長すぎる等、リクエスト単位の 4xx/5xx) でクライアント全体を無効化しない。接続不能のような恒常的な失敗と区別する。
- 残留の再現条件 (通常終了・強制終了・`wails dev` の再ビルド) を確認し、残らないようにする。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 512 トークンを超えるチャンクを含むドキュメントでも、同梱サイドカーで全チャンクの埋め込みが作られる (または切り詰め・分割の方針どおりに作られる) ことをテストか実機で確認している
- [x] #2 リクエスト単位の失敗 1 件で EmbeddingClient が無効化されず、後続のドキュメント追加で埋め込みが作られる。この挙動にテストがある
- [x] #3 既存の DB で埋め込みが欠けているドキュメントが、修正後の起動時 (または再構築) に埋め込まれる
- [x] #4 アプリ終了後に同梱の llama-server が残らない。確認した終了経路が記録されている
- [x] #5 説明文付きの画像ドキュメントが、説明文と同じ語を含まない言い換えのチャットで参照にヒットすることを確認している (TASK-38 AC #9 の埋め込み側)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. サイドカー: ruri-v3 の n_ctx_train (8192) に合わせて -c/-b/-ub を 8192 に揃える (ModelSpec に ContextLength を持たせる)。実測で 1000 字チャンク x32 のバッチが通り、RSS は 115MB→169MB。切り詰めはしない (8192 を超えるのは実データで起きない長さ)。
2. EmbeddingClient: 接続不能など transport の失敗だけで無効化し、HTTP の非 2xx・応答不正・タイムアウトはリクエスト単位の失敗として nil を返すだけにする。
3. EmbeddingSyncService: バッチ失敗時は 1 件ずつ再送し、それでも失敗した項目だけ飛ばして残りを保存する。
4. 起動時: onEmbeddingReady の「1 件でもあれば再構築しない」ガードを、現モデルの埋め込みが欠けたチャンク・メモリだけを埋める SyncMissing に置き換える。
5. 残留: wails dev は再ビルド・終了時にアプリを SIGKILL する (v2.16 internal/process.Kill) ため OnShutdown が走らない。unix では llama-server を /bin/sh の見張りでくるみ、親 (アプリ) が消えたら llama-server を kill する。通常終了・SIGKILL の両経路を確認する。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 原因 (調査で分かったこと)

Description の 3 点に加え、内蔵モードでは **4 点目の既存不具合** が埋め込みを止めていた:

- 内蔵オーバーレイの base URL がサイドカーの origin (`http://127.0.0.1:<port>`) のままで、クライアントは `<base>/embeddings` = llama-server のネイティブ endpoint へ送っていた。応答が配列形式のため decode に失敗し、旧実装では最初の 1 回で client が無効化されていた。`onEmbeddingReady` で `/v1` を付けて渡すよう修正 (回帰テスト `TestEmbeddingReadyUsesSidecarV1Endpoint`)。
- サイドカー残留の原因: wails dev v2.16 は再ビルド時と終了時にアプリを SIGKILL する (`internal/process.Kill`)。OnShutdown が走らず、`Setpgid` で別グループにしたサイドカーは launchd 配下に残っていた。調査時点で 3 個残留していた (手動で kill 済み)。

## 方針の選択

- 長い入力: 切り詰めではなく `-c/-b/-ub` を ruri-v3 の n_ctx_train (8192) に揃えた (`ModelSpec.ContextLength`)。チャンクは本文 1000 字 + タイトル等なので実データの最大は 746 トークン、切り詰めが必要な長さに届かない。RSS は 115MB → 169MB (実測)。8192 を超える入力は 1 件ずつ再送で当該項目だけ飛ばす。
- 無効化: 接続不能 (dial 失敗等) だけで無効化し、HTTP 非 2xx・decode 失敗・タイムアウトはそのリクエストだけの失敗にした。
- 起動時: 「現モデルの埋め込みが 1 件でもあれば再構築しない」ガードを、欠けた (または別モデルの) チャンク・メモリだけを埋める `SyncMissing` に置き換えた。外部モードの起動時 `RebuildAll` は変えていない。
- 残留: unix ではサイドカーを /bin/sh の見張りでくるみ、親 (アプリ) が消えたら llama-server を SIGKILL する。SIP が /bin/sh 実行時に DYLD_* を落とすため、ライブラリパスは `SNZ_SIDECAR_LIB_DIR` で渡してスクリプト内で戻す。

## AC の根拠

- #1: 開発 DB (内蔵モード) で修正後に起動し、document_chunks 213 件すべてに ruri-v3-30m の埋め込みが作られた。サイドカーの /tokenize で数えると 213 件中 180 件が 512 トークン超 (最大 746) で、その 180 件もすべて埋め込み済み。
- #2: `TestCreateEmbeddingsRequestFailureKeepsClientEnabled` / `TestSyncDocumentSkipsOnlyTheRejectedChunk` (拒否されたチャンクだけ飛ばし、後から追加した文書は埋め込まれる)。接続不能で無効化される側は `TestCreateEmbeddingsUnreachableEndpointDisablesClient`。
- #3: 上記 #1 の起動がそのまま該当 (修正前は ruri-v3-30m の埋め込み 0 件、既存は cl-nagoya/ruri-v3-130m のみ)。メモリも 6/6。テストは `TestSyncMissingEmbedsOnlyTheGaps`。
- #4: 確認した終了経路 (macOS / wails dev): (a) wails dev の再ビルド (アプリ SIGKILL) を複数回 — 旧サイドカーは数秒以内に消滅、(b) 通常終了 (AppleScript quit = Cmd+Q 相当) — 残留なし、(c) wails dev への Ctrl-C (アプリ SIGKILL) — 残留なし。自動テスト `TestSidecarDiesWithItsParent` (見張りを外すと失敗することを確認済み)。**Windows は未確認・未対応** (CREATE_NEW_PROCESS_GROUP のみ。Job object が対応策) 。
- #5: 開発 DB のコピーで RetrievalService を直接実行。クエリ「夕暮れどきの薄明光線と里山を写したスナップはある？」(説明文と共通語なし) は FTS のみで 0 件、ハイブリッドで IMG_8037.jpg が 1 位 (score 0.915)。チャット画面経由の確認はしていない (参照一覧は同じ retrieval の結果)。

## 検証

`go vet ./...` / `go test ./...` 通過。実 llama-server の統合テスト (`TestManagerIntegrationRealSidecar`) も見張り経由の起動で通過。
<!-- SECTION:NOTES:END -->
