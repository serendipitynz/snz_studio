---
id: TASK-21
title: '多人数会話: 特定の参加者を 1 人おきに挟むターン規則を追加する (GM 進行用)'
status: Done
assignee: []
created_date: '2026-09-20 00:18'
updated_date: '2026-09-21 22:34'
labels: []
milestone: m-1
dependencies: []
references:
  - docs/multi-agent-chat-design.md
  - internal/model/model.go
  - internal/service/turnengine.go
  - frontend/src/api/turnOrder.ts
  - frontend/src/components/ParticipantPanel.tsx
  - presets/multi-agent/README.md
ordinal: 21000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

ターン規則は現在 `round_robin` (名簿順に循環) と `manual` (UI が毎回指名) の 2 つ
(`model.go` の `TurnRule*`、`turnengine.go` `selectSpeaker`)。同梱プリセットは全部 round_robin。
TRPG の進行は GM → プレイヤー A → GM → プレイヤー B → GM → … と GM が毎回挟まる。
manual で GM が毎回指名する運用は可能だが、自動進行が使えず人手の操作が増える。

## 方針

第 3 のターン規則を追加する: 名簿の中から 1 人を「進行役」として指定し、進行役と他の参加者が
交互に話す。他の参加者側は名簿順に循環する。進行役の指定は chat に持たせる (turn rule の付随設定)。
round_robin と同様、次の話者は保存済みの直近発言から導出し、サーバー側に進行状態を持たない
(設計書 §2 の再起動・複数ウィンドウ耐性を保つ)。

進行役が名簿から削除された場合の扱い (round_robin に落とす、またはエラー) と、人間の介入発言を
挟んだ後に誰が話すか (進行役が応じるのが自然) を設計時に決めて設計書に記録する。
フロントの次話者表示 (`turnOrder.ts`) も対応させる。同梱プリセットに TRPG 卓のプリセット
(GM + プレイヤー 2〜3 名、この規則を使用) を 1 つ追加する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 進行役を 1 人指定する新しいターン規則が選べ、進行役と他の参加者が交互に話し、他の参加者は名簿順に循環する
- [x] #2 次の話者は保存済み発言から導出され、サーバー再起動後も順番が保たれる
- [x] #3 進行役の削除時と人間の介入発言後の話者の扱いが設計書に記録され、実装と一致している
- [x] #4 編成パネルで規則と進行役を設定でき、次話者の表示が正しい
- [x] #5 この規則を使う TRPG 卓のプリセットが同梱されている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 語彙の確定 (対応表): ターン規則の値 = facilitator_alternating (表示「進行役交互」)、chat の列 = facilitator_participant_id / API は facilitatorId、§7 将来の「進行役モデルによる発言者指名」とは別物として予約名 facilitator_nominated を書き分ける。
2. 着手時判断 (ユーザー確認済み): (a) 進行役が名簿に居ない (除籍 / 未指定) ターンは round_robin と同じ導出に落とし、UI に警告を出す。(b) 介入直後のターン (participant_id を持たないメッセージが末尾) と開幕ターンはどちらも進行役が話す。
3. migration 014_facilitator_turn_rule: chats に facilitator_participant_id TEXT NOT NULL DEFAULT ''。model.Chat.FacilitatorID / TurnRuleFacilitatorAlternating を追加し、repository の chatColumns・CreateChat・UpdateMultiAgentSettings を通す。
4. turnengine.selectSpeaker に facilitator_alternating を追加: 保存済み messages だけから導出する (末尾が participant_id なし → 進行役 / 参加者発言 0 件 → 進行役 / 直前が進行役 → 直近の「他の参加者」の名簿順の次 / 直前が他の参加者 → 進行役)。
5. HTTP: PATCH /api/chats/{chatId} が facilitatorId を受け、値が名簿の参加者でなければ 400。preset 適用時は preset が指名した参加者 ID を設定する。
6. preset: participants[] に facilitator (真偽値、省略時 false) を足し、turnRule が facilitator_alternating のときだけ 1 人必須という Validate を書く。TRPG 卓のプリセットを bundled に 1 件追加 (GM だけ receivesProjectMaterial: true)。
7. フロント: client.ts の TurnRule / ChatRecord、turnOrder.ts の predictNextSpeaker、ParticipantPanel の規則 select + 進行役 select + 不在警告、MultiAgentChatPage の自動進行注記、i18n (en/ja)。export.go の turnRuleDescription。
8. 設計書 (§2 用語・§3 スキーマ・§4.2 手順 1・§5 API・§6 UI・§7 将来・§8.1) と current-spec (en/ja)・README (en/ja)・presets/multi-agent/README.md を実装に合わせて改訂。
9. go test ./... と pnpm check:client。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装したもの

3 つ目のターン進行ルール `facilitator_alternating` (表示「進行役交互」) を追加した。進行役は chat の
`facilitator_participant_id` (migration 14、既定は空文字) で 1 人指定する。導出は round_robin と同様に
保存済み発言だけから行い、サーバー側に進行状態を持たない (設計書 §4.5 に新設)。

- 語彙: 着手前に対応表 (referent table) で指示対象を固定した。`facilitator_alternating` = 決定的に交互に挟む
  今回のルール、進行役 = それが挟む名簿上の参加者 1 人、進行役による指名 (将来の `facilitator_nominated`) =
  進行役がモデル出力で次の話者を選ぶ別ルール。設計書 §0・§2・§7 で両者を書き分けた。
- 導出 (`selectSpeaker` / `alternatingSpeaker`): 末尾が participant_id を持たない or 発言 0 件 → 進行役 /
  末尾が進行役以外 → 進行役 / 末尾が進行役 → 進行役以外で最後に発言した参加者の次 (名簿順、居なければ先頭)。
- API: `PATCH /api/chats/{chatId}` が `facilitatorId` を受ける。空文字は指定の解除、それ以外はその chat の
  編成に居る参加者のみ (他 chat・除籍済み・未知の id は 400)。プリセットは `participants[].facilitator` の印を
  適用時に生成した participant id へ解決し、印の無いプリセットは進行役を空にする (置換前の id を残さないため)。
- フロント: `turnOrder.ts` にエンジンと同じ導出を写した。編成パネルはルール選択の下に進行役の select を出し、
  chat が持つ id を編成と突き合わせて除籍済みは「未選択」として表示する。進行役不在の注記は編成パネル
  (折り畳める) と観戦ビューの両方に出す。
- 同梱プリセット `trpg-table` (GM + プレイヤー 2 名) を追加。GM だけ `receivesProjectMaterial: true` +
  `facilitator: true`。同梱は 7 件 → 8 件になったので README・current-spec の件数も直した。

## 着手時判断 (ユーザー確認済み、2026-09-22)

TASK-21 が着手時に残していた 2 点。決定と理由は設計書 §4.5 に記録し、§8.1 に決定記録を足した。

- 進行役が編成に居ない (除籍 / 未指定) ターンは round_robin の導出に落とす。エラーにすると自動進行が止まり
  原因が画面の文言でしか伝わらないのに対し、落とせば会話は進み、編成パネルは直せる場所に不在を出せる。
  ただし編成パネルからの新規指定は編成の参加者に限る (除籍が残した値を許容することと、新しく書けることは別)。
- 人間の介入発言の次と開幕ターンは進行役。介入は進行役以外の巡回位置を動かさない (手順 4 は進行役以外の
  発言だけを見るため)。

## 設計の判断

- 除籍時に `facilitator_participant_id` を消す処理は入れていない。指し先の消えた id と未指定はエンジンにとって
  同じ場合で、消しても増えるのは書き込みだけだからである。
- プリセット側は participant id ではなく名簿の要素に `facilitator: true` の印を付ける。id は適用の瞬間まで
  存在しない。`facilitator_alternating` のプリセットは印をちょうど 1 件必須、他のルールでは印を 400 にした
  (保存されても誰も読まない値になるため)。
- markdown エクスポートは進行役名を出し、進行役が編成に居ないときは実際に取られる縮退 (編成順) を書く。

## 検証

- `go test ./...` 全パス。新規: `TestTurnEngineFacilitatorAlternating` (進行役を名簿の中央に置き、
  GM → P1 → GM → P2 → GM → P1。途中で engine を作り直して DB 越しに続きが導出できることも確認 = AC #2)、
  `TestTurnEngineFacilitatorAnswersIntervention`、`TestTurnEngineFacilitatorFallsBackToRoundRobin`、
  `TestMultiAgentFacilitatorSetting`、`TestMultiAgentPresetFacilitator`、`TestParseFacilitator`、
  `TestBuildChatMarkdownFacilitatorRule`、migration 14 を db_test に追加。
- `pnpm check:client` / `pnpm build:client` / `go vet ./...` 通過。
- `turnOrder.ts` は tsc で JS に落として node から直接呼び、上の Go テストと同じ 12 系列 (交互 6 ターン・
  介入後・進行役不在・round_robin・manual) がエンジンと一致することを確認した。

## 未検証 (AC #4 を未チェックにしている理由)

編成パネルの進行役 select と不在注記の実機での見え方・操作は確認していない。フロントにテスト基盤が無く、
Wails の実アプリ起動が要るため。API 側 (規則と進行役の保存・拒否) と次話者導出は上記のとおり確認済みなので、
残るのは画面の描画と onChange の配線だけである。実機で編成パネルを開いて確認してほしい。

## 実機確認 (2026-09-22、ユーザー)

AC #4 の残り (編成パネルの進行役 select と、生成中に出る次話者の表示) をユーザーが実アプリで確認し、
問題なしとの回答を得たのでチェックした。表示箇所は生成中の発言バブルのヘッダと、コンポーザー上の
「〜が発言中」の行の 2 箇所 (MultiAgentChatPage.tsx)。どちらの名前も turnOrder.ts の predictNextSpeaker が
エンジンと同じ規則で先に導いたもの。
<!-- SECTION:NOTES:END -->
