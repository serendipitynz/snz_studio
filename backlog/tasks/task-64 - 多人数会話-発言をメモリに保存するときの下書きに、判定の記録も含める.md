---
id: TASK-64
title: '多人数会話: 発言をメモリに保存するときの下書きに、判定の記録も含める'
status: To Do
assignee: []
created_date: '2026-09-27 01:31'
labels: []
dependencies:
  - TASK-37
references:
  - internal/httpapi/multiagent_memory.go
  - docs/multi-agent-chat-design.md
type: feature
ordinal: 64000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-37 の確認 (2026-09-27) で分かったこと。発言の「メモリに保存」の下書き (GET /api/messages/{messageId}/memory-draft、設計書 §4.4) は発言本文 (messages.content) だけから作る。/roll の判定は本文から剥がして messages.dice_rolls に保存するので (§4.8.3 の 2)、コマンドだけの発言 (「/roll 2d6 目標7 成功判定」) では下書きが空になる。本文のある発言でも判定が下書きに入らない。

コピーボタンは TASK-37 の中で、本文の後にチップと同じ文面の判定の行を付けるように直した。結論の下書き (§4.4) は、prompt と同じ【ダイス】行を含めている。

## やること

- 下書きの本文に、判定の記録を本文の後の行として含める。書式はコピー (チップの文面) か prompt の【ダイス】行のどちらに揃えるかを着手時に決める
- 種類の推定 (InferMemoryKind) に判定の行を渡すか決める
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 コマンドだけの発言の「メモリに保存」で、判定の記録が下書きに入る (テストで確認)
- [ ] #2 本文と判定のある発言では、本文の後に判定の行が入る (テストで確認)
- [ ] #3 go test / フロントの型検査が通る
<!-- AC:END -->
