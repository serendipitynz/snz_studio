---
id: TASK-81
title: '依存: フロント依存の既知の脆弱性を解消し、CI で pnpm audit --prod と govulncheck を走らせる'
status: Done
assignee: []
created_date: '2026-09-28 20:23'
updated_date: '2026-10-04 00:45'
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
- [x] #1 pnpm audit の high が 0 件になっている。残る moderate / low は理由とともに記録されている
- [x] #2 vite のメジャーバージョンを上げた後も、pnpm dev・wails dev・配布ビルドが動く
- [x] #3 CI で pnpm audit --prod と govulncheck が走り、脆弱性が見つかったら失敗する。走らせる契機が記録されている
- [x] #4 CI の actions をコミット SHA で固定するか、署名のシークレットを入れるまで見送るかが記録されている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. vite を ^6.4.3 へ、@vitejs/plugin-react を vite 6 対応の最新 (5.x) へ、react-router-dom を ^7.18 へ上げ、lockfile を作り直す (着手時の合意: router は v7、vite は 6 系)
2. pnpm audit / pnpm audit --prod で high 0 件・prod 0 件を確認し、残りがあれば理由を記録
3. check:client / test:client / build:client / go test、pnpm dev:client、wails dev、wails build で動作確認
4. .github/workflows/audit.yml を追加: pull_request・push (main)・週次 schedule・workflow_dispatch で pnpm audit --prod と govulncheck を実行。contents: read、シークレット不使用
5. actions の SHA 固定は署名シークレットを CI に入れるまで見送る、と workflow のコメントと Implementation Notes に記録
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 着手時に決めたこと (オーナー確認済み)

- react-router: v7 (react-router-dom 7.18.4) に上げた。GHSA-wrjc-x8rr-h8h6 (`<Link>` / `useNavigate` のバックスラッシュによる open redirect) と GHSA-337j-9hxr-rhxg (SSR hydration。宣言的モードには影響しない) は 7.18.0 でしか直っておらず、6 系最新の 6.30.6 では `--prod` の audit に残るため。使っている API (HashRouter / Routes / Route / Navigate / Link / Outlet / useParams / useNavigate) は v7 にそのまま残っていて、コードの変更は不要だった。
- vite: タスクの方針どおり 6 系 (6.4.3)。7 系はビルドの既定 target が上がり、8 系はバンドラが Rolldown に変わるため見送った。8 系が出ている今、6 系は vite 9 が出るまで security patch だけを受け取る。@vitejs/plugin-react は vite 6 に対応する 5.2.0。
- CI の契機: `main` への pull_request と push、週 1 回の schedule (月曜 00:00 UTC)、workflow_dispatch。advisory はコードを変えなくても公開されるので、週次の実行で拾う。build.yml とは別の `.github/workflows/audit.yml` に置き、シークレットなし・`contents: read` で動かす。

## AC ごとの根拠

- #1: lockfile は `node_modules` ごと消して作り直した (`pnpm add` だけでは postcss・nanoid・browserslist などの推移的な依存が古いまま残り、high 7 件が消えなかった)。作り直した後は `pnpm audit` も `pnpm audit --prod` も 0 件で、残る moderate / low はない。主な変化: postcss 8.5.8→8.5.28、nanoid 3.3.11→3.3.19、browserslist 4.28.2→4.29.3、@babel/core 7.29.0→7.29.7、esbuild 0.21.5→0.25.12。
- #2: `pnpm dev` (= wails dev。中で `dev:client` の Vite 6.4.3 が起動する) を一時的な DATA_DIR で起動し、34115 の画面で `/` → プロジェクト作成 → プロジェクト詳細 (Link・useParams) → チャット作成 (useNavigate で `/chats/:id`) → 存在しないパス (Navigate で `/` に戻る) を確認した。react-router の警告はコンソールに出ていない。`pnpm build:app` (wails build) も成功し、ビルドした app を一時的な DATA_DIR で起動すると API が listen してダッシュボードが表示された。`check:client`・`test:client` (5/5)・`build:client`・`go vet`・`go test ./...` も通る。バンドルは 603.39 kB → 641.77 kB (gzip 185.63 → 194.36 kB)。500 kB 超の警告は変更前から出ている。
- #3 (未チェック): 実際に走ったことは PR 上の CI で確かめてからチェックする。ローカルでは、変更前の lockfile で `pnpm audit --prod` が exit 1、変更後で exit 0 になることを確認した。`govulncheck` v1.8.0 は GOTOOLCHAIN=go1.27.1・`-tags desktop,production` (`wails build` が付けるタグ) で darwin / windows とも No vulnerabilities found。frontend/dist の .gitkeep があるので、clean checkout でも go:embed は解決できる。
- #4: actions の SHA 固定は、署名のシークレットを build.yml に入れるまで見送ると決め、audit.yml のコメントに書いた。どちらの workflow もシークレットと書き込み権限のあるトークンを持たないので、タグを書き換えられても読めるのはこの公開リポジトリだけ。

## 実装上の判断

- govulncheck は macOS と Windows の native runner で走らせる。Wails の darwin 向け frontend は cgo のファイルだけでできているので、Linux runner から GOOS=darwin でスキャンすると、そのファイルが落ちて到達解析から漏れる。
- `pnpm audit --prod` は lockfile しか読まないので、CI ではインストールを省いた。dev 依存は app に含まれないので CI の失敗条件からは外し、ローカルの `pnpm audit` で見る。

- #3: PR #80 の CI (run 37162919793) で pnpm-audit・govulncheck (macos-latest / windows-latest) の 3 ジョブが走り、すべて成功した。govulncheck は setup-go が入れた go1.27.1 で動いた。マージ後の push でも audit.yml が起動している (run 37165887167)。
<!-- SECTION:NOTES:END -->
