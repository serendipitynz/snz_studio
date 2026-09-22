---
id: TASK-23
title: '多人数会話: ダイスと判定の扱いを設計する (TRPG の乱数)'
status: To Do
assignee: []
created_date: '2026-09-20 00:18'
updated_date: '2026-09-22 08:43'
labels: []
milestone: m-1
dependencies: []
references:
  - docs/multi-agent-chat-design.md
  - internal/service/turnengine.go
  - internal/httpapi/multiagent.go
type: spike
ordinal: 23000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TRPG の判定はダイスの出目に依存するが、LLM に「d20 を振れ」と言っても出目は偏り、
物語の都合に合わせた値を返しがちで、判定の公正さが成り立たない。

## このタスクで決めること (実装はしない)

1. 出目の生成元: 人間が介入発言で出目を書く (現状でも可能) / アプリがサーバー側で乱数を生成し
   「system の発言」として履歴に挿入する / 参加者の発言中の `1d20` のような記法をアプリが検出して
   出目を後置する。
2. 履歴上の表現: 出目を誰の発言として保存するか (participant_id なしの system 相当の role が
   要るか)。既存の履歴マッピング (`mapHistoryForSpeaker`) との整合。
3. UI: 介入欄の隣に「ダイスを振る」操作を置くか、記法検出だけにするか。
4. ルール判定 (目標値との比較、修正値) をどこまでアプリが持つか。最小は「出目を公正に出す」だけ。

各案の利点・欠点と推奨を設計書に追記し、実装タスクを切る。最小案 (出目をアプリが生成して履歴に挿入)
で足りるかを先に判断する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 出目の生成元・履歴上の表現・UI・判定範囲の 4 点について、案と推奨が設計書に記録されている
- [ ] #2 推奨案の実装タスクが起票されている (または現状の人間による介入で足りると判断した根拠が記録されている)
- [ ] #3 状態項目へのアクションの効果 (set / add、設計書 §4.7、TASK-36) をどのアクションが生むか、および前提条件 (持っていない物は使えない等) をどこで判定するかが記録されている。最小案 (人間の介入で足りる) を採る場合は TASK-36 の扱い (取り下げ・保留) も記録されている
<!-- AC:END -->
