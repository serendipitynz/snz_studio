---
id: TASK-33
title: '多人数会話: ターンの SSE で話者と重みの内訳をサーバーから通知し、フロントの話者導出の複製を消す'
status: Done
assignee: []
created_date: '2026-09-21 22:55'
updated_date: '2026-09-22 05:55'
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
- [x] #1 ターンの SSE が、生成開始の前に話者 (と、規則が計算する場合は重みの内訳) を伝える `speaker` イベントを 1 つ流す
- [x] #2 ターン前の失敗がステータスで返ることがテストで確認されている: ターン重複 409 / chat・参加者不在 404 / 規則と指名の不整合 400 / エンドポイント不通 502
- [x] #3 生成そのものの失敗が、delta の有無にかかわらずストリーム内の `error` イベントになることがテストで確認されている (従来の HTTP 500 からの変更を意図したものとして固定する)
- [x] #4 観戦ビューの発話中の話者名がサーバーの通知だけで決まり、`frontend/src/api/turnOrder.ts` が削除されている
- [x] #5 既存の round_robin / manual / facilitator_alternating の話者選択そのものは変わっていない
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. service: SpeakerChoice (participant / 実効モデル名 / weights[]) を定義し、RunTurn に onSpeaker を足して endpointAccepts の直後に呼ぶ。既存 3 規則の weights は空配列。
2. httpapi: handleRunTurnStream が onSpeaker で SSE を開き speaker イベントを流す。以降の失敗はすべてストリーム内 error。
3. テスト: speaker イベントの内容・順序 (delta より前)、ターン前の 409/404/400/502、生成失敗 (delta 無し / 有り) がストリーム内 error になること。
4. frontend: turnOrder.ts を削除。ターン実行中フラグを runningSpeaker から分離し、話者名は speaker イベントだけで決める。重みの内訳は発話中バブル内の畳んだ <details> に出し、空なら出さない。
5. go test / go vet / tsc。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装

- `TurnEngine.RunTurn(chatID, participantID, onSpeaker, onDelta)`。`onSpeaker` は `endpointAccepts` を通った直後に 1 回だけ呼ぶ。渡す `SpeakerChoice` は `{ participant, modelName, weights }`。
  - `participant` は参加者の行そのもの。ID だけにしなかったのは、走行中に追加された参加者でもフロントが手元の編成を引き直さずに名前を出せるようにするため。
  - `modelName` は継承を解決した後のモデル名。以前の観戦ビューは参加者の生の `modelName` を出していたので、ワークスペース既定を継承する参加者では空欄だった。これが埋まる。
  - `weights` は既存 3 規則では空配列 (`null` にしない)。形は `[{ participantId, weight, factors: [{ name, value }] }]` で、TASK-34 の `weighted` がこれを埋める。
- `handleRunTurnStream` は `speaker` のタイミングで SSE ライタを開く。そのため「delta が 0 個なら 500、1 個以上ならストリーム内 error」の分岐と、delta が 0 個で成功したときにライタを後から開く分岐がどちらも消えた。
- フロント: `turnOrder.ts` を削除。実行中フラグ (`turnRunning`) と話者 (`runningSpeaker`) を別の state に分けた。以前は `runningSpeaker !== null` を「実行中」の判定に兼用していたので、話者が `speaker` イベントで届くまでの間にボタンが押せてしまう。`speaker` が届くまでは「ターン実行中」とだけ出す。`TurnInput` から編成・transcript・進行役を外し、規則と指名だけにした (予測にしか使っていなかったため)。

## 着手時判断: 重みの内訳の見せ方

生成中の発言の吹き出しに `<details>` で畳んだ「この話者になった理由」を置く。開くと参加者ごとに `名前: 重み = 係数名 ×値 · …` を 1 行ずつ並べる。内訳が空なら何も出さない (既存 3 規則ではこの UI は出ない)。内訳は発言と一緒には保存しないので、生成が終わると消える。実データで見た目を確かめられるのは TASK-34 の後で、そこで見直す前提。

## 検証 (AC)

- #1: `TestMultiAgentTurnAnnouncesSpeaker`。イベント順が speaker → delta… → done で speaker が 1 個、Bob / model-bob / `weights: []`、保存された発言の participantId と一致することを確認。
- #2: `TestMultiAgentTurnRefusalsKeepTheirStatus`。chat 不在 404・他 chat の参加者を指名 404・空の編成 400・manual で指名なし 400・派生規則なのに指名 400・除籍済みの指名 400・接続先不通 502。どれも Content-Type が JSON (ストリームが開いていない) であることまで確認。409 は既存の `TestMultiAgentTurnConflict` で確認 (こちらも JSON を確認している)。
- #3: `TestMultiAgentGenerationFailureIsAStreamError`。completion が delta の前に 500 を返すケースでは 200 で speaker → error、delta の後に不正 JSON を返すケースでは 200 で speaker → delta → error になる。どちらも発言は保存されない。
- #4: `turnOrder.ts` を削除し、`predictNextSpeaker` への参照が無いことを grep で確認。`pnpm check:client` と `pnpm build:client` が通る。
- #5: 話者選択 (`selectSpeaker` 以下) は変更していない。既存の選択テスト (`turnengine_test.go`) は、呼び出しに引数を 1 つ足しただけで通る。
- `go vet ./...` と `go test ./internal/...` はすべて ok。

## 未確認 (目視が必要)

- 実機 (Wails + 実 LLM) での観戦ビューは確認していない。確認してほしい点: 「1 ターン進める」を押してから話者名が出るまでのあいだ「ターン実行中」と表示されること、自動進行で話者名がターンごとに切り替わること、継承モデルの参加者でモデル名が出ること。

## 文書

design §4.2 手順 2・§4.6.6・§5・§6・§8.1、current-spec (en/ja)、README (en/ja) を `speaker` イベントに合わせて更新した。
<!-- SECTION:NOTES:END -->
