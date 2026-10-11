---
id: TASK-99
title: 'チャット: 思考中のモデルの思考内容をストリームで受け取り、本文の上に折りたたみ表示する'
status: In Review
assignee: []
created_date: '2026-10-10 22:58'
updated_date: '2026-10-11 00:07'
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
- [x] #1 ストリームの delta.reasoning_content と delta.reasoning が本文とは別の差分として取り出され、本文には混ざらない (テストで確かめる)
- [x] #2 単独アシスタントのチャットと多人数会話の両方で、思考中に思考内容が画面へリアルタイムに出る
- [x] #3 本文が始まると思考の欄が畳まれ、開けば中身を読める
- [x] #4 次のターンのプロンプト履歴に思考内容が含まれない (テストで確かめる)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
着手時の判定 (オーナー, 2026-10-11): 思考は DB に保存し、書き出しには含めない。本文に混ざる <think> も思考欄へ振り分ける。doc-17 の更新はこのタスクでは行わない (アプリ側の記録だけ)。

1. llmresponse に SplitThinking を足す (先頭の <think>…</think>、開き無しの …</think>、途中の閉じタグ断片)。SanitizePromptContent は assistant の思考を落とす。
2. LLMClient: delta.reasoning_content / delta.reasoning と <think> 部分を思考として集め、StreamDelta.Reasoning で本文と別に渡す (append / replace は本文と同じ扱い)。非ストリームも message の同名フィールドを読む。usage.completion_tokens_details.reasoning_tokens を ReasoningTokens に取る (無ければ思考の文字から見積もる)。
3. DB: migration 020 で messages に reasoning (TEXT) と reasoning_tokens (INTEGER) を足し、model / repository / ChatService / TurnEngine で保存する。レビューの stream は思考を流さない。
4. SSE: 思考は reasoning / reasoning-replace の frame で送る。
5. フロント: streamText に思考の適用を足し、ReasoningFold (吹き出しの中の <details>) を単独チャットと多人数会話の両方で本文の上に出す。本文が始まったら畳む。出力トークンの表示は「N tok (思考 M)」にする。
6. テスト: 思考が本文に混ざらないこと (AC#1)、次のターンの履歴に思考が入らないこと (AC#4) を Go のテストで、streamText の適用を node --test で確かめる。実機の LM Studio (gpt-oss-20b / gemma) で画面を確かめる。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 着手時の判定 (オーナー, 2026-10-11)
- 思考は DB に保存する (messages.reasoning / reasoning_tokens、migration 020)。Markdown の書き出しには含めない。
- 本文に混ざる `<think>…</think>` も思考欄へ振り分ける。先頭の `<think>` ブロックのほか、開きタグの無い `…</think>` (チャットテンプレートが開きタグをプロンプト側に書く型) も扱う。ただし本文中に `<think>` が出てくる場合は、タグについて話している回答とみなして触らない。llm_jp_thinking 形式の analysis チャネルは今までどおり除去するだけで、思考欄には出さない。
- doc-17 (snz-design の適用記録) はこのタスクでは更新しない。思考欄は吹き出し (doc-17 §4 の会話。アプリ固有の例外) の中に置き、吹き出しに既にある「この話者になった理由」と同じ `<details>` + `Summary` の形にした。後で doc-17 §4 の会話の行に足す。

## 実装の要点
- `LLMClient` は `delta.reasoning_content` / `delta.reasoning` と `<think>` 部分を思考として集め、`StreamDelta.Reasoning` で本文と別に渡す。本文と同じく全体を毎回読み直し、伸びるだけなら追記、そうでなければ置き換え (`</think>` だけの型で本文に出ていた部分が思考へ移るとき)。SSE は `reasoning` / `reasoning-replace` の frame で送るので、知らない読み手 (レビューのダイアログ) は読み飛ばす。レビューの stream はサービス側でも思考を流さない。
- 単独チャットは本文を逐次 DB に書いているが、思考は完了時に `FinalizeMessage` で1回だけ書く。表示中は frame から描くので逐次保存は要らない。
- 出力トークン: `outputTokens` は思考を含む全生成トークンのまま (tok/s もこれ ÷ 全時間のまま)。思考の分を `usage.completion_tokens_details.reasoning_tokens` から取り、無ければ思考の文字から見積もり、「313 tok (思考 302)」と添える。usage が無いときの全体の見積もりも思考の文字を含めるようにした。思考の無い返答は reasoning_tokens を NULL で保存する。
- `SanitizePromptContent` は assistant の `<think>` ブロックを落とす。この変更より前に保存された `<think>` 入りの返答が履歴に戻らないようにするため。

## 検証
- AC#1: `TestCreateChatCompletionStreamSeparatesReasoning` (reasoning_content / reasoning / `<think>` を断片で送る3通り。本文と思考がそれぞれ正しく、思考が終わる前に本文が出ないこと、usage から 12 tok・思考 5 tok を取ること)、`TestCreateChatCompletionNonStreamSeparatesReasoning`、`TestSplitThinking` (8通り)。
- AC#4: `TestChatKeepsReasoningOutOfHistory` (単独チャット。2ターン目の要求に1ターン目の思考が無く、回答はあること)、`TestTurnEngineKeepsReasoningOutOfHistory` (多人数会話。3ターン)、`TestSanitizePromptContentDropsThinking`。
- SSE の frame 名: `TestSSEStreamDeltaFrames`。フロントの適用: `frontend/test/streamText.test.ts` に3件。
- AC#2・#3: 実機の LM Studio を相手に、ビルドした SPA と API を同じ origin で配る使い捨てのサーバー (`_sandbox` に置き、確認後に削除) を立て、Claude の内蔵ブラウザ (Chromium 系。WKWebView ではない) で確かめた。gpt-oss-20b (delta.reasoning) で、単独チャットは送信 1.5 秒後に「思考中…」の欄が開いて思考が流れ、本文のストリーム中 (送信ボタンが処理中のまま) に「思考内容」へ畳まれ、開くと全文を読めた。表示は「3.5s · 313 tok (思考 302) · 88.3 tok/s」。多人数会話も、ターン中に思考が流れ、保存後の発言では畳まれていた。gemma-4-12b (delta.reasoning_content) は API 経由で、reasoning frame 132 個 → 本文 "399"、保存された思考 148 tok / 全体 156 tok を確かめた。
- `go vet ./...`・`go test ./...`・`pnpm check:client`・`pnpm test:client` (27件) が通る。

## オーナーに見てほしいこと
- 実窓 (Wails の WKWebView) での見え方と4配色での思考欄の比は測っていない。本文の色は吹き出しの補助の語と同じ `--meta-ink` (`onAccentSoft`) を使い、左の罫は装飾として `lineMedium` を当てた。
- 思考を長く読みたいとき、欄は高さを制限していない (吹き出しがそのまま伸びる)。

## レビュー対応 (PR #99 第1ラウンド)
- 開きタグの無い `…</think>` を思考の終わりとみなす扱いは撤回した。`</think>` を引用するだけの普通の回答 (例: 「閉じタグは `</think>` です」) が後ろの数文字に切り詰められ、その形で保存・書き出し・履歴に入ってしまうため。応答からは両者を見分けられず、そういうテンプレートを示す設定も無いので、思考として扱うのは応答の先頭の `<think>` ブロックだけにした。上の「着手時の判定」の該当箇所はこれで置き換わる。
<!-- SECTION:NOTES:END -->
