---
id: TASK-100
title: '検索: FTS5 の bm25() 列重みが UNINDEXED 列にずれて当たっているのを直す'
status: In Review
assignee: []
created_date: '2026-10-11 07:31'
updated_date: '2026-10-11 07:51'
labels: []
milestone: m-3
dependencies: []
references:
  - internal/service/retrieval.go
  - internal/db/schema.go
  - internal/service/retrieval_test.go
modified_files:
  - internal/service/retrieval.go
  - internal/service/retrieval_test.go
priority: medium
type: bug
ordinal: 104000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

SQLite の bm25(table, w1, w2, ...) は UNINDEXED 列を含む FTS5 テーブルの全列に、引数の位置どおりに重みを割り当てる。modernc.org/sqlite で実測すると、fts5(pid UNINDEXED, title, content) に対して第 1 引数に 10 を渡しても順位は変わらず、第 2 引数の 10 は title を押し上げた。

internal/service/retrieval.go の 3 つの検索クエリは UNINDEXED 列のぶんを数えずに重みを渡しており、すべて左にずれている。Node 版 (backend/src/services/retrievalService.ts, 1cabd66) の初期実装から続いている。

| クエリ | 渡している重み | 実際に効いている重み |
|---|---|---|
| ドキュメント (2 箇所) | bm25(document_chunks_fts, 10, 2, 1, 1, 4) | title 1, note 4, tags 1, derived_text 1, content 1 |
| メモリ | bm25(memories_fts, 2, 6, 8) | kind 8, title 1, content 1 |

## 作業

- UNINDEXED 列のぶん 0.0 を前置きし、引数の並びどおりの重みを各列に当てる (オーナーと確認済み: ドキュメントは title 10, note 2, tags 1, derived_text 1, content 4、メモリは kind 2, title 6, content 8)。
- 列の追加や並べ替えで重みが再びずれたら落ちるテストを足す。

## 影響

ハイブリッドの配分 (ドキュメントは FTS 正規化 0.55 + cosine 0.45、メモリは 0.5/0.5) と semantic fallback の閾値は、ずれた重みのもとで調整されてきた。直すと検索結果の順位が変わるので、実データで手動確認する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 ドキュメントの 2 つの検索クエリで、bm25 の重みが UNINDEXED 3 列に 0.0、title 10 / note 2 / tags 1 / derived_text 1 / content 4 の順に当たる
- [x] #2 メモリの検索クエリで、bm25 の重みが UNINDEXED 2 列に 0.0、kind 2 / title 6 / content 8 の順に当たる
- [x] #3 ドキュメント: タイトルだけに語がある文書が、同じ条件で本文だけに語がある文書より上に来ることをテストで確かめる
- [x] #4 メモリ: 本文だけに語があるメモリが、同じ条件でタイトルだけに語があるメモリより上に来ることをテストで確かめる
- [ ] #5 PR に、ハイブリッドの配分と fallback 閾値が旧重みで調整されていたこと、実データでの手動確認が必要なことを書く
- [x] #6 embedding 無効時のドキュメント検索で、正規化後 0 の最下位行が生の bm25 値に戻って首位へ上がらない
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- 追加したテストは、同じ文字列をタイトルと本文で入れ替えた 2 件を使う。列ごとの重みが等しいと同点になるので、旧重み (ずれた状態) ではスコアの厳密な大小比較で落ちることを確かめた。
- 着手中に見つけた既存の不具合も同じタスクで直した (オーナーと合意)。rankDocumentCandidates の FTS-only 分岐は、正規化で 0 になった最下位行だけ生の bm25 値を使っていた (Node 版の `ftsScores[index] || Number(row.score)` から移植したもの)。IDF が下限に張り付かない規模のコーパスでは生の値が 1 を超え、最も弱い一致が首位になる。22 件のコーパスで実測すると、旧重みでは正しい順だったが新重みでは逆転した。重みを直すと生の値が大きくなるので、重みだけ直すと退行になる。メモリ側はもともと正規化後の値だけを使っている。
- TestSearchDocumentsResultsAcrossBothPaths のスナップショットは、竜の年代記 chunk 1 のスコアだけが 0.868932 から 0.762864 に変わった。文書の順位は変わらない。
- go vet ./... と go test ./... が通る。
<!-- SECTION:NOTES:END -->
