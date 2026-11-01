---
id: TASK-111
title: >-
  依存: golang.org/x/net を v0.62.0 に上げ、govulncheck が検出した GO-2026-6866 と
  GO-2026-6860 を解消する
status: In Progress
assignee: []
created_date: '2026-11-01 10:04'
updated_date: '2026-11-01 10:04'
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
- [ ] #1 go.mod の golang.org/x/net が v0.62.0 以上で、go mod tidy 後に差分が残らない
- [ ] #2 audit ワークフローの govulncheck が macOS と Windows の両方で通る
- [ ] #3 go test ./... が通り、scripts/wails.mjs 経由の wails build でビンディング生成が通る
<!-- AC:END -->
