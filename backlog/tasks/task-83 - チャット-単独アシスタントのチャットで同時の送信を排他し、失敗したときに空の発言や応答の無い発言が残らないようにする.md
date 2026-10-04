---
id: TASK-83
title: 'チャット: 単独アシスタントのチャットで同時の送信を排他し、失敗したときに空の発言や応答の無い発言が残らないようにする'
status: In Review
assignee: []
created_date: '2026-09-28 20:23'
updated_date: '2026-10-04 06:58'
labels: []
dependencies: []
references:
  - internal/service/chat.go
  - internal/service/turnengine.go
priority: low
type: bug
ordinal: 83000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F17 (Low、確信度 Medium)。2026-09-29 に `756d32b` で未修正を確認した。

- 多人数会話はターンの実行中を排他している (409) が、単独アシスタントのチャット (`chat.go`) には排他が無い。2 つのウィンドウや二重送信で同じチャットに並行して生成すると、要約の更新 (`UpsertSummary`) が後勝ちになり、片方の往復が要約から落ちる。
- ストリーミングの経路は空のアシスタント発言を先に作るため、生成中にプロセスが終わると空の発言が残る。
- `prepareTurn` でユーザー発言を保存した後にメモリの保存が失敗すると、応答の無いユーザー発言が残る。

## 方針

- 多人数会話と同じ仕組みでチャット単位の排他を入れ、生成中の送信は 409 にする。
- 起動時に空のアシスタント発言を片付けるか、ユーザー発言とメモリの保存を 1 トランザクションにする。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 生成中の単独チャットへの送信が 409 で拒否される。テストがある
- [x] #2 生成中にプロセスが終わった後に起動し直しても、空のアシスタント発言が画面に残らない
- [x] #3 メモリの保存に失敗したとき、応答の無いユーザー発言が残らない (またはエラーとして画面に示される)。テストがある
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 多人数会話の実行中チャット集合 (TurnEngine.running) を小さな型 turnSlots に切り出し、TurnEngine と ChatService がそれぞれ 1 つ持つ。ChatService.SendMessage / SendMessageStream は冒頭で枠を取り、取れなければ ErrTurnInProgress を返す (AC#1)
2. SendMessageStream に onStart コールバックを足し、枠の取得と prepareTurn の成功後に呼ぶ。ハンドラは onStart で初めて SSE を開くので、それより前の失敗 (409・メモリ保存の失敗) は SSE のエラーフレームではなく HTTP ステータスで返る。非ストリーミングの経路も ErrTurnInProgress を 409 にする
3. prepareTurn で、メモリの保存 (と埋め込み同期) をユーザー発言の保存より前に行う。メモリ保存は LLM 呼び出し (覚えて の抽出) を挟むため 1 トランザクションにはせず、順序で「失敗したらユーザー発言が無い」を保証する (AC#3)
4. 起動時 (Server.RunStartupTasks) に、内容が空で response_ms も無いアシスタント発言 (生成中にプロセスが終わったときの置き場) を削除する。正常に終わった空の応答は response_ms を持つので消えない (AC#2)
5. service と httpapi にテストを足す: 生成中の 409 (両経路)・メモリ保存失敗でユーザー発言が残らないこと・起動時の掃除
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装

- 排他 (AC#1): 多人数会話の実行中チャット集合を `turnSlots` (internal/service/turnslots.go) に切り出し、TurnEngine と ChatService がそれぞれ 1 つ持つ。単独チャットの `SendMessage` / `SendMessageStream` は冒頭で枠を取り、取れなければ多人数会話と同じ `ErrTurnInProgress` を返し、HTTP では両経路とも 409 (JSON) にした。
  - TurnEngine と枠を共有しない理由: 単独チャットの経路はハンドラが chat.Kind で振り分けた後にしか呼ばれず、同じチャットで両者が重なる経路が無い。
- ストリームを開く時点: `SendMessageStream` に `onStart` を足し、枠の取得と `prepareTurn` (ユーザー発言とメモリの保存) の成功後に呼ぶ。ハンドラは `onStart` で初めて SSE を開くので、409 とメモリ保存の失敗は SSE のエラーフレームではなく HTTP ステータスで返り、画面ではコンポーザーのエラーとして出る。多人数会話の `onSpeaker` と同じ形。
  - 副作用: 「覚えて」を含む送信では、メモリ抽出の LLM 呼び出しが終わるまで SSE のヘッダーが返らない。フロントは fetch のヘッダーを待つだけで時間制限は無く、サーバーにも WriteTimeout は無いので、表示上は楽観的に出した発言がそのまま待つだけになる。
- メモリ保存の失敗 (AC#3): 1 トランザクションにはせず、`prepareTurn` でメモリの保存 (と埋め込み同期) をユーザー発言の保存より前に移した。「覚えて」の抽出が途中で LLM を呼ぶため、トランザクションを LLM 応答待ちの間開いたままにすることになるのを避けた。メモリ抽出はもともと保存前に組み立てた `assembled.RecentMessages` だけを見ており、ユーザー発言を読み直さないので、順序を変えても抽出結果は変わらない。
  - 残る状態: 埋め込み同期だけが失敗した場合はメモリ行が残り、ユーザー発言は残らない。メモリ側は起動時と再接続時の `SyncMissing` が埋め直し、再送しても `HasSimilarMemory` で重複しない。
- 空のアシスタント発言 (AC#2): 起動時 (`Server.RunStartupTasks`、HTTP の受付開始前) に、role=assistant・content が空・response_ms が NULL の発言を削除する (`ChatRepository.DeleteUnfinishedAssistantMessages`)。LLM が空文字を返して正常終了した応答は response_ms を持ち、LLM 失敗時の代替応答は空でないので、どちらも消えない。途中まで delta を受けた発言 (内容あり) は消さずに残す。

## 検証

- `go test ./... -count=1` 全パス、`go vet ./...` 指摘なし。関連テストは `-race -count=3` でもパス。
- AC#1: `TestChatTurnRefusedWhileGenerating` (service、両経路が ErrTurnInProgress・何も保存しない・終了後は再び送れる)、`TestSendMessageTurnConflict` (httpapi、両ルートが JSON の 409)。
- AC#2: `TestDeleteUnfinishedAssistantMessages` (repository、消すのは空のプレースホルダーだけ)、`TestStartupRemovesUnfinishedAssistantMessage` (httpapi、RunStartupTasks 後の GET /api/chats/{id} に空の発言が出ない)。
- AC#3: `TestChatTurnMemoryFailureLeavesNoUserMessage` (memories への INSERT を失敗させるトリガーで、両経路ともエラーになり onStart が呼ばれず発言が 0 件)。
- 実アプリでのプロセス強制終了→再起動の手動確認はしていない。
- `gofmt -l` は internal/search/model.go を挙げるが、この変更では触っていない既存の差分。
<!-- SECTION:NOTES:END -->
