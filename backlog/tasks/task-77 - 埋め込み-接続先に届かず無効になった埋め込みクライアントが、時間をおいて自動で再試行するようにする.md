---
id: TASK-77
title: '埋め込み: 接続先に届かず無効になった埋め込みクライアントが、時間をおいて自動で再試行するようにする'
status: To Do
assignee: []
created_date: '2026-09-28 20:23'
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
- [ ] #1 外部モードで接続先を止めて無効にさせた後、接続先を戻すと、設定を保存しなくても一定時間内に埋め込みが再開する。テストがある
- [ ] #2 復帰したときに、無効の間に作られたドキュメント・メモリの埋め込みが作られる
- [ ] #3 接続先が止まったままの間、再試行が要求ごとの遅延や大量のログを生まない
- [ ] #4 設定画面の接続状態の表示が、無効になったことと復帰したことに食い違わない
<!-- AC:END -->
