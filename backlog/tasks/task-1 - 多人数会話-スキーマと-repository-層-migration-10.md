---
id: TASK-1
title: '多人数会話: スキーマと repository 層 (migration 10)'
status: Done
assignee: []
created_date: '2026-09-08 22:28'
updated_date: '2026-09-18 06:30'
labels: []
dependencies: []
references:
  - docs/multi-agent-chat-design.md
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/multi-agent-chat-design.md §2〜§3 に従い、chats.kind / turn_rule / scene_prompt 列の追加、participants テーブル新設 (deleted_at による論理削除を含む)、messages.participant_id 列の追加を migration 10 として実装し、repository 層に参加者の CRUD と participant_id つきメッセージ保存を足す。参加者取得は編成 (deleted_at IS NULL) と帰属解決用の全行を別メソッドで返す。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 migration 10 が既存 DB に適用でき、既存 chat (kind='assistant') の動作に影響がない
- [x] #2 participants の CRUD が repository 層で動き、テストがある
- [x] #3 go build / go vet / go test が全グリーン
- [x] #4 参加者を除籍しても過去の発言の表示名・モデル名が解決でき、round_robin の起点が失われない (設計 §3)
- [x] #5 編成取得と帰属解決用の全行取得が別メソッドとして存在し、除籍済み参加者を含むケースのテストがある
- [x] #6 repository 層の参加者取得・更新・削除が chat_id つきで引ける形になっており (サービス層が同一 chat 検証を行える)、除籍済みを含むケースのテストがある
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. internal/db/schema.go に migration 010_multi_agent_chat を追加 (設計 §3 の DDL をそのまま)。001 の初期スキーマは触らない — 触らなければ新規 DB でも既存 DB でも素の ALTER が通り、006/008 のような hasColumn ガードが不要になる。
2. internal/model に Participant を追加。Chat に Kind / TurnRule / ScenePrompt、Message に ParticipantID を追加 (JSON タグはフロントの命名に合わせ camelCase)。
3. repository/chat.go: chatColumns・messageColumns・scanChat・scanMessage を新列に追随。CreateChatInput に Kind/TurnRule/ScenePrompt (空なら 'assistant'/'round_robin'/'')、AddMessageInput に ParticipantID。turn_rule/scene_prompt の部分更新メソッドを COALESCE 方式で追加 (§5 の PATCH /api/chats/{chatId} 用)。
4. repository/participant.go を新設: ListRoster (編成)、ListAll (帰属解決用の全行)、GetParticipant / CreateParticipant / UpdateParticipant / RemoveParticipant (除籍 = deleted_at)。取得・更新・削除は chat_id を含む *model.Participant を返し、サービス層が同一 chat 検証をできる形にする (AC #6)。
5. テスト: db_test.go に「009 まで適用した DB に既存 chat/message を入れてから 010 を適用」する既存 DB シナリオ (AC #1)、repository_test.go に参加者 CRUD・除籍後の表示名解決・除籍済みが編成から消えて全行には残ること (AC #2/#4/#5/#6)。
6. go build ./... / go vet ./... / go test ./... を通す (AC #3)。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## migration 10 (internal/db/schema.go)
設計 §3 の DDL をそのまま `010_multi_agent_chat` として追加。001 の初期スキーマは意図的に触っていない — 触らなければ新規 DB・既存 DB のどちらでも素の `ALTER TABLE` が通り、006/008 が必要とした `hasColumn` ガードを持ち込まずに済む。設計に無い追加は `idx_participants_chat_id (chat_id, sort_order)` のみで、既存の `idx_chats_project_id` 等と同じ粒度。

## repository 層
- `participants.go` を新設。設計の「編成」「帰属解決用の全行」を `ListRoster` / `ListAll` に対応させ、除籍は `RemoveParticipant`（`deleted_at` を立てる論理削除、二度目は no-op）。`GetParticipant` / `UpdateParticipant` / `RemoveParticipant` はいずれも `chat_id` を含む `*model.Participant` を返すので、サービス層は他 chat の参加者指名を chat_id 比較だけで弾ける (AC #6)。
- 巡回順は `ORDER BY sort_order, created_at, id`。sort_order 同値でも後続が一意に決まらないと round_robin の「次」が非決定になるため、全順序にしてある。
- `CreateParticipant` の sort_order は chat 内 max+1 (`CreateProject` の前例に合わせた)。max は除籍済みも含めて取るので、除籍後に追加しても既存の順序と衝突しない。
- `UpdateParticipant` は COALESCE による部分更新。除籍済みも更新可能なまま残した — 編集を許すかはサービス層の方針で、repository が行を到達不能にする理由がない。
- `chat.go`: 新列を chatColumns/messageColumns と scan に追随させ、`CreateChatInput` に Kind/TurnRule/ScenePrompt (空なら 'assistant'/'round_robin')、`AddMessageInput` に ParticipantID を追加。§5 の `PATCH /api/chats/{chatId}` 用に `UpdateMultiAgentSettings(chatID, turnRule, scenePrompt)` を COALESCE 方式で追加。
- kind / turn_rule の許容値は DDL 側に CHECK が無い (設計どおりコメントのみ) ので、`model` 側の定数 `ChatKindAssistant` / `ChatKindMultiAgent` / `TurnRuleRoundRobin` / `TurnRuleManual` を唯一の定義とした。

## 検証 (2026-09-18)
- `go build ./...` / `go vet ./...` / `go test ./...` すべてグリーン (全 14 パッケージ ok)。
- AC #1: `TestMultiAgentMigrationOnExistingDB` — 009 までを適用した DB に project/chat/message を入れてから `ApplyMigrations` で 010 を適用し、chat が `kind='assistant' / turn_rule='round_robin' / scene_prompt=''` で既存値そのまま、message の `participant_id` が NULL のままであることを確認。participants の chat 削除カスケードも同テストで確認。
- AC #2/#5/#6: `TestParticipantRepository` — CRUD、編成順、別 chat の参加者が編成に混ざらないこと、部分更新が他フィールドを壊さないこと、除籍済みを含む `GetParticipant`/`UpdateParticipant`/`ListAll` と、存在しない ID が (nil, nil) になること。
- AC #4: 同テストで、除籍後も `ListAll` から過去発言の `participant_id` → 表示名・モデル名が解決でき、編成 (`ListRoster`) に次の起点が残ることを確認。除籍済みが直近発言者だったときの「編成の先頭から」フォールバック自体は TurnEngine の責務なので TASK-2 で検証する。

## 未測定・申し送り
- HTTP レスポンスに `kind` / `turnRule` / `scenePrompt` / `participantId` が増えるが、フロント型の追随は TASK-3/4 の範囲として触っていない (既存 UI は余剰フィールドを無視するため動作影響なし)。
- 実 DB (`data/`) への 010 適用は次回アプリ起動時に走る。手元の実データでの適用は未実施。
<!-- SECTION:NOTES:END -->
