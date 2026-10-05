---
id: TASK-90
title: 'リリース: v0.1.0 としてバージョンを上げ、README にアプリのアイコンを載せ、次のマイルストーン v0.2.0 を用意する'
status: Done
assignee: []
created_date: '2026-10-04 22:26'
updated_date: '2026-10-05 00:18'
labels: []
milestone: m-1
dependencies: []
references:
  - package.json
  - wails.json
  - README.md
  - README.ja.md
priority: low
type: chore
ordinal: 90000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

マイルストーン v0.1.0 (m-1) のタスクがすべて終わったので、v0.1.0 としてリリースする。`wails.json` の `info.productVersion` はすでに `0.1.0` だが、`package.json` の `version` は `0.0.0` のまま残っている。

## 方針

- `package.json` の `version` を `0.1.0` にする。
- README.md / README.ja.md の先頭に、アプリのアイコンを幅 128px で載せる。書き方は serendipitynz/backlog-atlas の README に合わせる。`build/appicon.png` は 1024px・767KB で README には重いので、そこから縮小した 256px 版 (`docs/assets/appicon-256.png`、Retina 用に表示幅の 2 倍) を置いて参照する。
- マイルストーン v0.2.0 を作り、To Do に残っている TASK-52 と TASK-60 をそこへ移す。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 package.json の version が 0.1.0 になっている
- [x] #2 README.md と README.ja.md の先頭に docs/assets/appicon-256.png が幅 128px で表示される
- [x] #3 マイルストーン v0.2.0 があり、TASK-52 と TASK-60 が割り当てられている
<!-- AC:END -->
