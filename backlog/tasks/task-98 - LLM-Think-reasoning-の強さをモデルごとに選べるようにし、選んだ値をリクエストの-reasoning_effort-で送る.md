---
id: TASK-98
title: 'LLM: Think (reasoning) の強さをモデルごとに選べるようにし、選んだ値をリクエストの reasoning_effort で送る'
status: Done
assignee: []
created_date: '2026-10-10 22:58'
updated_date: '2026-10-10 23:47'
labels: []
milestone: m-3
dependencies: []
references:
  - internal/service/llmclient.go
  - internal/config/config.go
  - internal/model/model.go
  - internal/service/turnengine.go
  - frontend/src/components/SettingsModal.tsx
  - 'https://lmstudio.ai/changelog/lmstudio-v0.4.8'
  - 'https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/1990'
priority: medium
type: feature
ordinal: 102000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

オーナーの指摘 (2026-10-11)。gemma-4 や qwen3.5 を使うと、gpt-oss-20b より応答が鈍く感じる。

LM Studio の `/api/v1/models` は、モデルごとの reasoning 設定を `reasoning.allowed_options` と `reasoning.default` で返す。手元の値は次のとおり。

- openai/gpt-oss-20b: low / medium / high、既定は low
- google/gemma-4-12b ほか gemma-4 系: off / on、既定は on
- qwen/qwen3.5-9b: off / on、既定は on
- qwen3.8-27b、llm-jp-4.1-8b-thinking など: `null` (API で切り替えられない)

「日本の首都はどこ？一文で」を投げて測った (2026-10-11、LM Studio)。

- gemma-4-12b を既定のまま: 本文の最初の文字まで 8.45 秒、reasoning トークン 116
- gemma-4-12b に `reasoning_effort: "none"`: 0.68 秒、reasoning トークン 0
- gpt-oss-20b を既定のまま: 0.89 秒、reasoning トークン 55

今の `LLMClient` はリクエストに reasoning の指定を一切付けない。そのためモデルの既定どおり思考が走る。LM Studio 0.4.8 から `/v1/chat/completions` で `reasoning_effort` を受け付ける。

## 作業

- reasoning の指定をモデル単位で持てるようにする。単独アシスタントのチャットはアプリ設定の LLM モデルを使い、多人数会話の参加者は自分の接続先とモデルを持つ。どちらの経路にも効くようにする。
- 未指定ならリクエストに `reasoning_effort` を付けない。LM Studio 以外の OpenAI 互換サーバーが知らないパラメータを拒否するおそれがあるため。
- 設定画面では `/api/v1/models` の `allowed_options` を選択肢に出す。`null` のモデルや LM Studio 以外の接続先では選択肢を出さない (または「モデルの既定」だけにする)。
- LM Studio の `off` は OpenAI 互換の語彙では `none` で送る (gemma-4-12b で `none` が効くことは確認済み)。

## 着手時に決めること

- **設定の置き場所**: アプリ設定にモデル名をキーにした表で持つか、参加者の行に列を足すか。同じモデルを単独チャットと参加者の両方で使うとき、同じ値を共有するかどうか。
- **既定値**: 未指定を「モデルの既定」とするか、snz studio として off を既定にするか。
- **qwen3.5 で効くか**: LM Studio 0.4.15 で qwen3.5-9b の thinking 無効化が効かなかった報告がある (lmstudio-bug-tracker#1990)。実機で `none` を確かめ、効かないモデルの扱いを決める。
- **要約・メモリ整理・画像説明などの裏方の呼び出し**: ユーザーが待っていない呼び出しにも同じ指定を掛けるか、常に off にするか。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 単独アシスタントのチャットと多人数会話の参加者の両方で、モデルごとに reasoning の値を選んで保存できる
- [x] #2 値を選んだモデルへのリクエストに reasoning_effort が載り、LM Studio の off は none で送られる (テストで確かめる)
- [x] #3 値を選んでいないモデル、または LM Studio 以外の接続先へのリクエストには reasoning_effort が載らない (テストで確かめる)
- [x] #4 設定画面の選択肢が /api/v1/models の allowed_options と一致し、reasoning が null のモデルでは切り替えを出さない
- [x] #5 gemma-4 で off を選ぶと、本文の最初の文字が出るまでの時間が既定のときより短くなる (実機で確かめる)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 着手時判定 (オーナー確認済み 2026-10-11): 設定は app-config.json の llmReasoning に「LM Studio 接続先ルート → モデル名 → 値」の表で持ち、単独チャットと参加者で共有する。未選択はモデルの既定 (reasoning_effort を付けない)。要約・メモリ整理・結論・レビューも同じ表を引く (画像説明は別 HTTP 経路なので対象外)。
2. config: llmReasoning を persistedFile に足し、SetReasoning で該当キーだけを書き換える (copy-on-write の map)。UpdateEditable の書き出しでも消えないようにする。
3. LLMClient: 解決済みの接続先とモデルから値を引き、off→none、on→medium (LM Studio は on/off を 400 で拒否するため) に変換して reasoning_effort に載せる。値が無ければ付けない。
4. API: POST /api/configuration/models (llm) の応答に /api/v1/models の reasoning (allowed_options / default) と、その接続先で選択済みの値を足す。PUT /api/configuration/reasoning で 1 件を保存 (allowed_options に無い値と LM Studio で無い接続先は拒否)。
5. UI: 設定画面の LLM モデル欄と参加者パネルのモデル欄に Think の選択を出す (選択肢は「モデルの既定」+ allowed_options、reasoning が null なら出さない)。保存ボタンで一緒に保存する。
6. テスト: llmclient のリクエスト本文 (AC#2/#3)、config の永続化、handler の検証。実機で gemma-4 の TTFT を計測 (AC#5)。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 着手時判定 (オーナー確認 2026-10-11)

- 置き場所: app-config.json の `llmReasoning` に「LM Studio API ルート → モデルキー → 値」で持つ。単独チャットと参加者で共有し、設定画面と参加者カードのどちらからでも同じ値を書き換える。キーに接続先を含めるので、LM Studio 以外の接続先にある同名モデルには値が付かない。参加者の行に列を足す案は、DB 移行が要るうえ、モデルを替えたときに古い値 (gemma に high など) が残るので採らなかった。
- 既定: 未選択はモデルの既定で動かし、`reasoning_effort` を付けない。
- 裏方の呼び出し: 要約・メモリ整理・結論・レビューも、LLMClient が解決済みの接続先とモデルから同じ表を引く。画像説明は別の HTTP 経路なので対象外。
- qwen3.5: この環境 (LM Studio、2026-10-11) では `none` が効いた (reasoning トークン 0)。lmstudio-bug-tracker#1990 の症状は出ず、qwen3.5 だけの特別扱いは入れていない。

## 実装の要点

- LM Studio の `/v1/chat/completions` は `reasoning_effort` に none / minimal / low / medium / high / xhigh だけを受け付け、`on` と `off` には 400 を返す (実測)。保存は LM Studio の語彙のまま行い、送るときに off を none、on を medium に変換する。gemma-4-12b に medium・low を送っても思考が走ることを確認した。
- 保存は `PUT /api/configuration/reasoning` で 1 件ずつ行う。値を受け付けるのは、その接続先の `/api/v1/models` が挙げる allowed_options に含まれる場合だけ (LM Studio 以外の接続先では一覧が取れないので拒否される)。空の値で「モデルの既定」に戻す操作は、接続先に問い合わせずに通す。
- `POST /api/configuration/models` (llm) の応答に `reasoning` (allowedOptions / default / selected) を足した。reasoning が null のモデルは含めない。LM Studio は同じキーのモデルを形式ごとに複数並べることがあり、その中で reasoning が食い違うこと (片方が null) があるので、選択肢を持つ最初のものを使う。
- app-config.json の書き込みは autoCheckUpdates と同じく該当キーだけを書き換える (rewriteKey に切り出した)。UpdateEditable で全体を書き出しても値が消えないようにした。

## 確認したこと

- AC#2・#3: `internal/service/reasoning_test.go`。off→none、on→medium、high→high が非ストリームとストリームの両方で載ること、参加者の解決済みターゲットで引かれること、未選択・選択の解除・別の接続先の同名モデル・LM Studio 以外の接続先では載らないことを確かめた。保存・再読込は `internal/config/config_test.go`、API の検証は `internal/httpapi/handlers_test.go`。`go test ./...`、`pnpm run check:client`、`pnpm run test:client` はすべて通った。
- AC#1・#4: `DATA_DIR` を一時ディレクトリにして `pnpm dev` で起動し、内蔵ブラウザ (Chromium、localhost:34115) で確かめた。設定画面と参加者カードの選択肢は、gemma-4-12b が「モデルの既定 (オン) / オフ / オン」、gpt-oss-20b が「モデルの既定 (低) / 低 / 中 / 高」で、/api/v1/models の allowed_options と一致した。qwen3.8-27b (reasoning が null) では欄が出ない。設定画面で gemma をオフ、参加者カードで gpt-oss を高にしてそれぞれ保存すると、app-config.json に両方が入った。設定画面で保存した値は、参加者カードで同じモデルを選んだときに表示された。実窓 (WKWebView) での見え方は未確認。
- AC#5: LLMClient を通して LM Studio の実機で計測した (2026-10-11)。「日本の首都はどこ？一文で」を送り、本文の最初の文字までの時間を各 3 回測った。
  - gemma-4-12b: 既定 6.06 / 6.99 / 6.72 秒、オフ 0.63 / 0.51 / 0.50 秒、オン 6.69 / 6.79 / 5.57 秒
  - qwen3.5-9b: 既定 31.7 / 29.2 / 32.6 秒、オフ 3.31 (直後の 1 回目) / 0.11 / 0.10 秒、オン 34.6 / 38.3 / 22.0 秒

実窓 (WKWebView) での見え方はオーナーが確認した (2026-10-11、PR #98 のマージ時)。
<!-- SECTION:NOTES:END -->
