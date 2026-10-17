---
id: TASK-106
title: '多人数会話: 選抜発言と末尾の発言の切り詰めを発言の参照として保存し、参照一覧に表示する'
status: To Do
assignee: []
created_date: '2026-10-17 19:25'
updated_date: '2026-10-17 19:58'
labels: []
dependencies:
  - TASK-104
references:
  - docs/multi-agent-chat-design.md
  - internal/db/schema.go
  - internal/service/turnengine.go
type: feature
ordinal: 110000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

設計書 §4.9.5。AGENTS.md の Required UX「回答に使った参照を見せる」を過去の発言にも広げ、各発言が直近区間より前のどの発言を読んだかを示す。
直近区間は毎ターン必ず読むので参照に入れない。あわせて、末尾の発言を窓上限に収めるために中略したとき (§4.9.2 手順 3) は、
画面に見えている発言とモデルが読んだ発言が食い違う唯一の場合なので、同じ参照一覧で示す。

## やること

- `assistant_message_references.source_type` の CHECK 制約に発言を指す値を足す (テーブルの作り直しを伴う migration)
- ターンの完了時に、そのターンの選抜発言と、末尾の発言を中略したこと (どの発言を何字に縮めたか) を参照として保存する
- 発言の参照一覧に、選抜発言 (話者名と抜粋) と中略の記録を表示する
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 選抜発言がそのターンの発言の参照として保存され、直近区間の発言は保存されない (テストで確認)
- [ ] #2 観戦ビューの参照一覧で、選抜発言が話者名と抜粋つきで表示される
- [ ] #3 既存の参照 (project / document / memory) の保存と表示が変わらない
- [ ] #4 末尾の発言を中略したターンでは、どの発言を何字に縮めたかが参照一覧に表示される
<!-- AC:END -->
