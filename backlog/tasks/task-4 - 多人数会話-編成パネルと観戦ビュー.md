---
id: TASK-4
title: '多人数会話: 編成パネルと観戦ビュー'
status: To Do
assignee: []
created_date: '2026-09-08 22:28'
labels: []
dependencies:
  - TASK-3
references:
  - docs/multi-agent-chat-design.md
ordinal: 4000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/multi-agent-chat-design.md §6 のフロントエンドを実装する。参加者の CRUD と接続確認・モデル選択 (POST /api/configuration/models 流用) を行う編成パネル、発言者名つきストリーミング表示の観戦ビュー、進行コントロール (1 ターン進める / 自動進行の開始・停止 = フロント側ループ / manual 時の指名)。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 UI から参加者編成〜自動進行〜停止まで操作できる (Phase B 完了条件)
- [ ] #2 発言に参加者の表示名とモデル名が表示される
<!-- AC:END -->
