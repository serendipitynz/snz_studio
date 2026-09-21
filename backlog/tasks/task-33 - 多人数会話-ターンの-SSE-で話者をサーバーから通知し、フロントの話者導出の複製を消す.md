---
id: TASK-33
title: '多人数会話: ターンの SSE で話者をサーバーから通知し、フロントの話者導出の複製を消す'
status: To Do
assignee: []
created_date: '2026-09-21 22:55'
labels: []
dependencies: []
references:
  - docs/multi-agent-chat-design.md
  - internal/httpapi/multiagent.go
  - internal/service/turnengine.go
  - frontend/src/api/turnOrder.ts
  - frontend/src/pages/MultiAgentChatPage.tsx
type: feature
ordinal: 33000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

観戦ビューは生成中の発言に話者名を付けるため、`frontend/src/api/turnOrder.ts` の `predictNextSpeaker` が
エンジンと同じ導出 (`round_robin` と `facilitator_alternating`) を複製している。規則が増えるたびに複製も増え、
ターン開始「前」に予測する形なので、編成が走行中に変わると予測が外れる。設計は
`docs/multi-agent-chat-design.md` §4.6.6 で決めてある。

## やること

- `TurnEngine.RunTurn` に `onSpeaker func(*model.Participant)` を足し、`endpointAccepts` を通った直後に呼ぶ。
  **この位置でなければならない**: `handleRunTurnStream` は最初の delta まで SSE ライタを開かず、それがターン前の
  失敗を本物の HTTP ステータス (409 / 404 / 503、§5) で返すための作りである。`endpointAccepts` より前に
  ストリームを開くと、それらのステータスがストリーム内の `error` イベントに化ける。
- `handleRunTurnStream` が `onSpeaker` で `speaker` イベント (`{ participantId, displayName }`) を 1 つ流す。
- 観戦ビューは `speaker` イベントで発話中の話者名を出す。`predictNextSpeaker` と `turnOrder.ts` を削除する
  (`manual` の指名候補表示だけは編成から直接引く)。

## 採らなかった形 (§4.6.6)

- `GET /api/chats/{chatId}/next-speaker` の事前問い合わせ: ターンごとに 1 往復増え、問い合わせと実行のあいだに
  人間の介入が入ると答えが古くなる。
- `done` フレームに次ターンの話者を載せる: 次のターンの前提 (介入・編成の変更・規則の変更) はそのあいだに
  変わりうるので、載せた時点で正しいとは限らない。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 ターンの SSE が、生成開始の前に話者を伝える `speaker` イベントを 1 つ流す
- [ ] #2 ターン前の失敗 (ターン重複 409 / 参加者不在 404 / エンドポイント不通) が、従来どおり HTTP ステータスで返ることがテストで確認されている
- [ ] #3 観戦ビューの発話中の話者名がサーバーの通知だけで決まり、`frontend/src/api/turnOrder.ts` が削除されている
- [ ] #4 既存の round_robin / manual / facilitator_alternating の話者選択そのものは変わっていない
<!-- AC:END -->
