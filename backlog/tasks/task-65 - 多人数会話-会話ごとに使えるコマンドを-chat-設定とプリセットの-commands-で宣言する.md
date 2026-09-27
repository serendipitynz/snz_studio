---
id: TASK-65
title: '多人数会話: 会話ごとに使えるコマンドを chat 設定とプリセットの commands で宣言する'
status: In Review
assignee: []
created_date: '2026-09-27 06:45'
updated_date: '2026-09-27 07:44'
labels: []
dependencies:
  - TASK-63
references:
  - docs/multi-agent-chat-design.md
  - internal/service/dice.go
  - internal/preset/preset.go
  - internal/preset/bundled/25-trpg-table.json
type: feature
ordinal: 65000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-63 のレビュー (2026-09-27) で出た論点。アプリは汎用なのに、`/roll` の検出と `【ダイス】` 行の扱い (§4.8.6) はダイスを使わない会話を含むすべての多人数会話で走る。§4.8.3 の 3 は「行頭が /roll の行を会話で書くことはまず無いので、ダイスを使うかの chat 設定は持たない」としたが、`【ダイス】` は 【】 を見出しに使う日本語の地の文と重なり得る (TASK-63 では、行頭かつ記録の形をした行だけを剥がすことで緩和した)。

オーナーの提案: プリセット JSON に、この会話で使えるコマンドを宣言する欄を持たせ、場面設定の指示とセットで記述できるようにする。コマンドの実装は、後から追加できるよう `internal/service` 直下から切り出す。

## 着手時に決める案 (TASK-63 で合意した方向)

- 設定を持つのは chat。プリセットは適用時にそれを埋めるだけ (`diceTarget` → `chats.dice_target` と同じ関係)。インスペクタで見えて変更できる
- JSON は「コマンド名 → そのコマンドの設定」: `"commands": { "roll": { "target": 12 }, "add": {}, "use": {} }`。キーがあれば有効。既存のトップレベル `diceTarget` は `roll.target` として読む
- 剥がす語はプリセットの自由記述にせず、コマンドに持たせる (`roll` を有効にすると、`roll` が書く `【ダイス】` を守る)。書き出す書式と剥がす語がずれないようにするため。作者が決める語のリストは、実際に模倣が観測されてから
- パッケージは `internal/service/commands` (設計書 §4.8.1 の「スラッシュコマンド」「効果コマンド」に合わせる。`tools` は LLM のツール呼び出し、`skills` は Claude のスキル、`actions` は §4.7 の「アクション」と紛れる)
- 既存の chat の既定値: 今の挙動を変えないよう `roll` を有効にするか、判定の記録や既定の目標値がある chat だけにするか
- TASK-36 (効果コマンド) はこのタスクを前提にする

決定先行: §4.8.3 の 3 の決定を覆すので、設計書の改訂 (と必要なら snz-design doc-17 の記録) を実装より先に書く。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 会話ごとに使えるコマンドを chat 設定として保存し、インスペクタで確認・変更できる
- [x] #2 プリセット JSON の commands で設定を埋められ、既存のトップレベル diceTarget も読み込める
- [x] #3 roll が無効な会話では /roll も【ダイス】行も処理せず本文のまま扱う (テストで確認)
- [x] #4 既存の chat の移行後の既定値を決め、設計書に記録する
- [x] #5 コマンドの処理を internal/service/commands に移し、効果コマンドを後から足せる形にする
- [x] #6 設計書 §4.8.3 の「ダイスを使うかの chat 設定は持たない」を改訂する
- [x] #7 go test / フロントの型検査が通る
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 設計書: §4.8.7 (TASK-65) を新設し、§4.8.3 の 3 の「chat 設定は持たない」、§4.8.6 末尾、§3 (migration 18)、§5 (PATCH の commands・プリセットの適用)、§4.8.1 (既定の目標値の置き場所) を改訂する。docs/multi-agent-presets.md の diceTarget 行を commands に替える
2. model: ChatCommands { Roll *RollCommandSettings{Target} } を持ち、JSON は「コマンド名 → 設定」。知らないコマンド名は拒否
3. migration 018: chats.commands TEXT NOT NULL DEFAULT '{}' を足し、既定の目標値が 1 以上か判定の記録を持つ多人数会話だけ {"roll":{"target":dice_target}} を入れ、dice_target 列を落とす (オーナー判断 2026-09-27)
4. internal/service/commands: dice.go の /roll と【ダイス】行の処理を移し、Extract(body, enabled) で最終行のコマンドを有効なものだけ処理する。prepareUtterance は chat の commands を受ける
5. repository / httpapi: diceTarget を commands に置き換え (作成・PATCH・プリセットの適用)。preset: commands を読み、トップレベルの diceTarget は roll.target として読む。両方あれば拒否
6. 同梱 trpg-table を commands.roll.target=12 に。フロント: インスペクタに roll のチェックボックスと既定の目標値 (有効時のみ)、入力欄の補完は roll 有効時のみ
7. go test ./... / フロントの型検査
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 着手時に決めたこと (2026-09-27)

- 既存の chat の移行後の既定値 (AC #4): オーナー判断で「ダイスを使った会話だけ」。migration 018 の時点で既定の目標値が 1 以上か、判定の記録を持つ発言が 1 件以上ある多人数会話だけ roll を有効にし、dice_target を roll.target に移した。設計書 §3 (migration 18) と §4.8.7 に記録した
- 以下は判断して進めた点 (設計書 §4.8.7 の表)。chats.dice_target 列は落とし、chats.commands (JSON) に一本化した。二重の持ち方を避けるためで、API の diceTarget も commands に置き換えた (TASK-62 が未着手で、外から読んでいる箇所は無い)。プリセットを使わずに作った会話は {}。知らないコマンド名は 400。プリセットで commands とトップレベルの diceTarget を両方書くと 400
- 無効にしても、過去の判定の記録はチップにも prompt の【ダイス】行にも残す。止まるのは新しく保存する発言の検出だけ
- インスペクタでは、チェックボックスを既定の目標値の欄と同じく状態の区画に置いた (場面設定の側ではない)。同じコマンドの設定で、進行中に人間が変える値という点も同じため。オフにすると roll の設定ごと消えるので、同じ画面のあいだは直前の目標値を覚えておき、オンに戻したときに使う
- snz-design doc-17 は改訂していない。既存の部品 (Checkbox と Hint、数値欄) の組み合わせで、TASK-37 の目標値の欄も記録の対象になっていなかったため

## 検証

- go test ./... はすべて通過、go vet も警告なし、pnpm run check:client (tsc) も通過。gofmt で差分が出るのは internal/search/model.go だけで、これは既存のもの (今回は触っていない)
- AC #3: commands.TestExtractWithoutRoll、service.TestPrepareUtteranceWithoutRoll、TestTurnEngineWithoutRoll (ターンと介入のどちらも本文のまま保存し、ダイスを振らない。解釈できない /roll も 400 にならない。記録済みの判定は写像され続ける)
- AC #4: db.TestChatCommandsBackfill。加えて開発用 DB (data/app.sqlite) のコピーに migration 18 の SQL を当てた。多人数会話 11 件のうち roll が有効になったのは TRPG の卓 1 件 ({"roll":{"target":12}}) だけで、残り 10 件は {} になった
- AC #1: 開発用 DB のコピーを DATA_DIR / SQLITE_PATH で指して pnpm dev を起動し、headless Chromium (Playwright) で localhost:34115 を操作した。TRPG の卓はチェックが入って目標値 12 の欄が出る。オフにすると欄が消え、DB は {} になる。オンに戻すと目標値 12 で戻る。トークはチェックが外れていて欄も無い。入力欄に / を打つと、補完は TRPG の卓で 2 件、トークで 0 件。元の data/app.sqlite は 017 のまま
- 実窓 (Wails の WebView) では見ていない。素のブラウザでは Wails の実行時スクリプトから pageerror (reading 'nodes') が出るが、アプリのコードには .nodes の参照が無い
<!-- SECTION:NOTES:END -->
