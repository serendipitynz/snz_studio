---
id: TASK-105
title: '多人数会話: 観戦ビューで発言をピン留めでき、ピン留めした発言が履歴に優先して残るようにする'
status: In Review
assignee: []
created_date: '2026-10-17 19:25'
updated_date: '2026-11-03 07:10'
labels: []
dependencies:
  - TASK-104
references:
  - docs/multi-agent-chat-design.md
  - internal/db/schema.go
  - internal/service/turnengine.go
  - frontend/src/pages/MultiAgentChatPage.tsx
type: feature
ordinal: 109000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

設計書 §4.9.4。ピン留めは、ユーザーが発言に付け外しする印で、発言スコアに大きく加点する (§4.9.3、他の加点の合計の上限より大きい値)。
強制ではなく加点なので、履歴予算を超えるほどピン留めが多いときは、ピン留めどうしで他の加点の順になる。

## やること

- migration 022: `messages.pinned` (INTEGER NOT NULL DEFAULT 0)
- 発言のピン留めを付け外しする API。ターンの排他には入れず、ターンの実行中も操作できる
- 観戦ビューの発言ごとに付け外しの操作と、ピン留め中であることの表示
- 発言スコアにピン留めの加点を足す (値は TASK-103 で決めたもの)
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 migration で messages.pinned が追加され、既存の発言は 0 になる
- [x] #2 観戦ビューから発言のピン留めを付け外しでき、ターンの実行中も操作できる
- [x] #3 ピン留めした発言が、直近区間より前にあっても履歴予算の範囲で選抜発言に入る (テストで確認)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. migration 022_message_pinned: messages.pinned INTEGER NOT NULL DEFAULT 0。model.Message.Pinned と repository の列・SetMessagePinned を足す
2. PATCH /api/messages/{messageId}/pinned（body { pinned }）。ターンの排他は取らない。発言/chat 不在 404、単独 assistant 400、一時チャットは許可（メモリと違いプロジェクトに書かない）。chats.updated_at は動かさない
3. scoreTurnHistory に turnScorePinned = 100 を足す。テスト: 直近区間より前のピン留め発言が、他の加点の高い発言に勝って選抜発言に入ること・予算を超えるピン留めはピン留めどうしで他の加点の順になること
4. 観戦ビュー: 発言ごとの操作行にピン留めのトグル（メモリのロックと同じく語は操作、状態は発言者名の横のバッジ）。ターン中も押せる。done フレーム・介入の応答で発言一覧を置き換えるときは、表示中の発言のピン留めを手元の値で保つ（ターン中の付け外しと応答の読み取りが前後するため）
5. (?) の説明文にピン留めの優先を足す（ja/en）
6. 設計書 §4.9 の見出し・表・§5・§6、current-spec(.ja) を更新
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- API は `PATCH /api/messages/{messageId}/pinned`（body `{ pinned }`）。ターンの排他は取らない。一時チャットでも受け付ける（メモリ保存と違い、ピン留めは会話の外に何も書かないため）。単独 assistant の chat は 400。`chats.updated_at` は動かさない（会話の内容は変わらないため）。
- 加点は `turnScorePinned = 100`（設計書 §4.9.3 の値。他の加点の合計の上限 15 より大きい）。
- 観戦ビュー: メモリのロックと同じく、ボタンの名前と図形は「これから行う操作」（発言をピン留め / ピン留めを外す。Lucide 1.48.0 の pin / pin-off）、状態は発言者名の横のバッジ「ピン留め中」で示す。ターンの `done` と介入の応答が運ぶ発言一覧は、表示中の発言のピン留めを手元の値で保って取り込む。ターン中に付けたピン留めが、ターン側の読み取りより後に保存されて上書きされるのを防ぐため。
- (?) の説明文に「ピン留めした発言を優先」を足した（ja/en）。設計書 §4.9・§4.9.4・§5・§6 と current-spec（ja/en）を更新した。

検証:
- AC #1: `TestMessagePinnedMigration`（021 まで当てた DB に発言を入れて 022 を当て、既存の発言・後から入れた発言とも pinned = 0）。
- AC #2: `TestMultiAgentPinMessage` / `TestMultiAgentPinMessageRefusals` / `TestMultiAgentPinDuringTurn`（スタブの補完でターンを止めたまま 200 で付けられ、ターン完了後も残る）。実アプリ（`DATA_DIR` を一時ディレクトリに向けた `pnpm dev`、3 秒刻みで応答するスタブ LLM）で、ターン中にピン留めし、完了後もバッジが残ること、外す操作、再読み込み後の保持、(?) の文言を確認した。
- AC #3: `TestAssembleTurnHistoryPrefersPinned`（予算が直近区間 + 1 組しか入らない会話で、ピン留めなしなら人間の介入発言が選ばれ、直近区間より前の発言をピン留めするとそちらが選ばれる）、`TestAssembleTurnHistoryPinsOverBudget`（ピン留めが予算を超えるとピン留めどうしで他の加点の順になる）、`TestScoreTurnHistory/pinned`。
- `go test ./...`・`go vet ./...`・`gofmt -l`・`pnpm run check:client`・`pnpm run test:client` が通った。
- 見ていないもの: ダークテーマと Solarized でのバッジの見え方（既存の `Badge tone="warm"` をそのまま使ったので新しい配色の組み合わせはない）。snz-design doc-17 の適用記録は更新していない。
<!-- SECTION:NOTES:END -->
