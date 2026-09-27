---
id: TASK-64
title: '多人数会話: 発言をメモリに保存するときの下書きに、判定の記録も含める'
status: In Review
assignee: []
created_date: '2026-09-27 01:31'
updated_date: '2026-09-27 09:33'
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
- [x] #1 コマンドだけの発言の「メモリに保存」で、判定の記録が下書きに入る (テストで確認)
- [x] #2 本文と判定のある発言では、本文の後に判定の行が入る (テストで確認)
- [x] #3 go test / フロントの型検査が通る
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. memory-draft の content を「本文 + 空行 + 判定ごとの 🎲 + commands.DiceRollLine 行」にする (markdown エクスポートと同じ文面、引用の > は付けない)。/roll の有効・無効によらず記録済みの判定を含める
2. kind は本文だけから推定する (判定の行は渡さない)
3. multiagent_memory_test に、コマンドだけの発言と本文 + 判定の発言の下書きのテストを足す
4. 設計書 §4.4 の「保存の下書き」、§4.8.3 の細部、docs/current-spec (日英) の該当箇所を改訂
5. go test ./... / pnpm run check:client
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 着手時に決めたこと (2026-09-27、オーナー判断)

- 下書きの判定の書式: prompt の【ダイス】行でもチップの文面でもなく、markdown エクスポートと同じ `🎲 ` + commands.DiceRollLine の行 (引用の > は付けない) を、本文の後に空行を挟んで判定ごとに並べる。【ダイス】にしないのは、TASK-65 で /roll が会話ごとの設定になり、メモリはプロジェクト内のすべての会話に渡る一方、【ダイス】行をアプリの記録として扱うかは会話ごとに違うため (roll が有効な会話の資料に入ると、いま振った記録と見分けられず、模倣の材料にもなる)。チップの文面はフロントの i18n で組み立てるので、サーバへの移植が要り、保存するメモリの言語が UI の言語で変わる
- 種類の推定 (InferMemoryKind) には判定の行を渡さず、本文だけから推定する。行動の文面 (「必ず跳ぶ」) が cue に当たって kind を決めないため。コマンドだけの発言は既定の episodic になる
- TASK-65 の「無効にしても過去の判定の記録はチップにも prompt にも残す」に合わせ、roll を無効にした後も記録済みの判定を下書きに含める

## 検証

- AC #1 / #2: httpapi.TestMultiAgentMessageMemoryDraftRolls。コマンドだけの発言 (「/roll 2d6 目標7 毎回の成功判定」) の下書きが 🎲 行だけになり kind が episodic、本文 + /roll の発言が「本文 + 空行 + 🎲 行」で kind が本文どおり semantic (行動の「必ず」「毎回」は procedural の cue なので、判定の行を推定に渡すと落ちる)。commands を {} にした後も記録済みの判定が下書きに残る
- AC #3: go test ./... 通過、go vet 警告なし、pnpm run check:client (tsc) 通過。gofmt の差分は既存の internal/search/model.go だけ
- 設計書 §4.4 (保存の下書き)・§4.8.3 の細部・§5 の API 表と docs/current-spec.ja.md §6.5 を改訂した。英語版の current-spec には下書きの記述が無いので触っていない
- フロントは変更なし (ダイアログはサーバの下書きをそのまま開く)。実窓での確認はしていない
<!-- SECTION:NOTES:END -->
