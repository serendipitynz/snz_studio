---
id: TASK-73
title: '埋め込み: 設定を保存するたびに全件を再計算しないようにし、再計算を直列化してモデル名の取り違えを防ぐ'
status: In Review
assignee: []
created_date: '2026-09-28 20:22'
updated_date: '2026-10-02 22:33'
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
- [x] #1 埋め込みに関わる欄を変えずに設定を保存しても、全件の再計算が起動しない
- [x] #2 起動時・ready 時・設定保存時の再計算が同時に走らない (走行中に来た要求は、走行後の 1 回にまとまる)。テストがある
- [x] #3 再計算の最中に埋め込みモデルを切り替えても、旧モデルで計算したベクトルが新モデルの名前で保存されない。テストがある
- [x] #4 サイドカーの版を上げた後などに全件を作り直す手段があり、TASK-66 の移行がその手段で成り立つことが、README か TASK-66 に書かれている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. EmbeddingSyncService に直列ワーカーを足す (RequestRebuild / RequestSyncMissing / WaitIdle)。走行中の要求は保留の 1 件にまとめ、全件の要求は欠けた分の要求を包含する
2. RebuildAll / SyncMissing / SyncDocument / SyncMemories はモデル名を開始時に確定し、バッチごとと保存の直前に現行モデルと比べて、違えば捨てる
3. PUT /api/configuration: 保存前後の Editable を比べ、モード (外部なら接続先・モデルも) が変わったときだけ RequestRebuild。変わらなければ RequestSyncMissing (TASK-77 の穴埋めを残す。ユーザー判断 2026-10-03)。API キーは環境変数専用で PUT では変わらない
4. RunStartupTasks と onEmbeddingReady もワーカー経由にする
5. 作り直す手段: POST /api/embeddings/rebuild と、設定モーダルの独立した区画の「埋め込みを作り直す」ボタン (ユーザー判断 2026-10-03)。snz-design doc-16/17 に沿う
6. テスト: 直列化と合流、モデル切替中の破棄、変更なし保存で全件を再計算しない
7. README / README.ja に作り直す手段と版上げ後の使い方を書き、TASK-66 の移行がそれで成り立つことを記す
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 着手時の判断 (ユーザー, 2026-10-03)
- 全件を作り直す手段は、設定モーダルの独立した区画「埋め込みの作り直し」のボタンと POST /api/embedding/rebuild。版の検出による自動再計算は入れない (外部モードで同じ名前のモデルを差し替えた場合にも使えるため)
- 埋め込みの欄を変えずに保存したときは SyncMissing を走らせる。TASK-77 が書く「無効から戻った後の穴埋めは設定保存が担う」経路を、全件の再計算なしで残すため

## 変更
- EmbeddingSyncService に直列ワーカー (RequestRebuild / RequestSyncMissing / WaitIdle) を足した。走行中の要求は保留の 1 件にまとまり、全件の要求は欠けた分の要求を包含する。起動時・ready 時・設定保存・新しいエンドポイントはすべてワーカー経由
- モデル名をパスの開始時に確定し、バッチごとと保存の直前に現行モデルと比べ、違えば捨てる。開始時に確定するだけでは足りない: CreateEmbeddings は呼ぶたびに設定を読むので、切替後のバッチは新モデルで計算されて旧モデルの名前で保存される。ベクトルは 1 チャンク 1 行なので、混ざったまま保存すると新モデルのパスの結果を上書きし得る。捨てた分は、切替が起動するパス (保存時の RebuildAll か ready 時の SyncMissing) が埋める
- SyncDocument / SyncMemories も同じ経路 (embedChunks / embedMemories) に寄せ、モデル名を開始時に確定するようにした
- 変更の判定は保存前後の Editable で行う (embeddingSourceChanged)。モードが変われば変更、外部モードなら接続先とモデルも見る。内蔵モードでは保存済みの外部の欄は使われないので見ない。API キーは EMBEDDING_API_KEY (環境変数) だけで PUT では変わらないので判定に入れていない。実効設定 (cfg.Get) で比べなかったのは、保存の最中にサイドカーが ready になると変更と誤判定して全件を作り直すため
- README / README.ja の「内蔵 embedding のサイドカー」に、版を変えたときの作り直し方と、b9437 → b11126 は作り直し不要だったこと (TASK-66 の計測) を書いた

## AC の根拠
- #1: TestSavingConfigurationRebuildsEmbeddingsOnlyWhenTheSourceChanges。外部モードで LLM モデルだけを変えて保存すると埋め込みの入力は 0 件、埋め込みモデルを変えると全 2 件。TestEmbeddingSourceChanged で内蔵モードの外部欄の変更が変更扱いにならないことも確認
- #2: TestCorpusPassesRunOneAtATimeAndCollapseWhileRunning。1 回目のパスの最初の要求を止めている間に SyncMissing / RebuildAll を計 4 回要求し、同時に走った要求は最大 1、埋め込んだ入力は 6 (走行中の 1 回 + まとまった 1 回、各 3 件)。ワーカーを外すと失敗することを確認
- #3: TestRebuildDiscardsVectorsWhenTheModelSwitchesMidPass。最初のバッチの計算中にサイドカーのモデルを切り替えると、旧・新どちらの名前でも何も保存されず、次のパスで全件が新モデルで保存される。破棄の判定を外すと失敗することを確認
- #4: 設定モーダルのボタンと POST /api/embedding/rebuild (TestRebuildEmbeddingsEndpoint: 202 で全件、埋め込み無効なら 409)。pnpm dev の実アプリ (内蔵モード、data/ のチャンク 212 件・メモリ 7 件) でボタンを押し、全行の updated_at が押した時刻に更新されたことを確認。README に記述
- go test ./... (全パッケージ)、-race 付きの httpapi / service、go vet、pnpm check:client、pnpm test:client が通る

## 見ていないもの
- 設定モーダルの新しい区画の 4 配色の比と WebKit での見え方は測っていない。部品は既存の Card / SubsectionTitle / Subtle / ActionButton (normal) / FailureNotice だけで、Chromium の標準配色で表示を確認した
- 外部モードの起動時の RebuildAll (毎回全件) は変えていない。本タスクの範囲外

## レビュー 1 回目 (Codex, PR #74) への対応
- [P2] 外部モードで同じモデル名のまま接続先を変え、全件の再計算が失敗すると、旧接続先のベクトルがモデル名で区別できないまま残り、以後の変更なし保存 (SyncMissing) では入れ替わらない。全件の再計算を要求したらその旨 (rebuildOwed) を覚え、再計算が最後まで通るまでは欠けた分の要求も全件の再計算として走らせるようにした。入力ごとの拒否では解除を止めない (拒否される入力が 1 件あるだけで毎回全件に戻るため)。覚えておくのはメモリ上だけで、外部モードは起動時に全件を作り直すので再起動で失われても困らない。テスト TestFailedSourceChangeRebuildRunsOnTheNextSave (解除の条件を外すと失敗することを確認)
<!-- SECTION:NOTES:END -->
