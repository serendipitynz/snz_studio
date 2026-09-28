---
id: TASK-78
title: '埋め込み: サイドカーを止めた直後の起動要求が無視される競合と、外部モードに切り替えた後も status が ready のまま残る不具合を直す'
status: To Do
assignee: []
created_date: '2026-09-28 20:23'
labels: []
dependencies: []
references:
  - internal/embed/manager.go
  - internal/httpapi/server.go
priority: low
type: bug
ordinal: 78000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F12 (Low、確信度 Medium)。2026-09-29 に `756d32b` で未修正を確認した。

- `Manager.Shutdown` は cancel して sidecar を Stop するだけで、`started` を下ろさない。`started = false` になるのは `superviseSidecar` が終わったとき (`markStopped`)。その前に `EnsureInternalReady` が来ると起動済みとみなして何もしないため、内蔵モードなのにサイドカーが動かない。成立するのは外部から内蔵への素早い切り替えのときで、窓は短い。
- 意図して止めたときは、`superviseSidecar` が status を更新せずに return する (`ctx.Err() != nil` の分岐)。このため外部モードに切り替えた後も `GET /api/embedding/status` が `ready` を返し続ける。これは切り替えのたびに起きる。

## 方針

- `Shutdown` で status を `disabled` にし、`started` の解除を同期的に行う。または世代番号を持たせて、古い run を無視する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Shutdown の直後に EnsureInternalReady を呼ぶと、サイドカーが改めて起動する。偽のバイナリを使うテストがある
- [ ] #2 外部モードに切り替えた後、GET /api/embedding/status が ready を返さない。テストがある
- [ ] #3 内蔵モードの通常の起動と終了 (TASK-41 で確かめた終了経路を含む) が変わらない
<!-- AC:END -->
