---
id: TASK-15
title: '多人数会話: チャット画面で右サイドバー (編成パネル) の表示・非表示を切り替えられるようにする'
status: In Review
assignee: []
created_date: '2026-09-19 22:29'
updated_date: '2026-09-20 09:17'
labels: []
milestone: m-1
dependencies: []
references:
  - frontend/src/pages/MultiAgentChatPage.tsx
  - frontend/src/pages/ChatPage.tsx
ordinal: 15000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
単独アシスタントのチャット画面は、右サイドバー (コンテキストインスペクタ) の折りたたみ状態を
`isInspectorCollapsed` で持ち (`ChatPage.tsx:119`)、localStorage キー `snz.chat.inspectorCollapsed`
に保存し、ヘッダーのアイコンボタンで切り替え、`WorkspaceShell` の grid 列を変えて本文を広げている。

多人数会話のチャット画面 (`MultiAgentChatPage.tsx`) は `InspectorPane` に編成パネルを常時描画して
おり切り替えが無い。同じ仕組みを移植する。折りたたみ状態は単独チャットと共有せず別キーで保存する
(用途が異なるパネルなので、片方を閉じたらもう片方も閉じる連動は望まれない)。
ターン実行中は編成パネルが disabled になるが、表示・非表示の切り替えはターン実行中でも可能で
よい。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 多人数会話のチャット画面のヘッダーに、編成パネルの表示・非表示を切り替えるボタンがある
- [ ] #2 非表示にすると本文 (会話ログと入力欄) が右サイドバーの幅まで広がる
- [ ] #3 折りたたみ状態はアプリを再起動しても保持され、単独チャットのインスペクタの状態とは独立している
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. ChatPage の折りたたみ機構 (isInspectorCollapsed / localStorage / ヘッダーの IconButton / grid 列変更) を MultiAgentChatPage に移植する。
2. localStorage キーは単独チャットと別にする: snz.multiAgent.rosterCollapsed。
3. 列変更は WorkspaceShell の $columns prop を使う (ChatPage の inline style はメディアクエリを上書きしてしまうため、移植先では prop を採る)。
4. i18n に multiAgent.showRoster / multiAgent.hideRoster を en/ja 追加。
5. PanelOpenIcon / PanelCloseIcon はこのリポジトリの慣習 (アイコンはファイルローカル定義) に合わせ MultiAgentChatPage 内に置く。
6. pnpm check:client と pnpm build:client で検証。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
ChatPage のインスペクタ折りたたみ機構を MultiAgentChatPage へ移植した。ヘッダーの再読み込みボタンの隣にトグル用 IconButton を置き、`isRosterCollapsed` を localStorage キー `snz.multiAgent.rosterCollapsed` に保存する。単独チャットの `snz.chat.inspectorCollapsed` とは別キーなので連動しない。ターン実行中もトグルは押せる (`disabled` を付けていない)。

移植元と意図的に変えた 2 点:

1. grid 列の切り替えは inline `style` ではなく `WorkspaceShell` の `$columns` prop を使った。ChatPage は `style={{ gridTemplateColumns: ... }}` で渡しているが、inline style は styled-component 内のメディアクエリより強いため、1180px 以下でサイドバーが 250px に縮まる指定を潰してしまう。`$columns` はベース宣言に差し込まれるだけなのでメディアクエリが生きる。ChatPage 側は本タスクの範囲外なので触っていない (同じ潰れ方が残っている)。

2. 折りたたみ時に `InspectorPane` をアンマウントせず `display: none` で畳んだ。ChatPage のインスペクタは表示専用だが、ParticipantPanel は保存前の下書き (表示名・ロールプロンプト・接続先・モデル・シーン) をローカル state に持っている。アンマウントすると編集途中の内容が黙って消えるため、レイアウトから外すだけにしている。`display: none` の要素は grid トラックを占めないので、本文の幅は 2 列指定どおりに広がる。

アイコン (PanelOpenIcon / PanelCloseIcon) は ChatPage の定義を複製してこのファイルに置いた。共有モジュール化ではなくファイルローカル定義が既存の慣習 (PlusIcon / SpinnerIcon / EditIcon などが既に複数ファイルに重複している)。

i18n は `multiAgent.showRoster` / `multiAgent.hideRoster` を en/ja に追加。

検証: `pnpm check:client` (tsc --noEmit) と `pnpm build:client` がともに成功。このリポジトリにフロントエンドのテストランナーは無い。AC #1〜#3 はいずれも実機 (wails dev) での画面確認が必要なため未チェックのまま残している (トグルボタンの存在と動作、非表示時に本文が右端まで広がること、再起動後も状態が残り単独チャットと独立していること)。
<!-- SECTION:NOTES:END -->
