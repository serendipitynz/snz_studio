# SNZ Studio — Wails + Go デスクトップ化 引き継ぎ (HANDOFF)

最終更新: 2026-05-31（Phase 9 完了） / 対象: 次セッションの実装者（あなた）

> このファイル1枚＋ `~/.claude/plans/electron-tauri-elegant-naur.md`（承認済み計画）＋ project memory
> で文脈を完全復元できるように書いてあります。**まずこの順に読んでください**:
> 1. このファイル全体 → 2. 計画ファイル → 3. `internal/` 配下の各 `*.go` 冒頭コメント。

---

## 0. ゴールと確定方針（再検討不要・ユーザー承認済み）

- SNZ Studio（現: React/Vite SPA + Node/Express + better-sqlite3 のローカル Web アプリ）を、
  **インストールして起動するだけのスタンドアロン・デスクトップアプリ**にする。
- 採用: **Wails v2（Go コア + OS ネイティブ WebView）+ バックエンドを Go へ全面書き直し。フロントは React のまま。**
- 対象 OS: **macOS / Windows**。
- 通信方式: **HTTP + SSE を温存**。Wails AssetServer で SPA 配信、`/api`・`/files` は
  **ローカル `127.0.0.1` の Go `net/http` サーバー**で配信（AssetServer 経由は SSE フラッシュ不確実なため不採用）。
- Electron は不採用（重い）。`docs/current-spec.ja.md` の「Go 再実装」方針と一致。

---

## 1. 現状サマリ（DONE & TESTED）

リポジトリ直下に Go モジュール `snzstudio` を作成済み。**Phase 1（Wails 足場）+ Phase 4（Repository 層）+ Phase 5（Service 層）+ Phase 6（HTTP API + Wails 結線）+ Phase 7（フロント微修正 + dev 通信是正）+ Phase 8（データ移行 + パッケージング下地）+ Phase 9（旧 backend/Node 依存/vite proxy 削除・docs 更新）まで完了**。Phase 9 で旧 Node バックエンド（`backend/`）と Node 本番依存を削除し、リポジトリは Wails+Go 構成に一本化された（コード移植は完了済みのため Go 層への影響なし。検証グリーン）。Phase 8 では `migrate.go`（`SNZ_MIGRATE_FROM` 駆動の初回 seed・`VACUUM INTO` で WAL 安全コピー）を追加し、bundle id を `net.serenebach.snz-studio` に確定、ローカル `wails build`（prod）の疎通と bundle id 適用を実機確認、GitHub Actions の build マトリクス（`.github/workflows/build.yml`、署名/notarization は secrets ゲートで後送り）を雛形作成。Phase 7 でフロントを `window.__API_BASE__`（絶対 URL 直叩き）+ `HashRouter` に最小改修済み（`main.tsx`/`api/client.ts`/`ChatPage.tsx`/`ProjectDetailPage.tsx` + 新規 `global.d.ts`、`vite.config.ts` の proxy はブラウザ直開き用に残置）。それ以外の React ソースは未変更。旧 Node アプリ（`backend/`）も未変更で今もそのまま動く。`go build ./...` / `go vet ./...` / `go test ./...`（`-race` 含む）全グリーン、`gofmt` クリーン（`internal/search/model.go` は自動生成のため対象外）、`wails build` で `build/bin/snz-studio.app`（darwin/arm64・自己署名）まで生成確認済み（Phase1 時点）。新規本番依存なし（HTTP ルータは標準 `net/http` の method+pattern ServeMux を採用＝`chi` 不使用。`google/uuid` は Phase4 で昇格済みの既存直接依存）。

```
main.go                               ✅ Phase1 Wails 起動 + //go:embed all:frontend/dist（※embed は親参照 `..` 不可かつパス相対なのでルート必須）
app.go                                ✅ Phase6 startup で bootstrap.ResolveDataPaths→MkdirAll→bootstrap.MaybeSeedDataDir→db.Open→config.Load→httpapi.NewServer→Handler 注入→RunStartupTasks→goroutine Serve / shutdown で server+db close / GetApiBase()（Phase7: dev/prod とも絶対 URL `http://<ln.Addr>` を返す）。★Phase8: App はバインド対象なので main パッケージ＝ルート維持（バインドは `wailsjs/go/main/App`）
（ルート直下の .go は main.go と app.go のみ。Wails 制約=`wails build` は wails.json のあるルートでパッケージ引数なし `go build` する＝main パッケージはルート必須。それ以外の起動時インフラは internal/bootstrap へ集約＝Phase8 リファクタ）
wails.json                            ✅ Phase1 monorepo（frontend:* は pnpm -C .. / serverUrl=auto / wailsjsdir=frontend/src）。Phase8: author email=takuya.otani@serenebach.net
build/{appicon.png,darwin/Info.plist} ✅ Phase8 CFBundleIdentifier=net.serenebach.snz-studio（Info.plist + Info.dev.plist。既定 com.wails.{{.Name}} から置換）
.github/workflows/build.yml           ✅ Phase8 CI 雛形（mac universal+dmg / win NSIS+webview2、署名は secrets ゲートで後送り。Actions 未実行）
go.mod / go.sum                       module snzstudio (go 1.26) + wails v2.12.0
internal/bootstrap/                   ✅ Phase8 プロセス起動時インフラ（旧ルート .go を集約。パッケージ bootstrap）
  ├ paths.go                           ResolveDataPaths/Paths: DATA_DIR/SQLITE_PATH/UPLOAD_DIR env override、既定 dev=cwd/data・prod=os.UserConfigDir()/snz-studio
  ├ env_dev.go (//go:build dev)        IsDev=true, ListenAddr=127.0.0.1:8787（wails dev が dev タグ付与）
  ├ env_prod.go (//go:build !dev)      IsDev=false, ListenAddr=127.0.0.1:0（ephemeral）
  ├ migrate.go                         MaybeSeedDataDir/seedDataDir: SNZ_MIGRATE_FROM=旧dataDir→初回のみ seed（dest に app.sqlite が在れば no-op）。DB は mode=ro 接続から VACUUM INTO（source 非破壊・WAL も取り込み単一ファイル化）、uploads/ 再帰コピー、app-config.json は欠落時のみ
  └ migrate_test.go                    WAL 安全コピー（書込接続を開いたまま=未checkpointの行が dest に来る）/ dest既存=no-op / source無=no-op / env 結線
internal/search/                      ✅ 日本語トークナイザ + FTS クエリ生成（searchText.ts 移植）
  ├ segmenter.go                       TinySegmenter 0.2 アルゴリズム移植
  ├ model.go                           ★自動生成（編集禁止）: tools/segmenter-parity/gen_model.mjs
  ├ searchtext.go                      BuildSearchText / TokenizeSearchTerms / ToFtsQuery
  └ searchtext_test.go                 JS とのバイト単位パリティ（41ケース, 実サンプル全文込み）
internal/vector/                      ✅ CosineSimilarity / NormalizeScores（vector.ts 移植）
internal/db/                          ✅ DB 層
  ├ connection.go                      Open(path): modernc.org/sqlite, pragmas, SetMaxOpenConns(1)
  ├ schema.go                          9 migrations を schema.ts から忠実移植
  └ db_test.go                         FTS5 + bm25() + 冪等 migration + FK を検証
internal/httpapi/                     ✅ Phase6 HTTP API（index.ts 22ルート移植）
  ├ sse.go                             SSEWriter（frontend のパーサと同じ event:/data: 形式, 各 Event で Flush）
  ├ sse_test.go                        逐次配信（ハンドラ完了前に受信できる）を実証
  ├ server.go                          Server: NewServer(db,cfg,uploadDir) で service グラフ結線 / Handler()（mux+CORS） / RunStartupTasks（backfill+FTS再構築+async embedding rebuild） / 共有ヘルパ（writeJSON SetEscapeHTML(false)/writeError/fail/decodeBody/bodyString/bodyBool/checkConnections 並行3チェック/workspaceConfiguration）
  ├ handlers.go                        22ルートのハンドラ（config/projects/memories/documents/chats/messages/review）＋ /files static（path traversal ガード）＋ multipart upload（uuid+ext 保存・公開/files/{name}）＋ unlinkFile
  └ handlers_test.go                   httptest エンドポイントレス: 全ルートの JSON整形/ステータス/multipart/404・400 振り分け/SSE配線（delta+done/error）/CORS。死んだLLM(127.0.0.1:1)＋embedding無効でchat fallback・review error を end-to-end 検証
internal/model/                       ✅ Phase4 ドメイン型（lib/types.ts 移植, JSON タグはフロント契約に一致, nullable は *T）
internal/util/                        ✅ Phase4 lib/utils.ts 移植: NowISO（ms UTC Z）/ NewID（google/uuid v4）/ ParseTags
internal/chunk/                       ✅ Phase4 documentChunker.ts 移植: Document()=1000rune窓/150 overlap（rune基準, UTF-16差はBMP外のみ）
internal/doccategory/                 ✅ Phase4 documentCategory.ts 移植: Infer()/IsValid()（\s→[\s\p{Z}] で全角空白パリティ補正）
internal/repository/                  ✅ Phase4 4リポジトリ（projectRepository/document/memory/chat.ts 移植）
  ├ repository.go                      共有: dbtx/scanner if, NullX→*T 変換, boolToInt, ptrArg[T], inPlaceholders
  ├ project.go                         CRUD + reorder（ErrProjectReorderMismatch）+ chat_count JOIN
  ├ document.go                        CRUD + chunk/FTS upsert + embedding + rebuildSearchIndex + backfillInferredCategories
  ├ memory.go                          CRUD + FTS upsert + locked COALESCE + hasSimilarMemory + embedding + rebuild
  ├ chat.go                            chat/message/summary/参照 CRUD + ストリーミング保存(updateContent/finalize) + getMessagesWithReferences
  └ repository_test.go                 4リポジトリ網羅（FTS検索可否・順序・cascade・COALESCE 等）
internal/config/                      ✅ Phase5 config.ts のLLM/embedding/debug部を移植（Settingsスナップショット）
  ├ config.go                          Editable/Settings, Defaults(env)/Load(app-config.json)/Get/GetEditable/UpdateEditable（RWMutex）
  └ config_test.go                     env フォールバック連鎖・override 適用・update 永続化の round-trip
internal/llmresponse/                 ✅ Phase5 lib/llmResponse.ts 移植（純関数・format引数化、config非依存リーフ）
  ├ llmresponse.go                     ParseAssistantResponse / SanitizePromptContent（llm_jp_thinking の reasoning 除去）
  └ llmresponse_test.go                standard / llm_jp_thinking の各分岐（endpoint不要）
internal/service/                     ✅ Phase5 backend/src/services/*.ts 全移植
  ├ service.go                         共有: stripTrailingSlash/getLmStudioAPIRoot/round2/sliceFromRune/collapseWhitespace/lastN/extractJSONObject/marshalJSONIndentNoEscape
  ├ llmclient.go                       LLMClient: createChatCompletion(Stream)、SSEパーサ(\n\n分割/[DONE]/usage)、★スライディングtimeout(AfterFunc+Reset)、metrics、listModels/listAvailable/ensureModelLoaded
  ├ embeddingclient.go                 EmbeddingClient: 失敗で自己disable(mutex)・null返し・batch、空modelで初期disabled→FTS only
  ├ retrieval.go                       RetrievalService(*sql.DB直): bm25(10,2,1,1,4 / 2,6,8)*-1、doc 0.55/0.45×category、mem 0.5/0.5、semantic=(cos+1)/2、intent判定/重み表、cosineはinternal/vectorでアプリ側計算
  ├ context.go                         ContextService: 明示doc/chat解決(NFKC一致)・quote判定・procedural memory4・promptContext組立・references(slice8)
  ├ chat.go                            ChatService: ストリーミング保存(空assistant→delta更新→finalize)、LLM失敗時referenceフォールバック、temporaryはmemory抽出せず
  ├ memory.go                          MemoryService: ルール抽出(DURABLE_CUES/質問除外/一時依頼除外)＋『覚えて』LLM抽出(失敗時ヒューリスティック)
  ├ summary.go                         SummaryService: updateSummary/generateChatTitle（LLM失敗時フォールバック）
  ├ review.go                          ReviewService: reviewMessage(Stream)、review用base/model、assembled.PromptContext使用
  ├ memoryorganizer.go                 MemoryOrganizerService: analyze(LLM→sanitizePlan / 失敗時dedupe fallback)、apply(locked不可触)
  ├ embeddingsync.go                   EmbeddingSyncService: syncDocument/syncMemories/rebuildAll、batch=32
  └ *_test.go                          SSEパーサ(httptest)/metrics/intent/scoring/ルール抽出/chat フォールバック end-to-end(DB+死んだLLM)/organizer dedupe
tools/segmenter-parity/               パリティ再生成ツール
  ├ gen_model.mjs                      tiny-segmenter のモデル → internal/search/model.go
  └ gen_golden.ts                      JS の出力 → internal/search/testdata/golden.json
```

検証済みの3大リスク（headless で確認できる範囲）:
1. **日本語トークナイザのパリティ** … JS と完全一致（最大リスク、解消）。
2. **pure-Go SQLite で FTS5 + bm25()** … 動作確認済み（modernc 採用確定）。
3. **SSE 逐次配信（HTTP レベル）** … `http.Flusher` で確認済み。**WebView での消費は GUI 必要（未確認）**。

### テスト・再生成コマンド
```bash
go test ./...                                              # 全テスト
go vet ./... && go build ./...
node tools/segmenter-parity/gen_model.mjs                  # tiny-segmenter 更新時に model.go 再生成
pnpm exec tsx tools/segmenter-parity/gen_golden.ts         # golden 再生成（要 backend が読める状態）
~/go/bin/wails version                                     # v2.12.0 インストール済み
```

ツールチェーン: Go 1.26.3 / Wails v2.12.0(`~/go/bin/wails`) / Node 22 / pnpm 10.30.3 / clang あり。
依存（確定）: `modernc.org/sqlite`, `golang.org/x/text`（NFKC）, （後で `github.com/google/uuid` を直接利用予定）。

---

## 2. 確定済みの技術判断と勘所（蒸し返さないこと）

- **SQLite ドライバ = `modernc.org/sqlite`（pure Go）**。driver 名は `"sqlite"`。
  DSN は `file:<path>?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)`。
  中身は transpile された SQLite 本体なので FTS5/bm25/WAL の挙動は better-sqlite3 と同一。
  `SetMaxOpenConns(1)` で単一接続（better-sqlite3 相当）。multi-statement Exec は modernc で動く（migration 001 で確認済み）。
- **FTS は事前トークナイズ方式**（最重要不変条件）。`*_fts` に入れる各テキスト列は必ず
  `search.BuildSearchText(...)` を通す。検索クエリは `search.ToFtsQuery(input)`。
  生テキストを直接 FTS に入れてはいけない（検索が静かに壊れる）。
- **正規表現の差**: JS の `\p{Script=Han}` は Go では `\p{Han}`（`\p{Hiragana}`/`\p{Katakana}`/`\p{L}`/`\p{N}` も対応）。NFKC は `golang.org/x/text/unicode/norm`。
- **データ保存先**: `os.UserConfigDir()` + `"snz-studio"`（mac `~/Library/Application Support/snz-studio`, win `%AppData%\snz-studio`）配下に `app.sqlite` / `uploads/` / `app-config.json`。起動時 `os.MkdirAll`。dev は env で上書き可。
- **dev の通信（⚠ 当初設計から是正・2026-05-31）**: Go API は dev=固定 `127.0.0.1:8787` / prod=空きポートで立てる。**dev/prod とも `window.__API_BASE__`（= `GetApiBase()` の絶対 URL）でフロントから直接 :8787 を叩く**。当初は「dev は `vite.config.ts` の `/api`・`/files` proxy で無改修」を想定していたが、**この前提は GET でしか成立しない**: `wails dev` の WebView は Wails dev アセットサーバ（origin `wails.localhost:34115`）からロードされ、その external asset handler は **GET だけ Vite にプロキシし、非 GET（POST/PATCH/DELETE）は 405 を返す**（wails v2.12.0 `pkg/assetserver/assethandler_external.go:64-77` / `assethandler.go:84-109`）。よって相対 URL ではミューテーションが :8787 に届かない。絶対 URL なら Wails dev サーバを迂回して直接 :8787 に届き（Spike #1 で SSE 込み実証済みの経路）、クロスオリジンは Phase6 の `withCORS`（Origin 反射＋OPTIONS 204）が処理する。Vite proxy は localhost:5173 をブラウザ直開きする場合のみ有効＝当面残置、Phase9 で整理。**実装済み（Phase 7 完了 2026-05-31）**: ライブ稼働中の `wails dev` サーバ(:8787)へクロスオリジン直叩きで GET 200 / POST create-chat のプリフライト OPTIONS が 204 + `Allow-Methods: ...POST...` を返すことを headless 検証済み（=405 機構の解消を HTTP 層で実証）。
- **パッケージング**: クロスコンパイル不可（mac は CGO+SDK 必須）。**GitHub Actions `macos-latest` + `windows-latest` マトリクス**。mac は Developer ID 署名 + notarytool、win は Authenticode + WebView2(`-webview2 download`)。

---

## 3. 次の手順（この順で進める）

### Phase 1 — Wails 足場 ✅ DONE（手書き、`wails init` 不使用）
確立した規約（蒸し返さないこと）:
- **dev/prod 切替 = Go ビルドタグ**。`wails dev` は OutputType=`dev` で `-tags dev` を付与、`wails build` は `desktop production`（dev タグ無し）。
  `env_dev.go`(`//go:build dev`) / `env_prod.go`(`//go:build !dev`) が `const isDev` と `apiListenAddr()` を提供。
- **API サーバ**: `app.go` の `startup` で `net.Listen` → goroutine で `http.Server.Serve`。dev=固定 `127.0.0.1:8787`（vite proxy 先）、prod=`127.0.0.1:0`（OS 採番、`ln.Addr()` から実アドレス取得）。
  `GetApiBase()`= dev は `""`（vite proxy 利用）/ prod は `http://<実アドレス>`。`shutdown` で graceful close。現状ルートは `/api/health` のみ（22 ルートは Phase 6）。
- **embed**: `main.go` の `//go:embed all:frontend/dist`。vite `outDir` を `dist`（=`frontend/dist`）に変更済み。`frontend/dist/.gitkeep` を追跡（fresh checkout で `go build` が通るため。`emptyOutDir` がローカルで消すのは想定内）。
- **monorepo**: `wails.json` の `frontend:*` は cwd=`frontend/`・スペース分割実行のため `pnpm -C .. run ...` 形式（シェル機能不可）。`frontend:dev:serverUrl="auto"` で vite URL 自動検出。`wailsjsdir=frontend/src` → 生成バインドは `frontend/src/wailsjs/`（gitignore 済み、`GetApiBase():Promise<string>` 生成確認済み）。
- **GUI 確認 ✅ 完了（2026-05-30）**: `wails dev` で SPA が完全描画（HashRouter・スタイル動作）。`/api/health`=200 が WebView→vite proxy→Go:8787 の疎通を実証（データ系ルートは Phase 6 まで 404 が正常）。**⚠ この疎通は GET 限定**: Wails dev サーバは GET しか Vite にプロキシせず非 GET は 405（§2「dev の通信」参照）。Phase 7 で絶対 `__API_BASE__` 直叩きに切替える。
  **Spike #1（SSE 逐次表示）も解消**: 使い捨て probe `GET /api/_sse_probe`（SSEWriter で 500ms 間隔の delta）を WebView devtools から**絶対 URL `http://127.0.0.1:8787` へ直 fetch**（=prod の `__API_BASE__` 経路）し、+16/+511/+1013/+1514/+2016ms で**逐次受信**を確認 → 即削除。WKWebView は flush を逐次 JS に渡す。ローカル `net/http`+SSE 構成で確定、Wails events 化の保険は不要。
- 補足: `build/darwin/Info.plist` の bundle id は既定 `com.wails.snz-studio` → 署名/notarization 前に Phase 8 で要変更。

### Phase 4 — Repository 層（`internal/repository/`）✅ DONE & TESTED（2026-05-30）
移植元: `backend/src/repositories/{project,document,memory,chat}Repository.ts`（4ファイル全 public メソッドを移植。embedding 系・rebuildSearchIndex・backfill も含む）。
確立した規約・勘所（蒸し返さないこと）:
- **リーフパッケージ分離**: `documentChunker`/`documentCategory` は TS では services/lib にあるが、Go で repository に直接置くと将来 service→repository の循環参照になる。よって純関数を `internal/chunk`・`internal/doccategory`・`internal/util`・`internal/model` に分離（いずれも repository より下層・他に依存しないリーフ）。依存方向は `repository → {model, util, chunk, doccategory, search, db}` のみ。
- **単一接続のトランザクション規律（最重要）**: `db.Open` は `SetMaxOpenConns(1)`。`*sql.Tx` が唯一の接続を占有するため、**tx 中の全文は `*sql.Tx` 経由**（共有ヘルパは `dbtx` インタフェース引数）。**読み取り結果を tx に渡す処理は、`rows.Close()` 後に `Begin()`**（さもないと自己デッドロック）。`rebuildSearchIndex`/`backfill` は全行を先読み→close→tx の順。
- **`SELECT *` 禁止**: TS は名前マップだが Go は位置スキャン。全クエリを**明示列**に書き換え（マイグレーション後の最終列順に厳密一致。例: documents は末尾に `category`、memories は末尾に `source, locked`、messages は `... response_ms, output_tokens, tokens_per_second, model_name`、chats は `... is_temporary, created_at, updated_at`）。
- ID = `util.NewID(prefix)` = `prefix + "_" + uuid.NewString()`（google/uuid を直接 require に昇格済み）。時刻 = `util.NowISO()`（ms 精度 UTC `Z`、JS `toISOString` 一致）。`tags_json` は `encoding/json`。
- nullable 列は `*T`（JSON で null 化）。スキャンは `sql.NullX → *T` 変換ヘルパ、バインドは `ptrArg[T]`（nil→SQL NULL）。
- `*_fts` upsert はすべて `search.BuildSearchText`。**注意: createDocument の検索本文 joinedSearchBody は `", "` 結合だが FTS `tags` 列は `" "` 結合**（TS で区切りが異なる）— 厳密に踏襲済み。
- updateMemory の `locked = COALESCE(?, locked)` は `*bool`（nil で現状維持）。`hasSimilarMemory` は **Jaccard ではなく** `lower(title)=lower(?) AND lower(content)=lower(?)` の完全一致（SQLite `lower()` は ASCII のみ＝日本語は実質ノーオプ、TS と同じ）。← 旧 HANDOFF の「Jaccard 類似」は誤り。
- parity 補足: chunker/category の `slice`・`length` は JS=UTF-16 / Go=rune。対象は日本語 BMP のため一致（astral 文字のみ差、各 .go 冒頭コメント参照）。doccategory の `\s` は JS の Unicode 空白に合わせ `[\s\p{Z}]` に補正（全角空白 U+3000 のヘディング検出）。
- 返り値規約: 単一取得/更新の not-found は `(nil, nil)`、reorder の ID 不一致は `ErrProjectReorderMismatch`（HTTP 層で 404/400 振り分け用）、delete は `(*Record or bool, error)`。

### Phase 5 — Service 層（`internal/service/`）✅ DONE & TESTED（2026-05-31）
移植元: `backend/src/services/*.ts` 全11ファイル + `lib/llmResponse.ts`。下記の要点はすべて踏襲済み。

確立した規約・勘所（蒸し返さないこと）:
- **設定 = `internal/config`（不変スナップショット）**: TS は mutable な module singleton を全 service が live 参照していた。Go は並行実行のため `Config.Get() Settings`（RWMutex下のコピー）を各リクエスト冒頭で取得する方式に変更。編集7項目は `UpdateEditable`（app-config.json へ 2space+末尾改行で永続化）。port/dataDir 等インフラ設定は config に入れず app.go/env_*.go の管轄（Phase6/8）。clients は `cfg` を保持し毎回 `Get()`。
- **`internal/llmresponse` をリーフ化**: `parseAssistantResponse`/`sanitizePromptContent` は TS では config 直読みだが、Go では format を引数化して config 非依存の純関数に（endpoint 無し単体テスト可）。RE2 はlookahead無のため llm_jp の noAnalysis ブロックは end-of-string まで除去（到達条件＝タグ無し入力なので実質no-op、コメント参照）。`\s*`→`[\s\p{Z}]*`（doccategory と同じ補正）。
- **LLM クライアント（★最重要）**: `fetch`+AbortController → `net/http`+`context`。**非ストリーム=`context.WithTimeout`、ストリーム=`context.WithCancel`+`time.AfterFunc(timeout, cancel)` を各 read で `timer.Reset` するスライディング方式**（TS の resetTimeout 等価）。SSE は生バイトを蓄積し `\n\n` で分割（TS と同じく "\n\n" 固定。\r\n\r\n は分割しない）、各チャンクを `/\r?\n/` 分割→`data:` 行抽出→`trimStart`→`[DONE]` 終端→`choices[0].delta.content`/`usage.completion_tokens`。**可視デルタは `sliceFromRune(parse(rawAll), runeLen(prevVisible))`**（TS の `next.slice(prev.length)` を rune 基準で再現＝BMPパリティ）。malformed JSON はストリーム中断（TS の throw 相当）。temperature 既定 0.25 は `*float64` の nil で表現。
- **metrics**: `estimateTokenCount`=`max(wordLike, ⌈jp/1.8⌉, ⌈runeLen/4⌉)`、`buildGenerationMetrics`=elapsed の 1ms フロア / usage 優先 / tps=`round2(tokens/(ms/1000))`。length は rune 基準（UTF-16 と BMP一致）。
- **EmbeddingClient**: 失敗で `disabled=true`（mutex 保護、warn 1回）→以後 nil 返し。空 model で初期 disabled → retrieval は FTS only に degrade（**endpoint 無しでもテスト/動作可**）。`CreateEmbeddings` は error を返さず nil（TS の null 相当）。
- **RetrievalService は `*sql.DB` 直**: TS が better-sqlite3 ハンドルを持っていたのと同様、bm25 候補クエリと embedding 取得は raw SQL。**単一接続規律**: 各 read は slice に全部読み→close→次クエリ（カーソル開いたまま次を投げない）。**cosine は SQL でなく `internal/vector` でアプリ側計算**。bm25 重み doc=`10,2,1,1,4`/mem=`2,6,8`（×-1）、doc ハイブリッド=`fts*0.55+sem*0.45×categoryWeight(intent)`、mem=`fts*0.5+sem*0.5`、semantic=`(cos+1)/2` クランプ。intent 判定/category 重み表は retrievalService.ts 完全移植。**Map 挿入順＋安定ソート**で JS の stable sort + Map 反復順を再現（`sort.SliceStable` + `order []string`）。doc の no-embedding 時は `ftsScores[i] || rawScore`（0なら raw bm25）の JS truthy 挙動も踏襲。
- **AssembledContext.References は `[]model.SearchReference`（base のみ）**: TS は document ref が RetrievedDocumentReference のまま references に混ざるが、フロント(`ReviewReference`/`AssistantReference`)が消費するのは base 5項目だけ＆永続化も base のみ。よって Go は base に平坦化（promptContext 用の chunk 等は `normalizedDocumentRefs []RetrievedDocumentReference` をローカル保持）。
- **ChatService ストリーミング保存**: 空 assistant message → onDelta 毎に `UpdateMessageContent`（best-effort, error 無視＝TS同様）→ `FinalizeMessage` で metrics/references/summary/title 確定。LLM 失敗時は reference 抜粋フォールバック（metrics は nil）。temporary chat は memory 抽出スキップ。`prepareTurn` 内で systemPrompt 構築（explicitMemory/quote/temporary/llm_jp 分岐）。
- **MemoryService**: `splitSentences` は TS の lookbehind split `/\n|(?<=[.!?。！？])/` を手書き再現（newline は消費、terminator は前文に付随、空片は length≥10 フィルタで除去）。`generateMemoryTitle` は `\s+→" "` 折り畳み**後**に `[。！？.!?\n]` 分割するため改行は区切りにならない（TS の順序どおり）。長さ比較は rune。『覚えて』抽出は LLM/JSON 失敗のみフォールバックへ（repository error は伝播）。
- **MemoryOrganizer**: prompt の JSON は `JSON.stringify(_,null,2)` 相当（`SetEscapeHTML(false)`+2space）。`sanitizePlan`/`buildFallbackPlan`/`applyProjectPlan` 移植。locked は update/remove 不可。
- **ロギング**: TS の `config.debug*` ゲート付き verbose トレースは観測専用なので省略。無条件の warn（LLM 失敗・embedding self-disable）のみ標準 `log` で残置。
- **テストできる範囲**: SSE パーサ/metrics/intent/scoring/ルール抽出/title 生成/config round-trip は endpoint 無しで単体テスト済み。chat フォールバック経路・retrieval(FTS)・organizer dedupe は **DB（temp sqlite）+ 死んだ LLM(httptest 500/未接続)** で end-to-end テスト済み。**LLM/embedding が実際に動く chat/stream・review・semantic retrieval の正常系 end-to-end は LM Studio 等が必要（未確認、§5）**。

---
旧メモ（移植時の指針。済）:
- **llmClient.ts（★最重要）**: OpenAI 互換。`createChatCompletion` / `createChatCompletionStream`。
  SSE 受信は `data:` 行を `\n\n` で分割、`[DONE]` 終端、`choices[0].delta.content` を抽出、`usage.completion_tokens`。
  **タイムアウトはチャンク毎にリセット（スライディング）**。temperature 既定 0.25。`fetch`+AbortController → Go は `net/http`+`context`+`bufio.Scanner`。
  LM Studio 用の `listAvailableModels`/`ensureModelLoaded` あり。
- **embeddingClient.ts**: 失敗で自己 disable、`null` 返す。バッチ embedding。
- **retrievalService.ts**: ハイブリッド検索。移植時の**不変条件（必ず一致させる）**:
  - doc FTS: `bm25(document_chunks_fts, 10.0, 2.0, 1.0, 1.0, 4.0) * -1`
  - memory FTS: `bm25(memories_fts, 2.0, 6.0, 8.0) * -1`
  - doc ハイブリッド: `ftsScore*0.55 + semanticScore*0.45`、さらに `× categoryWeight(intent)`
  - memory ハイブリッド: `ftsScore*0.5 + semanticScore*0.5`
  - semantic スコアは `(cosine + 1) / 2` を [0,1] にクランプ（`semanticToUnitRange`）
  - intent 判定（translation/plot/writing/reference/general）と category 重みテーブルは
    `retrievalService.ts:39-130` をそのまま移植（正規表現も）。
  - `searchDocuments(limit=4, chunksPerDocument=3)` / `searchMemories(limit=4)` / `searchChunksInDocument(limit=5)`。
  - cosine は SQL ではなく**アプリ側**で計算（embeddings は JSON 文字列で保存）。`internal/vector` 利用。
- **contextService.ts**: コンテキスト組み立て（project/summary/recent6/procedural memories4/explicit doc・chat 参照/quote 判定）。
- **chatService.ts**: ターン進行。**ストリーミング保存方式**=開始時に空 assistant message を作成→delta 毎に `messages.content` を更新→完了時に final content / response_ms / output_tokens / tokens_per_second / model_name / references / summary を確定。LLM 失敗時は reference 抜粋でフォールバック。
- **memoryService.ts**: 自動抽出（rule ベース、question/長文/一時依頼を除外）＋明示 `覚えて` トリガー（LLM 抽出）。temporary chat では抽出しない。
- **summaryService.ts / reviewService.ts / memoryOrganizerService.ts / embeddingSyncService.ts / documentChunker.ts（1000字/150 overlap, 行境界尊重）**。
- **lib/llmResponse.ts**: `llm_jp_thinking` 時に reasoning タグ除去 → final answer のみ保存。`lib/documentCategory.ts`（自動推定）も移植。

### Phase 6 — HTTP API（`internal/httpapi/`）+ Wails 結線 ✅ DONE & TESTED（2026-05-31）
移植元: `backend/src/index.ts`（22 ルート）。**標準 `net/http` のみ**（method+pattern ServeMux、Go1.22+。`chi` 等の新規依存は不採用）。
確立した規約・勘所（蒸し返さないこと）:
- **service グラフは `httpapi.NewServer(db, cfg, uploadDir)` が構築**（app.go で組まず httpapi に集約＝temp DB でのエンドポイントレステストが容易）。順序は HANDOFF 既述どおり（repos → emb/llm → retrieval → embeddingSync → context → summary/memory → chat/review/organizer）。app.go の startup は `resolveDataPaths()` → `os.MkdirAll`（dataDir/uploadDir）→ `db.Open(sqlitePath)`（マイグレーション込み）→ `config.Load(appConfigPath)` → `NewServer` → `http.Server{Handler: srv.Handler()}` → **`srv.RunStartupTasks()` を Serve 前に同期実行**（backfill+FTS再構築。半再構築の index を配信しないため。embedding rebuild だけ goroutine）→ goroutine で `Serve`。shutdown で server.Shutdown + db.Close。
- **ルーティング = enhanced ServeMux**。`"GET /api/projects/{projectId}"` 形式、`r.PathValue("projectId")` で取得。メソッド不一致は mux が 405 を返す。CORS は `withCORS` ミドルウェアで mux をラップし、**Origin をそのまま反射**（loopback 専用なので安全。dev=vite 5173 / prod=WebView origin の両対応。OPTIONS は 204 で短絡）。
- **JSON 入出力ヘルパ**: レスポンスは `writeJSON`（`enc.SetEscapeHTML(false)`＝`JSON.stringify`/`res.json` とバイト一致）/ `writeError`（`{error}`）/ `fail`（500）。リクエストボディは TS の `req.body?.x` 動的アクセスを忠実再現するため **`decodeBody` で `map[string]any` にデコード**し、`bodyString`（`String(x)` 相当の coercion）/`bodyBool`（`typeof===="boolean"` 判定で 400 振り分け）/`stringifyJSONValue` で取り出す。空ボディは空 map（エラーにしない）、壊れた JSON は 400。**例外**: organize/apply の `plan` だけは `model.MemoryOrganizationPlan` への型付きデコード（nil/失敗→400 "plan is required"）。
- **設定 GET/PUT**: connected 3項目は `checkConnections` が `llm.CheckConnection("","")` / `llm.CheckConnection(reviewBaseURL, reviewModel)` / `emb.CheckConnection()` を **goroutine 3並行**で実行（死活チェックは最大 timeout ブロックするため Promise.all 相当の並行が必須）。レスポンス形は埋め込み `config.Editable` + `*Connected` 3 bool の `workspaceConfiguration`（client.ts:108-119 一致）。PUT は trim→空 review/embedding は llm 値で補完→`UpdateEditable`→**`emb.RefreshConfiguration()`**→ensureModelLoaded×3 並行→enabled なら async `RebuildAll`→connected 再取得。`llmBaseUrl` 空は 400。
- **manual memory のタイトル生成**: index.ts はこの関数を memoryService と二重定義していたが、Go は単一ソースにするため **`service.GenerateMemoryTitle(content, kind)` を export して HTTP 層から呼ぶ**（重複ドリフト防止）。`generateMemoryTitle` は見出し除去→`collapseWhitespace`（改行も空白に畳む）**後**に文区切りするため、`# 見出し\n本文` は「見出し 本文」に連結される（index.ts と一致＝正しい挙動。test 参照）。
- **SSE**: `NewSSEWriter(w)` は成功時に 200+ヘッダを即 flush。`delta`=`{content}`、chat done=`{chat,messages,summary}`、review done=`{review,references}`、error=`{message}`（フロント ChatPage.tsx の消費キーに一致）。`chat.SendMessageStream` は LLM 失敗でも fallback を返す（error なし→done）、`review.ReviewMessageStream` は LLM 失敗を error 伝播（→ヘッダ送出済みのため error イベント）。content 空は SSE 開始前に通常 400。
- **§4 のファイル unlink を実装（完了）**: document delete / project delete 時、repository の DB cascade に加え HTTP 層の `unlinkFile`（`/files/` prefix の実体を `os.Remove`、無ければ無視）。project delete は cascade 前に `documents.ListByProject` で対象ファイルをスナップショット。
- **アップロード**: `r.ParseMultipartForm(32MB)` → `r.FormFile("file")`（`http.ErrMissingFile` で未添付判定）。image は `uuid.NewString()+ext` で uploadDir に保存、`filePath=/files/{name}`・`mimeType=part の Content-Type`。非 image+file は本文を utf8 読みして contentText。title は `title || originalname || truncate(先頭行 || "Untitled document", 80)`。category は `doccategory.IsValid` 不通過なら空（repository が推定）。作成後 `embSync.SyncDocument(id)`。`/files/{name}` 配信は `http.ServeFile`＋単一セグメント＋セパレータ拒否で traversal 防止。
- **テスト範囲（handlers_test.go）**: 全ルートを `httptest` でエンドポイントレス検証。死んだ LLM（`127.0.0.1:1`）＋embedding 無効（空 model）で **chat send/stream の fallback・review の 500/error イベント・organize の dedupe fallback** まで end-to-end 通過。**LLM/embedding 実動の正常系（実 delta 表示・semantic retrieval・実 review）は LM Studio 等が必要＝未確認（§5 据え置き）**。

### Phase 7 — フロント微修正 + dev 通信の是正（全量・最小）✅ DONE（2026-05-31, GUI 起動 + transport headless 検証済 / WebView 操作系は手元確認待ち）
**前提（必読）**: `wails dev` の新規チャット作成（POST）が **405** になる事象を §2「dev の通信」で要因確定済み（Wails dev アセットサーバは GET のみ Vite プロキシ、非 GET は 405）。よって相対 URL（Vite proxy 依存）ではなく **dev/prod とも絶対 `window.__API_BASE__` で直接 :8787 を叩く**方式に統一する。CORS は Phase6 の `withCORS` が対応済み、SSE 直叩きは Spike #1 で実証済み。

確立した規約・勘所（蒸し返さないこと）:
- **dev/prod 通信を絶対 URL に統一**: `app.go` の `a.apiBase` は dev/prod とも `"http://" + ln.Addr().String()`（dev=127.0.0.1:8787 固定 / prod=ephemeral 実アドレス）。当初の dev 空文字（Vite proxy 依存）は廃止。`GetApiBase()` は常に絶対 URL を返す。
- **フロント起動シーケンス**: `main.tsx` は React レンダー前に `bootstrap()` で `GetApiBase()` を await→`window.__API_BASE__` に一度だけ設定→`HashRouter`（必須・`BrowserRouter` から変更）でレンダー。`window.go` が無い環境（ブラウザで Vite を直開き）では catch して `""` フォールバック（=相対 URL→Vite proxy）。型は新規 `frontend/src/global.d.ts` で `Window.__API_BASE__?: string` を宣言。
- **URL 前置箇所（5）**: `api/client.ts` の `request()`（string input のみ前置、`Request` はそのまま）、`ChatPage.tsx` の 2 ストリーム fetch（messages/stream・review/stream、**SSE パースは無変更**）、`ProjectDetailPage.tsx` の `<img src>`。いずれも `${window.__API_BASE__ ?? ""}` 前置。grep で他に raw fetch / `/api` / `/files` リテラルが無いことを確認済み。
- **`vite.config.ts` は無変更**: `/api`・`/files` proxy はブラウザ直開き(localhost:5173)用に残置（Phase9 で整理）。
- **検証結果**: `gofmt` クリーン / `go vet ./...`・`go vet -tags dev ./...`・`go build ./...`・`go test -race ./...` 全グリーン / `tsc --noEmit`（check:client）エラーなし / `vite build`（build:client）成功。`wails dev` は **Wails CLI 経由で dev ビルドが正常コンパイル＆リンク**（"Compiling application: Done."、後述の UTType リンク問題は出ない）し GUI 起動。ライブ :8787 へクロスオリジンで GET 200（Origin 反射）/ POST create-chat プリフライト OPTIONS が 204 + `Allow-Methods` に POST を含むことを headless 確認＝**405 機構の解消を実証**。
- **⚠ 既知の無害事項（Phase8 で留意）**: plain `go build -tags dev ./...` は **リンク段階**で `_OBJC_CLASS_$_UTType`（`UniformTypeIdentifiers` フレームワーク未リンク）で失敗する。ただし (1) 型検査は通る（`go vet -tags dev` 緑）、(2) **app.go の変更を revert しても同じく失敗＝既存事象・Phase7 とは無関係**、(3) `wails dev`/`wails build` の CLI 経由は正しくフレームワークを付与しリンク成功。よって dev/prod バイナリは wails CLI でビルドすること（plain `go build -tags dev` は dev バイナリの正規ビルド手段ではない）。prod の `go build .` は単体でもリンク成功。
- **手元確認待ち（WebView 操作系）**: 実 WebView でのクリック操作＝(a) 新規チャット作成 POST が UI で成功、(b) チャット送信の SSE 逐次表示、(c) 画像ドキュメントの `<img>` 表示、(d) LM Studio 接続での §5 正常系 end-to-end（設定 PUT→connected=true→semantic retrieval・実 review）。transport/CORS/ビルドは検証済みなので残るは UI 操作のみ。**注意: 既に起動中の `wails dev` で確認する場合、`main.tsx`(エントリ) の変更が HMR で完全反映されていない可能性があるため、WebView を一度ハードリロード（または `wails dev` 再起動）してから確認すること。**

1. **`app.go`**: dev 分岐の `a.apiBase = ""` を廃止し、**dev/prod とも `a.apiBase = "http://" + ln.Addr().String()`** に統一（dev は `127.0.0.1:8787` 固定、prod は ephemeral 実アドレス）。
2. **`frontend/src/main.tsx`**: React レンダー**前**に Wails バインド `GetApiBase()`（`frontend/src/wailsjs/go/main/App` から import、型 `(): Promise<string>` 生成済み）を await し `window.__API_BASE__` に一度だけ設定。あわせて `BrowserRouter` → `HashRouter`（必須）。`window.__API_BASE__` の型宣言（`global.d.ts` 等）を追加。
3. **`frontend/src/api/client.ts`** の `request()`: URL 先頭に `(window.__API_BASE__ ?? "")` を前置。
4. **`frontend/src/pages/ChatPage.tsx`**: 2つのストリーミング fetch URL（messages/stream・review/stream）に `__API_BASE__` を前置。**SSE のパースロジックは無変更**。
5. **`frontend/src/pages/ProjectDetailPage.tsx`**: `<img src={filePath}>` に `__API_BASE__` を前置。
6. **`frontend/vite.config.ts`**: `outDir` は調整済み。`/api`・`/files` proxy は WebView 経路では不要になるが、localhost:5173 をブラウザ直開きする場合用に当面残置（Phase9 で整理）。

**検証（手元 GUI 環境）**: `wails dev` で (a) 新規チャット作成（POST）が成功＝405 解消、(b) チャット送信が SSE 逐次表示、(c) 画像ドキュメントの `<img>`（/files 経由）が表示、(d) `pnpm -C frontend ... build:client`・`go build`/`go test ./...` が緑。LM Studio 稼働中なら設定画面（PUT /api/configuration）で LLM=`http://192.168.0.219:1234/v1`(gpt-oss-20b)・embedding=`http://192.168.0.219:7997/v1`(ruri-v3-130m) を設定し、connected=true と **semantic retrieval・実 review の正常系 end-to-end（§5 の積み残し）**もここで初確認できる。

### Phase 8 — データ移行 + パッケージング ✅ DONE & TESTED（2026-05-31, ローカル wails build まで実機確認 / 素の .app 起動による移行 GUI 確認は手元待ち）
確立した規約・勘所（蒸し返さないこと）:
- **移行は `SNZ_MIGRATE_FROM` 環境変数で明示駆動（ユーザー承認の確定方針）**。旧 Node は `data/` を **cwd 相対**（`backend/src/config.ts`: `process.cwd()/data`）に置いていたため、packaged app から辿れる「絶対の旧既定パス」は存在しない＝自動探索は原理的に不安定。よって移行元は env で明示。`maybeSeedDataDir` は env 未設定なら no-op、設定時のみ `seedDataDir(src, dataDir)` を呼ぶ。**dev は dataDir=cwd/data が既に app.sqlite を持つため、たとえ env を設定しても dest ガードで自動 no-op＝実質 prod 専用・初回専用**。
- **冪等＆非破壊ガード（2 段）**: (1) `dest/app.sqlite` が在れば no-op（＝初期化済み環境を絶対に上書きしない・二度目以降も走らない）、(2) `src/app.sqlite` が無ければ no-op（＝新規インストールはクリーン起動）。`app.go` startup の `MkdirAll(dataDir)` 直後・`db.Open` 直前に実行（コピー済み DB を開いて FTS 再構築させるため順序が重要）。
- **DB コピー = `VACUUM INTO`（WAL 安全の核）**: source を `file:<path>?mode=ro&_pragma=busy_timeout(5000)` で **read-only** に開き `VACUUM INTO '<dest>'`。SQLite は read-only source からの VACUUM INTO を公式サポートし、**checkpoint 済みの単一ファイル**を吐く＝source の `-wal` に残った未 checkpoint 行も取り込み、生ファイルコピーで起きる WAL 取りこぼしを構造的に回避。**source は一切変更されない**（実データ `data/`=6,291,456B が migration 後も同サイズ・`-wal`/`-shm` 生成なしを確認）。dest は VACUUM により compact 化（実データで 6.29MB→5.25MB、行数は projects5/documents25/memories4/chats14/messages73 で完全一致）。`-wal`/`-shm` の 3 点セット手動コピーは不採用（VACUUM INTO が上位互換）。
- **uploads/ は再帰コピー（regular file のみ・symlink等スキップ）、app-config.json は dest 欠落時のみコピー**（いずれも WAL 非依存なので素のファイルコピーで正）。
- **起動時 FTS 再構築は Phase6 の `RunStartupTasks` が既に同期実行**（移行後は自動で走る）。embedding rebuild は非同期。Phase8 で追加実装は不要。
- **bundle id = `net.serenebach.snz-studio`（ユーザー確定）**: `build/darwin/Info.plist` と `Info.dev.plist` の `CFBundleIdentifier` を既定 `com.wails.{{.Name}}` から置換。`wails.json` の author email も `takuya.otani@serenebach.net` に更新。`wails build`（prod・darwin/arm64）で生成 `.app` の `CFBundleIdentifier`＝`net.serenebach.snz-studio`、self-sign Identifier も一致を確認（`TeamIdentifier=not set`＝Developer ID 署名は後送り）。
- **ローカル wails build 疎通確認済み**: `wails build`（prod）で "Compiling application: Done." → `.app` 生成・self-sign 成功。**Phase7 の UTType リンク懸念は prod build でも再現せず**（wails CLI がフレームワークを付与）。
- **CI = `.github/workflows/build.yml`（雛形・未実行）**: `macos-latest`(darwin/universal→`.app`+`hdiutil` で dmg) / `windows-latest`(windows/amd64・`-nsis -webview2 download`・NSIS は choco 導入) のマトリクス。setup-go(go.mod) + pnpm + node22、`go install wails@v2.12.0`、GOPATH/bin を PATH へ。**トリガは tag `v*` と workflow_dispatch**。**署名/notarization は secrets ゲートで後送り**（YAML 内に手順・必要 secrets 名をコメント明記: mac=APPLE_CERT_P12_BASE64/…/APPLE_TEAM_ID, win=WINDOWS_CERT_PFX_BASE64/…）。当環境では Actions 実行不可のため **YAML 構文検証のみ（actionlint 未導入）＝CI 実走は次セッション/実際の push で要確認**。
- **ルート整理（root に Go を置かない方針）**: ルート直下の `.go` は **`main.go` と `app.go` の 2 つだけ**に集約。起動時インフラ（旧 `paths.go`/`migrate.go`/`env_dev.go`/`env_prod.go`）は **`internal/bootstrap`（package bootstrap）へ移動**し、`ResolveDataPaths`/`Paths`/`MaybeSeedDataDir`/`IsDev`/`ListenAddr` を export して app.go から呼ぶ。**Wails のハード制約（蒸し返さない）**: (1) `wails build` は `wails.json` のあるルートで**パッケージ引数なしの `go build`** を実行する（wails v2.12.0 `pkg/commands/build/base.go:286-293`, `cmd.Dir=projectData.Path`）＝**main パッケージはルート必須**、(2) `//go:embed all:frontend/dist` は embed パスが相対かつ `..` 禁止のため**ルートの .go にしか書けない**。よって main.go はルートから動かせない。`App`（バインド対象）も main パッケージに残すことで生成バインドは `wailsjs/go/main/App` のまま＝**フロント無改修**（`main.tsx` の import 変更不要）。`wails build` 後にバインド名前空間・bundle id・リンク成功を再確認済み。ユーザーデータディレクトリ（`~/Library/Application Support/snz-studio`）に DB/uploads/app-config が seed され、既存のチャット/文書/記憶が表示されること、および §5 の LM Studio 正常系。`open` は env を渡しにくいので確認時はバイナリ直叩き例: `SNZ_MIGRATE_FROM="$PWD/data" "build/bin/snz-studio.app/Contents/MacOS/SNZ Studio"`。

### Phase 9 — クリーンアップ ✅ DONE & TESTED（2026-05-31）
コミット: `chore: remove legacy Node backend and prune Node deps (Phase 9)` / `docs: update README/spec/AGENTS for the Wails+Go desktop app (Phase 9)`。
確立した規約・勘所（蒸し返さないこと）:
- **旧 `backend/` を全削除（29 ファイル）**。Go 層は移植元 `backend/src/*.ts` を**コメント（出典）でのみ**参照しており、削除してもビルド/テストに影響しない（go build/vet/test 全グリーンで確認済み）。各 .go の `// ... ports backend/src/...` コメントは履歴的出典として残置（削除は無益なチャーンのため）。
- **`package.json` 整理**: Node 本番依存（`better-sqlite3`/`cors`/`express`/`multer`）+ 各 `@types` + `concurrently`/`tsx` を削除。フロントのビルド系（`vite`/`typescript`/`@vitejs/plugin-react`/`@types/react(-dom)`/react 系）は残置。**`tiny-segmenter` は devDependencies へ降格**（`tools/segmenter-parity/gen_model.mjs` の model.go 再生成専用に必要なため）。`@types/node` は削除（フロント tsc は `types:["vite/client"]` のみで node 型不要＝`check:client` 緑で確認）。スクリプトは `dev:client`/`build:client`/`check:client` のみ残す（`wails.json` が `build:client`/`dev:client` を呼ぶため**この2名は改名禁止**）。`dev`/`dev:server`/`build`/`build:server`/`check`/`check:server`/`seed` は削除。`pnpm install` で lockfile 再生成（161 パッケージ prune）。`pnpm-workspace.yaml` の `onlyBuiltDependencies` から `better-sqlite3` を除去（`esbuild` は残す）。
- **`tools/segmenter-parity/gen_golden.ts` を削除（ユーザー承認）**: 削除した `backend/src/lib/searchText.ts` を import していたため。**golden は `internal/search/testdata/golden.json` に凍結済みで `searchtext_test.go` は引き続き通る**（gen_golden 削除後も `go test ./internal/search` 緑で実証）。`gen_model.mjs`（model.go 再生成）は `tiny-segmenter` のみ依存で backend 非依存＝無影響（再生成して byte-identical を確認済み）。**FTS パリティの JS リファレンス再生成手段は失った**点に留意（必要なら過去コミットの searchText.ts を tools/ へ退避して復元可能）。
- **vite proxy 削除（ユーザー承認＝wails dev 専用方針）**: `frontend/vite.config.ts` の `/api`・`/files` proxy を削除。dev/prod とも `GetApiBase()` の絶対 URL 直叩きなので不要。**ブラウザ直開き(localhost:5173)での開発は廃止**（`main.tsx`/`global.d.ts` の `__API_BASE__` 空フォールバック経路は残置するが proxy が無いので非 GET は届かない）。`server.port=5173` は wails の `serverUrl:auto` 検出先として残置。
- **`.env.example` 補正**: `PORT`/`APP_ORIGIN` を削除（Go は読まない＝port は bootstrap が採番・CORS は Origin 反射）。`DATA_DIR`/`SQLITE_PATH`/`UPLOAD_DIR`/`LLM_*`/`REVIEW_*`/`EMBEDDING_*`/`DEBUG_*` は Go config/bootstrap が引き続き honor するため残置。
- **docs 更新（Wails/Go 構成へ）**: `README.md`（起動手順 `pnpm dev`→`wails dev`/`wails build`、`internal/` レイヤ地図、データ移行 `SNZ_MIGRATE_FROM`、app-config パスと DEBUG_* 注記の補正）、`docs/current-spec.{ja,}.md`（技術構成/storage/process-model を Go+net/http+modernc+ネイティブ WebView へ、「desktop app 化での論点」を確定方針へ）、`AGENTS.{,ja.}md`（tech preferences を Go バックエンド+Wails へ、retrieval 層を `internal/service/retrieval.go` へ）。Go コード内の出典コメントは未変更。
- **検証（全グリーン）**: `gofmt`（model.go 除く）クリーン / `go vet ./...`・`go vet -tags dev ./...` / `go build ./...`・`go build .`（prod main リンク）/ `go test -race ./...` / `pnpm check:client`（tsc）/ `pnpm build:client`（vite build）/ `gen_model.mjs` 再生成で model.go byte-identical。`frontend/dist/.gitkeep` は vite の `emptyOutDir` が消すので **commit 前に `git checkout` で復元必須**（go:embed 解決のため追跡継続）。
- **未対応（手元 GUI / 証明書が要る・本セッション範囲外）**: §5 の「GUI 実機確認」と「CI 実走/署名」。Phase 9 はコード/依存/docs の整理のみ完了。

---

## 4. 移植時の不変条件チェックリスト（parity invariants）

- [x] `*_fts` 投入テキストは必ず `search.BuildSearchText`、クエリは `search.ToFtsQuery`。（repository=Phase4 / retrieval=Phase5）
- [x] bm25 重み: doc `10,2,1,1,4`、memory `2,6,8`、いずれも `*-1`。（retrieval.go）
- [x] ハイブリッド: doc `0.55/0.45`×category、memory `0.5/0.5`。semantic=`(cos+1)/2`。（retrieval.go）
- [x] ストリーミング保存: 空 assistant → delta 更新 → 完了時メトリクス確定。（chat.go）
- [x] temporary chat は memory 自動抽出しない（`覚えて` も無効、organizer 対象外）。（chat.go: IsTemporary 分岐）
- [x] `locked=true` の memory は organizer が触らない。（memoryorganizer.go）
- [x] `llm_jp_thinking` は reasoning 除去後の final answer のみ保存。（llmresponse.go + llmclient.go の可視デルタ）
- [x] LLM ストリームのタイムアウトはチャンク毎リセット。失敗時は reference 抜粋フォールバック。（llmclient.go / chat.go）
- [x] cascade delete とファイル削除（document/project 削除時に `/files` の実体を unlink）。← cascade=Phase4 / ファイル unlink=Phase6 `httpapi.Server.unlinkFile`（handlers.go）で完了。

---

## 5. 未解決リスク / 要・手元環境

- ~~**WebView での SSE 逐次表示**（Spike #1）~~ → ✅ **解消済み（2026-05-30）**。WKWebView から loopback Go への直 fetch で SSE が逐次描画されることを実機確認（詳細は §3 Phase 1）。Wails events 化の保険は不要。
- **署名・notarization・CI（2026-05-31 更新）**:
  - **mac ローカル署名は確立・実証済み**: `scripts/build-mac-signed.sh`（コミット `35a0fa1`）が build → Developer ID 署名（hardened runtime + secure timestamp）→ dmg → `notarytool submit --wait` → `stapler staple` → 検証まで一括実行。**arm64 / darwin/universal（x86_64+arm64）の両方で notarization Accepted・`spctl ... source=Notarized Developer ID` を確認済み**＝配布可能 dmg をローカル生成可能。証明書 = `Developer ID Application: Yoko Otani (9EYB4D9GGQ)`（個人 serenebach アカウント・Team ID `9EYB4D9GGQ`）。notary profile 名 = `snzstudio`（`xcrun notarytool store-credentials` 済み）。⚠ スクリプトは macOS 同梱 bash 3.2 で動くよう空配列展開を `"${arr[@]+...}"` にしてある（蒸し返さない）。
  - **CI（`.github/workflows/build.yml`）は手動実行のみに変更**（コミット `6224dd5`）: private リポジトリの課金（macOS は分の 10倍消費）を避けるため `push: tags` トリガを外し `workflow_dispatch` 限定に。未署名アーティファクト生成自体は可能だが **Actions 実走は未実施**（特に win の NSIS/WebView2、mac universal+dmg は本番ランナーで要確認）。
  - **Windows 署名は未対応（ユーザー判断で当面保留）**: 無料の公式手段なし。2023/06 以降 OV/EV はハードウェアトークン/HSM 必須で `.pfx`-in-secrets が不可。CI 親和の最安は **Azure Trusted Signing（〜$10/月・要審査）**、専用 Action で署名する形。当面は **Windows 未署名（SmartScreen 警告をクリック回避）**で運用方針。
  - **CI 署名の実装（未）**: mac は上記 Developer ID を `.p12`→base64 secret 化すれば build.yml の guarded ステップで CI 署名可能（ローカルで dmg を作れるので必須ではない）。win は証明書/サブスク契約後。
- **GUI 正常系（Phase 7 積み残し）= ✅ ユーザー確認済み（2026-05-31）**: `wails dev`（実データ `./data`）でアプリ起動・操作。ローカル LM Studio（`127.0.0.1:1234`・モデルロード済み）に対しチャット送信が **mode=stream で正常応答**（dev サーバログに `[chat] completion start ... mode=stream`、runtime エラー/panic なし）。proxy 削除後も絶対 URL 直叩きで疎通。新規チャット POST の 405 は解消済み。**補足**: 既定 LLM endpoint は `127.0.0.1:1234/v1`（Go は `.env` 非読込）。LM Studio 未起動だと `chat.go` の fallback 定型文が返る（仕様）。`window.__API_BASE__` に出る `127.0.0.1:<ephemeral>` はアプリ自身の API ポート（GetApiBase）で LLM endpoint ではない。
- **データ移行の GUI 確認（Phase8 積み残し・未確認のまま）**: 移行ロジックは実データ headless 検証済み。配布用 `.app`/署名 dmg はビルド済みなので、**`SNZ_MIGRATE_FROM="$PWD/data" "build/bin/snz-studio.app/Contents/MacOS/SNZ Studio"`** で初回起動すれば `~/Library/Application Support/snz-studio` へ seed される（⚠ dest に `app.sqlite` があると no-op＝先に素で起動すると移行されない。リセットは `rm -rf` dest）。**「seed 後 UI に既存データが出る」end-to-end のユーザー目視確認はまだ取れていない**。
- **embedding/semantic retrieval の正常系**: チャット実応答は確認済み。embedding endpoint（例 `http://192.168.0.219:7997/v1` ruri-v3-130m）を設定した hybrid retrieval / 実 review の正常系 end-to-end は未確認。embedding 未設定なら FTS only で degrade（仕様）。

---

## 6. 参考（既存実装の地図）

（⚠ Phase 9 で `backend/` は削除済み。下記は Go 実装が正本。旧 TS は git 履歴に残る。Go の各ファイル冒頭コメントに移植元の `backend/src/...` パスが出典として残してある。）
- API 一覧・全サービスの責務: 計画ファイルと `internal/httpapi/`（22 ルート） / `internal/service/*`。
- DB スキーマ正本: `internal/db/schema.go`（9 migrations）。
- 型定義: `internal/model/`。ユーティリティ: `internal/util/`。
- サンプル文書（検索テスト用の実データ）: `sample-docs/`。

---

## 7. 移行後の追加機能（2026-05-31 着手）

承認済み計画: `~/.claude/plans/indexed-toasting-nova.md`（Track A=テーマ切替 / Track B=ruri-v3-30m 内蔵 embedding）。

### Track A — テーマ切替（ダークモード）✅ DONE & GUI 確認済み（2026-05-31, ユーザー目視 OK）
フロントのみ（Go/DB/API 不変）。**6系統 × light/dark = 12テーマ**（Solarized / Catppuccin / Rosé Pine / Tokyo Night / GitHub / One）+ light/dark/OS追従。確立した規約・勘所（蒸し返さないこと）:
- **セマンティックトークン方式**: `frontend/src/styles/themes/types.ts` が `ThemeTokens`（~55トークン・コンポーネント直消費）と `ThemeSpec`（compact 入力）+ `buildTokens(spec)`。各パレットは ThemeSpec を渡すだけ。**白オーバーレイ系の面（surfacePane/Card/Elevate/Field/Button/Dropzone/paneHeader/composer/floatBtn/modalScrim）は dark で muddy になるため spec に具体値で持つ**。line/accent/warm/danger の半透明 wash は spec の rgb triplet から固定 alpha で派生（dark は `overlayScale≈1.5` で alpha を底上げ、`min(α*scale,0.95)` でクランプ）。
- **Solarized Light は現リテラルを1:1移植**＝回帰ゼロ。`#ff7a6c`/`#dc322f` の2種エラー赤は `dangerText` トークンに統一（Solarized は `#dc322f`）。
- パレット: `frontend/src/styles/themes/{solarized,catppuccin,rosePine,tokyoNight,github,one}.ts`、registry=`themes/index.ts`（`PALETTES`/`THEME_FAMILIES`/`resolveTokens`/`isThemeFamily`）。light テーマ共通の白オーバーレイ面は `themes/presets.ts` の `LIGHT_SURFACES`（Solarized Light を除く）。
- **Emotion ThemeProvider 化**: `frontend/src/styles/ThemeController.tsx`（context + localStorage `snz.theme.{family,mode}` + `matchMedia('(prefers-color-scheme: dark)')` で auto 追従 + `<ThemeProvider>` ラップ + `useThemeController()`）。`main.tsx` で `<ThemeController>` がツリー最上位、`<Global>` を theme 関数化（`color-scheme` を `theme.scheme` で light/dark 切替）。型は `frontend/src/styles/emotion.d.ts` が `@emotion/react` の `Theme extends ThemeTokens`（tsconfig `include:["src"]` で自動取り込み）。
- 旧 `frontend/src/styles/theme.ts` は**削除**。`ui.tsx` は全色を `({ theme }) => theme.X` に置換し共通 `ErrorText` を追加。inline style の色は `useTheme()` で参照（MarkdownPreview / ProjectDetailPage の3 popover・delete ボタン / ProjectListPage のドラッグ色）。**注意: ProjectDetailPage の3 popover は同一値だがインデント差で `replace_all` が1件しか当たらない**ため個別置換した。
- **切替 UI**: `ProjectListPage` の InspectorPane に "Appearance" Card（family Select + mode Select、API 呼び出し無し）。`WorkspaceSidebar` ヘッダに light/dark クイックトグル（Sun/Moon IconButton、チャット画面からも切替可）。
- **検証**: `pnpm check:client`（tsc）クリーン / `pnpm build:client`（vite, 336 modules）成功 / `go build ./...`（`//go:embed frontend/dist` 解決）OK。`frontend/dist/.gitkeep` は vite の emptyOutDir が消すので commit 前に `git checkout` で復元（実施済み）。**GUI 目視＝ユーザー確認済み（2026-05-31, 問題なし）**。Track A はコミット前（main 上で未コミット・ユーザー判断）。

### Track B — ruri-v3-30m 内蔵 embedding（llama.cpp サイドカー）⏳ 未着手・**別セッションで対応**（スパイク先行）
**次セッションの開始手順**: (1) この §7 → (2) 計画ファイル `~/.claude/plans/indexed-toasting-nova.md` の §Track B（file-by-file 設計・接合点・検証まで網羅）→ (3) 既存 Go 層（`internal/service/{embeddingclient,embeddingsync,retrieval}.go`, `internal/config/config.go`, `internal/httpapi/{server,handlers}.go`, `app.go`）。

**着手前関門（最優先・ここが通らなければ実装に入らない）**: ModernBERT 対応 llama.cpp（パッチ版・upstream マージ未確認）+ ruri-v3-**30m** GGUF（既製は無く 310m のみ確認＝自前変換要）を mac arm64/x86_64・win amd64 で用意し `llama-server --embedding` が 256次元 embedding を返すことを実機確認（S1）、`sample-docs/` で 1+3 prefix（検索クエリ:/検索文書:）の retrieval 品質確認（S2）。

**通過後の実装順**: config overlay（`internal/config` に runtime-only internal 上書き + `embeddingMode`、`Get()` で internal 時のみ上書き）→ 新規 `internal/embed`（sidecar/downloader/manager）→ app.go 結線（startup で非ブロッキング `EnsureInternalReady`、shutdown で stop）→ prefix 分岐（query=retrieval.go・doc=embeddingsync.go、internal のみ）→ サイドカー ready コールバックで run-once `RebuildAll` → パッケージング（mac は Resources 同梱+先に sidecar 署名・entitlements、win は exe 同梱）。**S1 失敗時のフォールバック**: external-only 出荷 / 310m GGUF 内蔵（768次元）/ ONNX in-process（CGO・pure-Go 方針に反するため再判断時のみ）。

**確認済みの設計の要（蒸し返さない）**: `EmbeddingClient` は毎回 `cfg.Get()` から base URL/model を解決するので、サイドカーへ向けるのは config overlay だけでよく EmbeddingClient/retrieval/sync は無改修。モデル切替は次元不一致で cosine=0 になるため必ず `RebuildAll`。既存ユーザー移行は「永続 embeddingModel が非空なら external、それ以外 internal」。
