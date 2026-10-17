---
id: TASK-105
title: '多人数会話: 観戦ビューで発言をピン留めでき、ピン留めした発言が履歴に優先して残るようにする'
status: To Do
assignee: []
created_date: '2026-10-17 19:25'
labels: []
dependencies:
  - TASK-104
references:
  - docs/multi-agent-chat-design.md
  - internal/db/schema.go
  - internal/service/turnengine.go
  - frontend/src/pages/MultiAgentChatPage.tsx
type: feature
ordinal: 109000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

設計書 §4.9.4。ピン留めは、ユーザーが発言に付け外しする印で、発言スコアに大きく加点する (§4.9.3、他の加点の合計の上限より大きい値)。
強制ではなく加点なので、履歴予算を超えるほどピン留めが多いときは、ピン留めどうしで他の加点の順になる。

## やること

- migration 022: `messages.pinned` (INTEGER NOT NULL DEFAULT 0)
- 発言のピン留めを付け外しする API。ターンの排他には入れず、ターンの実行中も操作できる
- 観戦ビューの発言ごとに付け外しの操作と、ピン留め中であることの表示
- 発言スコアにピン留めの加点を足す (値は TASK-103 で決めたもの)
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 migration で messages.pinned が追加され、既存の発言は 0 になる
- [ ] #2 観戦ビューから発言のピン留めを付け外しでき、ターンの実行中も操作できる
- [ ] #3 ピン留めした発言が、直近区間より前にあっても履歴予算の範囲で選抜発言に入る (テストで確認)
<!-- AC:END -->
