---
id: TASK-88
title: 'CI: build.yml の actions を Node.js 24 で動く版に上げ、Node.js 20 の非推奨警告をなくす'
status: To Do
assignee: []
created_date: '2026-10-02 20:15'
labels: []
dependencies: []
references:
  - .github/workflows/build.yml
priority: low
type: chore
ordinal: 88000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-66 の確認で回した CI (workflow_dispatch、run 37057977503、2026-10-02) は成功したが、両ジョブの最後に次の警告が出た。

- `Node.js 20 is deprecated. The following actions target Node.js 20 but are being forced to run on Node.js 24: actions/checkout@v4, actions/setup-go@v5, actions/setup-node@v4, actions/upload-artifact@v4, pnpm/action-setup@v4.`

いまはランナーが Node.js 24 で強制的に動かしているので動作に問題は出ていない。ただし Node.js 20 向けの版はいずれ動かなくなる (GitHub の告知: https://github.blog/changelog/2025-09-19-deprecation-of-node-20-on-github-actions-runners/)。

`.github/workflows/build.yml` で使っている版と、2026-10-03 時点の最新版 (いずれも action.yml の `runs.using` が node24):

| action | 使用中 | 最新 |
|---|---|---|
| actions/checkout | v4 | v7.0.1 |
| actions/setup-go | v5 | v7.0.0 |
| actions/setup-node | v4 | v7.0.0 |
| actions/upload-artifact | v4 | v7.0.1 |
| pnpm/action-setup | v4 | v6.1.0 |

## 方針

- 5 つの action を、Node.js 24 で動くメジャー版に上げる。最新まで上げるか、node24 になった最初のメジャー版で止めるかは、着手時に各版の破壊的変更 (入力名・既定値の変更、upload-artifact の成果物の扱いなど) を読んで決める。
- 上げた後に workflow_dispatch で CI を回し、成果物 (snz-studio-Windows / snz-studio-macOS) の中身が上げる前と変わらないことを確かめる。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 build.yml の 5 つの action (checkout / setup-go / setup-node / upload-artifact / pnpm/action-setup) が、action.yml の runs.using が node24 の版になっている
- [ ] #2 workflow_dispatch の CI が Windows / macOS とも成功し、ログに Node.js 20 の非推奨警告が出ない
- [ ] #3 成果物 snz-studio-Windows / snz-studio-macOS に、上げる前と同じファイル (アプリ本体、llama-server と DLL / dylib、LICENSE と THIRD_PARTY_NOTICES.md、インストーラー) が入っている
<!-- AC:END -->
