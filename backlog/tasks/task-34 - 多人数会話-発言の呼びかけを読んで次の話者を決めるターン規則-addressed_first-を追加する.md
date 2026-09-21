---
id: TASK-34
title: '多人数会話: 発言の呼びかけを読んで次の話者を決めるターン規則 addressed_first を追加する'
status: To Do
assignee: []
created_date: '2026-09-21 22:56'
labels: []
dependencies:
  - TASK-33
references:
  - docs/multi-agent-chat-design.md
  - internal/service/turnengine.go
  - internal/model/model.go
  - internal/db/migrations.go
  - internal/preset/bundled
  - frontend/src/components/ParticipantPanel.tsx
type: feature
ordinal: 34000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

「呼ばれた人が答える」会話は既存の 3 規則では出ない。TASK-28 の spike が、スコア方式 (係数の積で話者の重みを
求める案) と呼びかけ優先を 60 ターンのシミュレーションで比較し、**呼びかけ優先を推奨**として
`docs/multi-agent-chat-design.md` §4.6 に記録した。スコア方式を採らない理由も §4.6.3 に測定として残っている
(呼びかけが無いあいだスコア方式は round_robin と 1 発言も違わず、係数が買っているのは呼びかけ追従だけで、
それは係数なしで同等以上に達成できる)。

**呼びかけ**とは、ある発言の本文が、次に話す相手として編成の参加者 1 人の表示名を挙げていることを指す。
`manual` の人間による指名とも、§7「将来」の進行役による指名とも別物である。

## やること (設計は §4.6.4 / §4.6.5 / §4.6.7)

- migration で `messages` に `addressed_participant_id` 列を足す (NULL = 呼びかけ無し)。
- 呼びかけの検出を発言の**保存時**に確定して列へ書く。後から本文を読み直す形にしない (§2: 再起動をまたいで
  同じ答えになること)。判定のための追加の LLM 呼び出しはしない。
  - (a) 役割リマインド (§4.3) に `[次: 表示名]` の末尾指示子を足し、保存前に末尾の 1 個だけ剥がして id に解決する。
    表示名が編成に一致しないときは呼びかけ無しとしたうえで指示子は剥がす (本文に制御用の記法を残さない)。
  - (b) (a) が空振りしたときだけ名前の照合。最終文に編成の表示名がちょうど 1 人分現れ、自分自身でなければ
    呼びかけとみなす。複数一致は呼びかけ無し。表示名 2 文字以上に限る (日本語の表示名は単語境界で切れないため
    部分文字列一致になり、短い名前が誤検出しやすい)。
  - 人間の介入発言にも (b) をかける (`POST /messages` 側。介入が直前の話者を呼ぶ場合、かけないと届かない)。
- `turn_rule` に 4 つ目の値 `addressed_first` を足す。導出は上から順に:
  1. 末尾のメッセージに呼びかけ先があり、その参加者が編成に居れば → その参加者。
  2. それ以外 → 進行役が編成に居れば §4.5 の導出、居なければ `round_robin` の導出。
- 編成パネルのターン進行ルール選択に `addressed_first` を足し、1 行の説明を添える。
- TASK-33 で入る `speaker` イベントに乗せる (フロント側の導出は足さない)。

## 前提

既存の `round_robin` / `manual` / `facilitator_alternating` の振る舞いは**一切変えない**。呼びかけ追従を
chat 単位の真偽値設定にして既存 3 規則すべてに効かせる形は採らない (呼びかけで名簿順を外れる `round_robin` は
もはや `round_robin` ではない、§4.6.4)。

## 検証シナリオ

同梱プリセット `trpg-table` (GM + プレイヤー 2 名) を `addressed_first` で回し、GM が名指ししたプレイヤーが
次に話すこと、名指しの無いターンは §4.5 の交互に落ちることを実機で確認する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 `messages` に呼びかけ先の列があり、検出結果が発言の保存時に確定して書かれる (保存後に本文を読み直さない)
- [ ] #2 末尾指示子と名前照合の 2 段で呼びかけを検出し、指示子は本文に残らない。人間の介入発言にも名前照合がかかる
- [ ] #3 `turn_rule` = `addressed_first` のとき、呼びかけられた参加者が次に話し、呼びかけが無ければ進行役の有無に応じて §4.5 / round_robin の導出に落ちる
- [ ] #4 既存の round_robin / manual / facilitator_alternating の話者選択が変わっていないことがテストで確認されている
- [ ] #5 編成パネルから `addressed_first` を選べ、同梱プリセット trpg-table で呼びかけ追従が実機で確認できる
<!-- AC:END -->
