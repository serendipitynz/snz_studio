---
id: TASK-106
title: '多人数会話: 選抜発言と末尾の発言の切り詰めを発言の参照として保存し、参照一覧に表示する'
status: Done
assignee: []
created_date: '2026-10-17 19:25'
updated_date: '2026-11-07 05:05'
labels: []
dependencies:
  - TASK-104
references:
  - docs/multi-agent-chat-design.md
  - internal/db/schema.go
  - internal/service/turnengine.go
type: feature
ordinal: 110000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

設計書 §4.9.5。AGENTS.md の Required UX「回答に使った参照を見せる」を過去の発言にも広げ、各発言が直近区間より前のどの発言を読んだかを示す。
直近区間は毎ターン必ず読むので参照に入れない。あわせて、末尾の発言を窓上限に収めるために中略したとき (§4.9.2 手順 3) は、
画面に見えている発言とモデルが読んだ発言が食い違う唯一の場合なので、同じ参照一覧で示す。

## やること

- `assistant_message_references.source_type` の CHECK 制約に発言を指す値を足す (テーブルの作り直しを伴う migration)
- ターンの完了時に、そのターンの選抜発言と、末尾の発言を中略したこと (どの発言を何字に縮めたか) を参照として保存する
- 発言の参照一覧に、選抜発言 (話者名と抜粋) と中略の記録を表示する
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 選抜発言がそのターンの発言の参照として保存され、直近区間の発言は保存されない (テストで確認)
- [x] #2 観戦ビューの参照一覧で、選抜発言が話者名と抜粋つきで表示される
- [x] #3 既存の参照 (project / document / memory) の保存と表示が変わらない
- [x] #4 末尾の発言を中略したターンでは、どの発言を何字に縮めたかが参照一覧に表示される
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. migration 023: assistant_message_references を作り直し、source_type に utterance (選抜発言) と shortened_utterance (中略した末尾の発言) を足し、中略後に読んだ字数の read_chars (NULL 可) を足す
2. assembleTurnHistory が prompt 用の履歴と一緒に、選抜発言 (直近区間より前で prompt に入った発言。応答先も含む) と中略した末尾の発言を返す。選抜発言の score は発言スコア、抜粋は 80 字
3. RunTurn が資料の参照と並べて同じトランザクションで保存する
4. MessageReferences: 既存の参照はそのまま、その後に中略の記録と選抜発言を出す。話者名は観戦ビューの現在の名前で引く
5. 設計書 §4.9 / §4.9.5 / §5 の参照の記述と、turnhistory.go の extension point を更新
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装

- migration 023 で `assistant_message_references` を作り直し、`source_type` に `utterance`（選抜発言）と `shortened_utterance`（中略した末尾の発言）を足した。中略後に読んだ字数は新しい列 `read_chars` に持つ（他の参照では NULL）。
- `assembleTurnHistory` が prompt 用の履歴と一緒に参照を返し、`RunTurn` が資料の参照と同じトランザクションで保存する。選抜発言には応答先として一緒に入った直前の発言も含める（どちらも直近区間より前から prompt に入った発言なので、§4.9.1 の選抜発言の定義に当たる）。
- `MessageReferences` は資料の参照を従来どおり先に出し、その後に中略の記録、選抜発言を並べる。話者名は観戦ビューが表示中の名簿から引き直す。

## 着手前の提案から変えた点

- 事前の検討では「message ID だけ保存して抜粋は画面側で引く」案を挙げていたが、ほかの参照と同じく抜粋を保存する形にした。代わりに抜粋を 80 字（資料の参照は 220 字）に抑え、保存量を小さくした。参照を単体で読める形のまま保ち、画面側の引き当ては話者名だけにした。
- 読んだ字数を `score` に入れる案は、並べ替えに使われる欄に別の意味を持たせることになるので採らず、列を足した。画面の言語と処理言語は別に選べるので、文として保存する案も採らなかった。

## 確認

- AC #1: `TestAssembleTurnHistoryReferencesSelected`（prompt に入った直近区間より前の発言と参照が一致し、直近区間は含まれない）、`TestTurnEngineStoresUtteranceReferences`（実際のターンで DB に保存され、直近 4 発言は含まれない）。
- AC #2 / #4: 偽の LLM サーバー（コンテキスト長 6,000 トークン）と一時データで `pnpm dev` を起動し、2 人の会話を 10 ターン進めた後に 6,035 字の介入を入れて 1 ターン進めた。観戦ビューで、10 ターン目の参照一覧に選抜発言 6 件が話者名（Alice / Bob / ユーザー）と抜粋つきで出ること、最後のターンに「ユーザー」「中略」「3659 字に中略して読みました」と抜粋が出ることを目で確認した。テストは `TestAssembleTurnHistoryReferencesShortened`・`TestTurnEngineStoresShortenedReference`。
- AC #3: `TestUtteranceReferencesMigration`（既存の参照行が migration 後も同じ値で残り、未知の source_type は拒否され、message 削除の cascade も残る）と既存の repository / service テスト。資料の参照の描画部分は変えていない。単独 chat の画面は目では確認していない。
- `go vet ./...`、`go test ./...`、`pnpm run check:client`、`pnpm run test:client` がすべて通る。
<!-- SECTION:NOTES:END -->
