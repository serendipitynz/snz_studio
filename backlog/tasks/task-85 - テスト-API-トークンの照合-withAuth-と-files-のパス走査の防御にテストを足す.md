---
id: TASK-85
title: 'テスト: API トークンの照合 (withAuth) と /files のパス走査の防御にテストを足す'
status: To Do
assignee: []
created_date: '2026-09-28 20:23'
labels:
  - security
dependencies: []
references:
  - internal/httpapi/server.go
  - internal/httpapi/handlers.go
priority: medium
type: task
ordinal: 85000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の §3 (テストの評価) の優先 1・2。2026-09-29 に `756d32b` でも該当するテストが無いことを確認した (`SetAuthToken` を使うテストが無い)。

- API トークン (起動ごとに生成され `/api` と `/files` の全リクエストで照合される 256bit のランダム値) は、ループバック API の唯一の防御。ところがハンドラのテストはすべて `token == ""` (照合しない) で走っていて、`withAuth` そのものを確かめるテストが無い。
- `/files` のパス走査の防御 (`fileHandler`) にも回帰テストが無い。

レビューの §3 の優先 3〜8 (整理プラン、送信先ごとの Authorization、ストリームの境界、サイドカーの状態遷移、フロントの外部画像) は、それぞれの指摘のタスクの AC に含めた。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 SetAuthToken を設定した Handler() に対し、トークンなし・誤ったトークン・正しいトークンで /api/* と /files/* がそれぞれ 401・401・200 になり、OPTIONS だけがトークンなしで通る。テストがある
- [ ] #2 /files/..%2fapp.sqlite、/files/%2e%2e、バックスラッシュを含む名前で 404 になり、アップロード先の外のファイルが返らない。テストがある
- [ ] #3 テストで欠陥が見つかった場合に、このタスクで直したか、別のタスクを起こしたかが記録されている
<!-- AC:END -->
