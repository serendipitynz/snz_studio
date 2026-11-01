---
id: TASK-111
title: >-
  依存: golang.org/x/net を v0.62.0 に上げ、govulncheck が検出した GO-2026-6866 と
  GO-2026-6860 を解消する
status: In Review
assignee: []
created_date: '2026-11-01 10:04'
updated_date: '2026-11-01 11:00'
labels: []
milestone: m-4
dependencies: []
references:
  - go.mod
  - go.sum
  - .github/workflows/audit.yml
priority: high
type: chore
ordinal: 115000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

2026-11-01、PR #108 の audit ワークフローの govulncheck が macOS と Windows の両方で失敗した。golang.org/x/net v0.60.0 の html パッケージに、新しく公開された脆弱性 GO-2026-6866 と GO-2026-6860 が見つかった。修正版は v0.62.0。

- PR #108 は go.mod と go.sum を変えていない。main の audit は 2026-10-26 に通っているので、それ以降に公開された advisory で、main も同じ理由で落ちる。
- govulncheck が挙げる呼び出し経路は internal/updater/install_darwin.go:147 の updater.Target.Relaunch から html.Render / html.Parse に届くもの。
- PR #108 とは独立した PR で対応する。TASK-93・TASK-110 と同じ形の対応になる。

## 作業

- go get golang.org/x/net@v0.62.0 と go mod tidy を実行する。toolchain と Wails の版は動かさない見込み。
- 連動して上がる x/* を記録し、go ディレクティブ (1.26.3) が動くならその理由を記録する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 go.mod の golang.org/x/net が v0.62.0 以上で、go mod tidy 後に差分が残らない
- [x] #2 audit ワークフローの govulncheck が macOS と Windows の両方で通る
- [x] #3 go test ./... が通り、scripts/wails.mjs 経由の wails build でビンディング生成が通る
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- `go get golang.org/x/net@v0.62.0` と `go mod tidy` を実行した。x/net に引きずられて x/crypto v0.58.0、x/sys v0.49.0、x/text v0.43.0 (直接依存) も上がった。toolchain は go1.27.3、go ディレクティブは 1.26.3、Wails は v2.16.0 のまま。
- 変更前に手元の govulncheck v1.8.0 (`-tags desktop,production`、audit.yml と同じ) で GO-2026-6866 と GO-2026-6860 が出ることを確かめた。変更後は darwin と GOOS=windows の両方で No vulnerabilities found。GOOS=windows は `go run` だと Windows 用バイナリを作って実行できないので、darwin 用に go install した同じ版で走らせた。
- #1: go.mod の golang.org/x/net は v0.62.0。`go mod tidy` を再度実行しても go.mod は変わらない。
- #3: GOTOOLCHAIN=go1.27.3 で `go vet ./...` と `go test ./...` が通った。`pnpm build:app` で「Generating bindings: Done」まで通り、app が出来上がった (`ld: warning: ... newer 'macOS' version (13.0) than being linked (11.0)` は TASK-93・TASK-110 の記録と同じ警告)。

- #2: PR #109 の audit (run 41296001766、2b7123a) で govulncheck (macos-latest / windows-latest) と pnpm-audit がすべて成功した。
<!-- SECTION:NOTES:END -->
