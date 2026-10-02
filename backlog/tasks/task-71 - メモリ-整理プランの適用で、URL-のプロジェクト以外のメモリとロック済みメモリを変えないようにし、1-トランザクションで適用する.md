---
id: TASK-71
title: 'メモリ: 整理プランの適用で、URL のプロジェクト以外のメモリとロック済みメモリを変えないようにし、1 トランザクションで適用する'
status: Done
assignee: []
created_date: '2026-09-28 20:22'
updated_date: '2026-10-02 08:48'
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
- [x] #1 別プロジェクトのメモリ ID (ロック済みを含む) を remove / update に含めたプランを適用しても、そのメモリが変わらない。テストがある
- [x] #2 同じプロジェクトのロック済みメモリは、従来どおり整理で変わらない
- [x] #3 途中に不正な kind を含むプランは全体がロールバックされ、どのメモリも変わらない。テストがある
- [x] #4 analyze から apply までの通常の流れの既存テストが通る
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. MemoryRepository に ApplyOrganization(projectID, changes) を追加し、create / update / remove を 1 トランザクションで適用する。update / remove は WHERE id = ? AND project_id = ? AND locked = 0 で絞る (別プロジェクトとロック済みはトランザクション内の SQL で no-op になる)。FTS 行の更新も同じトランザクションで行う
2. 呼び出し元が整理だけになっている UpdateMemory はこの経路に統合して削除する。DeleteMemory は DELETE /api/memories/{id} (プロジェクトを持たない手動削除) が使うので ID 指定のまま残す
3. ApplyProjectPlan は冒頭で sanitizePlan をかけ直してから ApplyOrganization を呼び、埋め込みの同期はコミット後に行う
4. テスト: リポジトリで別プロジェクト・ロック済みの不変と、途中の不正 kind (CHECK 制約違反) による全体ロールバック。サービスで別プロジェクト ID を含むプランの適用。既存の UpdateMemory 利用テストは ApplyOrganization に置き換える
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装の要点

- `MemoryRepository.ApplyOrganization(projectID, changes)` を追加し、整理プランの create / update / remove と FTS 行の更新を 1 トランザクションで適用する。`ApplyProjectPlan` は冒頭で `sanitizePlan` をかけ直してからこれを呼び、埋め込みの同期はコミット後に行う。
- 別プロジェクトとロック済みの判定は、事前に一覧を引いて照合するのではなく、UPDATE / DELETE 文の `WHERE id = ? AND project_id = ? AND locked = 0` に置いた。照合をサービスに残すと「一覧を引いた時点」と「書き込む時点」がずれる。条件を文に入れればトランザクション内で判定され、当てはまらない ID は 0 行更新の no-op になる。
- 方針にあった「`DeleteMemory` / `UpdateMemory` に project_id の条件を加える」は次の形で実現した。
  - `UpdateMemory` は呼び出し元が整理だけだったので `ApplyOrganization` に統合して削除した。`shared_with_all` を 0 に戻す理由のコメントも移した。`docs/multi-agent-chat-design.md` の該当箇所のメソッド名も直した。
  - `DeleteMemory` は手動削除の `DELETE /api/memories/{memoryId}` が使っており、このルートはプロジェクトを持たないので ID 指定のまま残した。整理での削除は `ApplyOrganization` 内の project_id 付き DELETE を通る。

## AC の根拠

- #1: `TestMemoryOrganizerApplyKeepsOtherProjectsAndLocked` (サービス) で、別プロジェクトのメモリ (ロック済み・未ロック) に update / remove を当てたプランを適用し、適用前と `reflect.DeepEqual` で一致することを確認した。リポジトリ層の `TestMemoryApplyOrganization` でも同じことと、FTS 行が書き換わらないことを確認した。
- #2: 上の 2 テストに同じプロジェクトのロック済みメモリへの update / remove を含め、変わらないことを確認した。
- #3: `TestMemoryApplyOrganizationRollsBackOnFailure` で、update → 不正 kind の create (CHECK 制約違反) → remove の順のプランがエラーを返し、メモリ一覧と FTS が適用前のままであることを確認した。
  - サービス経由では `sanitizePlan` が不正 kind の変更を先に捨てるので、CHECK 違反は起きない。そのためロールバックはトランザクションの入口であるリポジトリで確かめている。
- #4: `TestMemoryOrganizerFallbackDedupe` (analyze → apply) と `TestMemoryOrganizerLockedDuplicateKept` が通る。`go vet ./...` と `go test -count=1 ./...` は全パッケージ成功。
- テストが不具合を検出できることは変異で確かめた。WHERE から project_id / locked の条件を外すと #1 / #2 のテストが落ち、エラー時に途中コミットさせると #3 のテストが落ちる。

## 見ていないもの

- UI からの整理の実操作は確かめていない。フロントは analyze が返したプランをそのまま送り返すだけで、この変更で API の入出力の形は変わっていない。

## レビュー対応 (PR #70 第 1 ラウンド)

- Codex の [P2]: サービス経由では `sanitizePlan` が不正な kind の create を黙って捨てるため、統合プラン (統合先の create + 元メモリの remove) の create だけが落ち、元のメモリが置き換え先のないまま消える。AC #3 はプラン単位の話なので指摘どおりと判断した。
- 対応として、`sanitizePlan` が 1 件でも変更を落とすプランは `ErrMalformedPlan` で全体を拒否し、HTTP は 400 `plan has a malformed change` を返すようにした。analyze が返すプランは sanitize 済みなので、通常の流れで拒否されることはない。別プロジェクトの ID やロック済みメモリへの変更は、従来どおり拒否せず no-op にしている (AC #1 / #2)。
- AC #3 の根拠に次を加えた。`TestMemoryOrganizerApplyRejectsMalformedPlan` では、不正 kind の create と remove を組んだプランがエラーになり、メモリが変わらない。`TestMemoryOrganizeFallback` では、HTTP が 400 を返す。拒否の条件を外す変異を入れると、どちらのテストも落ちる。
<!-- SECTION:NOTES:END -->
