---
id: TASK-72
title: 'チャット: ストリーミング中のアシスタント発言の DB 保存と可視テキストの再計算を、受信のたびに全文で行わないようにする'
status: To Do
assignee: []
created_date: '2026-09-28 20:22'
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
- [ ] #1 長文生成 (例: 4000 トークン) 1 回の UPDATE 回数と所要時間を、変更の前後で計測して記録している
- [ ] #2 途中保存が時間か文字数で間引かれ、生成の完了時には最終内容が保存される。生成中にプロセスが終わったときに失われる範囲が記録されている
- [ ] #3 可視テキストの計算が delta ごとに全文を走査しない。走査を残す場合は、問題にならないことを計測で示している
<!-- AC:END -->
