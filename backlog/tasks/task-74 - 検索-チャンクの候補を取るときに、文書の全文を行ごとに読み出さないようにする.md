---
id: TASK-74
title: '検索: チャンクの候補を取るときに、文書の全文を行ごとに読み出さないようにする'
status: To Do
assignee: []
created_date: '2026-09-28 20:22'
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
- [ ] #1 FTS の候補取得と意味検索のフォールバックのどちらも、チャンクの行ごとに文書の全文を読み出さない
- [ ] #2 検索結果 (採用される文書・チャンクと参照の表示) が変更前と同じであることをテストで確かめている
- [ ] #3 大きな文書 (例: 1MB・数百チャンク) を持つプロジェクトで、検索 1 回の確保メモリか所要時間を変更の前後で計測して記録している
- [ ] #4 ベクトルの保存形式を変えるかどうかの判断が記録されている
<!-- AC:END -->
