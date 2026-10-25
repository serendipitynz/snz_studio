---
id: TASK-109
title: '言語: 表示言語とは別に処理言語を設け、同梱プリセットと LLM への指示を処理言語に合わせて日本語・英語で切り替える'
status: To Do
assignee: []
created_date: '2026-10-25 08:51'
labels: []
dependencies: []
references:
  - frontend/src/i18n/index.tsx
  - frontend/src/components/SettingsModal.tsx
  - internal/preset/preset.go
  - internal/preset/bundled
  - internal/service/chat.go
  - internal/service/summary.go
  - internal/service/memory.go
  - internal/service/memoryorganizer.go
  - internal/service/review.go
  - internal/service/conclusion.go
  - internal/service/turnengine.go
  - internal/service/turncontext.go
  - internal/service/addressing.go
  - internal/service/export.go
  - docs/multi-agent-presets.md
type: feature
ordinal: 113000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

設定の「表示言語」は画面の文言だけを切り替える。値はフロントエンドの `localStorage` に置かれ、API には送られず、Go 側はこの値を知らない。
LLM に送る指示は表示言語と関係なく固定で、言語がばらばらになっている。

- 1 対 1 チャットのシステムプロンプト (`chat.go`): 英語で、応答の言語の指示は無い。
- 要約・タイトル生成 (`summary.go`)、メモリ抽出・整理 (`memory.go`・`memoryorganizer.go`)、創作文レビュー (`review.go`): 日本語で、「日本語で」と指示している。
- 多人数会話の役割リマインド (`turnengine.go`)・プロジェクト資料の見出し (`turncontext.go`)・結論の抽出 (`conclusion.go`): 日本語で、言語の指示は無い。
- 同梱プリセット 25 件 (`internal/preset/bundled/*.json`) は日本語だけ。

そのため、英語で使いたいときにプリセットと指示を英語にできない。逆に、日本語で使っていても言語の指示が無い箇所では、gpt-oss-20b が英語で発言することがある (TASK-107)。

## 変えるもの

ここで処理言語とは、同梱プリセットの版と、アプリが LLM に送る指示の文面を決める言語を指す。表示言語 (画面の文言を決める言語) とは別に選べるようにする。対象は日本語と英語の 2 つ。

- 設定画面に、表示言語とは別に処理言語の選択欄を置く。処理言語は Go 側の設定に保存する。初期値は日本語にし、今の挙動を保つ。
- 同梱プリセットを日本語版と英語版の 2 組にする。2 組は同じ id の集まりを持ち、プリセットの選択欄は処理言語の版を出す。
- 上の各箇所の指示の文面を、処理言語ごとに用意する。処理言語が日本語のときに送る文面は、今と同じに保つ。
- 処理言語の語で応答するよう求める指示を、多人数会話の役割リマインドに足す (TASK-107 の英語化を、プリセットごとではなくここで防ぐ)。
- 文面に依存する判定も処理言語に合わせる: メモリの種類を決める正規表現 (`memory.go`)、次の話者の指名 `[次: 表示名]` の読み取り (`addressing.go`)。

## 対象外

- 検索の分かち書き (`internal/search`) と同梱の埋め込みモデル (ruri-v3-30m) は日本語向けのまま変えない。
- 処理言語を変えたときに、作成済みの多人数会話の場面設定・役割プロンプトを訳し直すことはしない (プリセットは適用時にチャットへ写されるので、写した後は処理言語と切り離れる)。

## 決めること

- 処理言語をワークスペース全体で 1 つ持つか、チャットごとに作成時の値を記録するか。全体で 1 つにすると、日本語で作った会話も、切り替えた後は英語の役割リマインド・要約の指示で動く。
- 1 対 1 チャットのシステムプロンプトにも「処理言語で答える」指示を足すか。足すと、ユーザーが処理言語と違う言語で書いたときも処理言語で答える。
- エクスポートする Markdown の見出し (`export.go`) を、処理言語と表示言語のどちらに合わせるか。
- 次の話者の指名の書式を処理言語ごとに変えるか (`[Next: name]` など)、両方の書式を常に受け付けるか。
- 同梱プリセットの 2 組の置き方 (言語ごとのディレクトリにするか、1 ファイルに両言語の値を持たせるか)。
- 着手時に、このタスクを子タスクに分けるか。

## TASK-107 との関係

TASK-107 は同梱 trpg-table の場面設定に「日本語で書く」1 行を足す案。このタスクで役割リマインドに処理言語の指示を入れれば、TASK-107 はプリセット側の追記なしで解決する可能性がある。どちらを先にするか、TASK-107 をこのタスクに含めるかは着手時にユーザーに確認する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 設定画面に表示言語と処理言語の選択欄が別々にあり、処理言語が Go 側の設定に保存され、初期値が日本語である
- [ ] #2 同梱プリセットに日本語版と英語版があり、両者の id の集まりが一致することをプリセットのテストが確かめている
- [ ] #3 プリセットの選択欄が処理言語の版を出し、英語版を適用した会話の場面設定・役割プロンプトが英語である
- [ ] #4 処理言語が英語のとき、要約・タイトル生成・メモリ抽出・メモリ整理・創作文レビュー・結論の抽出・役割リマインド・プロジェクト資料の見出しが英語の文面で送られることをテストが確かめている
- [ ] #5 処理言語が日本語のとき、各箇所の文面が変更前と同じであることをテストが確かめている
- [ ] #6 決めることの各項目の判断が Implementation Notes に記録されている
- [ ] #7 README と docs (current-spec・multi-agent-presets) が処理言語を説明している
- [ ] #8 go test ./... と frontend の型検査・テスト・lint が通る
<!-- AC:END -->
