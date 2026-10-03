---
id: TASK-79
title: '埋め込み: 同梱の llama-server を起動ごとの API キー付きで起動し、使わないエンドポイントを閉じる'
status: In Review
assignee: []
created_date: '2026-09-28 20:23'
updated_date: '2026-10-03 19:55'
labels:
  - security
dependencies:
  - TASK-66
references:
  - internal/embed/sidecar.go
  - internal/embed/manager.go
  - internal/config/config.go
priority: low
type: enhancement
ordinal: 79000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F13 (Low、確信度 Medium)。2026-09-29 に `756d32b` で未修正を確認した。

- `sidecar.go` は `llama-server` を `--api-key` なしで `127.0.0.1:<ランダムなポート>` に起動している。同じマシンの他のプロセスやブラウザのページ (ループバックのポートを探す必要がある) から要求を送れる。
- 要確認: llama-server は既定で CORS を許し、`/slots` などで直近の処理内容を返す版がある。そうであれば、検索クエリや文書チャンクの断片を読める可能性がある。
- TASK-66 でサイドカーを b11126 に上げるので、確認はその版で行う。

## 方針

- 起動ごとにランダムな `--api-key` を付け、内蔵のオーバーレイ (`SetInternalEmbedding`) で `EmbeddingAPIKey` として渡す。ready の判定に使う `/health` がキーを要するかも確かめる。
- `--no-slots` などで、使わないエンドポイントを閉じる。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 b11126 の llama-server の既定の CORS と、/slots などが返す内容を確かめた結果が記録されている
- [ ] #2 同梱のサイドカーがキーの無い要求 (/v1/embeddings など) を拒否し、アプリからの埋め込みは従来どおり動く (macOS / Windows)
- [x] #3 キーは起動ごとに作られ、app-config.json にもログにも残らない
- [x] #4 使わないエンドポイントを閉じたこと、または閉じる必要が無いと判断した理由が記録されている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. b11126 の llama-server をキー無し / キー付きで起動し、CORS・/slots・/props・各エンドポイントの応答を実測する
2. sidecar.Start で起動ごとに crypto/rand のキーを作り、argv ではなく環境変数 LLAMA_API_KEY で渡す (ps に出さない)。probe もキーを付けて送る。/health はキー不要なのでそのまま
3. --no-slots と --no-ui を付ける。/metrics・POST /props・slot 保存は既定で無効のまま
4. Manager の onReady に apiKey を足し、config の内蔵オーバーレイ (SetInternalEmbedding) で EmbeddingAPIKey として返す。永続化される Editable には入れない
5. embeddingAPIKeyFor: internal モードではオーバーレイのキーを、オーバーレイの URL と同じオリジンにだけ送る
6. 単体テスト (config / apikey / httpapi) と実サイドカーの統合テスト (キー無し要求が 401 になること) を足す
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## b11126 の llama-server の実測 (AC#1)

2026-10-04、macOS arm64、`build/sidecar/darwin-arm64/llama-server` (build 11126, commit b1ff4ca23) を従来の引数 (キー無し) で起動して確かめた。

- CORS: 既定は `--cors-origins *` かつ `--cors-credentials` 有効。`Origin: http://evil.example` 付きの POST /v1/embeddings に `Access-Control-Allow-Origin: http://evil.example` を返し、プリフライトにも `Allow-Credentials: true`・`Allow-Headers: *` を返した。つまりブラウザのページから埋め込みを取得でき、応答も読める状態だった。
- /slots: 既定で有効 (GET 200)。b11126 はスロットごとの id・n_ctx・n_prompt_tokens・サンプリング設定を返すが、プロンプト本文やトークン列は返さない (送った文書テキストの断片は応答に出なかった)。`/slots/0?action=save|erase` は `--slot-save-path` 未設定のため 501。
- /props: GET 200。`model_path` (ユーザーのホームディレクトリを含む絶対パス)・build_info・chat_template などを返す。POST /props は既定で無効 (501)。
- /metrics: 既定で無効 (501)。
- `/`: Web UI (gzip の静的ファイル) を返す。
- そのほか /tokenize・/detokenize・/apply-template・/embedding・/embeddings・/lora-adapters がキー無しで 200。

## 実装

- `sidecar.Start` で起動ごとに `crypto/rand.Text()` のキーを作る。`--api-key` ではなく環境変数 `LLAMA_API_KEY` で渡す: argv は `ps` で他ユーザーのプロセスからも読めるが、環境変数は読めないため。キーはメモリと子プロセスの環境変数にしか無い。
- キーは Manager の onReady → `Config.SetInternalEmbedding(baseURL, apiKey, model)` の内蔵オーバーレイに入り、internal モードの `Get()` が `EmbeddingAPIKey` として返す。永続化されるのは `Editable` だけなので app-config.json には書かれない。internal モードでは EMBEDDING_API_KEY もこのキーで置き換わるので、外部向けのキーがサイドカーへ行くことも無い。
- `embeddingAPIKeyFor`: internal モードではオーバーレイの URL と同じオリジンにだけキーを送る。internal のまま設定画面でモデル一覧を外部 URL に対して取ると、その URL にキーが行ってしまうため。
- `/health` はキー不要 (b11126 で確認) なので ready 判定はそのまま。次元の probe にはキーを付けた。

## キー付き起動の実測 (AC#2 / AC#3)

- 単体で LLAMA_API_KEY + `--no-slots --no-ui` で起動したときの結果: キー無しでは /health だけ 200、/v1/embeddings・/v1/models・/slots・/props・/tokenize などは 401、`/` は 404。キー付きでは /v1/embeddings 200、/slots 501、/metrics 501。
- `pnpm dev` (macOS) で起動したアプリのサイドカー: argv は `... -ngl 0 --no-slots --no-ui` でキーを含まない。キー無しの POST /v1/embeddings (Origin 付き) は 401、/health は 200。状態は ready。dev 用データにテスト用プロジェクトと文書を追加したところ、ruri-v3-30m の埋め込みが 1 チャンク保存された (確認後に削除した)。dev のログと data/app-config.json にキーは出ていない。
- 実サイドカーの統合テスト (`TestManagerIntegrationRealSidecar`) に、キー無しの要求が 401 になることと /slots が 200 を返さないことの確認を足し、b11126 で通過した。
- `go vet ./...`・`go test ./...` は通過。`GOOS=windows go vet` も通過。
- 未確認: Windows の実機での動作。AC#2 は macOS のみ確認済みなので未チェックのまま残す。キーは環境変数で渡しており、Windows では `cmd.Env` に os.Environ() を入れてから追加している。

## 閉じたエンドポイント (AC#4)

- `--no-slots` で /slots を閉じた (b11126 では本文は返さないが、アプリは使わない)。
- `--no-ui` で Web UI を閉じた。
- /metrics・POST /props・スロット保存は既定で無効なので、引数は足していない。
- GET /props・/tokenize などは残っているがキーが必要。CORS はキー無しの要求を 401 で拒否するので、既定のままにした。

## 範囲外で見つけた既存の問題

internal モードで ready でも、GET /api/configuration の embeddingConnected が false になる。サイドカーの /v1/models がモデルを GGUF の絶対パスで返し、`CheckConnection` が `ruri-v3-30m` との一致を見ているため。この変更を一時的に外した main でも同じだったので、本タスクによるものではない。別タスクとして扱う。
<!-- SECTION:NOTES:END -->
