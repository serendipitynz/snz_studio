---
id: TASK-79
title: '埋め込み: 同梱の llama-server を起動ごとの API キー付きで起動し、使わないエンドポイントを閉じる'
status: To Do
assignee: []
created_date: '2026-09-28 20:23'
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
- [ ] #1 b11126 の llama-server の既定の CORS と、/slots などが返す内容を確かめた結果が記録されている
- [ ] #2 同梱のサイドカーがキーの無い要求 (/v1/embeddings など) を拒否し、アプリからの埋め込みは従来どおり動く (macOS / Windows)
- [ ] #3 キーは起動ごとに作られ、app-config.json にもログにも残らない
- [ ] #4 使わないエンドポイントを閉じたこと、または閉じる必要が無いと判断した理由が記録されている
<!-- AC:END -->
