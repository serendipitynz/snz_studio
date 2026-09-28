---
id: TASK-84
title: '設定: app-config.json をアトミックに書き、壊れたファイルを読んだときに黙って既定値に戻さないようにする'
status: To Do
assignee: []
created_date: '2026-09-28 20:23'
labels: []
dependencies: []
references:
  - internal/config/config.go
priority: low
type: bug
ordinal: 84000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F18 (Low)。2026-09-29 に `756d32b` で未修正を確認した。

- `config.go` の保存 (351 行付近) は `os.WriteFile` で直接上書きする。書き込みはロックを解放した後なので、同時の `PUT /api/configuration` で書き込みの順序が入れ替わり得る。
- 読み込み側は、JSON が壊れていると**黙って**環境変数の既定値に戻る。

## 方針

- 一時ファイルに書いて `os.Rename` する。書き込みもロックの中で行う。
- パースに失敗したらログに出し、壊れたファイルを `.bak` に退避する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 app-config.json の書き込みが一時ファイルと rename で行われ、ロックの中で行われる
- [ ] #2 同時の PUT /api/configuration の後、最後に受け付けた内容がファイルに残る。テストがある
- [ ] #3 壊れた app-config.json を読んだとき、ログに出し、壊れたファイルを退避してから既定値で起動する。テストがある
<!-- AC:END -->
