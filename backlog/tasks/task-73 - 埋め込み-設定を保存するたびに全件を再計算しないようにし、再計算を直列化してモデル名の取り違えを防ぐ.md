---
id: TASK-73
title: '埋め込み: 設定を保存するたびに全件を再計算しないようにし、再計算を直列化してモデル名の取り違えを防ぐ'
status: To Do
assignee: []
created_date: '2026-09-28 20:22'
labels: []
dependencies: []
references:
  - internal/httpapi/handlers.go
  - internal/httpapi/server.go
  - internal/service/embeddingsync.go
priority: medium
type: bug
ordinal: 73000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F7 (Medium)。2026-09-29 に `756d32b` で未修正を確認した。

- `PUT /api/configuration` (`handlers.go`) は、埋め込みの設定が変わっていなくても、埋め込みが有効なら毎回 `RebuildAll` をゴルーチンで起動する。
- `RebuildAll` と `SyncMissing` に排他が無く、起動時 (`RunStartupTasks`)・サイドカーの ready 時 (`onEmbeddingReady`)・設定保存時から同時に走り得る。
- `RebuildAll` はベクトルを計算した**後**に `s.embeddings.GetModel()` でモデル名を読んで保存する。再計算の最中に埋め込みモデルを切り替えると、旧モデルのベクトルが新モデルの名前で保存され得る (競合したときだけ)。TASK-41 で入った `SyncMissing` は冒頭でモデル名を確定しているので該当しない。
- 影響: 大きなコーパスでは、設定画面で保存するたびに長時間の CPU / ネットワーク負荷がかかる。モデル名の取り違えが起きると、次元の異なるベクトル同士でコサインを計算することになり、検索の質が黙って落ちる。

## TASK-66 との関係

TASK-66 は、サイドカーの版を上げた後の再計算を「起動時と設定保存時の `RebuildAll`」に任せている。ただし内蔵モードでは、起動時にはサイドカーが ready になっておらず埋め込みが無効なので `RunStartupTasks` の `RebuildAll` は走らない。ready 時に走るのは欠けた分だけを埋める `SyncMissing` なので、全件を作り直すのは設定保存時だけになっている。保存時の再計算を絞るなら、版を上げた後など全件を作り直したいときの手段を別に残す。

## 方針

- 埋め込みに関わる欄 (モード、接続先、モデル、API キー) が変わったときだけ再計算する。
- 再計算を 1 つのワーカーに直列化し、走行中に来た要求は「もう一度走らせる」フラグにまとめる。
- モデル名はバッチの開始時に確定し、保存するときに現行のモデルと一致しなければ捨てる。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 埋め込みに関わる欄を変えずに設定を保存しても、全件の再計算が起動しない
- [ ] #2 起動時・ready 時・設定保存時の再計算が同時に走らない (走行中に来た要求は、走行後の 1 回にまとまる)。テストがある
- [ ] #3 再計算の最中に埋め込みモデルを切り替えても、旧モデルで計算したベクトルが新モデルの名前で保存されない。テストがある
- [ ] #4 サイドカーの版を上げた後などに全件を作り直す手段があり、TASK-66 の移行がその手段で成り立つことが、README か TASK-66 に書かれている
<!-- AC:END -->
