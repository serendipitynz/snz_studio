---
id: TASK-82
title: 'LLM: ストリームで区切りの無い応答がバッファを際限なく伸ばす問題と、可視テキストが縮んだときに表示の差分がずれる問題を直す'
status: To Do
assignee: []
created_date: '2026-09-28 20:23'
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
- [ ] #1 区切り (\n\n) を含まない応答が上限を超えたら、ストリームを中断してエラーにする。テストがある
- [ ] #2 タグの断片が分かれて届く応答で、ストリーム中に表示される内容が最終結果と一致する (欠落も重複もしない)。テストがある
- [ ] #3 単独チャットと多人数会話の両方で、ストリーム中の表示が従来どおり動く
<!-- AC:END -->
