---
id: TASK-3
title: '多人数会話: HTTP API (participants CRUD + turns/stream SSE)'
status: In Review
assignee: []
created_date: '2026-09-08 22:28'
updated_date: '2026-09-18 10:58'
labels: []
dependencies:
  - TASK-2
references:
  - docs/multi-agent-chat-design.md
ordinal: 3000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/multi-agent-chat-design.md §5 のルートを追加する。chat 作成の kind 受け付け、PATCH /api/chats/{chatId} の turnRule / scenePrompt 拡張、participants の CRUD 4 ルート、POST /api/chats/{chatId}/turns/stream (SSE)。kind='multi_agent' の chat では既存 messages ルートは保存のみ行い生成しない。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 curl だけで多人数会話の作成〜参加者登録〜ターン実行 (SSE で発言が流れる) ができる (Phase A 完了条件)
- [x] #2 kind='multi_agent' への POST /messages と POST /messages/stream がいずれも生成を行わずユーザー発言として保存され、kind='assistant' の既存挙動は変わらない (設計 §4.4)
- [x] #3 当該 chat のターンが実行中に POST /turns/stream が重なると 409 を返す
- [x] #4 manual 指名に他 chat の participantId を渡すと 404 になる。PATCH / DELETE /api/participants/{id} は存在しない ID のみ 404 (設計 §3 末尾・§5)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. server.go: participants repository と TurnEngine を Server に配線し、§5 の 5 ルートを mux に登録する
2. handlers.go: POST /projects/{id}/chats で kind を受け付ける。PATCH /api/chats/{chatId} を title / turnRule / scenePrompt の部分更新に拡張する (キーが無い項目は据え置き)
3. handlers.go: POST /messages と /messages/stream で chat.kind='multi_agent' なら ChatService を呼ばず user 発言の保存だけ行う (§4.4)。ChatService 自体は無改修
4. multiagent.go (新規): participants CRUD 4 ルートと POST /turns/stream。SSE ライタは最初の delta で遅延生成し、409/404/400 を本物のステータスコードで返せるようにする
5. multiagent_test.go: 偽 LM Studio エンドポイントを立てて SSE 疎通・409 競合・404 境界・kind 別の /messages 挙動を検証
6. go build / go vet / go test ./... と、ローカル HTTP サーバに対する実 curl で AC #1 を確認
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## HTTP 層の実装 (design §5)

- `internal/httpapi/multiagent.go` を新設し、participants CRUD 4 ルートと `POST /api/chats/{chatId}/turns/stream` を置いた。handlers.go は 932 行あり、既存ファイルをさらに膨らませるより多人数会話の面を 1 ファイルにまとめるほうが読みやすいと判断した。
- participants の読み書きは repository を直接呼ぶ (memories / documents ルートと同じ形)。service を挟むのはターンだけで、そこにしかターン規則・接続確認・排他が無いため。
- `server.go` に ParticipantRepository と TurnEngine を配線。`PATCH /api/chats/{chatId}` のハンドラは `handleUpdateChatTitle` → `handleUpdateChat` に改名し、title / turnRule / scenePrompt の部分更新に拡張した。**キーの有無**で更新対象を決めるので、scenePrompt だけ送っても title は消えない (従来は body に title が無いと空文字で上書きされていた。フロントは常に title を送るので挙動変化は無い)。

## SSE とステータスコードの両立 (AC #3 の要)

`NewSSEWriter` はヘッダを書いた時点で 200 が確定するため、先に開くと 409 / 404 を本物のステータスで返せなくなる。ターンが拒否される経路 (実行中 409・不明/他 chat の参加者 404・規則違反 400) はすべてモデルが 1 文字も吐く前に起きるので、**最初の delta が来た時点で SSE ライタを遅延生成**する形にした。`CreateChatCompletionStream` の onDelta は読み取りループと同じ goroutine から同期的に呼ばれるので、この遅延生成に競合は無い。delta が 1 つも来なかった正常ターン (空補完) 用に、done 送信前のフォールバック生成も置いた。

エラー対応表 (`turnErrorResponse`): ErrTurnInProgress→409 / ErrChatNotFound・ErrParticipantNotInChat→404 / ErrNotMultiAgentChat・ErrRosterEmpty・ErrManualParticipantRequired・ErrParticipantNotNameable・ErrParticipantRemoved→400 / ErrEndpointUnavailable→502 (参加者側エンドポイントの障害でリクエストの不備ではないため) / その他→500。sentinel を `errors.Is` で判定しメッセージ文字列には依存しない。

## 設計判断 (design に明記が無く、着手時に決めた点)

- `GET /participants` は除籍済みを含む全行 (deletedAt 付き) を返す。編成パネルは `deletedAt === null` で絞れ、観戦ビューは過去発言の発言者名を解決できる。2 つの読み手に 1 つの形で足りる。
- 参加者作成の必須項目は displayName のみ。baseUrl / modelName が空なら `resolveTarget` がワークスペース既定のエンドポイントへ落ちるので、接続先を決める前に枠だけ作れる。
- kind は作成時固定 (§5 に更新ルートが無い) なので、assistant chat への participants 追加と turnRule / scenePrompt の更新は 400 で拒否する。読まれることのない行・設定を溜めないため。
- `presetId` は受け付けていない。プリセットの保存形式は設計 §8 で未決、同梱は TASK-5 の範囲。

## 検証

- `go build ./...` / `go vet ./...` / `go test ./...` すべて成功。
- 新規テスト `internal/httpapi/multiagent_test.go` 6 本:
  - AC #1: kind=multi_agent 作成 → 参加者 2 名 → turns/stream で delta + done、発言が participantId 付きで保存され、2 ターン目は round_robin で次の参加者になる。
  - AC #2: multi_agent への `/messages` は role=user 1 件のみ保存・生成なし、`/messages/stream` は delta 無しで done のみ。assistant chat は従来どおり assistant 応答 (既存の TestSendMessageFallback / TestSendMessageStream も無変更で通過)。
  - AC #3: 偽 LM Studio を補完の途中で止め、重なったリクエストが JSON の 409 を返すこと、拒否されたターンが 1 件も発言していないこと、解放後に次ターンが通ることを確認。
  - AC #4: manual で他 chat の participantId → 404、存在しない ID → 404、manual で無指名 → 400。`PATCH` / `DELETE /api/participants/{id}` は他 chat の参加者でも 200、存在しない ID のみ 404。
- 実 curl による Phase A 完了条件の確認 (一時ハーネスで API + スタブ LM Studio を起動し、確認後に削除): project 作成 → multi_agent chat 作成 → scenePrompt 更新 → 参加者 2 名登録 → turns/stream ×2 で SSE が流れ、transcript に qwen-local / gpt-cloud の 2 発言が participantId 付きで並ぶ → `/messages` 介入が role=user で 201 → 他 chat 指名 404 / 存在しない participant の PATCH・DELETE 404 / 自 chat の DELETE 200 まで一通り通ることを確認した。

## 積み残し

- 409 の curl 実測はブロックするエンドポイントが要るためテスト側のみで確認 (curl では未実施)。
- `internal/search/model.go` が gofmt 未整形 (本タスクの変更範囲外。既存のまま放置した)。
<!-- SECTION:NOTES:END -->
