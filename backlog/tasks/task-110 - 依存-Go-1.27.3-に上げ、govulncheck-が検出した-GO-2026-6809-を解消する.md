---
id: TASK-110
title: '依存: Go 1.27.3 に上げ、govulncheck が検出した GO-2026-6809 を解消する'
status: To Do
assignee: []
created_date: '2026-10-25 10:13'
labels: []
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
- [ ] #1 go.mod の toolchain が go1.27.3 で、scripts/wails.mjs 経由の wails build と wails dev でビンディング生成が通る
- [ ] #2 audit ワークフローの govulncheck が macOS と Windows の両方で通る
- [ ] #3 go test ./...、pnpm check:client、pnpm test:client が通る
<!-- AC:END -->
