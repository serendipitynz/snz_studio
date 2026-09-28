---
id: TASK-71
title: 'メモリ: 整理プランの適用で、URL のプロジェクト以外のメモリとロック済みメモリを変えないようにし、1 トランザクションで適用する'
status: To Do
assignee: []
created_date: '2026-09-28 20:22'
labels:
  - security
dependencies: []
references:
  - internal/service/memoryorganizer.go
  - internal/repository/memory.go
  - internal/httpapi/handlers.go
priority: medium
type: bug
ordinal: 71000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F4 (Medium)。2026-09-29 に `756d32b` で未修正を確認した。

- `MemoryOrganizerService.ApplyProjectPlan` は、クライアントが送り返したプランを検証し直さない (`sanitizePlan` をかけるのは analyze 側だけ)。
- ロックの判定は URL のプロジェクトのメモリ一覧から引いた `current` で行う。別のプロジェクトの `memoryId` では `current == nil` になってロックの判定が飛ばされ、`DeleteMemory` / `UpdateMemory` が ID だけで実行される (リポジトリも `project_id` で絞らない)。
- 変更はトランザクションの外で 1 件ずつ適用するため、途中で失敗する (例: `kind` の CHECK 制約違反) と一部だけ適用された状態で終わる。
- 成立条件: 通常の UI 操作では起きない (analyze が返す ID は当該プロジェクトのもの)。API を直接叩いたとき、フロントの不具合、LLM が別プロジェクトの ID を出したときに起きる。
- 影響: 単一ユーザーなので権限昇格ではないが、「ロックしたメモリは整理で消えない」という不変条件とプロジェクト単位のデータの境界が、API の段で守られていない。

## 方針

- `ApplyProjectPlan` の冒頭で `sanitizePlan` をかけ直し、`memoryId` が当該プロジェクトのメモリに無い remove / update は捨てる。
- `DeleteMemory` / `UpdateMemory` に `project_id` の条件を加える。
- 全変更を 1 トランザクションで適用し、埋め込みの同期はコミット後に行う。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 別プロジェクトのメモリ ID (ロック済みを含む) を remove / update に含めたプランを適用しても、そのメモリが変わらない。テストがある
- [ ] #2 同じプロジェクトのロック済みメモリは、従来どおり整理で変わらない
- [ ] #3 途中に不正な kind を含むプランは全体がロールバックされ、どのメモリも変わらない。テストがある
- [ ] #4 analyze から apply までの通常の流れの既存テストが通る
<!-- AC:END -->
