<img src="docs/assets/appicon-256.png" alt="" width="128">

# SNZ Studio（日本語版）

> English: [README.md](README.md)

個人用途向けの、ローカルで完結する軽量な LLM プロジェクトワークスペースです。  
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

## インストール

[Releases](https://github.com/serendipitynz/snz_studio/releases) ページから、OS に合ったインストーラを
ダウンロードします。同じ場所の `SHA256SUMS.txt` に、各ファイルのチェックサムがあります。

- macOS: `SNZ-Studio-<version>-macOS.dmg`（universal）。Developer ID で署名して公証してあるので、
  Gatekeeper の警告なしに開けます。アプリを Applications フォルダへドラッグしてください。
  同梱の embedding サイドカーは arm64 版だけなので、Intel Mac では内蔵 embedding が起動しません。
  embedding には外部の接続先を指定してください（「LLM 接続」）。
- Windows: `SNZ-Studio-<version>-Windows-amd64-installer.exe`。現在のユーザー用に
  `%LOCALAPPDATA%\Programs\SNZ Studio` へインストールし、管理者の承認は求めません。コード署名をしていないので、
  初回起動時に SmartScreen の警告が出ます（「詳細情報」→「実行」）。WebView2 ランタイムが無ければ、
  インストーラが取得します。

LLM の接続先（LM Studio・Ollama などの OpenAI 互換サーバ）は別に必要です。「LLM 接続」を参照してください。

## 更新

アプリは [Releases](https://github.com/serendipitynz/snz_studio/releases) ページから自分で更新します。

- 起動して 5 秒ほどたつと、新しいバージョンが公開されているかを GitHub に 1 回だけ問い合わせます。
  あれば、そのバージョン番号をダイアログで示します。「更新する」を選ぶまでは何もダウンロードしません。
  GitHub につながらないときは何も表示しません。利用者に頼まれずにアプリがインターネットへ接続するのは、
  この確認だけです。
- 確認をオフにするには、設定を開き、「更新」の「起動時に新しいバージョンを確認する」のチェックを外します。
  同じ区画に実行中のバージョンが表示され、手動で確認する「今すぐ確認」もあります。
- 「更新する」を選ぶと、新しいバージョンをダウンロードし、アプリに組み込まれた公開鍵で署名を確かめ、
  確かめられたときだけ入れ替えます。そのあとアプリを終了し、新しいバージョンで起動し直します。
  ダウンロードや署名の確認に失敗したとき、macOS で `.app` を入れ替えられなかったときは、ダイアログが
  「更新は行われませんでした」と伝え、インストール済みのバージョンはそのまま残ります。アプリが終了した
  あとのことは、どこにも知らされません。新しいバージョンが起動しなかったときは、自分でアプリを起動し、
  設定で実行中のバージョンを確かめてください。
- macOS では、インストールされている場所の `.app` を入れ替えます。入れ替えられないとき（ディスクイメージや
  ダウンロードした場所から起動している、そのフォルダにアカウントが書き込めない）は、代わりに Releases
  ページへ案内します。アプリは Applications フォルダに置いてください。
- Windows では、新しいバージョンのインストーラを実行し、現在のユーザー用にインストールされたものを
  入れ替えます。
- 更新のとき、システムがパスワードや管理者の承認を求めることがあります。Windows では、確認した範囲では
  インストールでも更新でも管理者の承認は求められませんでした。

### v0.1.0 からの移行

v0.1.0 には自分で更新する機能が無いので、新しいバージョンが出ても気づきません。一度だけ手で入れ替えて
ください。Releases ページから新しいインストーラをダウンロードし、「インストール」のとおりに入れます。
それ以降のバージョンは、アプリの中から更新できます。

Windows では、インストール先も変わりました。v0.1.0 は `Program Files` にインストールしていましたが、
それ以降のバージョンは現在のユーザー用に `%LOCALAPPDATA%\Programs\SNZ Studio` へインストールし、
インストーラは古いほうを消しません。新しいバージョンを入れる前に、設定 → アプリ（「インストールされている
アプリ」、Windows 10 では「アプリと機能」）から v0.1.0 をアンインストールしてください。プロジェクト・チャット・
設定は引き継がれます。これらは `%AppData%\snz-studio`（「データ保存先と移行」）にあり、アンインストーラも
インストーラも触れないためです。

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

- Go 1.26.3 以上（手元に無いバージョンは `go` コマンドが自動で取得するため、事前に特定のバージョンを
  入れておく必要はありません）
- Node 22 / pnpm（フロントのビルドに使用。`wails` が自動で実行します）
- [Wails CLI v2](https://wails.io/)（`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`）

> **Go のバージョンについて。** バインド生成に使う x/tools は Wails CLI バイナリに
> 埋め込まれていて go.mod からは差し替えられないため、CLI が読めるより新しい Go で
> ビルドすると `internal error: package "math" without types was imported from ...` で
> バインド生成が落ちます。そこで **`wails` は直接ではなく下記の pnpm スクリプト経由で
> 起動してください** — スクリプト（`scripts/wails.mjs`）が `GOTOOLCHAIN` に使用するバージョンを
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
- 同じバージョンと GGUF が置いてあれば何もしません。取得したファイルは `build/sidecar/.downloads/` に残るので、
  `pnpm build:app` をやり直した後の `pnpm sidecar --app` では取得し直しません。
- `--arch arm64|amd64` で CPU を指定できます（例: universal の `.app` に arm64 のサイドカーを置く）。
- 同梱する llama.cpp のバージョンと sha256 は `scripts/sidecar.mjs` の 1 か所で、GGUF の URL・サイズ・sha256 は
  `internal/embed/modelspec.go` の 1 か所で固定しています。CI と `scripts/build-mac-signed.sh` もこの
  スクリプトで両方を置きます。
- macOS の `pnpm dev` は `build/bin/` の `.app` からアプリを起動します。その `.app` にサイドカーが置いて
  あると（`pnpm sidecar --app` や署名ビルドの後）、`build/sidecar/` より先にそちらが使われます。
- バージョンを変えても、保存済みのベクトルは計算し直されません。新しいバージョンでベクトルが変わり得るときは、内蔵
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
サイドカーのバージョンは `scripts/sidecar.mjs` で、GGUF は `internal/embed/modelspec.go` で固定しています。

> Windows の署名は未対応です（当面は未署名配布）。
>
> `.github/workflows/audit.yml` は `main` への PR と push、週 1 回、手動実行で `pnpm audit --prod` と
> `govulncheck` を走らせ、既知の脆弱性が 1 件でもあれば失敗します。

### 3. リリース（GitHub Actions）

`.github/workflows/release.yml` が、バージョンのタグをビルドし、インストーラ・更新用ファイル・`latest.json` を
GitHub Release の**下書き**に添付します。ビルドは、`.github/workflows/build.yml` を署名ありで呼び出したものです。macOS の `.app` と
`.dmg` は、`scripts/build-mac-signed.sh` と同じ手順で CI 上で署名・公証・staple します。Windows は未署名の
ままです。`build.yml` を単独で手動実行したときは、これまでどおり未署名の成果物ができます。

最初に一度だけ、macOS の署名に使う `APPLE_*` の secrets 6 つをリポジトリに登録します。

1. キーチェーンアクセスから「Developer ID Application」証明書を、パスワード付きの `.p12` として書き出します。
2. `.env.signing.example` を `.env.signing`（git の追跡外）にコピーし、`APPLE_ID`、`APPLE_PASSWORD`
   （App 用パスワード）、`APPLE_TEAM_ID` を埋めます。
3. 次のスクリプトを実行し、`.p12` の書き出し時のパスワードを入力します。最初に登録先のリポジトリを表示し、
   secrets の値は表示しません。

```bash
./scripts/setup-ci-signing-secrets.sh path/to/DeveloperID.p12
```

続けて、更新用ファイルに署名する Ed25519 の秘密鍵 `UPDATE_SIGNING_KEY` を登録します。スクリプトは
先に、コミット済みの公開鍵 `internal/updatesig/update-signing-key.pub` と対になっているかを確かめ、
対でない鍵は登録しません。ほかの secrets には触れません。

```bash
./scripts/setup-ci-signing-secrets.sh --update-key ~/.config/snz-studio/update-signing.key
```

リリースの手順:

1. `package.json`（`version`）と `wails.json`（`info.productVersion`）を新しいバージョンにして、`main` に
   マージします。
2. マージしたコミットにタグ（`vMAJOR.MINOR.PATCH`）を打ち、push します。

   ```bash
   git tag v0.1.0
   ```

   ```bash
   git push origin v0.1.0
   ```

3. secrets が欠けている、`UPDATE_SIGNING_KEY` がコミット済みの公開鍵と対でない、タグが 2 つのバージョンと
   一致しない、そのタグの Release がすでに公開されている、のどれかに当てはまると、ワークフローはビルドの前に
   止まります。そうでなければ、前のバージョンのタグ以降にマージされた PR から自動生成したノート（分類は
   `.github/release.yml`）で下書きを作り、`.dmg`、macOS の更新用アーカイブ（`.app.zip`）、Windows の
   インストーラ、`latest.json`、`SHA256SUMS.txt` を添付します。更新用アーカイブとインストーラには
   `UPDATE_SIGNING_KEY` で署名し、アップロードの前にコミット済みの公開鍵で検証します。
4. ノートと添付ファイルを確認し、GitHub 上で下書きを公開します。自動では公開されません。
   **公開した時点で更新が配られ始めます。** インストール済みのアプリが見るのは、公開済みで pre-release の
   印が付いていないリリースだけなので、下書きのあいだは見えません。公開すると、確認をオンにしているアプリは
   次の起動から新しいバージョンを案内します。

失敗した実行は、Actions タブから（`release` →「Run workflow」でタグを指定）やり直せます。下書きは
作り直さず、既存のものを使います。

アプリ内の更新を載せた最初のリリースでは、公開する前に、下書きのノートの日本語の半分に次の段落を、
英語の半分にその英語の文面（README.md にあります）を足します。v0.1.0 は更新を案内しないので、v0.1.0 の利用者は
ここで知ることになります。

> **v0.1.0 からの移行。** v0.1.0 には自分で更新する機能が無いので、このバージョンは一度だけ手でインストール
> してください。このバージョンからは、アプリが新しいバージョンを確認して自分で更新します（確認は設定でオフに
> できます）。**Windows では**、このバージョンは `Program Files` ではなく、現在のユーザー用に
> `%LOCALAPPDATA%\Programs\SNZ Studio` へインストールし、インストーラは v0.1.0 を消しません。先に設定 →
> アプリ（「インストールされているアプリ」、Windows 10 では「アプリと機能」）から v0.1.0 をアンインストール
> してください。プロジェクト・チャット・設定は別の場所にあるので引き継がれます。

#### 更新署名鍵

インストール済みのアプリは、自分のバイナリに組み込まれた公開鍵で署名を確かめられた更新だけを受け付けます。
`UPDATE_SIGNING_KEY` はその公開鍵と対になる秘密鍵です。保管場所は 2 つで、リリース作業をするマシンの
`~/.config/snz-studio/update-signing.key` と、パスワードマネージャです。

**秘密鍵を失くすか差し替えると、インストール済みのアプリはすべて更新できなくなります。** 新しい鍵で署名した
更新は、インストール済みのアプリに組み込まれた公開鍵では確かめられないので、どのアプリも受け付けず、
利用者全員に新しいバージョンを手でインストールし直してもらうしかありません。リリースの手順の側では
取り戻せないので、これは後から対処する障害ではなく、バックアップの問題です。2 つの保管場所を保ち、
古い鍵を失くすか漏らすかしない限り、新しい鍵（`go run ./tools/updatesig keygen`）は作りません。

#### リリースのワークフローを変えるときに守ること

- **`latest.json` を書くのは `attach` ジョブだけです。** このジョブは両方のプラットフォームのビルドが終わってから
  動くので、ファイルには常に `darwin-universal` と `windows-amd64` の両方が載ります。並列のビルドジョブから
  書くと、一方のプラットフォームの項目が他方を上書きしたり抜け落ちたりしえます。
- **URL はすべてタグに固定します**（`releases/download/vX.Y.Z/…`）。アプリが `latest.json` を読む URL も、
  その中の URL も同じです。`releases/latest/…` の URL は後のリリースを公開すると移り、隣に署名が置かれている
  ファイルを指さなくなります。
- **モデルのリリースは更新の確認に影響しません。** このリポジトリはモデルのファイルも別のタグ
  （`ruri-v3-30m-q8_0-…`）で公開していて、それが GitHub の「latest」になることがあります。そのためアプリは
  `releases/latest` を使わず、公開済みのリリースを一覧して最大の `vMAJOR.MINOR.PATCH` のタグを選びます。
  モデルのリリースが更新として案内されることはありません。

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

スクリプト自体は macOS でしか動きませんが、手順は OS に依存しません。同じバージョンの Windows の
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
- 多人数会話の、進行役モデルによる発言者指名と生成中断（[設計書](docs/multi-agent-chat-design.md) §7）

## ライセンス

本体は MIT License（[LICENSE](LICENSE)）です。

同梱・移植した第三者成果物（`internal/search` の TinySegmenter 由来コード、配布物に同梱する
llama.cpp と ruri-v3-30m）の著作権表示とライセンス全文は
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) にまとめてあります。

## 注意

- 認証なし、単一ユーザー前提です
- PDF / OCR / vector search / Electron は含みません
- 開発速度と読みやすさを優先した最小構成です
