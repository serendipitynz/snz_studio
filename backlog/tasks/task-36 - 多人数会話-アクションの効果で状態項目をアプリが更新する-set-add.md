---
id: TASK-36
title: '多人数会話: アクションの効果で状態項目をアプリが更新する (set / add)'
status: Done
assignee: []
created_date: '2026-09-22 08:37'
updated_date: '2026-09-28 10:28'
labels: []
milestone: m-1
dependencies:
  - TASK-35
  - TASK-23
  - TASK-37
  - TASK-65
references:
  - docs/multi-agent-chat-design.md
  - internal/service/turnengine.go
type: feature
ordinal: 36000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 状態: 保留 (2026-09-22、TASK-23)

効果コマンドをモデルが書くかは測っていない。TASK-37 (`/roll`) の実装後に、GM が判定の結果を受けて `/add`・`/use` を書くかを測ってから着手する (設計書 §4.8.4)。
書かれなくても人間が同じコマンドを書けるので、下の設計はそのまま成り立つ。

## 背景

TASK-22 の spike (設計書 §4.7) で、状態項目を更新するのは人間の編集とアクションの効果の 2 経路と決めた。
モデルが発言末尾に更新ブロックを書く案は測定で不採用 (状態が変わる 10 回のうち書いた回数: gemma-4-e4b 差分形式 0/10、gpt-oss-20b 差分形式 2/10・全書き直し形式 1/10。場所の移動は両モデルとも 0)。
発言から抽出する案は測定しておらず、毎ターン LLM 呼び出しが増える遅延 (§8.1) と、精度が上の書き込み率を超える根拠が無いことから不採用。
また状態シートを prompt に置いても、直前の宣言・履歴の方が勝つ (0/15) ので、「持っていない物は使えない」の担保も prompt ではできない。

TASK-23 の spike (設計書 §4.8.4) で、効果を生むアクションを効果コマンド (発言末尾のスラッシュコマンド) と定めた。
`/roll` の成否から効果を自動で導くことはしない (条件を書くにはスクリプト言語が要るため)。判定の結果を見た GM か人間が次の発言で効果コマンドを書く。

## やること

- 効果コマンドを TASK-37 と同じ検出 (発言の最終行の 1 つ、人間の介入発言・参加者の発言の両方) で受ける:
  - `/add <持ち主> <項目名> <±整数 または 式>`: `add` (値の先頭の整数に加減する。`HP: 7/10` に -3 → `4/10`)。式なら振った合計を使う。前提条件: 値の先頭が整数
  - `/use <持ち主> <項目名>`: `add` で -1。前提条件: 値の先頭の整数が 1 以上 (持っていない物は使えない)
  - `/set <持ち主> <項目名> <値>`: `set` (行を置き換え、無ければ末尾に追加)
- 持ち主は参加者の名前 (表示名そのものか括弧書きを除いた部分) か「共通」
- 前提条件はアプリが効果を当てる時点で状態シートの値を読んで判定する。満たさない効果・上限 (共通 400・参加者 200 字) を超える効果は適用せず、その旨を記録する
- 効果の記録は判定の記録 (`messages.dice_rolls`) と同じ列に並べ、どの発言による効果かを観戦ビューのチップで追えるようにする
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 TASK-23 で定義したアクションの結果から set / add の効果が状態シートに適用される (テストで確認)
- [x] #2 適用された効果がどの発言によるものかを観戦ビューで確認できる
- [x] #3 上限を超える効果・先頭が整数でない値への add は適用されず、その旨が分かる
- [x] #4 go test / フロントの型検査・lint が通る
- [x] #5 /use は値の先頭の整数が 0 (または行が無い) とき適用されず、その旨がチップで分かる (テストで確認)
- [x] #6 コマンドごとの仕様 (書式・前提条件・chat とプリセットの commands に書く設定・記録の残り方) を docs/multi-agent-commands.md に移し、docs/multi-agent-presets.md と docs/current-spec (日英) からリンクする
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
着手時の判断 (2026-09-28、オーナー): 測定は実装後 / 効果の記録は別の列 messages.state_effects / 以後の話者の写像に【効果】行を入れる / 同梱 trpg-table で add・use・set を有効にし、所持品を項目ごとの行に分ける。

1. 設計書を先に改訂: §4.8.4 の「同じ列」を state_effects に、保留を解除。§4.8.8 (TASK-36) を新設し、書式・持ち主の照合・前提条件・当てなかった理由・記録・写像・画面を決める。§3 migration 19、§5、§6
2. model: ChatCommands に Add / Use / Set (設定なし {})、StateEffect 型、Message.StateEffects。migration 019: messages.state_effects TEXT NOT NULL DEFAULT '[]'
3. internal/service/commands: /add・/use・/set の検出 (最終行、/roll と同じ語境界、有効なものだけ、1 発言 1 コマンド)・解析 (持ち主は名簿の名前の最長一致か「共通」、項目名、±整数 か [±]NdM[±K])・状態シートへの適用 (純関数、前提条件と上限の判定)・【効果】行の書式
4. repository: AddMessageInput に効果 (持ち主と適用関数) を持たせ、発言の挿入と同じトランザクションで状態シートを読み・判定し・書く
5. service: prepareUtterance が効果を返し、RunTurn / StoreHumanMessage で適用。解釈できない効果コマンドは参加者では本文に残し、人間では 400。効果だけの発言は保存する。写像・結論の下書き・エクスポート・メモリ保存の下書きに効果の記録を足す
6. httpapi: 介入の応答に participants を足す (状態シートが変わるため)
7. preset: 同梱 trpg-table に add/use/set と項目ごとの状態シート・効果コマンドの規則
8. フロント: 型、効果のチップ (当てなかった効果は danger と理由)、コピー、インスペクタのチェックボックス 3 つ、入力欄の補完、介入後の参加者の更新
9. docs/multi-agent-commands.md を新設し、presets と current-spec (日英) からリンク
10. テスト (commands の解析・適用、service の保存順・400・写像、repository のトランザクション、export、preset) と go test / go vet / pnpm check:client / build
11. 実装後の測定: gpt-oss-20b・gemma-4-e4b で、GM が判定の結果を受けて /add を書くか、プレイヤーが消耗品に /use を書くか。結果を設計書に記録し trpg-table の規則を確定

12. (測定の後、オーナー判断) 効果コマンドは最終行にコマンドが無いとき最終行より上の読めるものも読む。trpg-table の規則文から【効果】の語を外し、影響する条件を測り直す
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 着手時に決めたこと (2026-09-28、オーナー判断)

- 保留の条件 (GM が効果コマンドを書くかの測定) は実装してから測った。設計は書く率に依らず成り立ち、測るには規則文と【効果】行の写像が要るため
- 効果の記録は判定の記録と別の列 messages.state_effects (migration 019)。1 発言 1 コマンドなので両方が 1 発言に並ぶことが無く、dice_rolls の名前と型を保てる。設計書 §4.8.4 の「同じ列」を改訂した
- 以後の話者の写像に【効果】行を入れる (当てなかった効果も理由つき)
- 同梱 trpg-table で add/use/set を有効にし、消耗品を項目ごとの行に分けた (たいまつ: 2・包帯: 3)。「所持品: たいまつ 2 本、…」の 1 行では /use が当たる行が無いため

## 測定の後に決めたこと (オーナー判断)

- 効果コマンドに限り、最終行にコマンドが無いときは最終行より上の行も読む。上から最初の読めるもの 1 つだけ。最終行より上の読めない効果コマンドは本文として残し、人間の介入でも 400 にしない (コマンドの説明文を弾かないため)。§4.8.3 の 2 と §4.8.7 に例外を追記
- 同梱 trpg-table の規則文から【効果】の語を外した (GM が【効果】行を自分で書く模倣の元だったため)。アプリは模倣を剥がさない

## 判断して進めた点

- 持ち主の照合は §4.6.5 と同じく表示名か括弧書きを除いた名前 (大文字小文字を区別しない)、最長一致、2 人以上が持つ名前は除く。「共通」は常に共通の状態。編成にいない持ち主は解釈できない効果コマンド (参加者は本文に残す、人間は 400)
- /add の式の先頭の符号は式全体に掛かる (-1d6+1 は 1d6+1 を引く)。量は 9999 まで。結果が 0 未満でも当てる (下限は /use だけ)。式の出目は前提条件を満たしたときだけ振る
- 効果は発言の挿入と同じトランザクションで状態シートを読み・判定し・書く (当てる時点の値で判定、発言の無い効果が残らない)
- 介入の応答 (POST /api/chats/{chatId}/messages) は保存後の chat と、多人数会話では participants も返す。観戦ビューはそれで編成を更新する
- チップは StateBadge。当てた効果は neutral、当てなかった効果は danger と理由。図形は既存の Lucide pencil (size 引数を足しただけ)。エクスポート・メモリ保存の下書き・コピーは 📝
- 設定には細目を持たせず、コマンドごとのチェックボックス 3 つ (説明の (?) は 1 つ)。設定の中の知らないフィールドは roll と同じく無視する
- 効果が状態シートを書き換えると、インスペクタのその欄の未保存の下書きは保存済みの値に置き換わる (StateSheetField が保存値をキーにしているため。プリセットの適用と同じ扱い)
- snz-design doc-17 は改訂していない。既存の部品の組み合わせで、TASK-37 の判定のチップも記録の対象になっていないため

## 検証

- AC #1: commands.TestApplyEffect / TestApplyEffectDice / TestApplyEffectSheetShapes、service.TestTurnEngineAppliesEffect (参加者の /add が状態シートに当たり、記録が発言に保存され、次の話者の写像に【効果】行と更新後の【現在の状態】が入る)、TestTurnEngineEffectReadsSheetWhenStored (生成中の人間の編集で判定)、TestTurnEngineEffectLineAbove、TestStoreHumanMessageEffects (/set で共通の状態に行を足す)、httpapi.TestMultiAgentInterventionEffects
- AC #2・#3・#5: TestStoreHumanMessageEffects で not_positive (たいまつ 0)・missing_item・not_integer・over_limit が当たらず状態シートが変わらないこと、TestEffectLine で理由の文面。画面は _sandbox/task-36/uiserve (ビルド済み SPA と API を 1 オリジンで出す使い捨てサーバー) に trpg-table の会話を作り、介入で /set・/add -1d6+1・/use×3・/use ミラ 聖水 を送って、headless Chromium (Playwright) で 1440px ライト・ダーク、390px を撮った。当てた効果は中立のチップ、3 回目の /use は「適用されず（たいまつが 1 未満）」、聖水は「聖水の行が無い」の赤いチップ。インスペクタのレンの状態は HP: 6/10・たいまつ: 0 に更新。横スクロール無し、pageerror 無し。入力欄の / で候補 5 つ、/use の候補を押すと「持ち主」が選択される
- AC #4: go test ./... / go vet ./... / pnpm run check:client / pnpm run build:client が通る。フロントの lint スクリプトはリポジトリに無いため型検査のみ (TASK-35・37 と同じ)。gofmt -l は今回触っていない internal/search/model.go だけを挙げる
- AC #6: docs/multi-agent-commands.md を新設し、multi-agent-presets.md と current-spec.ja.md / current-spec.md のコマンドの記述を要約とリンクに替えた
- 測定 (設計書 §4.8.8 に表と読み取り): gpt-oss-20b の GM は判定失敗の後に /add を 6/10 書き、半分は最終行より上 (文の末尾に続けて描写を続ける形)。gemma-4-e4b は /add・/use を 0。当てなかった /use の【効果】行があると gemma の点火の描写は 6/10 → 0/10。規則文から【効果】を外すと GM による【効果】行の模倣は gpt-oss 4/10 → 0、gemma 3/10 → 0

## 測っていないこと

- 実窓 (WKWebView) での見え方、チップの 4 配色の比 (判定のチップと同じ StateBadge の neutral / danger を使う)
- 最終の検出規則での G-fail の測定は 9 回だけ (書いた 3 回中 2 回が当たる。1 回は全角の ／add)。全角スラッシュと英語名 (Ren) の持ち主は扱っていない
- 実際の自動進行 (約 30 ターン) で効果コマンドが卓を回すか
<!-- SECTION:NOTES:END -->
