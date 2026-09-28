---
id: TASK-81
title: '依存: フロント依存の既知の脆弱性を解消し、CI で pnpm audit --prod と govulncheck を走らせる'
status: To Do
assignee: []
created_date: '2026-09-28 20:23'
labels:
  - security
dependencies: []
references:
  - package.json
  - pnpm-lock.yaml
  - .github/workflows/build.yml
priority: low
type: chore
ordinal: 81000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F15 (Low)。

- レビュー時の `pnpm audit` は 20 件 (high 8 / moderate 11 / low 1)。`pnpm-lock.yaml` は 2026-05-31 から更新されていない (2026-09-29 時点でも同じ)。
- 大半は dev のときだけ使うツールチェーン (vite 5.4.21、esbuild、postcss、nanoid、browserslist、@babel/core)。`wails dev` の間は Vite の dev server (5173) が動くため、esbuild の GHSA-67mh-4wv8-2f99 (任意のサイトから dev server へ要求を送れる) は開発中に実害があり得る。
- ランタイムに入るのは react-router(-dom) 6.30.3 の open redirect 系。HashRouter で外部からの入力をナビゲーション先に使っていないので、悪用の経路は見つかっていない。
- `govulncheck` は No vulnerabilities found。CI (`build.yml`) は `workflow_dispatch` だけで、テストも lint も走らせていない。
- 補足: CI の `actions/*@v4` などはタグ参照で、コミット SHA で固定していない。今は `workflow_dispatch` だけ・`contents: read`・シークレット未使用なのでリスクは小さいが、署名のシークレットを CI に入れるなら SHA で固定する。

## 方針

- vite 6 系 (6.4.3 以上) と react-router の修正版へ上げて、lockfile を作り直す。
- CI に `pnpm audit --prod` と `govulncheck` を加える。走らせる契機 (push / PR / 手動) は着手時に決める。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 pnpm audit の high が 0 件になっている。残る moderate / low は理由とともに記録されている
- [ ] #2 vite のメジャーバージョンを上げた後も、pnpm dev・wails dev・配布ビルドが動く
- [ ] #3 CI で pnpm audit --prod と govulncheck が走り、脆弱性が見つかったら失敗する。走らせる契機が記録されている
- [ ] #4 CI の actions をコミット SHA で固定するか、署名のシークレットを入れるまで見送るかが記録されている
<!-- AC:END -->
