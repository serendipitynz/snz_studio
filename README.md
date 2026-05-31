# SNZ Studio

個人用途向けのローカル LLM プロジェクト管理ツールの最小実装です。  
ChatGPT / Claude の Project に近い体験を、**Wails v2（Go コア + OS ネイティブ WebView）+ React/Vite フロントエンド + SQLite + ローカル filesystem** だけで構成した、インストールして起動するだけのスタンドアロン・デスクトップアプリです。

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

Go バックエンドは `internal/` 配下にレイヤ分割されています。フロントは React のまま、ローカル
`127.0.0.1` の Go `net/http` サーバー（`/api`・`/files`）に絶対 URL で直接アクセスします（SSE 温存のため
Wails AssetServer 経由ではなくローカルサーバーを使用）。

```text
main.go                Wails 起動 + SPA を embed
app.go                 App ライフサイクル / ローカル API サーバー起動 / GetApiBase バインド
internal/
  bootstrap/           データパス解決・初回データ移行・dev/prod 環境切替
  config/              app-config.json + env 既定値
  db/                  SQLite 接続・schema・9 migrations
  repository/          project / document / memory / chat の永続化層
  search/              日本語トークナイザ移植 + FTS クエリ生成
  vector/              cosine 類似度
  service/             retrieval / context / llm / embedding / summary / memory / review
  httpapi/             22 ルートのハンドラ + SSE
frontend/
  src/
    api/               HTTP client
    components/        shell
    pages/             画面
    styles/            emotion styles
    wailsjs/           生成バインド（GetApiBase）
```

レイヤは以下の粒度に留めています。

- UI layer: `frontend/src/pages`
- API layer: `internal/httpapi`
- domain / service layer: `internal/service`
- persistence layer: `internal/repository`
- retrieval layer: `internal/service/retrieval.go`
- llm integration layer: `internal/service/llmclient.go`

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

## 必要なツール

- Go 1.26+
- Node 22 / pnpm（フロントのビルドに使用。`wails` が自動で実行します）
- [Wails CLI v2](https://wails.io/)（`go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`）

## 開発（wails dev）

リポジトリ直下で次を実行します。Go API（`127.0.0.1:8787`）とフロント（Vite）が起動し、OS ネイティブ
WebView 上に SPA が表示されます。

```bash
wails dev
```

- 開発時のデータは `./data`（cwd 相対）に作成されます。`.env`（任意・`cp .env.example .env`）で
  `LLM_BASE_URL` などの既定値を上書きできますが、通常は UI の `Configuration` から設定します。
- ブラウザ直開き（`localhost:5173`）での開発は廃止しました（API への非 GET が届かないため）。開発は
  `wails dev` を使ってください。

## ビルド（配布物）

ビルドには 2 段階があり、用途で使い分けます。

### 1. 動作確認用（素のビルド）

```bash
wails build                              # 現在の OS 向け
wails build -platform darwin/universal   # macOS universal（.app）
wails build -platform windows/amd64 -nsis -webview2 download
```

成果物は `build/bin/`（macOS は `SNZ Studio.app`、Windows は `.exe`）に出力されます。

> **注意**: `wails build` 単体では内蔵 embedding（llama.cpp サイドカー + モデル GGUF）は同梱されません。
> この `.app` を起動すると内蔵 embedding は `llama-server binary not found` になります（外部 embedding か
> FTS のみで動作）。内蔵 embedding を含む配布物は次の署名ビルドで作ります。

### 2. 配布用（macOS・署名 + 公証 + embedding 同梱）

macOS の配布可能 DMG はローカルスクリプトで一括生成します（署名・公証込みの確定手順）。

```bash
scripts/build-mac-signed.sh
```

`wails build`（既定 `darwin/universal`）→ llama.cpp サイドカー（`llama-server` + dylib）の staging と署名 →
モデル GGUF の staging → hardened runtime + secure timestamp で `.app`/`.dmg` 署名 → `notarytool submit --wait`
→ `stapler staple` までを実行し、`build/bin/SNZ-Studio.dmg` を生成します。

必要なもの:

- 「Developer ID Application」証明書（login keychain にインストール済み）
- notarytool の保存済みプロファイル（既定名 `snzstudio`。`xcrun notarytool store-credentials` で一度だけ作成）
- `data/models/ruri-v3-30m-q8_0.gguf`（次節のスクリプトで再生成）

主な env 上書き: `DEVELOPER_ID` / `NOTARY_PROFILE` / `PLATFORM` / `LLAMA_RELEASE` / `SIDECAR_ARCH` / `MODEL_SRC`。

> Windows の署名は未対応です（当面は未署名配布）。CI（`.github/workflows/build.yml`）は雛形で、
> `workflow_dispatch` 実行のみ・署名は secrets ゲートで後送りです。

### 内蔵 embedding モデル（GGUF）の再生成

内蔵 embedding は `cl-nagoya/ruri-v3-30m`（ModernBERT-Ja・256 次元・Apache-2.0）を llama.cpp で GGUF 化し
q8_0 量子化したものを `.app` に同梱し、初回起動時にユーザーデータ配下の `models/` へ展開します。この GGUF
（`data/models/ruri-v3-30m-q8_0.gguf`・約 42MB）は容量のため git 管理外なので、次のスクリプトで再現生成します。

```bash
scripts/build-ruri-gguf.sh
```

llama.cpp `b9437` の source（converter）と release（`llama-quantize`）、HF の固定 revision、pin した Python 依存
（torch / transformers / sentencepiece / gguf）を使い、HF ダウンロード → converter パッチ（SentencePiece 化）→
f16 → q8_0 → sha256 検証 → `data/models/` へ設置、までを冪等に実行します（各ステージは出力があれば skip）。
`internal/embed/modelspec.go` に pin した sha256 と一致しない場合は中断します。

## データ保存先と移行

- 配布版（prod）のデータは OS のユーザー設定ディレクトリ配下に保存されます。
  - macOS: `~/Library/Application Support/snz-studio`
  - Windows: `%AppData%\snz-studio`
  - 配下に `app.sqlite` / `uploads/` / `app-config.json`。
- 旧 Node 版の `data/` を引き継ぎたい場合は、初回起動時に `SNZ_MIGRATE_FROM` で移行元を指定します
  （初回・移行先が空のときだけ実行され、ソースは変更しません）。

```bash
SNZ_MIGRATE_FROM="/path/to/old/data" "build/bin/SNZ Studio.app/Contents/MacOS/SNZ Studio"
```

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
- `DEBUG_CHAT_FLOW`
- `DEBUG_RETRIEVAL`

例:

- LM Studio: `LLM_BASE_URL=http://127.0.0.1:1234/v1`
- Ollama OpenAI 互換 endpoint: その URL に差し替え

embedding を使う場合は `EMBEDDING_MODEL` を設定してください。未設定なら retrieval は FTS のみで動作します。設定されていれば、document / memory の retrieval は `FTS + embedding rerank` の hybrid になります。

`DEBUG_CHAT_FLOW` / `DEBUG_RETRIEVAL` は互換のため受け付けますが、Go 版はログを最小限に保つ方針のため
verbose トレースは出力しません。

Dashboard の `Configuration` から接続先、モデル、`LLM Response Format`、review 用 endpoint / model は更新できます。UI から保存した値はアプリのデータディレクトリの `app-config.json` に保存され、`.env` より優先して即時反映されます。`llm-jp-4-8b-thinking` のような thinking 系モデルでは `LLM-jp Thinking` を選ぶと、内部の reasoning / tagged response を除去して final answer のみを表示します。

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
- rerank 層の追加
- image document の manual annotation UX 改善

## 注意

- 認証なし、単一ユーザー前提です
- PDF / OCR / vector search / Electron は含みません
- 開発速度と読みやすさを優先した最小構成です
