---
id: TASK-15
title: '多人数会話: チャット画面で右サイドバー (編成パネル) の表示・非表示を切り替えられるようにする'
status: To Do
assignee: []
created_date: '2026-09-19 22:29'
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
