---
id: TASK-85
title: 'テスト: API トークンの照合 (withAuth) と /files のパス走査の防御にテストを足す'
status: Done
assignee: []
created_date: '2026-09-28 20:23'
updated_date: '2026-09-30 02:17'
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
- [x] #1 SetAuthToken を設定した Handler() に対し、トークンなし・誤ったトークン・正しいトークンで /api/* と /files/* がそれぞれ 401・401・200 になり、OPTIONS だけがトークンなしで通る。テストがある
- [x] #2 /files/..%2fapp.sqlite、/files/%2e%2e、バックスラッシュを含む名前で 404 になり、アップロード先の外のファイルが返らない。テストがある
- [x] #3 テストで欠陥が見つかった場合に、このタスクで直したか、別のタスクを起こしたかが記録されている
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
internal/httpapi/auth_test.go を追加 (本体コードの変更なし)。

- AC#1: SetAuthToken 済みの Handler() に対し、/api/projects と /files/<upload> をトークンなし・誤り・1 文字欠けの前方一致・正しいトークンで叩き、401/401/401/200 を確認。OPTIONS (プリフライト) は /api・/files とも無トークンで 204・本文なし、GET/HEAD/POST/PUT/PATCH/DELETE は無トークンで 401。加えて、トークンの運び手がルートごとに固定されていること (/api はクエリの t では通らず、/files はヘッダでは通らない) も確かめた。
- AC#2: テスト用の DB (<tmp>/app.sqlite) が uploads の 1 つ上にある配置を利用し、/files/..%2fapp.sqlite・..%2Fapp.sqlite・%2e%2e・%2e%2e%2fapp.sqlite・%2e・..%5coutside.txt・..%5Capp.sqlite が 404 で、本文に SQLite ヘッダも外部ファイルの中身も含まれないことを確認 (いずれも正しいトークン付き。withAuth より先で弾かれて 401 になるのでは防御を試せないため)。
- テストの識別力: fileHandler のガードを一時的に無効化すると走査の 7 ケースすべてが失敗、withAuth の照合を無効化すると認証系 3 テストが失敗し、戻すと通ることを確認した (a%5cb はガードなしでも 404 になり識別力がないため外した)。
- AC#3: テストで欠陥は見つからなかった。修正も別タスクも不要。
- go vet ./... と go test ./... は通過。gofmt -l は既存の internal/search/model.go を挙げるが今回の変更外。
<!-- SECTION:NOTES:END -->
