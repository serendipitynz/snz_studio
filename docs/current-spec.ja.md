# SNZ Studio 現在仕様

最終更新: 2026-09-19

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
- kind: `assistant`（単独 assistant）/ `multi_agent`（多人数会話、4.6）

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

多人数会話（§4.6）では自動抽出も `覚えて` トリガーも元々動きません。多人数会話の一時チャットは、
プロジェクトのドキュメント・メモリを背景資料として読みますが、発言のメモリ保存（§6.5）を受け付けません
（画面では保存ボタンが無効になり理由が表示され、API は 409 を返します）。作成フォームの「一時チャット」は
どちらの種別でも指定できます。

### 4.5 Memories

memory は raw message ではなく、durable な前提の保管場所です。

kind:

- `procedural`
- `semantic`
- `episodic`

metadata:

- `source`: `manual` / `chat` / `organized` / `multi_agent`（多人数会話の発言から人間が保存したもの、§6.5）
- `locked`: organizer からの update/remove を防ぐ

意味:

- `procedural`: どう振る舞うか
- `semantic`: 安定した事実
- `episodic`: 過去の決定や出来事

### 4.6 多人数会話

多人数会話は `kind = multi_agent` の chat です。2 名以上の参加者が、それぞれの接続先とモデルを通じて
順番に発言します。project / document / chat / memory と並ぶ別のドメインではなく chat の種別であり、
同じ `chats` テーブル・同じ project 配下に置きます。

多人数会話は次を持ちます。

- 参加者: 表示名・役割プロンプト・接続先・モデル・編成順・背景資料を渡すか（既定は渡す）
- ターン進行ルール: `round_robin`（編成順の循環）/ `manual`（発言者の指名）
- 場面設定: 全参加者のシステムプロンプトに前置される論題・シーン・世界観

**編成**とは、除籍されていない参加者の集合を指します（`round_robin` の巡回対象・編成パネルの表示対象）。
除籍は論理削除なので行は残り、過去の発言は表示名を保ち、巡回の起点も失われません。

**プリセット**は、参加者・ターン進行ルール・場面設定を一度で埋める雛形です。7 件をアプリに
同梱し、それ以外は同じ形式の JSON ファイルを読み込んで適用します（`presets/multi-agent/` に 17 件）。
適用できるのは新規作成時と、発言がまだ 1 件も無い多人数会話に対して（編成パネルから）です。後者は既存の
編成・ターン進行ルール・場面設定を置き換えます。発言が 1 件でもある会話には適用できません（transcript が
既に居ない話者を指すため）。適用後の chat はプリセットとの結びつきを持たず、以後の編集はすべて編成パネルから行います。

ユーザーは参加者ではありません。人間の発言は `participant_id` を持たない `user` message として保存され、
ターンを消費せずに任意の時点で会話へ介入できます（論題の追加投入・野次など）。

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

### 5.6 多人数会話のターン（単独 assistant フローとは別系統）

1 リクエストが 1 ターンを実行します。サーバー側に常駐の進行ジョブは持たず、自動進行はフロントエンドが
次のターンを呼び続けることで実現します。

ターンの処理順:

1. その chat のターン実行権を取る（重なった 2 本目は 409）
2. 発言者を決める（`round_robin`: `participant_id` を持つ直近メッセージの次の編成順 / `manual`: 指名された参加者）
3. 参加者の接続先を確認し、モデルのロードを確認する
4. 発言者本人の視点でプロンプトを組む。system = 背景資料（プロジェクト説明・検索で得たドキュメント抜粋・
   関連メモリ。後述）+ 場面設定 + 役割プロンプト + 役割リマインド。履歴は自分の
   過去発言を `assistant`、他の参加者と人間の発言を `user`（「表示名: 本文」の形）へ写像し、末尾はその参加者が
   答えるべき直前の発言にする
5. 発言を stream し、`participant_id` を持つ `assistant` message として、背景資料の元になった参照と
   1 トランザクションで保存する

現在の割り切り:

- 履歴は直近 30 発言に切り詰める。要約による圧縮は行わない
- 進行中のターンは中断しない。クライアントが切断してもモデル生成は完走して保存されるので、自動進行の停止が
  効くのはターン境界だけ
- 切断後の復帰は保存済み message の読み直し。取りこぼした delta の再送は行わない

このフローは単独 assistant のフローと分離されており、共有するのは LLM クライアント・永続化層・retrieval です。
chat summary と、単独 assistant のプロンプト文脈の残り（プロジェクトのシステムプロンプト・手続きメモリの無条件注入・
引用モード・他チャット参照）は使いません。背景資料はターンごとに 1 回、最新発言 + 直前 2 発言 + 場面設定の先頭
（開幕ターンは場面設定のみ）をクエリにして検索し、ドキュメント 2 件 × 2 chunk・メモリ 3 件・総量 2,000 字を上限とします。
使用した参照は参加者の発言と一緒に保存され、単独 chat の画面と同じ形で発言の下に表示されます。検索が失敗しても
ターンは失敗せず、背景資料なしで続行します。背景資料を渡さない参加者が発言するターンでは、検索そのものを行わず、
system prompt に背景資料を置かず、参照も保存しません（TRPG の GM だけがシナリオを知る編成のため）。
既定は渡すので、隠し情報を作るには隠す相手を明示的に「渡さない」にします。`multi_agent` の chat に既存の message ルートを叩くと、生成は
行わず人間の発言の保存だけを行います。

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

### 6.5 多人数会話からの発言のメモリ保存

多人数会話では自動抽出（§6.2）と remember trigger（§6.3）を動かしません（参加者のセリフも人間の介入発言も
ロールプレイになり得るため。理由の詳細は `docs/multi-agent-chat-design.md` §4.4）。代わりに、
transcript の任意の発言（参加者・人間どちらでも）から人間が明示的に保存します。

- 発言の「メモリに保存」からダイアログを開く
- 初期値は発言本文と、本文から推定した kind（単独 chat の自動抽出と同じ規則）
- content と kind を編集し、lock の有無を選んで保存する
- title は自動生成、既定で `locked = true`
- `source = multi_agent`、`source_chat_id` は会話の id。埋め込み同期と organizer の対象になる
- 一時チャットでは保存できない（§4.4）

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

- new chat（chat 種別の選択。多人数会話では群ごとのプリセット選択と「プリセットの JSON を読み込む」）
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

### 11.6 多人数会話画面（観戦ビュー + 編成パネル）

中央:

- 発言ごとに発言者の表示名とモデル名を付けた transcript
- 進行中ターンの stream 表示
- 「1 ターン進める」/「自動進行の開始・停止」/（`manual` のとき）次の発言者の指名
- 人間として会話に発言する composer
- 発言ごとの「メモリに保存」（§6.5）。一時チャットでは無効になり、理由を composer の注記に出す

右:

- 編成パネル: 参加者の CRUD（接続先入力 + モデル選択 + 接続確認）、参加者ごとの「背景資料を渡す」、編成順、
  ターン進行ルール、場面設定、発言が 1 件も無いあいだはプリセットの適用（既定は折り畳み）

除籍済みの参加者は編成とは別に一覧します（過去の発言が残るため）。
自動進行の停止はターン境界で効くことを UI にも明示します。

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
- `participants`
- `schema_migrations`

`chats` は `kind` / `turn_rule` / `scene_prompt`、`messages` は `participant_id`
（user 発言と単独 assistant 発言では NULL）を持ちます。

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
- participant CRUD（除籍は論理削除）
- 多人数会話の 1 ターン実行（SSE）
- 同梱プリセットの一覧
- 発言の無い多人数会話へのプリセット適用

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
