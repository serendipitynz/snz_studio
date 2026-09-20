---
id: TASK-21
title: '多人数会話: 特定の参加者を 1 人おきに挟むターン規則を追加する (GM 進行用)'
status: To Do
assignee: []
created_date: '2026-09-20 00:18'
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
- [ ] #1 進行役を 1 人指定する新しいターン規則が選べ、進行役と他の参加者が交互に話し、他の参加者は名簿順に循環する
- [ ] #2 次の話者は保存済み発言から導出され、サーバー再起動後も順番が保たれる
- [ ] #3 進行役の削除時と人間の介入発言後の話者の扱いが設計書に記録され、実装と一致している
- [ ] #4 編成パネルで規則と進行役を設定でき、次話者の表示が正しい
- [ ] #5 この規則を使う TRPG 卓のプリセットが同梱されている
<!-- AC:END -->
