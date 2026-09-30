---
id: TASK-70
title: 'LLM: LLM_API_KEY を、既定の接続先以外 (参加者・レビュー・モデル一覧の取得先) へ送らないようにする'
status: In Review
assignee: []
created_date: '2026-09-28 20:22'
updated_date: '2026-09-30 03:17'
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
- [x] #1 既定の LLM_BASE_URL とオリジンが異なる送信先 (参加者の baseUrl、review の接続先、モデル一覧の取得先) への要求に Authorization が付かない。httptest サーバーを 2 つ立てるテストで固定している
- [x] #2 オリジンが一致する送信先には従来どおり Authorization が付く
- [x] #3 EMBEDDING_API_KEY が未設定のときのフォールバックも同じ規則に従う (または廃止し、その判断が記録されている)
- [x] #4 接続先を持つ参加者がいるプリセットの読み込みで、それを画面に示すかどうかが決まり、記録されている
- [x] #5 README.md / README.ja.md / .env.example の LLM_API_KEY の説明に、キーが送られる範囲が書かれている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. llmclient に送信先オリジンの判定を足し、LLM_API_KEY は既定の LLM 接続先 (その時点の LLMBaseURL。UI で変えた値を含む) とスキーム・ホスト・ポートが一致する送信先にだけ付ける。対象: chat 補完 (参加者・review の target)、ListModels / ListAvailableModels / EnsureModelLoaded、画像説明
2. 埋め込み: EMBEDDING_API_KEY を明示したときはその接続先へ送る。未設定で LLM_API_KEY を借りるときだけ同じオリジン規則に従う (config に借用かどうかを持たせる)
3. 参加者ごとのキー欄は設けない (2026-09-30 ユーザー判断)。README に制約として書く
4. PresetPicker: 読み込んだファイルに baseUrl を持つ参加者がいれば、要約の下に参加者名とホストを示す (2026-09-30 ユーザー判断)。docs/multi-agent-presets.md に記録
5. httptest サーバー 2 つで、別オリジンに Authorization が付かず、同一オリジンには付くことをテストで固定する
6. README.md / README.ja.md / .env.example に LLM_API_KEY が送られる範囲を書く (TASK-75 と同じ PR)
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 着手時に決めたこと (2026-09-30、ユーザー判断)

- キーを結びつける先は、その時点の既定の LLM 接続先 (`LLM_BASE_URL`、または設定で保存した `llmBaseUrl`)。判定は要求先の URL (`req.URL`) のスキーム・ホスト・ポート (既定ポート 80/443 を補う) で行う。要求を組み立てた後の URL を見るので、`/api/v1/models` のように base から派生したパスでも同じ規則になる。
- 参加者ごとのキー欄は設けない。キーを DB やプリセットの書き出しに載せずに済むため。キーが要る API を参加者に使うときは既定の接続先と同じオリジンにする、と README に制約として書いた。
- `EMBEDDING_API_KEY` のフォールバックは残す (AC#3)。明示したキーは埋め込みの接続先へそのまま送る。空か未設定のときだけ `LLM_API_KEY` を借り、そのときは LLM キーと同じオリジン規則に従う。フォールバックを config から要求時の判定 (`embeddingAPIKeyFor`) へ移した。内蔵の埋め込み (internal) にはどちらも送らない。
- プリセットの読み込みで接続先を持つ参加者を画面に示す (AC#4)。PresetPicker の要約の下に「参加者名: baseUrl」を並べ、会話と資料がその接続先へ送られること、キーは同じオリジンにだけ送ることを添えた。記録は docs/multi-agent-presets.md。

## タスク本文に無かったが同じ規則に入れたもの

- 画像説明 (`imagedescription.go`) も `LLM_API_KEY` を `IMAGE_DESCRIPTION_BASE_URL` へ無条件に送っていたので、同じ判定にした。

## 検証

- `internal/service/apikey_test.go`: httptest サーバーを 2 つ (既定・別オリジン) 立て、補完 (非ストリーム・ストリーム。参加者と review の target)、ListModels / ListAvailableModels / EnsureModelLoaded、画像説明のそれぞれで、別オリジンには Authorization が付かず (AC#1)、既定オリジンには付く (AC#2) ことを確かめた。埋め込みは明示キー・借用 (同一オリジン)・借用 (別オリジン)・internal の 4 通り (AC#3)。sameOrigin は大小文字、既定ポートの補完、localhost と 127.0.0.1、スキーム違い、接尾辞の似たホストを確かめた。
- 判定を外す (常にキーを返す) と TestLLMKeyStaysWithDefaultOrigin と TestEmbeddingKey が落ちることを確かめてから戻した。
- `go test ./...` 全件成功、`pnpm check:client`・`pnpm build:client` 成功。
- 画面: `pnpm dev` の Wails ブラウザ用サーバー (localhost:34115) で、接続先 2 人・なし 1 人のファイルを読み込ませ、2 人だけが並ぶこと、同梱プリセット (debate) では注記が出ないことを確かめた。4 配色の比は測っていない (既存の Subtle と同じ `theme.muted` を使う)。実窓 (WKWebView) での見え方は未確認。
- README.md / README.ja.md / .env.example に、キーが送られる範囲を書いた (AC#5)。docs/current-spec(.ja).md の記述も合わせた。
<!-- SECTION:NOTES:END -->
