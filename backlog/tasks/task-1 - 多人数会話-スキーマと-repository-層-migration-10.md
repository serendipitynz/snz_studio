---
id: TASK-1
title: '多人数会話: スキーマと repository 層 (migration 10)'
status: To Do
assignee: []
created_date: '2026-09-08 22:28'
updated_date: '2026-09-09 03:13'
labels: []
dependencies: []
references:
  - docs/multi-agent-chat-design.md
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/multi-agent-chat-design.md §2〜§3 に従い、chats.kind / turn_rule / scene_prompt 列の追加、participants テーブル新設 (deleted_at による論理削除を含む)、messages.participant_id 列の追加を migration 10 として実装し、repository 層に参加者の CRUD と participant_id つきメッセージ保存を足す。参加者取得は編成 (deleted_at IS NULL) と帰属解決用の全行を別メソッドで返す。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 migration 10 が既存 DB に適用でき、既存 chat (kind='assistant') の動作に影響がない
- [ ] #2 participants の CRUD が repository 層で動き、テストがある
- [ ] #3 go build / go vet / go test が全グリーン
- [ ] #4 参加者を除籍しても過去の発言の表示名・モデル名が解決でき、round_robin の起点が失われない (設計 §3)
- [ ] #5 編成取得と帰属解決用の全行取得が別メソッドとして存在し、除籍済み参加者を含むケースのテストがある
- [ ] #6 repository 層の参加者取得・更新・削除が chat_id つきで引ける形になっており (サービス層が同一 chat 検証を行える)、除籍済みを含むケースのテストがある
<!-- AC:END -->
