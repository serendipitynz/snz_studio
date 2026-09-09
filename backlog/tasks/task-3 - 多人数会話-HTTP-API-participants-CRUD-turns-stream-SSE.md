---
id: TASK-3
title: '多人数会話: HTTP API (participants CRUD + turns/stream SSE)'
status: To Do
assignee: []
created_date: '2026-09-08 22:28'
labels: []
dependencies:
  - TASK-2
references:
  - docs/multi-agent-chat-design.md
ordinal: 3000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/multi-agent-chat-design.md §5 のルートを追加する。chat 作成の kind 受け付け、PATCH /api/chats/{chatId} の turnRule / scenePrompt 拡張、participants の CRUD 4 ルート、POST /api/chats/{chatId}/turns/stream (SSE)。kind='multi_agent' の chat では既存 messages ルートは保存のみ行い生成しない。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 curl だけで多人数会話の作成〜参加者登録〜ターン実行 (SSE で発言が流れる) ができる (Phase A 完了条件)
- [ ] #2 kind='multi_agent' への POST /messages が生成を行わずユーザー発言として保存される
<!-- AC:END -->
