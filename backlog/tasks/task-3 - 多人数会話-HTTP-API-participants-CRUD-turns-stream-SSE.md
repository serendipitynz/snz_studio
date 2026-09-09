---
id: TASK-3
title: '多人数会話: HTTP API (participants CRUD + turns/stream SSE)'
status: To Do
assignee: []
created_date: '2026-09-08 22:28'
updated_date: '2026-09-09 03:13'
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
- [ ] #2 kind='multi_agent' への POST /messages と POST /messages/stream がいずれも生成を行わずユーザー発言として保存され、kind='assistant' の既存挙動は変わらない (設計 §4.4)
- [ ] #3 当該 chat のターンが実行中に POST /turns/stream が重なると 409 を返す
- [ ] #4 manual 指名に他 chat の participantId を渡すと 404 になる。PATCH / DELETE /api/participants/{id} は存在しない ID のみ 404 (設計 §3 末尾・§5)
<!-- AC:END -->
