---
id: TASK-98
title: 'LLM: Think (reasoning) の強さをモデルごとに選べるようにし、選んだ値をリクエストの reasoning_effort で送る'
status: To Do
assignee: []
created_date: '2026-10-10 22:58'
updated_date: '2026-10-10 22:59'
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
- [ ] #1 単独アシスタントのチャットと多人数会話の参加者の両方で、モデルごとに reasoning の値を選んで保存できる
- [ ] #2 値を選んだモデルへのリクエストに reasoning_effort が載り、LM Studio の off は none で送られる (テストで確かめる)
- [ ] #3 値を選んでいないモデル、または LM Studio 以外の接続先へのリクエストには reasoning_effort が載らない (テストで確かめる)
- [ ] #4 設定画面の選択肢が /api/v1/models の allowed_options と一致し、reasoning が null のモデルでは切り替えを出さない
- [ ] #5 gemma-4 で off を選ぶと、本文の最初の文字が出るまでの時間が既定のときより短くなる (実機で確かめる)
<!-- AC:END -->
