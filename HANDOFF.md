# SNZ Studio — Wails + Go デスクトップ化 引き継ぎ (HANDOFF)

最終更新: 2026-05-31 / 対象: 次セッションの実装者（あなた）

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

リポジトリ直下に Go モジュール `snzstudio` を作成済み。**Phase 1（Wails 足場）+ Phase 4（Repository 層）+ Phase 5（Service 層）まで完了**。React アプリのソースは未変更（フロントは `vite.config.ts` の `outDir` と `.gitignore` のみ調整）。旧 Node アプリ（`backend/`）も未変更で今もそのまま動く。`go build ./...` / `go vet ./...` / `go test ./...` 全グリーン、`gofmt` クリーン（`internal/search/model.go` は自動生成のため対象外）、`wails build` で `build/bin/snz-studio.app`（darwin/arm64・自己署名）まで生成確認済み（Phase1 時点）。

```
main.go                               ✅ Phase1 Wails 起動 + //go:embed all:frontend/dist
app.go                                ✅ Phase1 App: ローカル net/http を goroutine 起動 / GetApiBase() / shutdown / /api/health
env_dev.go (//go:build dev)           ✅ Phase1 isDev=true, apiListenAddr=127.0.0.1:8787（wails dev が dev タグを付与）
env_prod.go (//go:build !dev)         ✅ Phase1 isDev=false, apiListenAddr=127.0.0.1:0（ephemeral）
wails.json                            ✅ Phase1 monorepo 対応（frontend:* は pnpm -C .. 実行 / serverUrl=auto / wailsjsdir=frontend/src）
build/{appicon.png,darwin/Info.plist} ✅ Phase1 wails 生成の既定テンプレ（bundle id 既定 com.wails.snz-studio → Phase8 で要変更）
go.mod / go.sum                       module snzstudio (go 1.26) + wails v2.12.0
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
internal/httpapi/                     ✅ SSE 基盤
  ├ sse.go                             SSEWriter（frontend のパーサと同じ event:/data: 形式, 各 Event で Flush）
  └ sse_test.go                        逐次配信（ハンドラ完了前に受信できる）を実証
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
- **dev の通信**: Go API を固定 `127.0.0.1:8787` で立て、既存 `frontend/vite.config.ts` の proxy（`/api`・`/files`）をそのまま使う → `wails dev` がフロント無改修で回る。prod は空きポート + `window.__API_BASE__` 注入。
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
- **GUI 確認 ✅ 完了（2026-05-30）**: `wails dev` で SPA が完全描画（HashRouter・スタイル動作）。`/api/health`=200 が WebView→vite proxy→Go:8787 の疎通を実証（データ系ルートは Phase 6 まで 404 が正常）。
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

### Phase 6 — HTTP API（`internal/httpapi/`）+ Wails 結線
移植元: `backend/src/index.ts`（22 ルート）。標準 `net/http`（必要なら軽量 `chi`）。
- **service グラフの結線（Phase5 で全 `NewXxx` 用意済み）**: 順序は `cfg := config.Load(appConfigPath)` → repos（`repository.NewXxxRepository(db)`）→ `emb := service.NewEmbeddingClient(cfg)` / `llm := service.NewLLMClient(cfg)` → `retr := service.NewRetrievalService(db, emb)` → `embSync := service.NewEmbeddingSyncService(documents, memories, emb)` → `ctx := service.NewContextService(projects, chats, documents, memories, retr)` → `summary := service.NewSummaryService(llm)` / `mem := service.NewMemoryService(memories, llm)` → `chat := service.NewChatService(chats, ctx, llm, summary, mem, embSync, cfg)` / `review := service.NewReviewService(chats, ctx, llm, cfg)` / `org := service.NewMemoryOrganizerService(memories, chats, llm, embSync)`。
- **設定 GET/PUT**: `cfg.GetEditable()` / `cfg.UpdateEditable(...)`。connected 判定は `llm.CheckConnection("","")` 等＋ models は `llm.ListModels("")`/`emb.ListModels("")`。PUT 後は `emb.RefreshConfiguration()` を呼ぶ（disabled 再評価）。`WorkspaceConfiguration` の `*Connected` 3項目はフロント契約（client.ts:108-119）。
- 主要ルート群: configuration(GET/PUT, models), projects(CRUD+reorder), documents(multipart upload/category/delete), memories(CRUD/lock/organize analyze・apply), chats(CRUD/temporary), messages(send, **/stream SSE**), review(**/stream SSE**), `GET /files/*`（static）。
- SSE は `internal/httpapi/sse.go` の `SSEWriter` を使用（`delta`/`done`/`error` イベント）。`chat.SendMessageStream(chatID, content, onDelta)` / `review.ReviewMessageStream(messageID, onDelta)` の onDelta を `SSEWriter.Event("delta", ...)` に繋ぐ。
- **document/project 削除時のファイル unlink は HTTP 層で実施**（repository は DB cascade のみ。`/files` 実体の削除＝§4 の積み残し）。アップロードは multer → `r.FormFile`。保存名は uuid+ext、公開パスは `/files/{name}`。
- document 作成後は `embSync.SyncDocument(id)` を呼ぶ（embedding 同期）。
- CORS: dev で vite(5173) からのアクセスを許可。

### Phase 7 — フロント微修正（全量・最小）
1. `frontend/src/main.tsx`: `BrowserRouter` → `HashRouter`（必須）。
2. `frontend/src/api/client.ts` の `request()`: URL 先頭に `window.__API_BASE__ ?? ""`。
3. `frontend/src/pages/ChatPage.tsx`: 2つのストリーミング fetch URL に `__API_BASE__`。
4. `frontend/src/pages/ProjectDetailPage.tsx`: `<img src={filePath}>` に `__API_BASE__`。
5. 起動時に `GetApiBase()`（dev 空文字）から `window.__API_BASE__` を一度だけ設定。
6. `frontend/vite.config.ts`: `outDir` を Wails 参照先に整合。dev proxy は当面維持。
   SSE のパースロジックは**無変更**。

### Phase 8 — データ移行 + パッケージング
- 既存 `data/`（app.sqlite/uploads/app-config.json）→ ユーザーデータディレクトリへ初回起動時にコピー。
- 起動時に FTS index 再構築（既存 FTS は JS トークナイズ済みのため Go 版で作り直す）。
- `wails build`（mac universal + dmg、win NSIS + webview2 download）。CI マトリクス・署名/notarization。

### Phase 9 — クリーンアップ
- 旧 `backend/`・Node 依存・vite proxy を削除。`package.json` の整理。

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
- [ ] cascade delete とファイル削除（document/project 削除時に `/files` の実体を unlink）。← cascade=Phase4済 / ファイル unlink は **Phase 6（HTTP 層）** で対応。

---

## 5. 未解決リスク / 要・手元環境

- ~~**WebView での SSE 逐次表示**（Spike #1）~~ → ✅ **解消済み（2026-05-30）**。WKWebView から loopback Go への直 fetch で SSE が逐次描画されることを実機確認（詳細は §3 Phase 1）。Wails events 化の保険は不要。
- **mac/win の署名・notarization・WebView2**: CI と証明書が要る。早めに最小アプリで通すこと（Spike #3）。
- **統合テスト**には起動中の OpenAI 互換エンドポイント（LM Studio 等）が必要。chat/stream・retrieval・review の end-to-end はそれ無しでは確認不可。

---

## 6. 参考（既存実装の地図）

- API 一覧・全サービスの責務: 計画ファイルと `backend/src/index.ts` / `backend/src/services/*`。
- DB スキーマ正本: `backend/src/db/schema.ts`（Go 版は `internal/db/schema.go` に移植済み）。
- 型定義: `backend/src/lib/types.ts`。ユーティリティ: `backend/src/lib/utils.ts`。
- サンプル文書（検索テスト用の実データ）: `sample-docs/`。
