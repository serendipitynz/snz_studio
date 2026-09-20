---
id: TASK-13
title: サイドバーのチャット「＋」で多人数会話も作成できるようにする
status: To Do
assignee: []
created_date: '2026-09-19 22:29'
labels: []
milestone: m-1
dependencies: []
references:
  - frontend/src/components/WorkspaceSidebar.tsx
  - frontend/src/pages/ProjectDetailPage.tsx
  - frontend/src/api/client.ts
  - docs/multi-agent-chat-design.md
ordinal: 13000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
左サイドバーのチャット欄の「＋」は `api.createChat(projectId, { title: "" })` を呼ぶだけで
(`WorkspaceSidebar.tsx:41`)、kind 未指定のため常に単独アシスタントのチャットになる。
多人数会話を作れるのはプロジェクト画面の作成フォームだけ。

対応方針: 「＋」を押したときに種別を選ぶ小メニュー (単独アシスタント / 多人数会話) を表示する。
多人数会話を選んだ場合は `kind: "multi_agent"` で空の名簿のチャットを作成し、そのチャット画面へ
遷移する。編成は既存の参加者パネルで行う。プリセットの適用は現状チャット作成時にしかできないため
(適用 API が無い)、この小メニューではプリセットを扱わない。作成後にプリセットを選べるようにする
件は別タスクとする。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 サイドバーの「＋」を押すと、単独アシスタントか多人数会話かを選ぶメニューが表示される
- [ ] #2 多人数会話を選ぶと kind=multi_agent の空の名簿のチャットが作成され、そのチャット画面に遷移する
- [ ] #3 単独アシスタントを選んだときの挙動は従来と同じ (即時作成して遷移)
<!-- AC:END -->
