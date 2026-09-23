---
id: TASK-41
title: 同梱の埋め込みサイドカーで長いチャンクが拒否され、以後の埋め込みが黙って止まる不具合を直す
status: To Do
assignee: []
created_date: '2026-09-23 09:01'
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
- [ ] #1 512 トークンを超えるチャンクを含むドキュメントでも、同梱サイドカーで全チャンクの埋め込みが作られる (または切り詰め・分割の方針どおりに作られる) ことをテストか実機で確認している
- [ ] #2 リクエスト単位の失敗 1 件で EmbeddingClient が無効化されず、後続のドキュメント追加で埋め込みが作られる。この挙動にテストがある
- [ ] #3 既存の DB で埋め込みが欠けているドキュメントが、修正後の起動時 (または再構築) に埋め込まれる
- [ ] #4 アプリ終了後に同梱の llama-server が残らない。確認した終了経路が記録されている
- [ ] #5 説明文付きの画像ドキュメントが、説明文と同じ語を含まない言い換えのチャットで参照にヒットすることを確認している (TASK-38 AC #9 の埋め込み側)
<!-- AC:END -->
