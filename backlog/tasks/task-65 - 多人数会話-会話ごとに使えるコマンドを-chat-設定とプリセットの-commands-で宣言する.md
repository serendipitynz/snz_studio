---
id: TASK-65
title: '多人数会話: 会話ごとに使えるコマンドを chat 設定とプリセットの commands で宣言する'
status: To Do
assignee: []
created_date: '2026-09-27 06:45'
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
- [ ] #1 会話ごとに使えるコマンドを chat 設定として保存し、インスペクタで確認・変更できる
- [ ] #2 プリセット JSON の commands で設定を埋められ、既存のトップレベル diceTarget も読み込める
- [ ] #3 roll が無効な会話では /roll も【ダイス】行も処理せず本文のまま扱う (テストで確認)
- [ ] #4 既存の chat の移行後の既定値を決め、設計書に記録する
- [ ] #5 コマンドの処理を internal/service/commands に移し、効果コマンドを後から足せる形にする
- [ ] #6 設計書 §4.8.3 の「ダイスを使うかの chat 設定は持たない」を改訂する
- [ ] #7 go test / フロントの型検査が通る
<!-- AC:END -->
