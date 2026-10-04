# SNZ Studio（日本語版）

> English: [README.md](README.md)

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
- 多人数会話 chat（2 名以上の参加者が順番に発言する chat）
  - 参加者ごとに表示名・役割プロンプト・接続先・モデルを設定（接続先を分ければ複数の LM Studio を混在させられる）
  - ターン進行ルールは編成順の循環（`round_robin`）・発言者の指名（`manual`）・
    進行役が 1 人おきに挟まる交互進行（`facilitator_alternating`、TRPG の GM 向け）・
    呼ばれた人や長く黙っていた人が次に話す進行（`weighted`）
  - 全参加者に共通する場面設定（論題・シーン・世界観）
  - TRPG 向けに、全参加者が読む状態シート（場所・HP・所持品など）と、参加者や人間が書いた `/roll` をアプリが振って目標値と比べるダイス判定、
    `/add`・`/use`・`/set` でアプリが状態シートを書き換える効果コマンド（持っていない物は使えない。コマンドの仕様は `docs/multi-agent-commands.md`）
  - 同梱プリセット 25 件（ディベート・即興劇・TRPG の卓など）で選ぶだけで開始、自作のプリセットも JSON ファイルの読み込みで適用（形式は `docs/multi-agent-presets.md`）
  - 観戦ビューで 1 ターンずつ進める / 自動進行、任意の時点で人間として会話に発言
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
  db/                  SQLite 接続・schema・10 migrations
  repository/          project / document / memory / chat / participant の永続化層
  search/              日本語トークナイザ移植 + FTS クエリ生成
  vector/              cosine 類似度
  service/             retrieval / context / llm / embedding / summary / memory / review / turnengine
  preset/              多人数会話の同梱プリセット（bundled/*.json を go:embed）と検証
  httpapi/             35 ルートのハンドラ + SSE
frontend/
  src/
    api/               HTTP client
    components/        shell + 編成パネル（ParticipantPanel）
    pages/             画面（単独 assistant は ChatPage、多人数会話は MultiAgentChatPage）
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
- multi-agent turn layer: `internal/service/turnengine.go`（単独 assistant の `chat.go` とは独立。共有するのは
  LLM クライアント・repository・retrieval。ターンはプロジェクトのドキュメントとメモリを `turncontext.go` 経由で
  プロジェクト資料として受け取る）

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
- `participants`（多人数会話の参加者。除籍は `deleted_at` の論理削除で、過去の発言の帰属は残る）

`chats` には種別（`kind`）・ターン進行ルール（`turn_rule`）・場面設定（`scene_prompt`）・
交互進行の進行役（`facilitator_participant_id`）、`messages` には
発言者（`participant_id`）の列があります。既存 chat は `kind = 'assistant'` のままです。

## 必要なツール

- Go 1.26.3 以上（手元に無い版は `go` コマンドが自動で取得するため、事前に特定の版を
  入れておく必要はありません）
- Node 22 / pnpm（フロントのビルドに使用。`wails` が自動で実行します）
- [Wails CLI v2](https://wails.io/)（`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`）

> **Go のバージョンについて。** バインド生成に使う x/tools は Wails CLI バイナリに
> 埋め込まれていて go.mod からは差し替えられないため、CLI が読めるより新しい Go で
> ビルドすると `internal error: package "math" without types was imported from ...` で
> バインド生成が落ちます。そこで **`wails` は直接ではなく下記の pnpm スクリプト経由で
> 起動してください** — スクリプト（`scripts/wails.mjs`）が `GOTOOLCHAIN` に使用する版を
> 厳密指定するので、手元にどの Go が入っていても結果が変わりません。
>
> go.mod の `toolchain` ディレクティブは**下限**（これ未満ではビルドしない）であって
> 上限ではありません。手元の Go がそれより新しければそちらが使われるため、素の
> `wails dev` は固定になりません。上限を効かせられるのは `GOTOOLCHAIN` の厳密指定だけで、
> pnpm スクリプトがやっているのはそれです。
>
> 使用する Go のバージョンは go.mod の `toolchain` が唯一の出所で、`scripts/wails.mjs` と
> CI はそこを読みます。上げるときは、それを読める Wails CLI とセットで上げてください
> （go.mod の `toolchain` と `require` / `.github/workflows/build.yml` の `go install`）。
> CLI だけ古いままだと同じ失敗が起きるので、`wails build` が出す
> `go.mod is using Wails 'x' but the CLI is 'y'` の警告は無視しないでください。

## 開発（pnpm dev）

リポジトリ直下で次を実行します。Go API（`127.0.0.1:8787`）とフロント（Vite）が起動し、OS ネイティブ
WebView 上に SPA が表示されます。

```bash
pnpm dev
```

- 中身は `scripts/wails.mjs dev`（go.mod の `toolchain` を `GOTOOLCHAIN` に指定して `wails dev`）
  です。素の `wails dev` でも起動はしますが、その場合は手元の Go がそのまま使われるため、
  上の「必要なツール」の注意が当てはまります。
- 開発時のデータは `./data`（cwd 相対）に作成されます。接続先やモデルは、通常は UI の設定から変えます。
- アプリは `.env` ファイルを読みません。`LLM_BASE_URL` などの既定値や、UI から設定できない
  `LLM_API_KEY` は、環境変数として渡します（渡し方は「LLM 接続」）。
- ブラウザ直開き（`localhost:5173`）での開発は廃止しました（API への非 GET が届かないため）。開発は
  `pnpm dev` を使ってください。
- 内蔵 embedding を使うには、先に `pnpm sidecar` を実行します。サイドカーを `build/sidecar/<GOOS>-<GOARCH>/` に、
  モデル GGUF を `data/models/` に置きます（「内蔵 embedding のサイドカーとモデル（llama-server・GGUF）」）。
  `SNZ_LLAMA_SERVER_BIN` の指定は要りません。

## ビルド（配布物）

ビルドには 2 段階があり、用途で使い分けます。

### 1. 動作確認用（素のビルド）

```bash
pnpm build:app                                          # 現在の OS 向け
pnpm build:app -platform darwin/universal               # macOS universal（.app）
pnpm build:app -platform windows/amd64 -nsis -webview2 download
```

`build:app` は `scripts/wails.mjs build` で、追加の引数はそのまま `wails build` に渡ります。
ランチャは Node で書いてあるので、Windows でも同じコマンドが使えます。

成果物は `build/bin/`（macOS は `SNZ Studio.app`、Windows は `.exe`）に出力されます。

> **注意**: `wails build` 単体では内蔵 embedding（llama.cpp サイドカー + モデル GGUF）は同梱されません。
> この `.app` を起動すると内蔵 embedding は `llama-server binary not found` になります（外部 embedding か
> FTS のみで動作）。どちらも `pnpm sidecar --app` で後から置けます（次節）。Windows のインストーラに
> 入れるには、`pnpm build:app -platform windows/amd64 -nsis ...` の**前に** `pnpm sidecar --arch amd64` を
> 実行します。インストーラは `build/sidecar/windows-amd64/` と `build/sidecar/.downloads/` から取り込み、
> そこに無ければ内蔵 embedding なしのインストーラになります。

### 内蔵 embedding のサイドカーとモデル（llama-server・GGUF）

内蔵 embedding が使う llama.cpp の `llama-server` とモデル GGUF は、次のスクリプトで取得して置きます。
macOS でも Windows（PowerShell、WSL なし）でも同じコマンドです。

```bash
pnpm sidecar          # 開発用: llama-server を build/sidecar/<GOOS>-<GOARCH>/ に、GGUF を data/models/ に置く（pnpm dev が使う）
pnpm sidecar --app    # 配布用: どちらも pnpm build:app の成果物に置く（macOS は .app の Contents/Resources、Windows は exe の横）
```

- 実行中の OS と CPU に合うアーカイブを llama.cpp のリリースから取得し、sha256 を照合してから展開します。
  一致しなければ何も置かずに中断します。
- モデル GGUF（`ruri-v3-30m-q8_0.gguf`）は、このリポジトリの GitHub Release
  （[`ruri-v3-30m-q8_0-2a6cb2d9`](https://github.com/serendipitynz/snz_studio/releases/tag/ruri-v3-30m-q8_0-2a6cb2d9)）
  から取得します。サイズと sha256 を `internal/embed/modelspec.go` の値（アプリが起動時に照合するのと同じ値）と
  照合し、一致しなければ中断します。照合が通るコピーが `data/models/` にあれば、取得せずにそれを使います。
  アプリは置かれた GGUF を初回起動時にユーザーデータ配下の `models/` へコピーするので、GGUF を同梱した
  アプリがモデルをダウンロードすることはありません。
- 置くのは `llama-server` と共有ライブラリ（dylib / DLL）だけです（macOS では、署名していない実行ファイルが
  あると公証に通らないため）。
- 同じ版と GGUF が置いてあれば何もしません。取得したファイルは `build/sidecar/.downloads/` に残るので、
  `pnpm build:app` をやり直した後の `pnpm sidecar --app` では取得し直しません。
- `--arch arm64|amd64` で CPU を指定できます（例: universal の `.app` に arm64 のサイドカーを置く）。
- 同梱する llama.cpp の版と sha256 は `scripts/sidecar.mjs` の 1 か所で、GGUF の URL・サイズ・sha256 は
  `internal/embed/modelspec.go` の 1 か所で固定しています。CI と `scripts/build-mac-signed.sh` もこの
  スクリプトで両方を置きます。
- macOS の `pnpm dev` は `build/bin/` の `.app` からアプリを起動します。その `.app` にサイドカーが置いて
  あると（`pnpm sidecar --app` や署名ビルドの後）、`build/sidecar/` より先にそちらが使われます。
- 版を変えても、保存済みのベクトルは計算し直されません。新しい版でベクトルが変わり得るときは、内蔵
  embedding の準備が整った後に、設定画面の「埋め込みの作り直し」で「作り直す」を押します
  （`POST /api/embedding/rebuild`）。設定の保存で全件を作り直すのは、埋め込みソース（モード、外部の
  エンドポイントかモデル）を変えたときだけです。この区画には、作り直しの最中か（終わるまで「作り直す」は
  押せません。`GET /api/embedding/rebuild`）と、最後の作り直しが終わりきったかが出ます。`b9437` から
  `b11126` への更新では、同じ GGUF からビット単位で同じベクトルが出たので、作り直しは要りませんでした。

### 2. 配布用（macOS・署名 + 公証 + embedding 同梱）

macOS の配布可能 DMG はローカルスクリプトで一括生成します（署名・公証込みの確定手順）。

```bash
scripts/build-mac-signed.sh
```

`wails build`（既定 `darwin/universal`）→ `pnpm sidecar --app` による llama.cpp サイドカー（`llama-server` +
dylib）とモデル GGUF の staging → サイドカーの署名 → hardened runtime + secure timestamp で `.app`/`.dmg` 署名 → `notarytool submit --wait`
→ `stapler staple` までを実行し、`build/bin/SNZ-Studio.dmg` を生成します。

必要なもの:

- 「Developer ID Application」証明書（login keychain にインストール済み）
- notarytool の保存済みプロファイル（既定名 `snzstudio`。`xcrun notarytool store-credentials` で一度だけ作成）

主な env 上書き: `DEVELOPER_ID` / `NOTARY_PROFILE` / `PLATFORM` / `SIDECAR_ARCH`（`arm64` / `amd64`）。
サイドカーの版は `scripts/sidecar.mjs` で、GGUF は `internal/embed/modelspec.go` で固定しています。

> Windows の署名は未対応です（当面は未署名配布）。CI（`.github/workflows/build.yml`）は雛形で、
> `workflow_dispatch` 実行のみ・署名は secrets ゲートで後送りです。
>
> `.github/workflows/audit.yml` は `main` への PR と push、週 1 回、手動実行で `pnpm audit --prod` と
> `govulncheck` を走らせ、既知の脆弱性が 1 件でもあれば失敗します。

### 内蔵 embedding モデル（GGUF）の再生成

内蔵 embedding は `cl-nagoya/ruri-v3-30m`（ModernBERT-Ja・256 次元・Apache-2.0）を llama.cpp で GGUF 化し
q8_0 量子化したものをアプリに同梱し、初回起動時にユーザーデータ配下の `models/` へコピーします。この GGUF
（約 42MB）は容量のため git 管理外です。ビルドでは GitHub Release に公開した検証済みのコピーを使い
（上の `pnpm sidecar`）、次のスクリプトはそれを作った手順です。公開したファイルの検証や、新しい GGUF を
作るときに使います。

```bash
scripts/build-ruri-gguf.sh
```

llama.cpp `b9437` の source（converter）と release（`llama-quantize`）、HF の固定 revision、pin した Python 依存
（torch / transformers / sentencepiece / gguf）を使い、HF ダウンロード → converter パッチ（SentencePiece 化）→
f16 → q8_0 → sha256 検証 → `data/models/` へ設置、までを冪等に実行します（各ステージは出力があれば skip）。
`internal/embed/modelspec.go` に pin した sha256 と一致しない場合は中断します。

スクリプト自体は macOS でしか動きませんが、手順は OS に依存しません。同じ版の Windows の
`llama-quantize.exe` で量子化しても、Python 3.10 と 3.12 のどちらでも、同じ sha256 になりました。
HF からダウンロードしたディレクトリの名前が GGUF のメタデータに入るので、名前は `ruri-v3-30m` のままにします。
新しい GGUF は新しいリリースタグで公開し、`modelspec.go` の `URL` / `SHA256` / `SizeBytes` も変えます。
公開済みのアセットは差し替えません。

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

OpenAI 互換 API を前提にしています。次の値を環境変数で渡すと、既定値を上書きできます。アプリは `.env`
ファイルを読まないので、`.env.example` を写した `.env` に書いただけでは反映されません (渡し方は下の
「環境変数の渡し方」)。

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
- `IMAGE_DESCRIPTION_BASE_URL`
- `IMAGE_DESCRIPTION_MODEL`
- `IMAGE_DESCRIPTION_TIMEOUT_MS`
- `DEBUG_CHAT_FLOW`
- `DEBUG_RETRIEVAL`

例:

- LM Studio: `LLM_BASE_URL=http://127.0.0.1:1234/v1`
- Ollama OpenAI 互換 endpoint: その URL に差し替え

embedding を使う場合は `EMBEDDING_MODEL` を設定してください。未設定なら retrieval は FTS のみで動作します。設定されていれば、document / memory の retrieval は `FTS + embedding rerank` の hybrid になります。

画像を入力できるモデルを `IMAGE_DESCRIPTION_MODEL` (または設定画面の画像説明用モデル) に設定すると、
画像の追加ダイアログで「説明文を生成」が使えます。画像を 1 回だけモデルに送り、返った文を説明文欄に
下書きとして入れます。保存されるのは追加操作をしたときだけです。登録済みの画像は、ドキュメント一覧から
開いて「メモ・タグ・説明文を編集」を選ぶと、メモ・タグ・説明文を編集して保存でき、同じ生成操作も使えます。
エンドポイントは `LLM_BASE_URL` にフォールバックしますが、モデルはフォールバックしないので、空なら生成操作は無効になります。
`IMAGE_DESCRIPTION_TIMEOUT_MS` の既定値は 180000 です (根拠は `.env.example`)。

`DEBUG_CHAT_FLOW` / `DEBUG_RETRIEVAL` は互換のため受け付けますが、Go 版はログを最小限に保つ方針のため
verbose トレースは出力しません。

設定 (サイドバーの歯車ボタン、または Dashboard の「設定を開く」) から接続先、モデル、`LLM Response Format`、review 用 endpoint / model は更新できます。UI から保存した値はアプリのデータディレクトリの `app-config.json` に保存され、環境変数より優先して即時反映されます。`llm-jp-4-8b-thinking` のような thinking 系モデルでは `LLM-jp Thinking` を選ぶと、内部の reasoning / tagged response を除去して final answer のみを表示します。

ローカル LLM が起動していない場合でも、アプリ自体は動作します。  
その場合 chat 返答は fallback 文面になり、どの参照が選ばれたかの確認に使えます。

### 環境変数の渡し方

`pnpm dev` では、起動するシェルの環境変数がそのまま Go 側に渡ります。`.env.example` を写した `.env` に
まとめて書くなら、シェルに読み込んでから起動します (`.env` は `.gitignore` 済みです)。

```bash
set -a; . ./.env; set +a; pnpm dev
```

配布版の `SNZ Studio.app` を Finder や Dock から開くと、シェルの環境変数は渡りません。`LLM_API_KEY` が
要るときは、ターミナルから実行ファイルを直接起動します。

```bash
set -a; . ./.env; set +a; "/Applications/SNZ Studio.app/Contents/MacOS/SNZ Studio"
```

Windows では、PowerShell で `$env:LLM_API_KEY = "..."` のように設定してから、同じ PowerShell から
`pnpm dev` か `SNZ Studio.exe` を起動します。

`LLM_API_KEY=... pnpm dev` のようにコマンドへ直接書いても渡せますが、キーがシェルの履歴に残ります。

### API キーが送られる範囲

`LLM_API_KEY` は、既定の接続先 (`LLM_BASE_URL`、または設定で保存した接続先) とスキーム・ホスト・ポートが
同じ送信先にだけ付けます。review 用の接続先、画像説明の接続先、多人数会話の参加者ごとの接続先、設定画面で
モデル一覧を取る接続先がそれと違えば、キーは付きません。他人から受け取ったプリセットファイルに書かれた
接続先へキーが送られないようにするためです。参加者ごとのキーは設定できないので、キーが要る API を参加者に
使うときは、既定の接続先と同じスキーム・ホスト・ポートにしてください。

`EMBEDDING_API_KEY` は外部の埋め込み接続先 (`EMBEDDING_MODE=external`) にそのまま付けます。空か未設定の
ときは `LLM_API_KEY` を借りますが、上と同じ規則で、埋め込みの接続先が既定の接続先と同じスキーム・ホスト・
ポートのときだけです。内蔵の埋め込み (`internal`) には、どちらのキーも送りません。

## 実装方針

- ベクトル DB なし
- retrieval は SQLite FTS を基本とし、任意で OpenAI 互換 embeddings による hybrid rerank を追加
- document は `world / character / rule / plot / timeline / index / story / misc` に分類され、retrieval の優先度調整に使う
- ベクトルは SQLite に JSON で保存し、外部ベクトル DB は使わない
- 過去 chat は毎回全文を渡さず、`chat_summaries` と recent messages を中心に扱う
- memory は durable fact だけを保存する前提
- 初期実装では memory 抽出は軽量な rule-based heuristic
- 多人数会話は 1 リクエスト = 1 ターン（常駐の進行ジョブを持たず、自動進行はフロントのループ）。
  進行中のターンはクライアントが切断しても完走して保存されるため、停止できる粒度はターン境界だけ

## API の要点

- `GET /api/projects`
- `POST /api/projects`
- `GET /api/projects/:projectId`
- `POST /api/projects/:projectId/chats`
- `POST /api/projects/:projectId/memories`
- `POST /api/projects/:projectId/documents`
- `GET /api/chats/:chatId`
- `POST /api/chats/:chatId/messages`
- `GET /api/multi-agent-presets`
- `GET /api/chats/:chatId/export/preset`（会話の編成を接続先ごとプリセットとして返す）
- `GET / POST /api/chats/:chatId/participants`
- `PATCH / DELETE /api/participants/:participantId`
- `POST /api/chats/:chatId/turns/stream`（1 ターン実行・SSE。`speaker` → `delta` … → `done` の順に流す。実行中の重複呼び出しは 409）。
  `delta` の間に `replace` が来ることがある。テキストとして流れたタグの断片が取り除かれたときに送り、それまでの
  `delta` で足した内容に代えて、その時点の表示テキスト全体を運ぶ。チャットとレビューのストリームも同じ。
- `GET /api/messages/:messageId/memory-draft` / `POST /api/messages/:messageId/memory`（多人数会話の発言 1 件を
  プロジェクトのメモリとして保存。多人数会話は自動でメモリを抽出しない）
- `POST /api/chats/:chatId/conclusion-draft`（会話全体または選んだ発言以降の結論を既定 LLM で下書きする。
  保存は上の保存ルートで行い、下書き自体は何も永続化しない）

## 今後の拡張ポイント

- hybrid retrieval の重み調整と semantic-only fallback の改善
- chat summary 更新を LLM ベースに切り替え
- memory 抽出を LLM ベースに切り替え
- document 編集 / 削除 UI
- rerank 層の追加
- image document の manual annotation UX 改善
- 多人数会話の TRPG 対応の残り（状態シートを更新する効果コマンド）、進行役モデルによる発言者指名、
  生成中断（[設計書](docs/multi-agent-chat-design.md) §7）

## ライセンス

本体は MIT License（[LICENSE](LICENSE)）です。

同梱・移植した第三者成果物（`internal/search` の TinySegmenter 由来コード、配布物に同梱する
llama.cpp と ruri-v3-30m）の著作権表示とライセンス全文は
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) にまとめてあります。

## 注意

- 認証なし、単一ユーザー前提です
- PDF / OCR / vector search / Electron は含みません
- 開発速度と読みやすさを優先した最小構成です
