---
id: TASK-1
title: '多人数会話: スキーマと repository 層 (migration 10)'
status: To Do
assignee: []
created_date: '2026-09-08 22:28'
labels: []
dependencies: []
references:
  - docs/multi-agent-chat-design.md
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/multi-agent-chat-design.md §2〜§3 に従い、chats.kind / turn_rule / scene_prompt 列の追加、participants テーブル新設、messages.participant_id 列の追加を migration 10 として実装し、repository 層に参加者の CRUD と participant_id つきメッセージ保存を足す。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 migration 10 が既存 DB に適用でき、既存 chat (kind='assistant') の動作に影響がない
- [ ] #2 participants の CRUD が repository 層で動き、テストがある
- [ ] #3 go build / go vet / go test が全グリーン
<!-- AC:END -->
