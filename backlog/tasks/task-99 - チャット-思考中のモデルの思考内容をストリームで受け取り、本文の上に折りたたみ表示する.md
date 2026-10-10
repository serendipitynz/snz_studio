---
id: TASK-99
title: 'チャット: 思考中のモデルの思考内容をストリームで受け取り、本文の上に折りたたみ表示する'
status: To Do
assignee: []
created_date: '2026-10-10 22:58'
updated_date: '2026-10-10 22:59'
labels: []
milestone: m-3
dependencies: []
references:
  - internal/service/llmclient.go
  - internal/service/chat.go
  - internal/httpapi/sse.go
  - internal/httpapi/multiagent.go
  - frontend/src/api/streamText.ts
  - frontend/src/App.tsx
  - frontend/src/pages/MultiAgentChatPage.tsx
priority: medium
type: feature
ordinal: 103000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

オーナーの指摘 (2026-10-11)。思考するモデルを使うと、考えている間は画面に何も出ず、止まっているように見える。

LM Studio はストリームの中で思考を本文とは別のフィールドで送ってくる。gemma-4-12b は `delta.reasoning_content`、gpt-oss-20b は `delta.reasoning` だった (2026-10-11 に実測)。

`LLMClient.CreateChatCompletionStream` は `delta.content` しか読まないので、思考は全部捨てている。gemma-4-12b を既定のまま使うと、最初の思考トークンは 1.5 秒で届いているのに、画面に最初の文字が出るのは 8.45 秒後だった。

## 作業

- ストリームの `delta.reasoning_content` と `delta.reasoning` を読み、本文とは別の種類の差分として呼び出し元へ渡す。
- SSE で思考の差分をフロントへ流す。単独アシスタントのチャットと多人数会話の両方で。
- チャット画面と多人数会話の画面で、思考を本文の上の折りたたみ欄にリアルタイムで出す。本文が始まったら畳む。
- 思考はプロンプト履歴に戻さない (今の挙動を保つ)。
- 出力トークン数と tokens/s の表示は、`completion_tokens` に思考の分が入ることを踏まえて見直す。

## 着手時に決めること

- **保存するか**: 思考を DB に残して後から開けるようにするか、表示中だけにするか。残すなら書き出し (export) に含めるか。
- **本文に混ざる `<think>` タグ**: LM Studio が思考を分けずに本文へ `<think>…</think>` で返す設定やモデルの場合も、同じ欄に振り分けるか。
- **見た目**: snz-design の adoption guide (doc-16) と snz_studio の adoption record (doc-17) に沿う部品があるか確かめ、無ければ例外として記録する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 ストリームの delta.reasoning_content と delta.reasoning が本文とは別の差分として取り出され、本文には混ざらない (テストで確かめる)
- [ ] #2 単独アシスタントのチャットと多人数会話の両方で、思考中に思考内容が画面へリアルタイムに出る
- [ ] #3 本文が始まると思考の欄が畳まれ、開けば中身を読める
- [ ] #4 次のターンのプロンプト履歴に思考内容が含まれない (テストで確かめる)
<!-- AC:END -->
