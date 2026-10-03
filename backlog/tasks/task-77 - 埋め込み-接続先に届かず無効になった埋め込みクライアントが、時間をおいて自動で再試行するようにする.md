---
id: TASK-77
title: '埋め込み: 接続先に届かず無効になった埋め込みクライアントが、時間をおいて自動で再試行するようにする'
status: In Review
assignee: []
created_date: '2026-09-28 20:23'
updated_date: '2026-10-03 10:07'
labels: []
dependencies: []
references:
  - internal/service/embeddingclient.go
  - internal/service/embeddingsync.go
  - internal/httpapi/server.go
priority: low
type: bug
ordinal: 77000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F11 (Low、確信度 Medium) のうち、TASK-41 で直らずに残った部分。

- TASK-41 (`ae89f96`) で、エラー応答・タイムアウト・不正な応答ではクライアントを無効にしなくなった。
- 残っているのは接続できない場合 (dial の失敗など)。`embeddingclient.go` の `disable` で無効になると、`RefreshConfiguration` (設定保存、サイドカーの ready / lost) まで戻らない。外部モードでは設定を保存するしか戻す手段が無い。
- 無効の間に作ったドキュメント・メモリは埋め込みを持たない。外部モードで戻るのは設定保存の `RebuildAll` だけなので、利用者が保存しない限り埋まらない。

## 方針

- 無効にしてから一定時間後に 1 回だけ試す状態 (サーキットブレーカーの半開) を設け、成功したら有効に戻して `SyncMissing` を走らせる。
- 設定画面の接続状態の表示と食い違わないようにする。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 外部モードで接続先を止めて無効にさせた後、接続先を戻すと、設定を保存しなくても一定時間内に埋め込みが再開する。テストがある
- [x] #2 復帰したときに、無効の間に作られたドキュメント・メモリの埋め込みが作られる
- [x] #3 接続先が止まったままの間、再試行が要求ごとの遅延や大量のログを生まない
- [x] #4 設定画面の接続状態の表示が、無効になったことと復帰したことに食い違わない
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. embeddingclient.go: 接続できずに無効にしたときだけ再試行タイマー (既定 30 秒、固定間隔) を張る。タイマーは小さな /embeddings 要求を 1 回だけ送り、届けば有効に戻して復帰コールバックを呼び、届かなければ黙って次のタイマーを張る。要求の経路は無効の間これまでどおり即座に nil を返す (要求ごとの遅延なし)。ログは無効化 1 回・復帰 1 回だけ。
2. 世代番号で、RefreshConfiguration / EnsureModelLoaded の後に古い再試行や古い要求の失敗が状態を書き換えないようにする。
3. server.go: 復帰コールバックに embeddingSync.RequestSyncMissing をつなぐ (無効の間に作られたドキュメント・メモリを埋める。rebuild が残っていれば rebuild になる)。
4. 設定画面の接続表示 (GET/PUT /api/configuration の embeddingConnected) が接続ありを返すときは、その場で再試行を 1 回走らせ、表示が「接続あり」なのに埋め込みが無効のまま、という食い違いを残さない。
5. テスト: service にタイマー復帰・遅延/ログ量・設定変更での取り消し、httpapi に設定読み込みでの即時復帰と欠落埋めを追加。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装

- `EmbeddingClient` は、接続できずに無効にしたときだけ再試行タイマー (`embeddingRetryInterval` = 30 秒、固定間隔) を張る。タイマーは `/embeddings` に 1 入力だけの要求を送り、届けば有効に戻して `onReconnect` を呼び、届かなければ何もログに出さずに次のタイマーを張る。
- 復帰の判定は「無効にする判定の裏返し」にそろえた。要求が届いたら復帰とみなし、エラー応答・タイムアウトも復帰に数える (TASK-41 以降、これらでは無効にしないため)。判定は `requestEmbeddings` の `reachable` 1 か所にまとめ、`CreateEmbeddings` と再試行の両方がこれを使う。
- 世代番号 `generation` を設けた。`RefreshConfiguration`・`EnsureModelLoaded` の成功・復帰で世代が進み、それより前に始まった再試行や要求の失敗は状態を書き換えない。サイドカー喪失 (`onEmbeddingLost`) でモデルが空になった後に、古い再試行がクライアントを有効に戻すことはない。
- `server.go` で `onReconnect` に `embeddingSync.RequestSyncMissing` をつないだ。無効の間に作ったドキュメント・メモリは `SyncDocument` / `SyncMemories` が飛ばすので、復帰時の gap fill で埋まる。rebuild が残っていれば `schedule` が rebuild に格上げする。
- 設定画面の接続表示 (`checkConnections` の `embeddingConnected`) が接続ありを返したときは、その場で `RetryIfUnreachable` を同期実行する。応答が返る時点でクライアントは有効になっているので、「接続あり」と表示しながら埋め込みが無効のまま、という状態が残らない。逆向きの「接続なし表示なのにクライアントは有効」は直していない。`/models` が取れないだけのサーバー (モデル一覧を返さない実装) で埋め込みまで止めないためで、その場合も次の埋め込み要求が届かなければ無効になる。

## 代替案を採らなかった理由

- 要求のたびに経過時間を見て 1 回だけ通す遅延評価の半開は採らなかった。`SyncDocument` / `SyncMemories` / `SyncMissing` は `IsEnabled()` で先に抜けるので、要求が来ず復帰が起きない。AC#1 の「一定時間内に」を満たせない。
- 指数バックオフは入れなかった。接続先はローカルで、止まっている間の要求は即座に connection refused で終わる。30 秒に 1 回の要求は負荷にもログにもならない。

## 検証

- `internal/service/embeddingclient_test.go`
  - `TestUnreachableClientReconnectsWithoutASettingsSave` (AC#1): 接続先を落として無効にさせ、戻すと設定を保存せずに有効に戻り、`onReconnect` が 1 回だけ呼ばれる。
  - `TestUnreachableClientDoesNotRetryPerRequestOrLogEachProbe` (AC#3): 無効の間の 1000 回の `CreateEmbedding` で接続先への要求は 0 回。450ms の間に 100ms 間隔の再試行が 1〜6 回。停止から復帰までのログは 2 行 (無効化・復帰)。
  - `TestReconfiguringCancelsTheReconnectProbe`: サイドカー喪失の経路で古い再試行が無効化を取り消さない。
  - `TestReconnectEmbedsWhatWasSavedWhileUnreachable` (AC#2): 停止中に作ったドキュメントのチャンクとメモリが、復帰後に埋まる。
- `internal/httpapi/embeddingreconnect_test.go` `TestReadingConfigurationReconnectsAnUnreachableEmbeddingEndpoint` (AC#2, AC#4): 停止中は `GET /api/configuration` の `embeddingConnected` が false でクライアントも無効。復帰後の 1 回目の読み込みで true になり、その応答の時点でクライアントは有効。停止中に作ったドキュメントとメモリが埋まる。
- ミューテーション確認: タイマー間隔を 1 時間にすると service の 3 テストが落ち、`checkConnections` の `RetryIfUnreachable` を外すと httpapi のテストが落ちる。
- `go vet ./...`、`go test ./... -race` はすべて通過。`gofmt -l` が `internal/search/model.go` を挙げるが、今回の変更より前からある (`9c816a1`)。

## 測っていないこと

- 実機での確認はしていない。外部モードで LM Studio などを止めて戻し、30 秒以内に意味検索が戻るかは見ていない。
- フロントエンドは変更していない。ダッシュボードの接続カードは読み込み時にしか確認しないので、開いたまま待っても表示は更新されない。ただし表示を更新すれば、その時点で上記の即時復帰が走る。
<!-- SECTION:NOTES:END -->
