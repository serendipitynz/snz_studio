---
id: TASK-82
title: 'LLM: ストリームで区切りの無い応答がバッファを際限なく伸ばす問題と、可視テキストが縮んだときに表示の差分がずれる問題を直す'
status: Done
assignee: []
created_date: '2026-09-28 20:23'
updated_date: '2026-10-04 10:45'
labels: []
dependencies: []
references:
  - internal/service/llmclient.go
  - frontend/src/api/sse.ts
priority: low
type: bug
ordinal: 82000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F16 (Low、確信度 Medium)。2026-09-29 に `756d32b` で未修正を確認した。

- `llmclient.go` のストリーム処理 (376〜407 行付近) の `buffer` に上限が無い。`\n\n` を含まない応答を送り続ける接続先では、際限なく伸びる (受信のたびにタイムアウトが延びる)。
- 可視テキストの差分を前回のルーン数で切り出している (`sliceFromRune(nextVisible, len(visibleContent))`、366〜371 行付近)。これは可視テキストが前方一致で伸び続けることを前提にしている。standard 形式でタグの断片 (`<|en` まで受信) が一時的に可視になり、次のチャンクで取り除かれると、以降の差分が先頭からずれて欠落・重複する。
- 影響: 保存する内容は最終結果の `result.Content` なので正しい。ずれるのはストリーム中の表示だけで、`done` フレームで上書きされる。バッファの肥大は、悪意のある接続先か壊れた接続先が前提。

## 方針

- バッファに上限 (例: 1MB) を設け、超えたら中断する。
- 差分を「前回の可視テキストとの共通接頭辞」から計算し、縮んだ場合は置き換えのイベントを送る (フロントの SSE の処理も合わせる)。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 区切り (\n\n) を含まない応答が上限を超えたら、ストリームを中断してエラーにする。テストがある
- [x] #2 タグの断片が分かれて届く応答で、ストリーム中に表示される内容が最終結果と一致する (欠落も重複もしない)。テストがある
- [x] #3 単独チャットと多人数会話の両方で、ストリーム中の表示が従来どおり動く
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. llmclient.go: ストリームの区切り待ちバッファに 1 MiB の上限を設け、区切りを処理した後もなお超えていればエラーで中断する。
2. llmclient.go: onDelta を func(StreamDelta) に変える。StreamDelta{Text, Replace}。次の可視テキストが前回の可視テキストを接頭辞に持てば差分 (追記)、持たなければ (縮んだ・分岐した) 可視テキスト全体での置き換えにする。置き換えを全文にするのは、Go のルーン数と JS の UTF-16 長がずれるため、位置指定の部分置換を避けるため。
3. 呼び出し側 (chat.go / review.go / turnengine.go / httpapi) を StreamDelta に合わせる。SSE は Replace のとき event: replace を送る。単独チャットの失敗時フォールバック本文も、途中まで流れた本文への追記ではなく置き換えで送る (保存内容と一致させる)。
4. フロント: delta / replace を表示テキストに反映する小さな関数を api/ に置き、ChatPage (チャット・レビュー) と MultiAgentChatPage から使う。node --test でテストする。
5. テスト: バッファ上限超過でエラー、タグ断片が分かれて届く standard / llm_jp_thinking 応答でストリーム表示が最終結果と一致、httpapi で replace イベントが流れる。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装の要点

- バッファ上限: `maxStreamEventBytes = 1 MiB`。区切り (`\n\n`) を処理し終えた後に残るバイト数で判定するので、上限は「区切り待ちの 1 イベント」に掛かり、応答全体の長さには掛からない。
- 差分: `onDelta` を `func(StreamDelta)` (`Text` / `Replace`) に変えた。次の可視テキストが前回の可視テキストを接頭辞に持てば差分を追記、持たなければ可視テキスト全体での置き換え (SSE `event: replace`)。
  - 「先頭 N 文字を残して追記」の部分置換にしなかった理由: Go はルーン数、フロントは UTF-16 長で数えるので、絵文字などで位置がずれる。置き換えはまれなので全文送信で足りる。
  - 縮む原因はタグ断片だけではない。llm_jp_thinking では、タグ無しで見えていた本文が遅れて来た channel タグで丸ごと消える (テストで再現)。
- 単独チャットの失敗時フォールバックも `replace` で送るようにした。従来は途中まで流れた本文の後ろに追記され、done で上書きされるまで二重に見えていた。保存内容 (フォールバック全文) と表示が一致する。
- 使われなくなった `sliceFromRune` を削除。`turnengine.go` の書き込みだけで読まれていない `streamed` も削除。
- フロントは `api/streamText.ts` (delta / replace の適用) を ChatPage のチャット・レビューと MultiAgentChatPage で共有。
- README (英日) と docs/multi-agent-chat-design.md の SSE の説明に `replace` を追記。

## 検証

- AC1: `TestCreateChatCompletionStreamBoundsUnseparatedData` (区切り無し 4 MiB を送る接続先でエラー)、`TestCreateChatCompletionStreamLongResponse` (区切りありで合計 1.25 MiB 超の応答は完走)。
- AC2: `TestCreateChatCompletionStreamFollowsVisibleText` (standard の `<|en`+`d|>`、3 分割の `<|message|>`、llm_jp_thinking の遅れた final channel の 3 例で、各チャンク後の表示が「その時点の可視テキスト」と一致し最終結果で終わる)。`frontend/test/streamText.test.ts`。
- AC3: `TestChatStreamShowsStoredContent` (単独チャット: タグ断片の撤回、途中失敗→フォールバック)、`TestMultiAgentTurnStreamWithdrawsTagFragment` (HTTP のフレーム列 delta→replace→delta を再生すると保存発言と一致)、既存の `TestSendMessageStream` / `TestMultiAgentTurnStream` ほか。
- 実アプリ (`DATA_DIR` を隔離した `pnpm dev` + ブラウザ): LM Studio の gemma-4-e4b-it-qat で単独チャット 343 delta・多人数会話 16 delta が従来どおり逐次表示。タグを分割して返す偽 LLM に向けると、単独チャット・多人数会話・レビューの 3 箇所とも表示が `前半の文です。<|en` → `前半の文です。` → … → 保存内容と同じ文、の順に推移した。
- `go vet ./...`、`go test ./...`、`pnpm check:client`、`pnpm test:client` (18 件)、`pnpm build:client` すべて成功。

## 見ていないもの

- `rawContent` (区切りを処理済みの応答全体) の長さには上限を設けていない。タスクの対象は区切り待ちバッファのみ。
- `gofmt -l` が `internal/search/model.go` を挙げるが、本タスク以前からの状態で未変更。
<!-- SECTION:NOTES:END -->
