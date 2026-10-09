---
id: TASK-93
title: '依存: Go 1.27.2 と golang.org/x/net v0.60.0 に上げ、govulncheck が検出した脆弱性を解消する'
status: Done
assignee: []
created_date: '2026-10-09 11:48'
updated_date: '2026-10-09 21:03'
labels: []
milestone: m-2
dependencies: []
references:
  - go.mod
  - scripts/wails.mjs
  - .github/workflows/audit.yml
priority: high
type: chore
ordinal: 97000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

2026-10-09 の PR #91 (TASK-92.2) で、audit ワークフローの govulncheck が macOS と Windows の両方で失敗した。Go 1.27.1 の標準ライブラリ (net/http、net/http/internal/http2、crypto/tls、net/textproto) と golang.org/x/net v0.56.0 に、新しく公開された 10 件の脆弱性 (GO-2026-6603、6605、6607〜6613、6617) が見つかった。修正版は Go 1.27.2 と x/net v0.60.0。

PR #91 は依存を増やしておらず、main も同じ状態で落ちる (main で最後に audit が通ったのは 2026-10-08 で、その後に公開された)。そのため PR #91 は止めずにマージし、この対応は別の PR にした。

## 影響の見立て

- 大半は HTTP/2 サーバ側の件。アプリの API サーバは loopback の平文 HTTP で、HTTP/2 を使わないので、直接は当たりにくい。
- ただし TASK-92.2 で、アプリ自身が GitHub へ HTTPS (HTTP/2 クライアント) で接続するようになった。GO-2026-6610 (HTTP/2 transport が不正なヘッダを受け入れる) などのクライアント側の件は無関係とは言えない。
- v0.1.0 も Go 1.27.1 でビルドされている。次のリリース (v0.2.0) は修正済みのツールチェーンでビルドする。

## 作業

- `go.mod` の `toolchain` を `go1.27.2` に上げる。`scripts/wails.mjs` は GOTOOLCHAIN をこの値に厳密に固定しているので、Wails CLI がこの版の export data を読めて、ビンディング生成が通ることを確かめる (TASK-7 で、版の差で `wails dev` が失敗した経緯がある)。
- `golang.org/x/net` を v0.60.0 以上に上げる (Wails 経由の間接依存)。
- go.mod の `go` ディレクティブを上げる必要があるかを確かめる。上げる場合は、その理由を記録する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 go.mod の toolchain が go1.27.2 で、scripts/wails.mjs 経由の wails build と wails dev でビンディング生成が通る
- [x] #2 golang.org/x/net が v0.60.0 以上になっている
- [x] #3 audit ワークフローの govulncheck が macOS と Windows の両方で通る
- [x] #4 go test ./...、pnpm check:client、pnpm test:client が通る
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- `toolchain` を go1.27.2 に上げ、`go get golang.org/x/net@v0.60.0` と `go mod tidy` を実行した。x/net に引きずられて x/crypto v0.57.0、x/sys v0.48.0、x/text v0.42.0 (直接依存) も上がった。Wails は v2.16.0 のまま。go.mod のコメントにある「wails の require と build.yml の go install と揃えて上げる」は CLI が読める Go の minor が変わるときの制約で、今回は同じ 1.27 系の patch なのでバインド生成が通ることの確認で足りる。
- go ディレクティブは 1.26.3 のまま。上がった x/* はどれも `go 1.26.0` を要求するだけで、tidy もディレクティブを動かさなかった。標準ライブラリの修正は toolchain (scripts/wails.mjs と audit.yml がこの値を GOTOOLCHAIN に固定する) で入る。
- #1: 手元 (macOS arm64) では GOTOOLCHAIN=go1.27.2 の `pnpm build:app` で「Generating bindings: Done」、出来上がったバイナリを `go version` で見ると go1.27.2。`pnpm dev` もバインド生成から API の起動 (127.0.0.1:8787) まで通った。CI では build.yml を PR のブランチで dispatch し (run 37990075228)、macOS (universal) と Windows (amd64) の両ジョブが go1.27.2 でバインド生成を通って成功した。build.yml は dispatch 専用で PR では自動で走らないので、手で起動した。
- #2: go.mod の golang.org/x/net は v0.60.0。
- #3: PR #92 の audit (run 37989848928) で govulncheck (macos-latest / windows-latest) と pnpm-audit がすべて成功した。手元でも govulncheck v1.8.0 を GOTOOLCHAIN=go1.27.2・`-tags desktop,production` で走らせ、darwin と GOOS=windows の両方で No vulnerabilities found。
- #4: `go vet ./...`、`go test ./...` (FAIL 0)、`pnpm check:client`、`pnpm test:client` (18 件 pass) が通った。
- ビルド時に `ld: warning: object file ... was built for newer 'macOS' version (13.0) than being linked (11.0)` が出る。Go の minor 版で決まる最小 macOS と Wails の -mmacosx-version-min=11.0 の差なので 1.27.1 でも同じはずだが、変更前のビルドと比べてはいない。
<!-- SECTION:NOTES:END -->
