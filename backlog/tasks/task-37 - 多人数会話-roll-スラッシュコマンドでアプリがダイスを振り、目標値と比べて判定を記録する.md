---
id: TASK-37
title: '多人数会話: /roll スラッシュコマンドでアプリがダイスを振り、目標値と比べて判定を記録する'
status: In Review
assignee: []
created_date: '2026-09-22 09:56'
updated_date: '2026-09-26 22:29'
labels: []
milestone: m-1
dependencies: []
references:
  - docs/multi-agent-chat-design.md
  - internal/service/turnengine.go
  - internal/httpapi/multiagent.go
  - frontend/src/pages/MultiAgentChatPage.tsx
  - internal/service/export.go
type: feature
ordinal: 37000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-23 の spike (設計書 §4.8) で、TRPG の出目はアプリがサーバー側の乱数で出し、きっかけは発言末尾のスラッシュコマンド `/roll` とすると決めた。
人間の介入発言と参加者の発言の両方で同じ書式を検出する。gpt-oss-20b は場面設定の指示だけで 10/10 書き、gemma-4-e4b は 3/10 なので、書かないモデルでは人間が同じ 1 行を送る。
合計だけを GM に渡すと失敗を成功として描写した例が 3/10、アプリが成否を付けると 1/10 だったので、目標値との比較までアプリが持つ。

## やること (設計書 §4.8.3 のとおり)

- 書式 `/roll <式> [目標<整数>] [行動]`。式は `NdM` / `NdM+K` / `NdM-K` (N 1〜20、M 2〜100)。変数・条件分岐・マクロは持たない
- 検出は保存時、発言の最終行の 1 つだけ (本文に続けて書かれたものも受ける)。`[次: …]` の指示子を先に剥がしてから探す。コマンド行は本文から剥がす
- migration: `messages.dice_rolls` (JSON 配列、既定 `'[]'`) と `chats.dice_target` (整数、0 = 無し)。目標値はコマンドの `目標N` → 既定の目標値の順。成功は合計 ≥ 目標値
- 乱数は `math/rand/v2`
- prompt の写像 (§4.3) で本文の後に「【ダイス】行動 — 1d20+3 → 4+3 = 7（目標 12、失敗）」の 1 行を足す (自分の発言にも他人の発言にも)
- 空の発言の扱いは「本文が空で判定の記録も無い」ときだけに狭める。解釈できない `/roll` は参加者の発言では本文に残し、人間の介入発言では 400
- UI: 発言の下にチップ、介入欄で `/` を打つと書式の候補。専用ボタンは置かない。`PATCH /api/chats/{chatId}` と編成パネルで既定の目標値を編集
- markdown エクスポートに判定の記録を含める。プリセットの `diceTarget`。同梱 trpg-table の「ダイスは使いません」を `/roll` の規則に差し替え、既定の目標値 12
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 参加者の発言・人間の介入発言の最終行の /roll がアプリの乱数で振られ、判定の記録が発言に保存され、コマンド行は本文から剥がされる (テストで確認)
- [x] #2 目標値 (コマンドの 目標N、無ければ既定の目標値) との比較で成否が付き、どちらも無ければ合計だけが記録される (テストで確認)
- [x] #3 prompt の写像で判定の記録が【ダイス】行として本文の後に入り、4 つのターン規則の話者の選択は変わらない (テストで確認)
- [x] #4 観戦ビューで判定の記録がチップとして表示され、介入欄で /roll を送れる。markdown エクスポートと同梱 trpg-table が判定を扱う
- [x] #5 go test / フロントの型検査・lint が通る
- [x] #6 保存時の処理が 指示子 → コマンド → (指示子が空振りしたときだけ) 名前の照合 の順で、参加者の発言・人間の介入発言の両方で動く。コマンド行の前の呼びかけが残り、コマンドの行動に含まれる名前は呼びかけにならない (テストで確認、設計書 §4.8.3 の 2)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. migration 017: messages.dice_rolls (JSON '[]') と chats.dice_target (INTEGER 0)。model.DiceRoll / Chat.DiceTarget / Message.DiceRolls、repository の列・scan・AddMessageInput.DiceRolls・MultiAgentSettings.DiceTarget・CreateChatInput.DiceTarget
2. service/dice.go: `/roll <式> [目標N] [行動]` の解析 (最終行・本文に続けたものも受ける)、乱数は math/rand/v2 (テスト用に差し替え可能)、目標は 目標N → chat.dice_target、成功は合計 ≥ 目標、prompt/エクスポート用の 1 行の書式
3. 保存時の処理を 1 つの関数にまとめる: 指示子を剥がす → 残りの最終行でコマンドを探して剥がす → 指示子が空振りしたときだけ最終文で名前の照合。参加者の発言 (RunTurn) と人間の介入 (storeHumanMessage を TurnEngine 側へ) の両方で使う。解釈できない /roll は参加者では本文に残し、人間では 400 (ストリーム版の介入も SSE を開く前に保存して 400 を返せるようにする)
4. 空の発言は「本文が空で判定の記録も無い」ときだけ失敗。mapHistoryForSpeaker で本文の後に【ダイス】行、結論の下書きの写像にも同じ行
5. PATCH /api/chats/{chatId} の diceTarget (0〜9999 の整数)、プリセットの diceTarget (検証・作成・適用)、同梱 trpg-table の規則を /roll に差し替え diceTarget 12
6. markdown エクスポートで本文の後に判定の行
7. フロント: 型、発言の下のチップ (StateBadge の成功/失敗/中立 + ダイスの図形)、介入欄で最終行が / で始まると書式の候補、編成パネルの設定区画に既定の目標値
8. テスト (解析・保存順・4 規則の話者不変・HTTP の 400/PATCH・エクスポート・プリセット)、設計書 §3/§4.8/§5/§6 と multi-agent-presets.md の更新
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- 保存時の処理を prepareUtterance (internal/service/addressing.go) 1 つにまとめ、RunTurn と TurnEngine.StoreHumanMessage (httpapi の storeHumanMessage を移した) の両方が通る。人間の介入でも指示子を剥がすようになったのは設計書 §4.8.3 の 2 が両経路で同じ順を求めるため (§4.6.5 に追記)。指示子を剥がすと空になる介入は 400。
- ストリーム版の介入ルートは SSE を開く前に保存するよう順を変えた。解釈できない /roll を 400 で返すため (生成を伴わないので失うものが無い)。
- 設計書に無かった値: 修正値の上限 999、目標N と既定の目標値の上限 9999 (model.DiceTargetMax)。全角の ＋ − － と全角数字・全角空白、「目標 12」「目標:12」も受ける。URL の一部の /roll はコマンドにしない。いずれも設計書 §4.8.3 末尾「実装で決めた細部」に記録。
- 結論の下書きの写像にも【ダイス】行を入れ、プロジェクト資料の検索クエリには入れない。
- AC #1: TestTurnEngineRollsDice (参加者、本文に続けたコマンド) / TestStoreHumanMessageRolls / TestMultiAgentInterventionRolls (両ルート、ストリーム版で 400 が SSE 前に返る)。
- AC #2: TestRollCommandRoll (コマンドの目標が既定より優先、合計 = 目標で成功、どちらも無ければ success null) / TestStoreHumanMessageRolls。
- AC #3: TestTurnEngineRollsDice (他人の user 行と自分の assistant 行の両方に【ダイス】行) / TestTurnEngineRollsKeepSpeakers (4 規則で、判定つき・コマンドだけの発言を含む transcript と判定なしの transcript で同じ話者)。
- AC #4: TestBuildChatMarkdownDiceRolls / TestPresetDiceTarget / TestMultiAgentDiceTarget (PATCH・作成時・適用時・別プリセットで 0 に戻る)。チップと入力欄の補完は vite preview + Playwright (Chromium headless shell、API は route で差し替え) で 1440px ライト・ダーク、390px を撮って確認: 成功・失敗・中立のチップ、コマンドだけの発言、/r で候補 2 つ、候補を押すと最終行が置き換わり「行動」が選択される、横スクロール無し。編成パネルの既定の目標値は 15 の保存で PATCH {diceTarget:15}、-3 で無効の理由が出る。
- AC #5: go test ./... / go vet ./... / pnpm check:client / pnpm build:client が通る。フロントの lint スクリプトはリポジトリに無いため型検査のみ (TASK-35 と同じ)。gofmt -l は今回触っていない internal/search/model.go だけを挙げる。
- AC #6: TestPrepareUtteranceOrder (コマンド行の前の呼びかけが残る、行動の名前は呼びかけにならない、指示子の後にコマンド、解釈できない /roll を残しても行動の名前は呼びかけにならない)。
- 測っていないこと: 実窓 (WKWebView) での見え方、チップの 4 配色の比 (既存の StateBadge をそのまま使うので doc-17 の接続の状態の測定値 6.93〜8.34 が当たる想定)、実モデルが新しい trpg-table の規則で /roll を書く率 (TASK-23 の測定と同じ規則文だが、プロンプト全体は変わっている)。
<!-- SECTION:NOTES:END -->
