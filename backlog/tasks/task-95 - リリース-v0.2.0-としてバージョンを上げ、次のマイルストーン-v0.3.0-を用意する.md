---
id: TASK-95
title: 'リリース: v0.2.0 としてバージョンを上げ、次のマイルストーン v0.3.0 を用意する'
status: Done
assignee: []
created_date: '2026-10-10 03:21'
updated_date: '2026-10-10 06:10'
labels: []
milestone: m-2
dependencies: []
references:
  - package.json
  - wails.json
  - .github/workflows/release.yml
priority: low
type: chore
ordinal: 99000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

v0.2.0 では自動更新 (TASK-92) を初めて載せる。マイルストーン v0.2.0 (m-2) に残っている TASK-52 と TASK-60 は今回の版に入れず、次の版へ送ってリリースする (2026-10-10 に決定)。自動更新を早く届けるほど、v0.1.0 からの手動の入れ替えが要る利用者を増やさずに済む。

## 方針

- `package.json` の `version` と `wails.json` の `info.productVersion` を `0.2.0` にする。release.yml の prepare はこの 2 つがタグと一致しないと止まる。
- マイルストーン v0.3.0 を作り、TASK-52 と TASK-60 をそこへ移す。
- マージ後に `v0.2.0` タグを push し、release.yml が作った下書きのノートを v0.1.0 と同じ和英の書式で書く。英語・日本語の両方に README の v0.1.0 からの移行の段落を入れる (TASK-92 DoD #7)。公開はオーナーが行う。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 package.json の version と wails.json の info.productVersion が 0.2.0 になっている
- [x] #2 マイルストーン v0.3.0 があり、TASK-52 と TASK-60 が割り当てられている
- [x] #3 v0.2.0 タグの release.yml の実行が通り、下書きのリリースに dmg・app.zip・Windows インストーラ・latest.json・SHA256SUMS.txt が添付されている
- [x] #4 下書きのノートが和英で書かれ、両方に v0.1.0 からの移行の段落が入っている
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- package.json の version と wails.json の info.productVersion を 0.2.0 にした。release.yml の prepare が確かめる 2 か所 (jq で `.version` / `.info.productVersion`) を同じく jq で読み、どちらも 0.2.0 を返すことを確認した。scripts/wails.mjs が要求する MAJOR.MINOR.PATCH の形も満たす。
- `backlog milestone add v0.3.0` で m-3 を作り、TASK-52 と TASK-60 を m-2 から m-3 へ移した。
- go vet / go test ./... / pnpm check:client / pnpm test:client (24 件) が通る。
- AC #3・#4 はマージ後に v0.2.0 タグを push してから確かめる。リリース下書きの添付物とノートは、この PR の範囲では作れない。

- マージコミット 1bdc8e6 に v0.2.0 タグを打って push した。release.yml の実行 38029596641 は prepare / build (macOS・Windows) / attach がすべて success。prepare のノート生成は v0.1.0...v0.2.0 で比べている。
- 下書き (release id 408709966) には latest.json・SHA256SUMS.txt・macOS の .app.zip と .dmg・Windows インストーラの 5 つが付いている。.app.zip と latest.json は、手元でダウンロードして測った SHA-256 が SHA256SUMS.txt と一致した。
- ノートは v0.1.0 と同じ英語 → === → 日本語の書式で書き直した。両方の冒頭に README の v0.1.0 からの移行の段落を置き、生成された PR の一覧は英語の半分の末尾に残した。公開はオーナーが行う。
<!-- SECTION:NOTES:END -->
