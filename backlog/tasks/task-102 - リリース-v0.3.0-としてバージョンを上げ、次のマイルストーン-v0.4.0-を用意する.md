---
id: TASK-102
title: 'リリース: v0.3.0 としてバージョンを上げ、次のマイルストーン v0.4.0 を用意する'
status: In Review
assignee: []
created_date: '2026-10-12 05:34'
updated_date: '2026-10-12 05:34'
labels: []
milestone: m-3
dependencies: []
references:
  - package.json
  - wails.json
  - .github/workflows/release.yml
priority: low
type: chore
ordinal: 106000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

v0.3.0 は Windows arm64 版 (TASK-97)、モデルごとの Think の強さ (TASK-98)、思考内容の折りたたみ表示 (TASK-99)、検索の改善 (TASK-100 / TASK-101) を届ける。マイルストーン v0.3.0 (m-3) に残っている TASK-52 と TASK-60 は今回の版に入れず、次の版へ送ってリリースする (2026-10-12 に決定)。

TASK-97 の AC #2 (release.yml が arm64 のインストーラを添付し、latest.json の windows-arm64 が公開鍵での検証を通る) は、このリリースの release.yml の実行で初めて確かめられる。

## 方針

- `package.json` の `version` と `wails.json` の `info.productVersion` を `0.3.0` にする。release.yml の prepare はこの 2 つがタグと一致しないと止まる。
- マイルストーン v0.4.0 を作り、TASK-52 と TASK-60 をそこへ移す。
- マージ後に `v0.3.0` タグを push し、release.yml が作った下書きのノートを v0.2.0 と同じ和英の書式で書く。Windows on Arm で amd64 版を使っている人向けに、arm64 版を入れ直す案内を入れる。ノートを API で直すときは `tag_name` を一緒に送る (TASK-96)。公開はオーナーが行う。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 package.json の version と wails.json の info.productVersion が 0.3.0 になっている
- [x] #2 マイルストーン v0.4.0 があり、TASK-52 と TASK-60 が割り当てられている
- [ ] #3 v0.3.0 タグの release.yml の実行が通り、下書きのリリースに dmg・app.zip・Windows amd64 / arm64 インストーラ・latest.json・SHA256SUMS.txt が添付されている
- [ ] #4 下書きのノートが和英で書かれ、tag_name が v0.3.0 のままになっている
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- package.json の version と wails.json の info.productVersion を 0.3.0 にした。release.yml の prepare と同じく jq で `.version` / `.info.productVersion` を読み、どちらも 0.3.0 を返す。リポジトリ内でアプリの版を持つのはこの 2 か所だけ (README の v0.2.0 は手順の経緯、updater_test.go の版はテストの値)。
- `backlog milestone add v0.4.0` で m-4 を作り、TASK-52 と TASK-60 を m-3 から m-4 へ移した。
- go vet / go test ./... / pnpm check:client / pnpm test:client (27 件) が通る。
- AC #3・#4 はマージ後に v0.3.0 タグを push してから確かめる。TASK-97 の AC #2 も同じ実行で確かめる。
<!-- SECTION:NOTES:END -->
