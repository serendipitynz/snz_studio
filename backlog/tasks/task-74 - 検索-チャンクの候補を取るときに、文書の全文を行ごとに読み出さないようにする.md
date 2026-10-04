---
id: TASK-74
title: '検索: チャンクの候補を取るときに、文書の全文を行ごとに読み出さないようにする'
status: In Review
assignee: []
created_date: '2026-09-28 20:22'
updated_date: '2026-10-04 06:16'
labels: []
dependencies: []
references:
  - internal/service/retrieval.go
  - internal/repository/document.go
priority: medium
type: enhancement
ordinal: 74000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F8 (Medium、影響は条件付き)。2026-09-29 に `756d32b` で未修正を確認した。

- `retrieval.go` の FTS の候補取得 (326 行付近) と意味検索のフォールバック (587 行付近) は、1 行 = 1 チャンクなのに、行ごとに `d.content_text AS full_content` (文書の全文) を SELECT する。
- 意味検索のフォールバックは WHERE がプロジェクトとモデルだけなので、プロジェクト内の全チャンクについて文書の全文を読み、さらに全ベクトルの JSON をパースする。
- 問題になる条件: 大きな Markdown (例: 1MB の原稿で数百チャンク) を持つプロジェクトで、FTS の候補が `limit` に満たないとき (よくある)。確保するメモリはおおよそチャンク数 × 文書サイズになる。

## 方針

- 全文は、採用が決まった文書についてだけ後から 1 回取得する。
- ベクトルの保存形式 (JSON から float32 の BLOB へ) やパース結果のキャッシュも候補。保存形式の変更はマイグレーションを伴うので、このタスクで行うかは着手時に決める。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 FTS の候補取得と意味検索のフォールバックのどちらも、チャンクの行ごとに文書の全文を読み出さない
- [x] #2 検索結果 (採用される文書・チャンクと参照の表示) が変更前と同じであることをテストで確かめている
- [x] #3 大きな文書 (例: 1MB・数百チャンク) を持つプロジェクトで、検索 1 回の確保メモリか所要時間を変更の前後で計測して記録している
- [x] #4 ベクトルの保存形式を変えるかどうかの判断が記録されている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 変更前のコードで、FTS の経路と意味検索のフォールバックの両方を通る小さなコーパスの検索結果 (採用文書の順・チャンク・抜粋・スコア・全文) をテストに固定する
2. fetchDocumentFtsCandidates と searchDocumentsBySemantic の SELECT から d.content_text を外し、候補行と docGroup から全文を除く
3. SearchDocuments が limit で切った後の採用文書だけ、documents から content_text を 1 回の IN クエリで取り、FullDocumentContent に入れる (案 A)
4. 1 のテストが変更後も通ることを確かめ、BenchmarkRetrievalLargeDocument で変更後を計測する
5. ベクトルの保存形式は変えない (JSON パースは 478 チャンクで約 11 ms。移行コストに見合わない) と記録する
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 変更前の計測 (main @ 308cd51, Apple M2, Go 1.27.1)

`go test ./internal/service/ -run '^$' -bench BenchmarkRetrievalLargeDocument -benchtime 5x -count 5` (retrieval_bench_test.go)

コーパス: 1 プロジェクトに原稿 1 本 (1,048,589 バイト、418 チャンク) とメモ 20 本 (各 6KB、計 60 チャンク)。全 478 チャンクに 256 次元のベクトル (embedding_json 平均 3,105 バイト、ruri-v3-30m のモデル名)。クエリ「飛竜騎士団」は原稿の全 418 チャンクに当たり、FTS の候補 48 行がすべて原稿になるため、SearchDocuments は意味検索のフォールバックまで進む。

| 対象 | 所要時間 (中央値) | 確保メモリ/回 |
|---|---|---|
| fetchDocumentFtsCandidates | 約 563 ms | 101.8 MB |
| searchDocumentsBySemantic | 約 216 ms | 894.2 MB |
| SearchDocuments (全体) | 約 739 ms | 996.7 MB |

内訳の切り分け (一時ベンチ、コミットしない):
- FTS のクエリから d.content_text を外すと約 2.7 ms / 0.33 MB。ORDER BY score LIMIT のソーターが、一致した 418 行すべての全文を抱えるため時間もかかっている。
- 意味検索のクエリから d.content_text を外すと約 15〜17 ms / 9.6 MB。
- そのうちベクトルの JSON パースはメモリ上だけで 478 本 11.3 ms / 3.5 MB。同じベクトルを float32 の BLOB から読む場合は 0.37 ms / 0.98 MB。

## 変更後の計測 (同じコマンド・同じコーパス)

| 対象 | 変更前 | 変更後 |
|---|---|---|
| fetchDocumentFtsCandidates | 約 563 ms / 101.8 MB | 約 2.7 ms / 0.35 MB |
| searchDocumentsBySemantic | 約 216 ms / 894.2 MB | 約 16.1 ms / 9.7 MB |
| SearchDocuments (全体) | 約 739 ms / 996.7 MB | 約 22.0 ms / 13.0 MB |

SearchDocuments 全体で、時間は約 97%、確保メモリは約 98.7% 減った。各値は 5 回 × 5 セットの中央値。

## 全文の扱い (AC #1)

候補取得の 2 つの SQL から d.content_text を外し、limit で切った後の採用文書 (最大 limit 件) についてだけ、1 回の IN クエリで全文を取って FullDocumentContent に入れる (attachFullDocumentContent)。検索結果の FullDocumentContent は現在どこからも読まれていない (IncludeFullDocument は検索結果では常に false で、全文を使う明示参照は文書レコードから取り直している)。それでも空にする案は採らなかった。将来、検索結果に IncludeFullDocument を立てる呼び出し元ができたとき、空の全文が黙って使われるのを避けるため (ユーザー判断、案 A)。

## 結果が変わらないことの確認 (AC #2)

TestSearchDocumentsResultsAcrossBothPaths を追加した。5 文書のうち FTS で 2 文書、意味検索のフォールバックで 2 文書が採用され、1 文書は下限で落ちる。採用文書の順・カテゴリ・スコア・チャンク (番号・スコア・長さ)・抜粋を文字列で固定し、FullDocumentContent が文書の全文と一致することも確かめる。期待値は変更前のコードで出力を取って固定し、変更後も同じテストが通ることを確認した。

## ベクトルの保存形式 (AC #4)

変えない。全文をやめた後に残る意味検索の約 16 ms のうち、JSON パースは約 11 ms (478 本)。float32 の BLOB にすれば 0.37 ms になるが、短縮はクエリの埋め込み計算や LLM の生成と比べて体感できない差にとどまる。一方で document_chunk_embeddings と memory_embeddings の移行が要り、sqlite3 で中身を読めなくなり、既存の float64 値との差で同点付近の順位が入れ替わりうる。パースの時間はチャンク数に比例するので、数千〜1 万チャンク (約 240 ms) 規模のプロジェクトが現れたら別タスクで扱う。
<!-- SECTION:NOTES:END -->
