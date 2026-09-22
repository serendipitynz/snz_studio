---
id: TASK-35
title: '多人数会話: 共通の状態と参加者の状態 (状態シート) を保存し、人間が編集でき、ターンの prompt に入るようにする'
status: To Do
assignee: []
created_date: '2026-09-22 08:37'
labels: []
milestone: m-1
dependencies: []
references:
  - docs/multi-agent-chat-design.md
  - internal/service/turnengine.go
  - internal/db/migrations.go
  - frontend/src/components/ParticipantPanel.tsx
  - internal/service/export.go
type: feature
ordinal: 35000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-22 の spike で、TRPG の刻々と変わる状態 (HP・所持品・場所・時刻) の置き場を設計書 §4.7 に決めた。
状態は「項目名: 値」の行を並べたテキスト (状態シート) で、持ち主は chat (共通の状態) と参加者 (参加者の状態) の 2 種。
このタスクは保存・人間による編集・prompt への注入までを作る。アクションの効果による更新は別タスク (TASK-23 の設計後)。

## やること

- migration 16: `chats.state_sheet` / `participants.state_sheet` (TEXT NOT NULL DEFAULT '')
- 上限: 共通 400 字・参加者ごと 200 字 (rune 数)。保存時に検査し、超えたら 400 (prompt 側で切り詰めない)
- API: 既存の `PATCH /api/chats/{chatId}` と `PATCH /api/participants/{participantId}` に `stateSheet` を足す
- prompt: 場面設定の直後・役割プロンプトの前に「【現在の状態】」節を 1 つ置く。共通の状態 → 参加者の状態 (編成順、参加者名の見出し付き)。
  全参加者に全員分を渡す (`receives_project_material` に依らない)。全シートが空なら節ごと出さない
- UI: 編成パネルで共通の状態 (場面設定の下) と参加者ごとの状態 (役割プロンプトの下) を編集できる。会話中も編集できる
- markdown エクスポートに現在の状態を含める
- プリセットは `stateSheet` を任意で持てる。同梱 trpg-table に初期値を入れる
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 状態シートの列が migration で追加され、共通 400 字・参加者 200 字を超える保存は 400 で拒否される
- [ ] #2 話者の system prompt で【現在の状態】節が場面設定の直後・役割プロンプトの前に入り、全参加者に全員分が渡る (テストで確認)
- [ ] #3 編成パネルから会話中に共通の状態と参加者の状態を編集でき、次のターンから prompt に反映される
- [ ] #4 markdown エクスポートと同梱 trpg-table プリセットが状態シートを扱う
- [ ] #5 go test / フロントの型検査・lint が通る
<!-- AC:END -->
