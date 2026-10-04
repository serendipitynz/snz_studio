---
id: TASK-72
title: 'チャット: ストリーミング中のアシスタント発言の DB 保存と可視テキストの再計算を、受信のたびに全文で行わないようにする'
status: Done
assignee: []
created_date: '2026-09-28 20:22'
updated_date: '2026-10-04 20:36'
labels: []
dependencies: []
references:
  - internal/service/chat.go
  - internal/service/llmclient.go
  - internal/llmresponse
priority: medium
type: enhancement
ordinal: 72000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F6 (Medium、確信度 Medium、未実測)。2026-09-29 に `756d32b` で未修正を確認した。

- `chat.go` の単独チャットのストリーミングは、delta を受け取るたびに `UpdateMessageContent(assistantMessage.ID, streamedContent.String())` で全文を UPDATE する。
- `llmclient.go` は delta を受け取るたびに、それまでの全文へ `ParseAssistantResponse` (正規表現 5〜10 本) をかけて可視テキストを求める。
- 出力長 n に対し、DB の書き込み量と正規表現の走査量がどちらも O(n²)。DB は `SetMaxOpenConns(1)` なので、UPDATE の間は他のリクエスト (別チャットの表示、多人数会話のターン保存など) がすべて同じ接続を待つ。
- 問題になりそうな条件 (推測): 数千トークンの長文生成、遅いディスク、多人数会話の自動進行と並行した単独チャットの生成。
- 多人数会話のターンは `UpdateMessageContent` を呼んでいない (呼ぶのは `chat.go` だけ)。

## 方針 (計測してから決める)

- まず長文生成 1 回あたりの UPDATE 回数と所要時間を計測する。
- DB への途中保存を時間 (例: 500ms) か文字数で間引く。
- 可視テキストの計算は、タグが現れたときだけ評価し直すなど差分にする。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 長文生成 (例: 4000 トークン) 1 回の UPDATE 回数と所要時間を計測して記録している (計測の結果、変更は行わない)
- [x] #2 途中保存を間引く必要があるかを、UPDATE 1 回の所要時間と並行する読み出しの待ち時間の計測で判断し、その判断を記録している
- [x] #3 可視テキストの計算が delta ごとに全文を走査しない。走査を残す場合は、問題にならないことを計測で示している
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 計測 (2026-10-05、main @ ecefa6b、Apple M2 / 内蔵 SSD、modernc SQLite の WAL)

使い捨てのテスト (コミットしていない) で、`CreateChatCompletionStream` に httptest の SSE サーバーから 4000 チャンク (1 チャンク 2 文字の日本語、最終 22KB) を流し、`chat.go` と同じく delta ごとに `UpdateMessageContent` で全文を保存した。並行して、200 発言ある別チャットへ `GetMessagesWithReferences` を 2ms 間隔で繰り返し、待ち時間を測った。書き込み側は待ち無しで流しており、実際の生成 (1 トークンあたり数 ms〜数十 ms) より厳しい条件。2 回計測して同じ傾向。

| 形式 | UPDATE 回数 | UPDATE 合計 | 1 回 (p50 / p99 / 最後の 100 回の平均) | 書いた本文の合計 | 可視テキストの計算 (最後の 100 回の平均) |
|---|---|---|---|---|---|
| standard | 3600 | 425〜870ms | 90µs / 0.8〜2.5ms / 150〜200µs | 38.5MB | 36〜41µs |
| llm_jp_thinking (前半 analysis) | 1800 | 160〜206ms | 70µs / 0.7〜0.9ms / 115〜140µs | 9.6MB | 16〜22µs |

- UPDATE 回数が 4000 より少ないのは、可視テキストが変わらない delta (末尾の空白、analysis の間) で `onDelta` が呼ばれないため。
- 並行する読み出しの待ち時間: 書き込み中 p50 0.5〜0.8ms / p99 0.9〜13.8ms、書き込みなし p50 1.0〜1.3ms / p99 2.9〜3.4ms。1 回目の計測でだけ UPDATE の最大 51ms と読み出しの最大 37ms が出た (WAL の自動チェックポイントと見ている。2 回目は最大 2ms)。

## 判断: 間引きも差分計算も行わない

- 4000 トークン目でも、delta 1 回あたりの保存と再計算は合わせて約 0.25ms。ローカル LLM の 1 トークンの間隔 (100 tok/s でも 10ms) の数 % で、生成時間全体 (4000 トークンで 40〜130 秒) に対して合計 0.5〜0.9 秒。
- `SetMaxOpenConns(1)` による他のリクエストの待ちは、書き込みを待ち無しで流しても読み出しの p50 が悪化しなかった。実際の生成では書き込みは間隔の数 % しか接続を占有しない。
- O(n²) が数字に現れるのは書き込み量だけ (長文 1 回で本文 38.5MB、ページ単位ではこれより多い)。1 日に長文を 100 回生成しても SSD の書き込み寿命に対して無視できる量のため、対応の理由にはしなかった。
- 生成中にプロセスが終わったときに失われる範囲は、今の delta ごとの保存のまま (最後に受け取った delta まで残る。最初の delta の前なら TASK-83 の起動時の削除で空の発言は残らない)。
- 1 回目の計測で出たチェックポイントの数十 ms の待ちは、間引いても頻度が下がるだけで無くならない。問題になる報告が出たら、そのとき間引き (500ms 間隔) を入れる。

AC は計測で閉じる形に書き換えた (#1 の「変更の前後で」と #2 の「間引かれ」を、計測と判断の記録に置き換え)。
<!-- SECTION:NOTES:END -->
