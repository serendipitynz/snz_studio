# SNZ Studio 現在仕様

最終更新: 2026-05-09

この文書は、現在の SNZ Studio 実装（Wails v2 + Go バックエンド + React フロント のデスクトップアプリ）の
仕様を、動作上の責務と境界を優先してまとめたものです。実装コードの断片ではなく振る舞いを記述します。

## 1. 目的

SNZ Studio は、単一ユーザー向けのローカル LLM ワークスペースです。  
主な用途は次です。

- 小説執筆補助
- 翻訳補助
- 設定資料の検索と参照
- project 単位の継続的な chat

UX の狙いは、ChatGPT / Claude の Projects に近い体験を、軽量なローカル構成で実現することです。

## 2. 非機能要件

- 単一ユーザー
- ローカルマシンで完結
- 認証なし
- 外部インフラ不要
- SQLite 使用
- filesystem ベースの file storage
- document retrieval は FTS ベース
- embedding は任意
- ベクトル DB なし
- PDF / OCR なし
- Electron なし

## 3. 現在の技術構成

### Frontend

- React
- Vite
- TypeScript
- Emotion
- React Router

### Desktop shell

- Wails v2（Go コア + OS ネイティブ WebView）
- SPA は AssetServer で配信

### Backend

- Go
- 標準 `net/http`（loopback `127.0.0.1`、`/api`・`/files` を配信）
- `modernc.org/sqlite`（pure Go、FTS5 / bm25 同梱）
- multipart アップロードは `net/http`（`r.FormFile`）

### Storage

データディレクトリ配下に保存します。配布版は OS のユーザー設定ディレクトリ
（macOS `~/Library/Application Support/snz-studio` / Windows `%AppData%\snz-studio`）、
開発時は `./data`（cwd 相対）。

- SQLite database: `<dataDir>/app.sqlite`
- app config: `<dataDir>/app-config.json`
- upload files: `<dataDir>/uploads/`

### Process model

- OS ネイティブ WebView（Wails）上の SPA
- 同一プロセス内の loopback HTTP API サーバー（SSE 温存のため AssetServer ではなく `net/http`）
- OpenAI-compatible LLM endpoint
- optional OpenAI-compatible embedding endpoint

## 4. 主要ドメイン

### 4.1 Projects

project は最上位の作業単位です。  
各 project は次を持ちます。

- title
- description
- system prompt
- documents
- memories
- chats

特徴:

- 複数 chat で documents / memories を共有
- sort order を持つ
- 削除時は関連データを cascade delete

### 4.2 Documents

document type:

- `markdown`
- `text`
- `image`

各 document は少なくとも次を持ちます。

- title
- type
- category
- note
- tags
- derived text
- content text
- file path

#### Document category

現在の category:

- `world`
- `character`
- `rule`
- `plot`
- `timeline`
- `index`
- `story`
- `misc`

category は追加時に自動推定され、UI から手動変更できます。

### 4.3 Chats

chat は必ず 1 つの project に属します。  
各 chat は次を持ちます。

- title
- messages
- summary
- references history
- temporary flag

特徴:

- title は空文字で作成可能
- 空タイトルの chat は最初の assistant 応答後に自動命名
- 右ペイン inspector とは独立に chat 自体を複数保持可能

### 4.4 Temporary chats

chat には `is_temporary` があります。

temporary chat の意味:

- 草案
- 試し書き
- 没案
- 一時的な検討

temporary chat では:

- 自動 memory 抽出しない
- `覚えて` トリガーを無効
- memory organizer の分析対象にしない

ただし:

- messages は保存する
- summary は保存する
- 他 chat から明示参照できる

### 4.5 Memories

memory は raw message ではなく、durable な前提の保管場所です。

kind:

- `procedural`
- `semantic`
- `episodic`

metadata:

- `source`: `manual` / `chat` / `organized`
- `locked`: organizer からの update/remove を防ぐ

意味:

- `procedural`: どう振る舞うか
- `semantic`: 安定した事実
- `episodic`: 過去の決定や出来事

## 5. Chat 生成フロー

現在の assistant 応答生成は、概ね次の順で動きます。

1. chat / project を取得
2. context assembly
3. user message 保存
4. memory 自動抽出
5. assistant 生成
6. references 保存
7. summary 更新
8. 必要なら chat title 自動生成

### 5.1 Context assembly

context assembly は次を組み合わせます。

- project title / description / system prompt
- current chat summary
- procedural memories
- relevant documents
- relevant memories
- recent messages
- optional explicit chat reference

### 5.2 Cross-chat reference

user が明示的に別 chat title を含めると、その chat を参照できます。

例:

- `チャット「第3話・作業」の内容を確認して...`

現在は次を追加コンテキストとして使います。

- referenced chat summary
- referenced chat recent messages

### 5.3 Document retrieval

retrieval は hybrid 構成です。

- primary: SQLite FTS5
- optional: embedding rerank
- optional: semantic fallback

embedding が未設定または失敗した場合は、FTS のみで動きます。

### 5.4 Category-aware retrieval

document retrieval は query intent に応じて category weight を変えます。

例:

- 設定確認: `world`, `index`, `timeline` 優先
- 執筆: `rule`, `story`, `character` 優先
- プロット相談: `plot`, `timeline` 優先
- 翻訳: `rule`, `story`, `index` 寄り

### 5.5 Explicit document behavior

document title が user input に明示されると、その document を優先できます。

現在サポートしている動作:

- quote mode
- multiple relevant chunks
- conditional full document inclusion

## 6. Memory の生成と整理

### 6.1 手動追加

project detail の memory modal から追加します。

- kind を選ぶ
- content を入力する
- title は自動生成
- manual memory は既定で `locked = true`

### 6.2 自動抽出

各 user message 後に、rule-based に memory 化することがあります。

ただし現在は保守的です。

- question は除外
- 長文は除外
- 一時依頼は除外
- `してください` だけでは procedural memory にしない

### 6.3 明示的 remember trigger

次のような user 指示で memory 化を試みます。

- `覚えて`
- `メモリに保存して`
- `今後の前提にして`

このときは直前会話から 1 件だけ memory を抽出します。

### 6.4 Organizer

memory organizer は手動実行です。  
毎 turn 自動ではありません。

入力:

- current memories
- recent chat summaries

提案:

- create
- update
- remove

制約:

- `locked = true` は organizer が触らない
- ただしユーザーは手動で lock/unlock/delete できる

## 7. Streaming

assistant 生成は streaming 対応です。

### 7.1 現在の保存方式

stream 開始時に空の assistant message を DB に作成し、delta ごとに `messages.content` を更新します。  
そのため、chat 画面から移動しても backend 側の chat データは進行中の本文を保持します。

完了時には次を確定します。

- final content
- response time
- output tokens
- tokens per second
- model name
- references
- chat summary

### 7.2 UI 上の見え方

同一 chat を開き直すと、保存済みの途中本文が見える可能性があります。  
ただし stream 管理は backend job queue ではなく、現在の HTTP stream request に依存しています。

## 8. Review 機能

assistant message ごとに review を実行できます。

概念上の分担:

- generation model: 作者
- review model: 編集者

review では:

- 対象 assistant message
- project context
- retrieved references

を使って markdown のレビューを生成します。

review は streaming 表示対応です。

review 用設定:

- review endpoint
- review model

未設定時は LLM 設定へフォールバックします。

## 9. モデル・接続設定

workspace-wide configuration として次を持ちます。

- LLM endpoint
- LLM model
- LLM response format
- review endpoint
- review model
- embedding endpoint
- embedding model

保存先:

- `data/app-config.json`

`.env` は初期値であり、UI 保存後は app config が優先されます。

## 10. LLM response format

現在は次を区別します。

- `standard`
- `llm_jp_thinking`

`llm_jp_thinking` では tagged response や reasoning 部分を除去し、final answer だけを保存・表示します。

## 11. UI 構成

### 11.1 Dashboard

- Projects 一覧
- drag and drop reorder
- project 作成
- Configuration 表示 / 編集

### 11.2 Left sidebar

- home
- project list
- chat list
- quick create chat

project には chat count を表示します。  
temporary chat は `⏱️` prefix で表示します。

### 11.3 Project detail

中央:

- new chat
- documents card

右:

- system prompt
- memories
- chats

documents card:

- file picker
- drag and drop
- overwrite confirm
- modal preview
- markdown render
- category edit

### 11.4 Chat screen

中央:

- message list
- stream display
- auto-growing composer
- document insert select
- document add modal

右:

- context inspector
- collapse state persisted in localStorage

assistant footer:

- review button
- copy button
- timestamp
- metrics
- model name

### 11.5 Memory modal

- add memory
- lock/unlock
- delete
- organize

## 12. Database の主なテーブル

現在の主テーブル:

- `projects`
- `documents`
- `document_chunks`
- `document_chunks_fts`
- `document_chunk_embeddings`
- `chats`
- `messages`
- `chat_summaries`
- `memories`
- `memories_fts`
- `memory_embeddings`
- `assistant_message_references`
- `schema_migrations`

## 13. API の責務

主要 API 群:

- project CRUD
- document CRUD
- chat CRUD
- message send / stream
- memory CRUD
- memory organize
- configuration read/write
- review

API は local-only の browser client を前提としています。

## 14. ログと診断

debug flags:

- `DEBUG_CHAT_FLOW=1`
- `DEBUG_RETRIEVAL=1`

これにより、chat prepare / context assembly / retrieval / embedding の切り分けログを出せます。

## 15. Go 再実装・デスクトップ化で意識すべき境界

再実装時に維持すべき責務の境界は次です。

### UI 層

- project / chat / document / memory の操作
- stream 表示
- review 表示

### API / application 層

- request validation
- turn orchestration
- streaming relay
- summary / memory update orchestration

### persistence 層

- SQLite schema
- migrations
- message / summary / reference persistence

### retrieval 層

- FTS
- embedding rerank
- explicit document/chat handling
- category-aware weighting

### llm integration 層

- OpenAI-compatible chat
- stream parsing
- model-specific response cleanup

### desktop app 化で確定した方針

- backend は Wails アプリに内包（同一プロセス内の loopback `net/http` サーバー）
- config / upload / data path は OS のユーザー設定ディレクトリ配下に集約（dev は `./data`）
- stream lifecycle は現在の HTTP stream request に依存（app-level job queue は持たない）
- local LLM process control は app が持たない（外部の OpenAI-compatible endpoint に接続するだけ）

## 16. 現在の割り切り

- auth なし
- multi-user なし
- ACL なし
- PDF なし
- OCR なし
- image understanding なし
- external vector DB なし
- distributed job system なし

## 17. 現状の重要な設計意図

このアプリは、

- 原資料は documents
- durable 前提は memories
- 会話の流れは summaries

という三層で情報を分けています。

この分離を保つことが、Go 再実装でも最重要です。
