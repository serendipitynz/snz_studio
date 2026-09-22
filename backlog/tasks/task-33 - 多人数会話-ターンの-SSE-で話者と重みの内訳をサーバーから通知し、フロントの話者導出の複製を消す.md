---
id: TASK-33
title: '多人数会話: ターンの SSE で話者と重みの内訳をサーバーから通知し、フロントの話者導出の複製を消す'
status: To Do
assignee: []
created_date: '2026-09-21 22:55'
updated_date: '2026-09-22 01:40'
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
ターン開始「前」に予測する形なので、編成が走行中に変わると予測が外れる。TASK-34 で入る `weighted` は
4 係数と同点規則を持つので、複製したままにはできない。設計は `docs/multi-agent-chat-design.md` §4.6.6。

## やること

- `TurnEngine.RunTurn` に `onSpeaker` を足し、`endpointAccepts` を通った直後に呼ぶ。渡すのは選ばれた話者と、
  規則が重みを計算する場合はその内訳 (参加者ごとの重みと、掛かった係数の一覧)。既存 3 規則は内訳を持たないので空。
- `handleRunTurnStream` が `onSpeaker` で `speaker` イベントを 1 つ流す。
- 観戦ビューは `speaker` イベントで発話中の話者名を出す。`predictNextSpeaker` と `turnOrder.ts` を削除する
  (`manual` の指名候補表示だけは編成から直接引く)。重みの内訳の見せ方はこのタスクで決める
  (最小で「なぜこの人か」が分かれば十分。既存 3 規則では内訳が空なので、出す場所は畳んでおける形にする)。

## イベントの位置 (§4.6.6)

`endpointAccepts` の直後でなければならない。`handleRunTurnStream` は最初の delta まで SSE ライタを開かず、
ストリームが開く前の失敗だけが HTTP ステータスとして返る作りである。この位置なら次のステータスは残る:
ターン重複 409 / chat・参加者不在 404 / 規則と指名の不整合・除籍済みの指名 400 / エンドポイント不通 502。

**ステータスが 1 つ変わることは受け入れる**: `endpointAccepts` より後の失敗 (参加者一覧の読み出し・生成・発言の保存) は、
現在は delta が 1 つも届いていなければ `turnErrorResponse` の既定分岐で HTTP 500、届いていればストリーム内の
`error` になる。`speaker` を先に出すとストリームが常に開くので、この 500 は一律にストリーム内 `error` に変わる。
同じ失敗が「delta が 1 つでも届いたか」で 2 通りに分かれる現在の挙動より揃うので、これは改善として扱う。
フロントは既にストリーム内 `error` を扱える。

## 採らなかった形 (§4.6.6)

- `GET /api/chats/{chatId}/next-speaker` の事前問い合わせ: ターンごとに 1 往復増え、問い合わせと実行のあいだに
  人間の介入が入ると答えが古くなる。
- `done` フレームに次ターンの話者を載せる: 次のターンの前提 (介入・編成の変更・規則の変更) はそのあいだに
  変わりうるので、載せた時点で正しいとは限らない。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 ターンの SSE が、生成開始の前に話者 (と、規則が計算する場合は重みの内訳) を伝える `speaker` イベントを 1 つ流す
- [ ] #2 ターン前の失敗がステータスで返ることがテストで確認されている: ターン重複 409 / chat・参加者不在 404 / 規則と指名の不整合 400 / エンドポイント不通 502
- [ ] #3 生成そのものの失敗が、delta の有無にかかわらずストリーム内の `error` イベントになることがテストで確認されている (従来の HTTP 500 からの変更を意図したものとして固定する)
- [ ] #4 観戦ビューの発話中の話者名がサーバーの通知だけで決まり、`frontend/src/api/turnOrder.ts` が削除されている
- [ ] #5 既存の round_robin / manual / facilitator_alternating の話者選択そのものは変わっていない
<!-- AC:END -->
