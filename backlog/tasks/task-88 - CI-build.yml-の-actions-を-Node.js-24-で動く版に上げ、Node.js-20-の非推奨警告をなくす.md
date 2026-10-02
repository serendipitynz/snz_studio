---
id: TASK-88
title: 'CI: build.yml の actions を Node.js 24 で動く版に上げ、Node.js 20 の非推奨警告をなくす'
status: Done
assignee: []
created_date: '2026-10-02 20:15'
updated_date: '2026-10-02 22:10'
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
- [x] #1 build.yml の 5 つの action (checkout / setup-go / setup-node / upload-artifact / pnpm/action-setup) が、action.yml の runs.using が node24 の版になっている
- [x] #2 workflow_dispatch の CI が Windows / macOS とも成功し、ログに Node.js 20 の非推奨警告が出ない
- [x] #3 成果物 snz-studio-Windows / snz-studio-macOS に、上げる前と同じファイル (アプリ本体、llama-server と DLL / dylib、LICENSE と THIRD_PARTY_NOTICES.md、インストーラー) が入っている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. build.yml の 5 action を最新メジャーへ: checkout/setup-go/setup-node/upload-artifact を v7、pnpm/action-setup を v6 (各版の破壊的変更はこのワークフローに影響しないことを release notes で確認済み)
2. 作業ブランチを push し、そのブランチで workflow_dispatch の CI を回す
3. 両ジョブの成功と、ログに Node.js 20 の非推奨警告がないことを確認する
4. run 37057977503 (上げる前) と新しい run の成果物を取得し、ファイル一覧を比較する
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 上げた版
checkout / setup-go / setup-node / upload-artifact を v7、pnpm/action-setup を v6 (いずれも最新メジャー、action.yml の runs.using は node24)。node24 になった最初の版 (v5/v6/v5/v6/v5) で止める案もあったが、途中の破壊的変更がどれもこのワークフローに当たらないため最新に揃えた (着手時にユーザーと合意)。
- checkout v6 (認証情報を別ファイルに保存) と v7 (pull_request_target での fork checkout 禁止) は workflow_dispatch だけのこのワークフローでは無関係。
- setup-node v5/v6 の自動キャッシュは npm だけが対象。cache: pnpm を明示しているので影響なし。
- upload-artifact v7 の archive 入力は既定 true (従来どおり zip)。
- pnpm/action-setup v6 は pnpm 11 で起動して self-update で 10.30.3 に切り替える方式。README は「pnpm v10 以下は引き続き pnpm/action-setup」としており、後継の pnpm/setup は pnpm v11 以降向けなので移行しない。
- setup-go v6 以降は go.mod の toolchain 行 (go1.27.1) を入れ、GOTOOLCHAIN=local を設定する。scripts/wails.mjs が固定する版と同じなので、以前の「1.26.3 を入れてから 1.27.1 を追加ダウンロード」がなくなった。

## 確認 (run 37059852082、ブランチ TASK-88-actions-node24)
- AC#2: Windows / macOS とも success。check-run の annotation は macOS ランナーの混雑 notice だけで、上げる前の run 37057977503 では両ジョブに出ていた「Node.js 20 is deprecated」警告は 0 件。
- AC#3: 両 run の成果物を取得し、Windows zip の 34 ファイルと macOS dmg 内の 43 エントリ (シンボリックリンク込み) の名前が一致。LICENSE / THIRD_PARTY_NOTICES.md / llama-server と DLL・dylib は sha256 も一致。
- ハッシュが変わったのはアプリ本体 (SNZ Studio.exe / .app の実行ファイル) とそれを含むインストーラーだけ。ソースは両 run で同一 (backlog と .github 以外に差分なし)、go version -m のビルド情報 (go1.27.1、ldflags、tags) も同一。差は埋め込まれる GOROOT パスで、以前は ~/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.1、今回は hostedtoolcache/go/1.27.1 になっている (上の setup-go の変化による)。

## 未確認・気づいた点
- 上げた後のアプリを実機で起動しての確認はしていない (コンパイラ・フラグ・依存が同じなので挙動差はない想定)。
- pnpm/action-setup v6 の Windows ジョブで Node の DEP0190 (shell: true の child_process に引数を渡している) の DeprecationWarning がログに 1 件出る。action 内部の実装由来で、annotation にはならず、ビルドへの影響もない。
<!-- SECTION:NOTES:END -->
