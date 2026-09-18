---
id: TASK-2
title: '多人数会話: ターンエンジン service'
status: In Review
assignee: []
created_date: '2026-09-08 22:28'
updated_date: '2026-09-18 06:56'
labels: []
dependencies:
  - TASK-1
references:
  - docs/multi-agent-chat-design.md
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/multi-agent-chat-design.md §4 のターンエンジンを internal/service/turnengine.go として実装する。1 リクエスト = 1 ターン。ターン進行ルール round_robin / manual、場面設定 + 役割プロンプト + 役割リマインドの system 組み立て、履歴の発言者視点への写像 (自分= assistant / 他者= user に「表示名: 本文」)、CompletionTarget による参加者ごとの接続先指定、EnsureModelLoaded / CheckConnection による事前確認、participant_id つき保存まで。既存 service/chat.go には手を入れない。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 round_robin で直近発言から次の参加者が決まる (サーバー側に進行状態を持たない)
- [x] #2 manual で指名した参加者がターンを実行する
- [x] #3 プロンプト写像と発言保存のユニットテストがある
- [x] #4 chat 単位のターン実行権 (in-process 排他) により、重なったターン要求が同じ参加者を二重に発言させない。並行要求のテストがある (設計 §4.2 手順 0)
- [x] #5 直近発言の参加者が除籍済みでも round_robin が編成の先頭から次の発言者を決められる
- [x] #6 manual 指名で当該 chat に属さない participantId を渡すとターンを実行せずエラーになる (設計 §3 末尾)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. internal/service/turnengine.go を新設。TurnEngine{chats, participants, llm, cfg} + chat 単位の in-process ロック (sync.Mutex で守る map[string]bool)。service/chat.go には触らない (設計 §4.4)。
2. sentinel error を turnengine.go に定義し、HTTP のステータス対応 (TASK-3) を errors.Is で決められる形にする: ErrTurnInProgress (409)、ErrParticipantNotInChat (404)、ErrNotMultiAgentChat、ErrRosterEmpty、ErrManualParticipantRequired、ErrParticipantRemoved、ErrEndpointUnavailable。既存の repository.ErrProjectReorderMismatch と同じ流儀。
3. RunTurn(chatID, participantID, onDelta) を設計 §4.2 の手順どおりに実装: (0) ターン実行権取得 → (1) 発言者決定 → (2) 接続確認 → (3) プロンプト組み立て → (4) CompletionTarget つきストリーム → (5) participant_id つき保存。
4. 発言者決定: round_robin は ListMessages から participant_id を持つ直近発言を探し、ListRoster の順で次。直近発言者が編成にいなければ (除籍) 編成の先頭 (AC #1/#5)。manual は participantId 必須・GetParticipant の chat_id 一致を検証・除籍済みは拒否 (AC #2/#6)。
5. プロンプト写像 (§4.3): system = 場面設定 + 役割プロンプト + 役割リマインド。履歴は自分= assistant (素の本文)、他者・人間= user (「表示名: 本文」)。表示名は ListAll から解決 (除籍済みも引けるため)。LLMClient は無改修なので buildMessages が必ず付ける末尾 user メッセージにはターン指示文を入れる。
6. 保存はストリーム完走後に 1 回 AddMessage (chat.go の空メッセージ先出し方式は採らない — 生成失敗時に空行が履歴に残るため)。
7. turnengine_test.go: /models と /chat/completions を返す httptest サーバで、round_robin の巡回・manual・除籍フォールバック・他 chat 指名エラー・プロンプト写像・保存内容・並行 2 要求で片方が ErrTurnInProgress になること (AC #3/#4) を検証。
8. go build / go vet / go test を通す。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装 (internal/service/turnengine.go)
`TurnEngine.RunTurn(chatID, participantID, onDelta)` が設計 §4.2 の手順 0〜5 をそのまま実行する。`service/chat.go` は未変更 (設計 §4.4)、`LLMClient` も無改修 (§1) で、参加者ごとの接続先は `CompletionTarget{BaseURL, Model}` で渡している。

- **ターン実行権 (手順 0)**: `sync.Mutex` で守る `map[string]bool` の in-process ロック。取れなければ `ErrTurnInProgress`。DB 側の予約列は持たない (単一ユーザー・単一プロセスのため、設計 §4.2 の Why どおり)。
- **発言者決定 (手順 1)**: `round_robin` は `ListMessages` から `participant_id` を持つ直近発言を探し、`ListRoster` 順の次を選ぶ。サーバー側に進行状態を持たないので、プロセス再起動後も履歴だけで巡回が続く。直近発言者が編成にいなければ (除籍) 編成の先頭に戻す。`manual` は `GetParticipant` の `chat_id` と引数 `chatID` を突き合わせて他 chat の指名を弾く (§3 の「比較対象が 2 つある」検証)。
- **エラーは sentinel** (`ErrTurnInProgress` / `ErrParticipantNotInChat` / `ErrNotMultiAgentChat` / `ErrChatNotFound` / `ErrRosterEmpty` / `ErrManualParticipantRequired` / `ErrParticipantNotNameable` / `ErrParticipantRemoved` / `ErrEndpointUnavailable`)。既存 `repository.ErrProjectReorderMismatch` と同じ流儀で、TASK-3 の HTTP 層が `errors.Is` でステータス (409 / 404) に落とせる。

## 設計に無い判断とその理由
- **接続確認 (手順 2) は `EnsureModelLoaded` → だめなら `CheckConnection` の順**。`EnsureModelLoaded` は LM Studio 固有の `/api/v1` を叩くので、素の OpenAI 互換エンドポイントでは必ず false になる。両方失敗のときだけ `ErrEndpointUnavailable` とすることで「LM Studio でない」ことを理由に弾かない。
- **保存はストリーム完走後に 1 回** (`AddMessage`)。`chat.go` の「空 assistant メッセージを先に作って delta で埋める」方式は採っていない — 生成に失敗すると空の参加者発言が履歴に残り、`round_robin` がそれを「その参加者は発言済み」と読んでしまうため。切断してもターンが完走して保存される設計 §4.1 の契約は、完走後保存でも変わらない (`CreateChatCompletionStream` が呼び出し元 context に縛られないため)。
- **生成失敗時のフォールバック本文を書かない**。`chat.go` はエンドポイント不達時にフォールバック文を保存するが、ターンエンジンでは「起きなかったターン」を履歴に入れないことを優先した (入れると巡回が発言していない参加者を飛ばす)。
- **`round_robin` の chat に `participantId` が来たら `ErrParticipantNotNameable`**。黙って無視すると UI 上は指名できたように見えて別人が発言するため、不一致として返す。
- **除籍済み参加者の `manual` 指名は `ErrParticipantRemoved`**。repository は除籍済みも引けるが、編成外に発言させる意味がない。
- **プロンプトの末尾 user メッセージ**: `LLMClient.buildMessages` は必ず `UserInput` を user として付けるため、ターンには新しい人間入力が無いこのスロットに「（進行）次は「X」の番です」というターン指示を入れた。`LLMClient` 無改修の制約 (§1) の下で空 user メッセージを送らないための選択。
- **system に編成の表示名一覧を 1 行入れた** (設計に明記は無い)。誰が同席しているかが分からないと役割リマインドだけでは他者への言及が成立しないため。
- 履歴上限は定数 `turnHistoryLimit = 20`。要約による圧縮は §8 未決なので設定値にしていない。
- 空本文のメッセージは写像から除外 (中断された行が「発言」として相手に見えないようにするため)。

## 検証 (2026-09-18)
- `go build ./...` / `go vet ./...` / `go test ./...` すべてグリーン (全 14 パッケージ ok)。
- AC #1: `TestTurnEngineRoundRobinCycle` — 3 人で 4 ターン回し Alice→Bob→Carol→Alice を確認。各ターンの `model` が発言者のモデルであること (= 接続先の上書きが効いていること) も検証。
- AC #2: `TestTurnEngineManualNomination` — `manual` で編成 3 番目の Carol を指名して Carol が発言。`participantId` 無しは `ErrManualParticipantRequired`。
- AC #3: `TestTurnEnginePromptMapping` — system に場面設定・役割プロンプト・役割リマインド・参加者一覧が入ること、履歴が「人間= `ユーザー: 本文`」「他参加者= `Bob: 本文`」「自分= assistant の素の本文」に写像されること、空本文が落ちること、末尾がターン指示であること。保存側 (`participant_id` / `role` / `model_name` / 生成メトリクス) は `TestTurnEngineRoundRobinCycle` で検証。
- AC #4: `TestTurnEngineConcurrentTurns` — 1 本目を completion の途中で止めた状態で 2 本目を投げ、`ErrTurnInProgress` になること、保存されたメッセージが 1 件だけであること、解放後は次の参加者 (Bob) に進むことを確認。
- AC #5: `TestTurnEngineRoundRobinAfterRemoval` — 直近発言者 Bob を除籍した状態で編成の先頭 Alice が選ばれること、かつ除籍済み Bob の過去発言が履歴写像で `Bob:` と名前解決されること。
- AC #6: `TestTurnEngineRejectsForeignParticipant` — 他 chat の participantId・存在しない ID は `ErrParticipantNotInChat`、除籍済みは `ErrParticipantRemoved`。いずれもメッセージが保存されず、エンドポイントも呼ばれないことを確認。
- 付随: `TestTurnEngineRejectsNonMultiAgentChat` — `kind='assistant'` の chat・存在しない chat・編成が空・`round_robin` での指名。

## 未測定・申し送り
- 実 LM Studio に対する動作確認は未実施 (テストは httptest のフェイクエンドポイント)。`EnsureModelLoaded` が実機で true を返す経路は TASK-3 以降の手動確認に回す。
- HTTP 配線 (`POST /api/chats/{chatId}/turns/stream`、409 / 404 への写像、server.go への TurnEngine 登録) は TASK-3 の範囲なので未着手。現状 `TurnEngine` はどこからも生成されていない。
- 役割リマインドの文面と履歴上限 20 は暫定値。実際の会話品質でのチューニングは TASK-5 の範囲。
<!-- SECTION:NOTES:END -->
