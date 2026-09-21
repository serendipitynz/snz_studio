---
id: TASK-20
title: '多人数会話: 参加者ごとに背景資料 (ドキュメント・メモリ) を受け取るかを選べるようにする (TRPG の GM 用)'
status: Done
assignee: []
created_date: '2026-09-20 00:18'
updated_date: '2026-09-21 11:09'
labels: []
milestone: m-1
dependencies:
  - TASK-18
references:
  - docs/multi-agent-chat-design.md
  - internal/model/model.go
  - internal/repository/participant.go
  - internal/service/turnengine.go
  - frontend/src/components/ParticipantPanel.tsx
  - internal/db/schema.go
ordinal: 20000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

多人数会話の目標の一つに AI による TRPG の再現がある。TASK-18 はプロジェクトのドキュメント・メモリを
背景資料としてターンごとに 1 回組み立て、全参加者に同じものを渡す。共有世界の文脈としては妥当だが、
TRPG では GM だけがシナリオの中身 (ダンジョンの構造、NPC の真意、伏線) を知っている必要があり、
プレイヤー役の参加者が背景資料を読むと答えを知った上で行動してしまう。

## 方針

参加者に「背景資料を受け取る」の真偽値を持たせ (既定は true、既存の参加者も true)、false の参加者の
ターンでは背景資料を system prompt に入れず、参照も保存しない。編成パネルに参加者ごとの
チェックボックスを 1 つ足す。プリセットの participants にも同じ項目を任意で持たせる (省略時 true)。

背景資料の組み立て自体は TASK-18 のままターンごとに 1 回で、受け取らない話者のターンでは検索を
省略する (性能面でも自然)。ドキュメント側に「GM 専用」などの分類を付けて参加者ごとに出し分ける案は、
このフラグで足りないことが確認されてから検討する。

足りるかどうかは 2 つの用途で見る。TRPG の GM (シナリオを GM だけが読む) は真偽値で足りる。
役割別レビュー (設計者は要件と設計書、レビュアーはそれに加えてコーディング規約、QA は要件と既知の問題、のように
参加者ごとに異なる資料の部分集合を読む) は真偽値では表現できない。後者を実現するときは、参加者 → ドキュメント ID の
集合を持たせる実装タスクを、TASK-18 の組み立て関数の話者引数 (TASK-18 項目 10) を前提に切る。
メモリを参加者ごとに絞る必要は、その時点で用途が見えてから判断する。

## 付随

設計書 `docs/multi-agent-chat-design.md` に、TRPG 再現を目標の一つとして明記し、
「背景資料は既定で全参加者に見える」ことを既知の制約として記録する (TASK-18 の §4.4 改訂と整合させる)。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 参加者ごとに背景資料を受け取るかを編成パネルで切り替えられ、既定と既存参加者は受け取る
- [x] #2 受け取らない参加者のターンでは背景資料が system prompt に入らず、参照も保存されず、検索も実行されない
- [x] #3 プリセット JSON で参加者ごとにこの項目を指定でき、省略時は受け取る
- [x] #4 設計書に TRPG 再現の目標と、背景資料が既定で全参加者に見えるという制約が記録されている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 用語: 「背景資料を渡す」= DB `receives_background` / Go `ReceivesBackground` / JSON・TS `receivesBackground`。既存語「背景資料」(設計書 §4.4) はそのまま使う。
2. migration 011: ALTER TABLE participants ADD COLUMN receives_background INTEGER NOT NULL DEFAULT 1 (既存行が true になるのは DEFAULT 1 による)。
3. model.Participant に ReceivesBackground bool を追加。
4. repository/participant.go: 列・scan・CreateParticipantInput (*bool、nil = 既定 true)・UpdateParticipantInput (*bool、nil = 変更なし)。bool の SQL バインドは既存の boolToInt に合わせた boolPtrArg を足す。
5. httpapi/multiagent.go: 作成・更新の body で receivesBackground を受ける。bodyStringPtr / bodyIntPtr と同じ形の bodyBoolPtr を server.go に足す。
6. preset: Participant.ReceivesBackground *bool を追加し、Validate で nil を true に正規化 (turnRule の空 → round_robin と同じ扱い)。createPresetParticipants が渡す。
7. turncontext.go: AssembleTurnBackground を話者引数で分岐させ、渡さない話者では project 取得・検索の前に空の TurnBackground を返す (TASK-18 項目 10 の受け口。エンジン側には手を入れない)。
8. frontend: client.ts の Participant / createParticipant / updateParticipant / MultiAgentPresetParticipant に receivesBackground を追加。ParticipantEditor にチェックボックスを 1 つ足し、既存の保存ボタンのまとめ保存に含める。説明は既存の FieldHint (上向きツールチップ) に載せる。i18n は en / ja 両方。
9. 文書: 設計書 §0 (TRPG 再現を目標に明記)・§3 (migration 11 の列)・§4.4 (参加者ごとの可否と、既定で全参加者に見えるという制約)・§5 (API の項目)・§6 (編成パネルのチェックボックス)、presets/multi-agent/README.md の形式表。
10. テスト: repository の既定 true と更新、preset の省略時 true、turncontext の早期 return (依存を持たない ContextService で検索が走らないことを示す)、turnengine の統合 (渡さない話者の system prompt に背景資料が無く参照も保存されない)、httpapi の作成・更新。go test ./... と frontend の lint / build。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## TASK-20 の実装 (2026-09-21)

**用語**: 「背景資料を渡す」= その参加者が話者になるターンで背景資料 (設計書 §4.4) を組み立て、system prompt に入れ、
参照として保存すること。DB `receives_background` / Go `ReceivesBackground` / JSON・TS `receivesBackground`。
既存語「背景資料」は §4.4 の定義をそのまま使い、新語を増やしていない。

**判定の置き場所**: `ContextService.AssembleTurnBackground` の先頭 (プロジェクト取得より前) で早期 return する。
TurnEngine 側で組み立て結果を捨てる案は採らなかった。理由は 2 つ: TASK-18 項目 10 が話者引数を用意したのはこの分岐のためで、
エンジンで判定すると話者引数が未使用のまま方針だけ二重化する。もう 1 つは AC #2 の「検索も実行されない」で、
組み立て後に捨てる形では検索の費用を払ってしまう。

**「検索が走らない」の検証方法**: 依存を 1 つも持たない `&ContextService{}` に対して
渡さない話者で `AssembleTurnBackground` を呼び、空の `TurnBackground` が返ることを確認する
(`TestAssembleTurnBackgroundSkipsSpeaker`)。repository も retrieval も nil なので、
それらに触れた瞬間 nil panic する = 触れていないことの証明になる。
検索呼び出し回数を数えるモックを入れるには `ContextService.retrieval` をインタフェース化する必要があり、
単独チャット側の構造まで変えることになるので採らなかった。

**既定値**: 列は `NOT NULL DEFAULT 1` (migration 011)。既存の参加者行が「渡す」になるのはこの DEFAULT による。
`CreateParticipantInput.ReceivesBackground` / preset の `receivesBackground` を *bool にしたのは、
省略を false ではなく既定 (true) として扱うため。preset は `Validate` で nil を true に正規化する
(turnRule の空欄 → round_robin と同じ扱い)。

**確認したこと**:
- `go test ./...` 全パス、`go vet ./...` 無指摘、変更した .go ファイルは `gofmt -l` で無指摘。
- `pnpm check:client` (tsc --noEmit) と `pnpm build:client` が成功。
- 追加・拡張したテスト: `TestAssembleTurnBackgroundSkipsSpeaker` / `TestTurnEngineSkipsBackgroundForSpeaker` (AC #2)、
  `TestParseReceivesBackground` と `TestMultiAgentPresets` の拡張 (AC #3)、
  `TestParticipantRepository` と `TestMultiAgentParticipantsCRUD` の拡張 (既定 true・更新・作成時指定)。
  `TestTurnEngineSkipsBackgroundForSpeaker` では、同じ会話・同じプロジェクトで Alice の system prompt に
  `[ドキュメント] 灯台守の記録` が入り、Bob の system prompt には「背景資料」も資料本文も入らず、
  保存された参照が 0 件になることを確認した (ターンログでも references=3 と references=0)。

**未確認 (AC #1 を未チェックのままにした理由)**: 編成パネルのチェックボックスを実機で操作しての確認。
既定値と既存参加者が「渡す」側になることは repository・HTTP のテストで確認済みだが、
チェックボックスの表示・保存操作そのものはフロントエンドのテスト基盤が無く (vitest 等は未導入。
導入は新規 dev 依存になるため本タスクでは足していない)、ビルドと型検査までしか確認していない。
アプリで 1 度確認したうえで AC #1 をチェックしたい。

**スコープ外として残したもの**: 渡す参加者どうしで資料を分ける (役割別レビューの用途) は真偽値では表現できないため、
設計書 §4.4 の「既知の制約」に書くだけにした。着手時は参加者 → ドキュメント ID の集合を持たせる別タスクになる。
<!-- SECTION:NOTES:END -->
