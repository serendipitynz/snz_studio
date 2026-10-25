---
id: TASK-110
title: '依存: Go 1.27.3 に上げ、govulncheck が検出した GO-2026-6809 を解消する'
status: Done
assignee: []
created_date: '2026-10-25 10:13'
updated_date: '2026-10-25 11:26'
labels: []
milestone: m-4
dependencies: []
references:
  - go.mod
  - scripts/wails.mjs
  - .github/workflows/audit.yml
priority: high
type: chore
ordinal: 114000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

2026-10-25、audit ワークフローの govulncheck が macOS と Windows の両方で失敗した。Go 1.27.2 の標準ライブラリ `crypto/x509` に、新しく公開された脆弱性 GO-2026-6809 が見つかった。修正版は Go 1.27.3。

- 呼び出し経路として `internal/service/llmclient.go:495` (`LLMClient.CreateChatCompletionStream` から `x509.Certificate.Verify` に届く) が挙がっており、コードから到達できると判定されている。
- main 自体も同じ理由で落ちる (TASK-108/109 を追加した 2026-10-25 08:57Z の push の audit が失敗。その前に通ったのは 2026-10-19 の定期実行)。
- PR #106 (TASK-108、frontend のみの変更) も同じ理由で赤くなったが、変更と無関係なので止めずにマージし、この対応は別の PR にした。
- TASK-93 (Go 1.27.1 → 1.27.2) と同じ形の対応になる。

## 作業

- `go.mod` の `toolchain` を `go1.27.3` に上げる。`scripts/wails.mjs` は GOTOOLCHAIN をこの値に厳密に固定するので、Wails CLI (v2.16.0) がこの版の export data を読めて、ビンディング生成が通ることを確かめる (TASK-7・TASK-93 の経緯)。同じ 1.27 系の patch なので、Wails の版は動かさない見込み。
- go.mod の `go` ディレクティブ (1.26.3) を上げる必要があるかを確かめる。上げる場合は、その理由を記録する。
- README などに現在の toolchain の版を書いた箇所があれば揃える。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 go.mod の toolchain が go1.27.3 で、scripts/wails.mjs 経由の wails build と wails dev でビンディング生成が通る
- [x] #2 audit ワークフローの govulncheck が macOS と Windows の両方で通る
- [x] #3 go test ./...、pnpm check:client、pnpm test:client が通る
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- 着手時の判断: マイルストーンは m-4 (v0.4.0) に入れた (ユーザー判断、2026-10-25)。次のリリースを修正済みのツールチェーンでビルドする前提になるため。前回の TASK-93 もその時のリリースの m-2 に入っていた。
- 変更は go.mod の `toolchain` を go1.27.2 から go1.27.3 に上げた 1 行だけ。`go mod tidy` は何も変えず、`go` ディレクティブは 1.26.3 のまま。Wails は v2.16.0 のまま。go.mod のコメントにある「wails の require と build.yml の go install と揃えて上げる」は CLI が読める Go の minor が変わるときの制約で、今回は同じ 1.27 系の patch なので、バインド生成が通ることの確認で足りる (TASK-93 と同じ判断)。版を書いた箇所は go.mod の外に無かった (README・scripts・workflow を grep)。
- #1: 手元 (macOS 26.7.1、arm64) で `pnpm build:app` が「Generating bindings: Done」まで通り、出来上がった `SNZ Studio` のバイナリを `go version` で見ると go1.27.3。`pnpm dev` もバインド生成から API の起動 (127.0.0.1:8787) まで通ったので止めた。`ld: warning: object file ... was built for newer 'macOS' version (13.0) than being linked (11.0)` は TASK-93 の記録と同じ警告。
- govulncheck v1.8.0 (`-tags desktop,production`、audit.yml と同じ) を GOTOOLCHAIN=go1.27.3 で走らせ、darwin と GOOS=windows の両方で No vulnerabilities found。同じバイナリを GOTOOLCHAIN=go1.27.2 で走らせると GO-2026-6809 (crypto/x509、Fixed in go1.27.3) が出ることも確かめた。#2 は PR の CI の結果で確かめる。
- #3: GOTOOLCHAIN=go1.27.3 で `go vet ./...`・`go test ./...` (exit 0、19 パッケージ ok)、`pnpm check:client`・`pnpm test:client` (27 件成功) が通った。

- #2: PR #107 の audit (run 40186943426、ca939f4) と、マージ後の main の audit (run 40188736275、4b2a076) で、govulncheck (macos-latest / windows-latest) と pnpm-audit がすべて成功した。main の audit は 2026-10-25 08:57Z の push から失敗していたが、これで通る状態に戻った。
<!-- SECTION:NOTES:END -->
