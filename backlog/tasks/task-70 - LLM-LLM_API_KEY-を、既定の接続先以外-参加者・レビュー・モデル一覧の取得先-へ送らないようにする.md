---
id: TASK-70
title: 'LLM: LLM_API_KEY を、既定の接続先以外 (参加者・レビュー・モデル一覧の取得先) へ送らないようにする'
status: To Do
assignee: []
created_date: '2026-09-28 20:22'
labels:
  - security
dependencies: []
references:
  - internal/service/llmclient.go
  - internal/service/embeddingclient.go
  - internal/config/config.go
  - internal/httpapi/handlers.go
  - internal/service/turnengine.go
  - internal/preset/preset.go
  - docs/multi-agent-presets.md
priority: high
type: bug
ordinal: 70000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F2 (Medium)。2026-09-29 に `756d32b` で未修正を確認した。

- `llmclient.go` は送信先に関係なく `Authorization: Bearer <LLM_API_KEY>` を付ける (`c.authHeader(req, s.LLMAPIKey)` の 6 か所)。`resolveTarget` で送信先が参加者の `baseUrl`・review の接続先・`POST /api/configuration/models` の本文の `baseUrl` に差し替わっても、そのまま付く。
- `EMBEDDING_API_KEY` が未設定のときは `LLM_API_KEY` にフォールバックする (`config.go` の `Defaults`) ので、埋め込みの接続先にも同じキーが送られる。
- 成立条件: `LLM_API_KEY` を設定している (クラウドの OpenAI 互換 API を併用している等)。送信先が `http://` なら平文で送られる。

### レビュー後に変わったこと

レビュー時点では「プリセットは接続先を持たないので、プリセットを読み込んでも成立しない」としていた。TASK-62 (2026-09-27) でプリセットの参加者が `baseUrl` / `modelName` を持てるようになり、読み込むと接続を確かめずにそのまま適用される (`docs/multi-agent-presets.md`)。このため、他人から受け取ったプリセットファイルを読み込んで会話を進めるだけで、ファイルに書かれたホストへキーが送られる。多人数会話の発言にはプロジェクト資料も載るので、資料もそのホストへ送られる。

## 方針 (着手時に確定する)

- キーを既定の `LLM_BASE_URL` のオリジンと結びつけ、送信先のオリジンが一致するときだけ付ける。review と埋め込みのフォールバックも同じ規則にする。
- 参加者ごとにキーが要るかを決める。要るなら別の欄にする。
- プリセットの読み込みで、既定と異なる接続先を持つ参加者がいることを画面に示すかを決める (キーの送信を止めても、プロジェクト資料の送り先はファイルが決めることになるため)。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 既定の LLM_BASE_URL とオリジンが異なる送信先 (参加者の baseUrl、review の接続先、モデル一覧の取得先) への要求に Authorization が付かない。httptest サーバーを 2 つ立てるテストで固定している
- [ ] #2 オリジンが一致する送信先には従来どおり Authorization が付く
- [ ] #3 EMBEDDING_API_KEY が未設定のときのフォールバックも同じ規則に従う (または廃止し、その判断が記録されている)
- [ ] #4 接続先を持つ参加者がいるプリセットの読み込みで、それを画面に示すかどうかが決まり、記録されている
- [ ] #5 README.md / README.ja.md / .env.example の LLM_API_KEY の説明に、キーが送られる範囲が書かれている
<!-- AC:END -->
