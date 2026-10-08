---
id: TASK-92.3
title: 'UI: 起動時の自動確認、設定画面の版表示・手動確認・自動確認の切り替え、更新ダイアログを作る'
status: To Do
assignee: []
created_date: '2026-10-07 22:53'
updated_date: '2026-10-07 22:57'
labels: []
dependencies:
  - TASK-92.2
references:
  - frontend/src/components/SettingsModal.tsx
  - frontend/src/components/Dialog.tsx
  - frontend/src/i18n/index.tsx
  - internal/config/config.go
parent_task_id: TASK-92
priority: medium
type: feature
ordinal: 95000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
TASK-92 の UI 側。TASK-92.2 のバインドを使い、自動確認・手動確認・更新ダイアログを作る。

## 作業

- **自動確認**: 起動後に遅らせて 1 回行い、初回の描画や起動時の読み込みと競合させない。失敗しても何も出さない。オフラインで起動するたびにエラーが出るアプリは、確認しないアプリより悪い。
- **設定画面 (`components/SettingsModal.tsx`)**: 実行中の版を表示する (今は macOS の About にしか版が出ず、Windows では更新されたかどうかを確かめられない)。手動確認のボタンを置き、「最新です」「新しい版があります」「確認できませんでした」の 3 通りの結果を出す。自動確認のオン/オフを置き、`app-config.json` (`internal/config`) に保存する。既定値は有効 (TASK-92 で決定)。
- **更新ダイアログ**: 新しい版の番号を見せ、利用者が承認するまでダウンロードしない。承認後は進捗を出す。
  - macOS は入れ替え後に新しい版で起動し直す。
  - Windows はインストーラに渡した時点でアプリが終了する。再起動を待つ状態のまま残らないようにする。
  - 「システムがパスワードや管理者の承認を求めることがある」と書く。仕組みの名前は出さない。Windows は per-user インストール (TASK-92.2) なので通常は承認を求めないが、確かめる前に「求めない」とは書かない。
  - latest.json の notes が空でも読めるようにし、Releases ページへのリンクを置く。
  - 承認後の失敗は「更新は行われませんでした」とだけ伝える。
  - macOS で入れ替えられないとき (TASK-92.2 が理由を返す場合) は、Releases ページを開く案内に切り替える。
- 確認には in-app のダイアログ (`components/Dialog` / `ConfirmDialog`) を使う。`window.confirm` は使わない (Wails の WebView では表示されず、常に false になる)。
- 新しい画面要素は snz-design の adoption guide (doc-16) と snz_studio の adoption record (doc-17) に沿わせる。
- 新しい文言は ja と en の両方の辞書 (`frontend/src/i18n`) に入れる。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 オフラインで起動しても、自動確認はエラーを何も出さない
- [ ] #2 設定画面に実行中の版が出て、手動確認が「最新」「新しい版あり」「確認できなかった」の 3 通りを出し分ける
- [ ] #3 新しい版の番号を見せて承認を得るまで、何もダウンロードもインストールもしない
- [ ] #4 進捗が見え、macOS は新しい版で起動し直し、Windows はアプリが終了した後に再起動を待つ状態のまま残らない
- [ ] #5 システムがパスワードや管理者の承認を求めることがあると伝え、notes が空でもダイアログが読める
- [ ] #6 新しい文言が ja と en の両方の辞書にあり、snz-design の doc-16 / doc-17 に沿っている
- [ ] #7 pnpm build と pnpm test が通る
- [ ] #8 起動後に初回の描画を遅らせずに自動確認が走り、既定で有効で、設定画面でオフにでき、その選択が app-config.json に保存される
- [ ] #9 承認後の失敗を「更新は行われなかった」と伝え、エラー文面で分岐していない
<!-- AC:END -->
