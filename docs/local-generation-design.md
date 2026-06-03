# 内蔵 llama.cpp によるローカル生成（LLM）— 設計たたき台

ステータス: **DRAFT / 議論用たたき台**（2026-06-02 起票） / 対象: 実装方針の合意形成

> 目的: 現在「埋め込み専用」で同梱している `llama-server` を、**生成（chat completion）にも使えるようにする**ための設計案。
> 既存の embedding サイドカー機構（[internal/embed/](../internal/embed/)）が良いテンプレートになるので、その対称形として設計する。
> この文書は確定仕様ではない。各 §末尾の「未決」を潰しながら更新する。

---

## 0. ゴールとスコープ

- ユーザーが**生成用 GGUF を用意すれば、外部 LLM（LM Studio 等）無しでチャット生成できる**。
- 既存の「外部 LLM エンドポイント」方式は温存し、**内蔵 / 外部を切り替えられる**（embedding の `internal`/`external` トグルと同じ体験）。
- 同梱バイナリ（`llama-server` release `b9437`）は**そのまま流用**。新規バイナリの追加・CGO 化はしない。
- 非ゴール（今回やらない）: 生成モデルのアプリ同梱、review サービスの内蔵化、複数モデル同時ロード、量子化 UI。

---

## 1. 現状の再確認（再利用できる資産）

生成は既存の埋め込み機構の「対称形」で実装できる。対応関係:

| 既存（embedding） | 生成（新規） | 備考 |
| --- | --- | --- |
| `embed.Manager`（DL+監視）[manager.go](../internal/embed/manager.go) | `gen.Manager` 相当 | ライフサイクル・バックオフ・状態機械をほぼ再利用 |
| `embed.sidecar`（`--embedding` 起動）[sidecar.go](../internal/embed/sidecar.go) | 生成モード起動の sidecar | **起動引数だけが違う**（§3.2） |
| `resolveServerBinary()` | 同じ関数を流用 | バイナリは共通、追加不要 |
| `downloadModel` / `verifyFile` [downloader.go](../internal/embed/downloader.go) | DL 方式を採るなら流用 | SHA256 + Range 再開つき |
| `Config` の internal overlay（`SetInternalEmbedding`/`Get` 置換）[config.go](../internal/config/config.go) | `SetInternalLLM` overlay | `EmbeddingMode=="internal"` と同じ仕組み |
| `Server.onEmbeddingReady/Lost` [server.go](../internal/httpapi/server.go) | `onLLMReady/Lost` | overlay の付け外し |
| `GET /api/embedding/status` | `GET /api/llm/status` | DL/起動/ready/error の表示 |
| 設定の embedding mode トグル [handlers.go](../internal/httpapi/handlers.go) | LLM mode トグル | mode 切替で sidecar を起動/停止 |

**最重要**: 生成リクエストを投げる [`LLMClient`](../internal/service/llmclient.go) は、リクエストごとに `cfg.Get()` を読んで `LLMBaseURL`/`LLMModel` を解決する。
→ **内蔵モードの実体は「`LLMBaseURL`/`LLMModel` を内蔵サイドカーに向ける overlay を足すだけ」**。クライアント本体（streaming 含む）は無改修で動くはず。

---

## 2. モデル取得戦略（最大の論点）

埋め込みモデル（ruri-v3-30m）は **41MB** なのでアプリ同梱できたが、実用的な生成モデルは q4 でも **0.7〜2GB+**。同じ「同梱」戦略は採れない。

| 案 | 内容 | 長所 | 短所 |
| --- | --- | --- | --- |
| **A. 同梱** | 小型 instruct を `.app` に同梱 | DL 不要 | アプリ肥大・ライセンス/署名/notarization の重量増。**却下寄り** |
| **B. オンデマンドDL** | pin した URL から初回DL（`downloadModel` 流用、SHA+再開） | ワンクリック | ホスティング責任・1GB級DL・モデル選定を運営が固定 |
| **C. ユーザー指定 GGUF** | 設定でローカル `.gguf` をファイル選択 | アプリ最小・自由・配布の法的負荷ゼロ | ユーザーがモデルを自力入手 |

**推奨: C を MVP**（Wails の `runtime.OpenFileDialog` で `.gguf` を選ばせ、パスを設定に保存）。LM Studio を使う層＝GGUF を持っている層なので UX 的に自然。
将来 B を「おすすめモデルのワンクリックDL」として上乗せできる（C の上位互換として共存可能）。

> 未決: MVP を C 単独にするか、C+B を同時に出すか。B を採るなら配布元（HF repo / 自前ミラー）と既定モデル（例: Qwen2.5-1.5B-Instruct q4 級）の選定が必要。

---

## 3. コア設計

### 3.1 設定（config）

[internal/config/config.go](../internal/config/config.go) に embedding と対称な要素を足す:

- `Editable` に永続フィールド追加:
  - `LLMMode string` … `"internal"` | `"external"`（既定 `external` = 現状維持）
  - `LLMModelPath string` … 内蔵モードで使うローカル GGUF の絶対パス（案 C）
  - 既存 `normalizeEmbeddingMode` と同様に **未知値は `""` を返して「現状維持」**にする（古いフロントが省略しても mode を勝手に反転させない）。
- 実行時 overlay（非永続）を追加:
  - `internalLLMURL`, `internalLLMModel`
  - `SetInternalLLM(baseURL, model)` / `ClearInternalLLM()`
  - `Get()` で `LLMMode=="internal"` のとき `LLMBaseURL`/`LLMModel` を overlay 値に差し替え（embedding と同じパターン）。

**baseURL の規約**: overlay には `http://127.0.0.1:<port>/v1` を入れる（末尾 `/v1` 込み）。
理由: `LLMClient` は `baseURL + "/chat/completions"` と `baseURL + "/models"` を組み立てる（LM Studio の `.../v1` 前提）。`/v1` 込みにすれば `llama-server` の `/v1/chat/completions`・`/v1/models` に一致する。
（embedding overlay は `/v1` 無し＝`/embeddings` 直叩きだった点と非対称なので注意。）

### 3.2 サイドカー起動引数

embedding（[sidecar.go:51-61](../internal/embed/sidecar.go#L51-L61)）との差分:

```
# 削除: --embedding / --pooling mean
# 生成用:
-m <LLMModelPath>
-c 4096                # 生成は文脈長を広めに（設定で可変に）
--jinja                # GGUF 同梱の chat template を使う（chat completions の整形）
-ngl <N>               # GPU offload: mac=99 既定 / Windows はバックエンド次第（§4）
--host 127.0.0.1 --port <ephemeral>
```

- ヘルスチェック: 既存 `waitHealthy` は `/health` をポーリング。`llama-server` は**モデルロード完了まで 503 を返す**ので、大きいモデルだと現状の **90s タイムアウトを超える可能性**。生成用は時間上限を引き上げる（例 300s）か、ロード進捗が無い分は素直に延長。
- 起動確認の probe: embedding は「埋め込み次元の検証」をしていた。生成は `/health` 200 で ready とみなすのが簡単（必要なら `max_tokens:1` の極小 chat を 1 回投げて 200 を確認）。

### 3.3 Manager とライフサイクル

- `embed.Manager` を**汎用化**して role（embedding/generation）と「起動引数ビルダ」「spec or modelPath」を注入できるようにするのが DRY。
  - 最小改修案: `sidecar` に `args []string` を外部から渡せるようにし、`Manager` は role ごとに 1 つずつ作る。
  - 代替案: `internal/gen/` を新設し `embed` を写経（重複は増えるが結合は緩い）。→ **汎用化を推奨**、ただし embedding の挙動を壊さないことを優先。
- **起動契機**: 埋め込みは起動時に自動。生成は **opt-in**（`LLMMode` が `internal` のときだけ起動）。メモリ数 GB を常駐させないため。
- ready/lost コールバックを `Server.onLLMReady/Lost` に配線 → `cfg.SetInternalLLM/ClearInternalLLM`。

### 3.4 HTTP / ハンドラ

- `PUT /api/configuration`（[handlers.go:60-125](../internal/httpapi/handlers.go#L60)）に LLM mode 切替を追加（embedding と同型）:
  - `internal` へ → `llmManager.EnsureReady()`
  - `external` へ → `llmManager.Shutdown()` + `cfg.ClearInternalLLM()`
- `s.llm.EnsureModelLoaded(...)` / `ListAvailableModels` は **LM Studio 専用 API**（`/api/v1/models/load`）なので、**内蔵モードでは skip**（`llama-server` は起動時にモデル確定済み。呼んでも無害に false が返るが、明示ガードが綺麗）。
- `checkConnections` は `cfg.Get()` の実効値を見るので、overlay が立っていれば**内蔵向けの接続判定は自動で通る**（`/v1/models` が応答する前提。§5 で要確認）。
- 新規 `GET /api/llm/status`（DL/起動/ready/error）を embedding status と同型で追加。

### 3.5 フロントエンド（設定画面）

- 「LLM ソース」トグル（内蔵 / 外部）。embedding の既存トグルを踏襲。
- 内蔵選択時: GGUF ファイル選択（Wails `OpenFileDialog`）＋ ステータス表示（ロード中/準備完了/失敗）。
- 生成本体（streaming 表示など）は既存 ChatPage のまま（エンドポイントが変わるだけ）。

---

## 4. 性能・GPU offload

- 埋め込みは `-ngl 0`（CPU 固定・37M なので問題なし）。**生成を CPU だけで回すと実用速度が出ない**。
- macOS: 同梱ビルドは Metal 入り（`llama-...-bin-macos-arm64`）。`-ngl 99` 既定でフルオフロード可能。
- Windows: **同梱バイナリのバックエンド（CPU か Vulkan か）を要確認**（§5）。CPU ビルドだと内蔵生成は小型モデルでも厳しい → Vulkan ビルド採用や、Windows は「外部 LLM 推奨」の案内も検討。
- `-c`（文脈長）, `-ngl`, スレッド数 `-t` は設定で可変にし、プラットフォーム別の妥当な既定値を持たせる。

---

## 5. 未決事項 / 確認すべき点

1. **モデル取得**: MVP は案 C 単独か、C+B 同時か（§2）。B なら配布元と既定モデルの確定。
2. **Windows バイナリのバックエンド**: 現同梱が CPU か Vulkan か。生成の現実的な体感速度に直結。
3. **`llama-server` の正確なルート**: `/v1/models`・`/v1/chat/completions` の有無（overlay baseURL を `/v1` 込みにする前提の裏取り）。`/models` 非対応だと `CheckConnection`/`ListModels` の baseURL 規約を要調整。
4. **Manager 汎用化の粒度**: embedding を壊さずに role 注入できるか（共通コア抽出 vs 写経）。
5. **メモリ常駐**: 埋め込み + 生成の 2 プロセス同時稼働時のメモリ上限・ユーザーへの注意喚起。
6. **review サービス**: 同じく内蔵に向けるか（今回は対象外、将来 `CompletionTarget` 経由で可能）。
7. **chat template**: `--jinja` 前提で問題ないモデル範囲（テンプレ未同梱 GGUF の扱い）。

---

## 6. 段階的実装プラン（案）

- **Phase G1 — config 土台**: `LLMMode`/`LLMModelPath` 永続フィールド + `internalLLM` overlay + `Get()` 置換 + テスト（config_test 対称ケース）。サイドカー無しでも overlay 単体で検証可能。
- **Phase G2 — 生成サイドカー**: `sidecar` の引数注入化 + 生成用 Manager + ready/lost 配線。`SNZ_LLAMA_SERVER_BIN` と手元 GGUF で疎通確認。
- **Phase G3 — HTTP/ステータス**: `PUT /api/configuration` の mode 切替、`EnsureModelLoaded` の内蔵 skip ガード、`GET /api/llm/status`。
- **Phase G4 — フロント**: ソーストグル + ファイル選択 + ステータス表示。
- **Phase G5（任意） — おすすめモデルDL**: 案 B を上乗せ。
- 各 Phase で `go build/vet/test`（`-race`）と `gofmt` をグリーンに保つ（既存 HANDOFF の検証規律に準拠）。

---

## 参考（既存実装の出典）

- 埋め込みサイドカー: [internal/embed/sidecar.go](../internal/embed/sidecar.go) / [manager.go](../internal/embed/manager.go) / [modelspec.go](../internal/embed/modelspec.go) / [downloader.go](../internal/embed/downloader.go)
- 設定 overlay: [internal/config/config.go](../internal/config/config.go)
- LLM クライアント: [internal/service/llmclient.go](../internal/service/llmclient.go)
- HTTP 結線: [internal/httpapi/server.go](../internal/httpapi/server.go) / [handlers.go](../internal/httpapi/handlers.go)
- 全体の経緯: [docs/HANDOFF.md](HANDOFF.md)
