---
id: TASK-113
title: 'リリース: v0.4.0 としてバージョンを上げ、次のマイルストーン v0.5.0 を用意する'
status: In Review
assignee: []
created_date: '2026-11-08 21:42'
updated_date: '2026-11-08 22:07'
labels: []
milestone: m-4
dependencies: []
references:
  - package.json
  - wails.json
  - .github/workflows/release.yml
priority: low
type: chore
ordinal: 117000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

v0.4.0 は多人数会話の履歴の選び方 (TASK-103 / TASK-104)、発言のピン留め (TASK-105)、選抜発言と切り詰めの参照表示 (TASK-106)、本文が空の応答での再試行 (TASK-112)、処理言語の切り替え (TASK-109)、trpg-table の日本語化 (TASK-107)、モーダルと設定行の見た目の統一 (TASK-108)、Go 1.27.3 と golang.org/x/net v0.62.0 での脆弱性の解消 (TASK-110 / TASK-111) を届ける。マイルストーン v0.4.0 (m-4) に残っている TASK-52 と TASK-60 は今回の版に入れず、次の版へ送ってリリースする (2026-11-09 に決定)。

TASK-103〜109 と TASK-112 はマイルストーンが付かないまま Done になっている。v0.4.0 で届けるものなので m-4 に割り当てる。

## 方針

- `package.json` の `version` と `wails.json` の `info.productVersion` を `0.4.0` にする。release.yml の prepare はこの 2 つがタグと一致しないと止まる。
- マイルストーン v0.5.0 を作り、TASK-52 と TASK-60 をそこへ移す。
- マージ後に `v0.4.0` タグを push し、release.yml が作った下書きのノートを v0.3.0 と同じ和英の書式で書く。ノートを API で直すときは `tag_name` を一緒に送る (TASK-96)。公開はオーナーが行う。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 package.json の version と wails.json の info.productVersion が 0.4.0 になっている
- [x] #2 マイルストーン v0.5.0 があり、TASK-52 と TASK-60 が割り当てられている
- [ ] #3 v0.4.0 タグの release.yml の実行が通り、下書きのリリースに dmg・app.zip・Windows amd64 / arm64 インストーラ・latest.json・SHA256SUMS.txt が添付されている
- [ ] #4 下書きのノートが和英で書かれ、tag_name が v0.4.0 のままになっている
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- package.json の version と wails.json の info.productVersion を 0.4.0 にした。jq で `.version` / `.info.productVersion` を読むと、どちらも 0.4.0 を返す。リポジトリ内でアプリの版を持つのはこの 2 か所だけ (updatesig_test.go と updater_test.go の 0.3.0 はテストの値)。
- `backlog milestone add v0.5.0` で m-5 を作り、TASK-52 と TASK-60 を m-4 から m-5 へ移した。
- マイルストーンが付いていなかった TASK-103〜109 と TASK-112 を m-4 に割り当てた。
- go vet / go test ./... / pnpm check:client / pnpm test:client (37 件) が通る。
- AC #3・#4 はマージ後に v0.4.0 タグを push してから確かめる。
<!-- SECTION:NOTES:END -->
