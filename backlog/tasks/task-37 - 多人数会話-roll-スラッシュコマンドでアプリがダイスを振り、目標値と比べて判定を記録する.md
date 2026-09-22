---
id: TASK-37
title: '多人数会話: /roll スラッシュコマンドでアプリがダイスを振り、目標値と比べて判定を記録する'
status: To Do
assignee: []
created_date: '2026-09-22 09:56'
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
- [ ] #1 参加者の発言・人間の介入発言の最終行の /roll がアプリの乱数で振られ、判定の記録が発言に保存され、コマンド行は本文から剥がされる (テストで確認)
- [ ] #2 目標値 (コマンドの 目標N、無ければ既定の目標値) との比較で成否が付き、どちらも無ければ合計だけが記録される (テストで確認)
- [ ] #3 prompt の写像で判定の記録が【ダイス】行として本文の後に入り、4 つのターン規則の話者の選択は変わらない (テストで確認)
- [ ] #4 観戦ビューで判定の記録がチップとして表示され、介入欄で /roll を送れる。markdown エクスポートと同梱 trpg-table が判定を扱う
- [ ] #5 go test / フロントの型検査・lint が通る
<!-- AC:END -->
