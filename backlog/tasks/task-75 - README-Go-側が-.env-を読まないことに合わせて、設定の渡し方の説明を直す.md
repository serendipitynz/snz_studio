---
id: TASK-75
title: 'README: Go 側が .env を読まないことに合わせて、設定の渡し方の説明を直す'
status: To Do
assignee: []
created_date: '2026-09-28 20:22'
labels: []
dependencies: []
references:
  - README.md
  - README.ja.md
  - .env.example
  - internal/config/config.go
priority: low
type: docs
ordinal: 75000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F9 (Low)。2026-09-29 に `756d32b` で未修正を確認した。

- README.md (137 行付近) / README.ja.md (130 行付近) は「`.env` で `LLM_BASE_URL` などの既定値を上書きできる」、README.md (226 行付近) / README.ja.md (209 行付近) は「`.env` の主な設定」と書いている。
- Go 側に `.env` を読む処理は無い (`config.go` のコメント「loadEnvFile is the OS env in Go」)。Vite の `envDir` が読むのはフロントの `VITE_` 変数だけ。`.env.example` に並ぶ値は、シェルの環境変数として渡さない限り反映されない。
- 影響: UI から設定できない唯一の資格情報である `LLM_API_KEY` を `.env` に書いた利用者は、キーが送られず認証エラーになり、原因を切り分けにくい。

## 方針 (着手時に確定する)

- `pnpm dev` で `.env` の値が反映されないことを実機で確かめる。
- README と `.env.example` を「環境変数として渡す」に改めるか、dev でだけ `.env` を読むようにするかを決める。後者で依存が増えるなら、先にユーザーに確認する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 pnpm dev で .env の値が反映されるかを実機で確かめた結果が記録されている
- [ ] #2 README.md / README.ja.md / .env.example の説明が実装と一致している (または実装を説明に合わせている)
- [ ] #3 LLM_API_KEY の渡し方が README.md / README.ja.md に具体的に書かれている
<!-- AC:END -->
