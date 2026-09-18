---
id: TASK-8
title: 'CI: build ワークフローを実走させて通ることを確認する'
status: To Do
assignee: []
created_date: '2026-09-18 23:32'
labels: []
milestone: m-0
dependencies:
  - TASK-7
references:
  - 'https://github.com/serendipitynz/snz_studio/pull/7'
ordinal: 8000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`.github/workflows/build.yml` は private repo の課金分 (macOS ランナーは 10x) を避けるため
`workflow_dispatch` 限定で、これまで一度も実走していない。TASK-7 でビルドステップ自体を
書き換えた (`wails build ...` → `node scripts/wails.mjs build ...`) ので、現状「CI が通る」
ことの根拠はローカルでの同等手順の再現だけで、実走の証拠が無い。TASK-7 の AC#2 は
この確認をここへ委ねる形に改めてクローズした。

実走で初めて分かる点が 2 つある:
- **Windows ジョブがランチャを通ること**。`scripts/wails.mjs` の Windows 動作は手元に
  Windows が無いため未検証で、根拠は「Node の spawn に env を渡す方式はシェル非依存」と
  いう設計上のものだけ。windows-latest ジョブがこれを実際に踏む唯一の場所。
- **setup-go が入れる floor 版 (go 1.26.3) から `toolchain` 指定の go1.27.1 への
  自動ダウンロードがランナー上で成立すること**。

実行するときは `gh workflow run build.yml` → Actions タブで両ジョブの結果を確認する。
失敗したら、ここで直すのか TASK-7 の follow-up として別に切るのかを判断する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 build ワークフローを workflow_dispatch で実走させ、macOS / Windows 両ジョブが成功する
- [ ] #2 失敗した場合は原因と対処 (この場で修正 / 別タスク化) が記録されている
<!-- AC:END -->
