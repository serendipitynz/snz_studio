---
id: TASK-35
title: '多人数会話: 共通の状態と参加者の状態 (状態シート) を保存し、人間が編集でき、ターンの prompt に入るようにする'
status: Done
assignee: []
created_date: '2026-09-22 08:37'
updated_date: '2026-09-24 01:28'
labels: []
milestone: m-1
dependencies: []
references:
  - docs/multi-agent-chat-design.md
  - internal/service/turnengine.go
  - internal/db/migrations.go
  - frontend/src/components/ParticipantPanel.tsx
  - internal/service/export.go
type: feature
ordinal: 35000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-22 の spike で、TRPG の刻々と変わる状態 (HP・所持品・場所・時刻) の置き場を設計書 §4.7 に決めた。
状態は「項目名: 値」の行を並べたテキスト (状態シート) で、持ち主は chat (共通の状態) と参加者 (参加者の状態) の 2 種。
このタスクは保存・人間による編集・prompt への注入までを作る。アクションの効果による更新は別タスク (TASK-23 の設計後)。

## やること

- migration 16: `chats.state_sheet` / `participants.state_sheet` (TEXT NOT NULL DEFAULT '')
- 上限: 共通 400 字・参加者ごと 200 字 (rune 数)。保存時に検査し、超えたら 400 (prompt 側で切り詰めない)
- API: 既存の `PATCH /api/chats/{chatId}` と `PATCH /api/participants/{participantId}` に `stateSheet` を足す
- prompt: 場面設定の直後・役割プロンプトの前に「【現在の状態】」節を 1 つ置く。共通の状態 → 参加者の状態 (編成順、参加者名の見出し付き)。
  全参加者に全員分を渡す (`receives_project_material` に依らない)。全シートが空なら節ごと出さない
- UI: 編成パネルで共通の状態 (場面設定の下) と参加者ごとの状態 (役割プロンプトの下) を編集できる。会話中も編集できる
- markdown エクスポートに現在の状態を含める
- プリセットは `stateSheet` を任意で持てる。同梱 trpg-table に初期値を入れる
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 状態シートの列が migration で追加され、共通 400 字・参加者 200 字を超える保存は 400 で拒否される
- [x] #2 話者の system prompt で【現在の状態】節が場面設定の直後・役割プロンプトの前に入り、全参加者に全員分が渡る (テストで確認)
- [x] #3 編成パネルから会話中に共通の状態と参加者の状態を編集でき、次のターンから prompt に反映される
- [x] #4 markdown エクスポートと同梱 trpg-table プリセットが状態シートを扱う
- [x] #5 go test / フロントの型検査・lint が通る
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. migration 016: chats.state_sheet / participants.state_sheet (TEXT NOT NULL DEFAULT '')。model に StateSheet と上限定数 (400 / 200 rune) を置く
2. repository: UpdateMultiAgentSettings を struct 引数にして StateSheet を足す。participant の Create/Update に StateSheet
3. httpapi: PATCH chats / participants で stateSheet を受け、rune 数超過は 400。プリセット適用 (新規作成・既存 chat) で共通・参加者の stateSheet を書く
4. preset: MultiAgentPreset と Participant に任意の stateSheet。Validate で上限検査。同梱 25-trpg-table に初期値、README にフィールド追記
5. turnengine: buildTurnSystemPrompt で場面設定の直後・役割の前に【現在の状態】節 (共通 → 編成順の参加者、見出し付き、空なら省略)。receives_project_material に依らない
6. export: 場面設定の後に「現在の状態」節 (全部空なら省略)
7. frontend: client 型、編成パネルに共通の状態 (場面設定の下) と参加者ごとの状態 (役割の下) を個別保存の欄で追加。rune 数カウンタ。ターン実行中も編集可 (次ターンの GetChat が読む)
8. テスト: prompt 位置・全員分、上限 400、export、preset。go test / pnpm typecheck / lint
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装メモ

- migration 016_state_sheets で `chats.state_sheet` / `participants.state_sheet` を追加。上限は `model.ChatStateSheetMaxRunes` (400) / `ParticipantStateSheetMaxRunes` (200)。前後の空白を除いた rune 数で検査し、PATCH chat / POST・PATCH participant / プリセットの Validate の 3 経路で同じ上限を当てる (prompt 側では切り詰めない)。
- `ChatRepository.UpdateMultiAgentSettings` は nil 許容の文字列ポインタが 4 個並ぶことになるので、位置引数から `MultiAgentSettings` struct に変えた。
- 【現在の状態】節の形式: 見出し行の直後に共通の状態、続いて編成順に「■ 表示名」+ シート。空のシートは飛ばし、全部空なら節ごと出さない。除籍済みの参加者のシートは入れない (卓に居ないため)。
- 既存 chat へのプリセット適用では、共通の状態をプリセットの値 (省略時は空) で上書きする。置き換える前の編成の状態が残らないようにするため (進行役と同じ扱い)。
- UI: 状態シートの欄だけ独立した保存ボタンにし、`disabled` (ターン実行中・自動進行中) の影響を受けないようにした。ターンは開始時の GetChat / ListAll で状態を読み、done フレームの chat はターン後に読み直すので、実行中に保存した値が画面で巻き戻ることはない。文字数カウンタは上限超過で赤くなり、保存ボタンも押せなくなる。
- markdown エクスポートは場面設定の後に「## 現在の状態」を置き、各行を箇条書きにした (そのままだとレンダラが行をつないで 1 段落にしてしまうため)。全部空なら節を出さない。
- 同梱 trpg-table の初期値: 共通「場所: 灰縁坑の入口 / 時刻: 1 日目の昼過ぎ」、レン「HP: 10/10 / 所持品: たいまつ 2 本、ロープ、盗賊道具」、ミラ「HP: 14/14 / 所持品: 包帯 3 巻、メイス、聖印」。場面設定の文言は変えていない (§4.7.2 で前置きの効果が出なかったため)。
- 設計書 §3 / §4.3 / §4.7 見出し / §5 / §6 / Phase 表と presets/multi-agent/README.md を実装に合わせて更新した。

## 検証

- AC #1: TestMultiAgentStateSheets。400 字 (マルチバイト) は保存でき 401 字は 400、参加者は 200 / 201 字で同様。POST participant も 201 字で 400。拒否後も保存済みの値は変わらない。単独 assistant の chat は 400。TestParseStateSheet でプリセットの上限も確認。
- AC #2: TestTurnEngineStateSection。receivesProjectMaterial=false の参加者の system prompt に、共通 → 編成順の参加者の状態が場面設定の後・役割プロンプトの前に入る。GM の空シート・除籍者のシートは出ない。TestTurnEngineNoStateSectionWhenEmpty で空なら節が無いことを確認。
- AC #3: 同テストで、ターン間に保存した値が次のターンの prompt に入ることを確認。実機 (pnpm dev + ブラウザ、LM Studio の gpt-oss-20b) でも、ターン実行中に編成パネルからレンの状態を保存し、API から読み直して反映を確認。201 字以上ではカウンタが赤くなり保存ボタンが無効になることも確認した。
- AC #4: TestBuildChatMarkdownStateSection、TestMultiAgentPresetStateSheets (作成時と後から適用で同じ状態、状態の無いプリセットの再適用で共通の状態が消える)。実機のエクスポートでも節の出力を確認。
- AC #5: go test ./... / go vet ./... / pnpm run check:client が通る。フロントの lint スクリプトはリポジトリに無いため型検査のみ。gofmt -l は今回触っていない internal/search/model.go だけを挙げる。

## 未測定

- 状態シートを入れたときに小型モデルの発言がどう変わるか (HP や所持品を描写に反映するか) は測っていない。§4.7.2 でシートは参考情報として読まれると分かっているので、効果の確認は TASK-36 の効果コマンドと合わせて行うのがよい。
<!-- SECTION:NOTES:END -->
