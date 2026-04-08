# SNZ Studio

個人用途向けのローカル LLM プロジェクト管理ツールの最小実装です。  
ChatGPT / Claude の Project に近い体験を、React + Vite、TypeScript API、SQLite、ローカル filesystem だけで構成しています。

## できること

- Project の作成、一覧、詳細表示
- Project ごとの `documents`、`chats`、`memories` 管理
- `markdown` / `text` / `image` document の登録
- document category の自動推定と手動変更
- SQLite FTS ベースの document / memory 検索
- chat ごとの summary 保存
- persistent memory の最小実装
- assistant 返答ごとに参照した `project` / `summary` / `document` / `memory` を UI で確認
- OpenAI 互換 API への接続
  - LM Studio
  - Ollama の OpenAI 互換 endpoint
  - その他互換サーバ

## 構成

```text
backend/
  src/
    db/              SQLite 接続、schema、seed
    repositories/    persistence layer
    services/        retrieval / context / llm / summary / memory
    storage/         file upload
    index.ts         API server
frontend/
  src/
    api/             HTTP client
    components/      shell
    pages/           画面
    styles/          emotion styles
```

レイヤは以下の粒度に留めています。

- UI layer: `frontend/src/pages`
- API layer: `backend/src/index.ts`
- domain / service layer: `backend/src/services`
- persistence layer: `backend/src/repositories`
- retrieval layer: `backend/src/services/retrievalService.ts`
- llm integration layer: `backend/src/services/llmClient.ts`

## データモデル

SQLite には最低限以下を持たせています。

- `projects`
- `documents`
- `document_chunks`
- `document_chunks_fts`
- `chats`
- `messages`
- `chat_summaries`
- `memories`
- `memories_fts`
- `assistant_message_references`

## 起動手順

1. 依存をインストール

```bash
pnpm install
```

2. 環境変数を作成

```bash
cp .env.example .env
```

3. サンプルデータを投入

```bash
pnpm seed
```

4. 開発サーバを起動

```bash
pnpm dev
```

5. ブラウザで開く

- Frontend: [http://127.0.0.1:5173](http://127.0.0.1:5173)
- API: [http://127.0.0.1:8787/api/health](http://127.0.0.1:8787/api/health)

## LLM 接続

OpenAI 互換 API を前提にしています。`.env` の主な設定は以下です。

- `LLM_BASE_URL`
- `LLM_MODEL`
- `REVIEW_BASE_URL`
- `REVIEW_MODEL`
- `LLM_API_KEY`
- `LLM_TIMEOUT_MS`
- `EMBEDDING_BASE_URL`
- `EMBEDDING_MODEL`
- `EMBEDDING_API_KEY`
- `EMBEDDING_TIMEOUT_MS`

例:

- LM Studio: `LLM_BASE_URL=http://127.0.0.1:1234/v1`
- Ollama OpenAI 互換 endpoint: その URL に差し替え

embedding を使う場合は `EMBEDDING_MODEL` を設定してください。未設定なら retrieval は FTS のみで動作します。設定されていれば、document / memory の retrieval は `FTS + embedding rerank` の hybrid になります。

Dashboard の `Configuration` から接続先、モデル、`LLM Response Format`、review 用 endpoint / model は更新できます。UI から保存した値は `data/app-config.json` に保存され、`.env` より優先して即時反映されます。`llm-jp-4-8b-thinking` のような thinking 系モデルでは `LLM-jp Thinking` を選ぶと、内部の reasoning / tagged response を除去して final answer のみを表示します。

ローカル LLM が起動していない場合でも、アプリ自体は動作します。  
その場合 chat 返答は fallback 文面になり、どの参照が選ばれたかの確認に使えます。

## 実装方針

- ベクトル DB なし
- retrieval は SQLite FTS を基本とし、任意で OpenAI 互換 embeddings による hybrid rerank を追加
- document は `world / character / rule / plot / timeline / index / story / misc` に分類され、retrieval の優先度調整に使う
- ベクトルは SQLite に JSON で保存し、外部ベクトル DB は使わない
- 過去 chat は毎回全文を渡さず、`chat_summaries` と recent messages を中心に扱う
- memory は durable fact だけを保存する前提
- 初期実装では memory 抽出は軽量な rule-based heuristic

## API の要点

- `GET /api/projects`
- `POST /api/projects`
- `GET /api/projects/:projectId`
- `POST /api/projects/:projectId/chats`
- `POST /api/projects/:projectId/memories`
- `POST /api/projects/:projectId/documents`
- `GET /api/chats/:chatId`
- `POST /api/chats/:chatId/messages`

## 今後の拡張ポイント

- hybrid retrieval の重み調整と semantic-only fallback の改善
- chat summary 更新を LLM ベースに切り替え
- memory 抽出を LLM ベースに切り替え
- document 編集 / 削除 UI
- assistant streaming
- rerank 層の追加
- image document の manual annotation UX 改善

## 注意

- 認証なし、単一ユーザー前提です
- PDF / OCR / vector search / Electron は含みません
- 開発速度と読みやすさを優先した最小構成です
