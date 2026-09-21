---
id: TASK-19
title: '多人数会話: 人間が明示的にメモリを保存できるようにし、一時チャットのフラグを効かせる'
status: In Review
assignee: []
created_date: '2026-09-19 23:58'
updated_date: '2026-09-21 06:50'
labels: []
milestone: m-1
dependencies:
  - TASK-18
  - TASK-17
references:
  - internal/service/memory.go
  - internal/httpapi/multiagent.go
  - internal/service/chat.go
  - frontend/src/pages/MultiAgentChatPage.tsx
  - frontend/src/pages/ProjectDetailPage.tsx
  - docs/current-spec.ja.md
  - docs/multi-agent-chat-design.md
ordinal: 19000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-18 で多人数会話がプロジェクトのドキュメント・メモリを読むようになるが、書き込みは無い。
単独チャットの自動抽出 (`memory.go` `MaybeStoreFromUserMessage`) と「覚えて」抽出
(`MaybeStoreFromExplicitRequest`) を多人数会話に持ち込む案は却下した。理由:

- 人間の介入発言もロールプレイになり得る。「I am the rightful king」は semantic の cue `i am` を通り、
  演出指示の「常に」は procedural として保存される。抽出器に虚構と事実の区別は無い。
- 「覚えて」抽出は直近メッセージを role でしか区別せず、フォールバックで最新の assistant メッセージ
  (= 参加者のセリフ) を優先して保存する (`memory.go` の Heuristic fallback)。人間の発言からだけ呼んでも
  参加者のセリフがメモリになる。

## 方針

多人数会話では自動抽出を行わず、人間が保存する事実を明示的に指定する経路だけを用意する:

- 多人数会話画面から、任意のメッセージ (参加者・人間どちらでも) を選んで
  「メモリに保存」できる。保存前に内容を編集でき、kind (semantic / procedural / episodic) を選べる。
  既定の内容は選んだメッセージ本文、既定の kind は既存の `inferKindFromText` で推定。
- 保存は既存の `MemoryRepository.CreateMemory` + 埋め込み同期 (`chat.go` の自動抽出後と同じ経路)
  を使い、`source` は既存の値体系に沿って多人数会話由来と分かるものにする (organizer の対象にはなる)。
- 会話の `isTemporary` が true のときは保存操作を無効化し、理由を表示する。

ファシリテーター参加者による自律的なメモリ書き込みは行わない (帰属と方針の問題に対して初期価値が
小さい)。参加者が「これは記録に値する」と提案する仕組みも今回は入れない。

## TASK-17 との関係

TASK-17 で無効化した作成フォームの「一時チャット」チェックボックスは、このタスクで意味を持つ
(読むが書かない) ようになるので、多人数会話でも再有効化し、説明文を単独チャットと同じ趣旨に戻す。
`docs/current-spec.ja.md` §4.4 に多人数会話での一時チャットの意味を追記する。

## 設計書への記録

TASK-18 が改訂する `docs/multi-agent-chat-design.md` §4.4 に、読む方向 (ドキュメント・メモリ → 会話) に加えて
書く方向 (会話 → 人間が選んだ発言 → プロジェクトのメモリ → 後続のチャット・多人数会話) を追記し、
自動抽出を行わない理由 (上の 2 点) をそこに残す。複数の発言にまたがる結論を要約して保存の下書きにする経路は
別タスク (本タスクに依存) で扱う。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 多人数会話画面から任意のメッセージを選び、内容と kind を編集した上でプロジェクトのメモリとして保存できる
- [x] #2 保存されたメモリは既存の埋め込み同期と organizer の対象になり、多人数会話由来であることが source から分かる
- [x] #3 多人数会話でも自動抽出・「覚えて」抽出は動かない (参加者・人間の発言いずれからも自動保存されない)
- [ ] #4 isTemporary な多人数会話では保存操作が無効化され、理由が表示される
- [x] #5 作成フォームの「一時チャット」チェックボックスが多人数会話でも有効に戻り、説明文と docs/current-spec.ja.md §4.4 が更新されている
- [x] #6 docs/multi-agent-chat-design.md §4.4 に、会話からメモリへ書く経路と自動抽出を行わない理由が記録されている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Go: service に InferMemoryKind (inferKindFromText の公開関数) を足す。
2. Go httpapi: GET /api/messages/{messageId}/memory-draft (保存の下書き: 発言本文 + 推定 kind) と POST /api/messages/{messageId}/memory (発言のメモリ保存: content/kind/locked を受け、source=multi_agent, sourceChatId=会話 id で CreateMemory → SyncMemories)。単独 assistant の発言は 400、一時チャットは 409、無い発言は 404。
3. Go test: 保存ルートの正常系/拒否系、multi_agent 会話で「覚えて」+ durable cue の人間発言と cue 入りの参加者発言から memory が生えないこと (AC #3)。
4. frontend: client.ts に MemorySource へ multi_agent 追加・2 API 追加。MultiAgentChatPage に発言ごとの「メモリに保存」ボタンと保存ダイアログ (kind select / content textarea / locked)、一時チャット時はボタン無効 + 理由表示。ヘッダに ⏱️。
5. frontend: ProjectDetailPage の作成フォームで一時チャットのチェックボックスを多人数会話でも有効に戻し、説明文を単独チャットと同趣旨の 1 文に置換 (両 kind で表示)。
6. docs: multi-agent-chat-design.md §4.4 に書く方向と自動抽出を行わない理由、§5 に 2 ルート。current-spec.ja.md §4.4 に多人数会話での一時チャットの意味、§4.5 source に multi_agent、§11.6 に保存 UI。README API 一覧に 1 行。
7. go test ./... と pnpm check:client。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装ノート (2026-09-21)

### 経路の形
- 発言のメモリ保存は `GET /api/messages/{messageId}/memory-draft` (下書き: 発言本文 + `inferKindFromText` の推定 kind) と `POST /api/messages/{messageId}/memory` (保存) の 2 ルート (`internal/httpapi/multiagent_memory.go`)。review ルートと同じくフラットな message id に掛ける。
- 下書きを別ルートにしたのは、既定 kind の推定規則 (cue 表) をサーバに 1 か所だけ置き、フロントへ複製しないため。`POST` で `kind` 省略時も同じ推定に落ちる。
- 保存は `MemoryRepository.CreateMemory` + `EmbeddingSyncService.SyncMemories` という手動追加と同じ経路。`source = multi_agent` (chat kind と同じ綴り)、`source_chat_id` = 会話 id、`locked` 既定 `true` (文言を人間が決めたので organizer に書き換えさせない)。
- 拒否: 発言なし 404 / 単独 assistant の発言 400 / 一時チャット 409 (リクエストは正しく chat の状態だけが拒む、プリセット適用と同じ扱い)。下書きも同じ条件で拒否し、保存できない下書きを出さない。
- 自動抽出は追加していない。ターンエンジンと `storeHumanMessage` はもともと `MemoryService` を呼ばず、今回はそれをテストで固定した。

### 測定・検証
- `go vet ./...` / `go test ./...` 全パス。新規テスト `internal/httpapi/multiagent_memory_test.go`:
  - `TestMultiAgentSaveMessageMemory` (AC #1 サーバ側・#2): 下書きが本文と推定 kind (semantic) を返す、編集した content/kind で保存される、`source = multi_agent` と `sourceChatId` が会話 id、kind 省略で推定 (episodic)、locked 省略で true、プロジェクト詳細の memories に 2 件並ぶ。
  - `TestMultiAgentSaveMessageMemoryRefusals` (AC #4 サーバ側): 一時チャットで下書き・保存とも 409、フラグを戻すと同じ body が 201、単独 assistant の発言は 400。
  - `TestMultiAgentNeverExtractsMemories` (AC #3): 「覚えて」「メモリに保存して」+ durable cue を含む人間の介入 (非 stream / stream) と、semantic・procedural cue を含む参加者発言のターンの後でプロジェクトのメモリが 0 件。対照として同じ文を単独 assistant chat に送るとメモリが生える。
- `pnpm check:client` (tsc) と `pnpm build:client` (vite) 通過。
- organizer は `ListByProject` の全メモリを payload に入れ、`locked` のものは update/remove しないだけなので、`multi_agent` も分析対象。埋め込みはテスト環境では `EmbeddingModel` 空で no-op (単独 chat と同じ)。

### 目視に残したもの (AC #1 / #4 の画面側)
- SPA は Wails 束縛 (`GetApiToken`) で API トークンを受け取るため、Go サーバ + vite 単体では起動できず、画面操作は確認していない。確認してほしい点: 発言ごとの「メモリに保存」ボタン → ダイアログ (kind / 内容 / ロック) → 保存後に composer 上部へ「保存しました: {title}」が 5 秒出る。一時チャットの多人数会話ではボタンが無効になり、composer の注記に理由が出る。ヘッダのタイトルに ⏱️ が付く。
- 作成フォームの説明文は単独チャット・多人数会話の両方で `project.temporaryChatNote` を常時表示に変えた (TASK-17 の多人数会話専用の注記を置換)。単独チャット側にも 1 行増える。

### 範囲外にしたこと
- 多人数会話画面から一時チャットのフラグを切り替える UI (単独 chat の設定にはある)。今回は作成時のみ指定できる。必要なら編成パネルの「会話設定」に 1 チェックボックスで足せる (`PATCH /api/chats/{id}/temporary` は既存)。
<!-- SECTION:NOTES:END -->
