---
id: TASK-66
title: '同梱: 内蔵 embedding のサイドカー llama.cpp を b9437 から b11126 に上げる'
status: Done
assignee: []
created_date: '2026-09-28 19:47'
updated_date: '2026-10-02 20:08'
labels: []
dependencies: []
references:
  - .github/workflows/build.yml
  - scripts/build-mac-signed.sh
  - THIRD_PARTY_NOTICES.md
  - internal/embed/sidecar.go
type: chore
ordinal: 66000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
内蔵 embedding のサイドカー (`llama-server`) は llama.cpp `b9437` (2026-05-30) に固定している。
動作確認で問題が無かった `b11126` (2026-09-23) に上げる。

対象はサイドカーとして同梱するバイナリの版だけ。GGUF の生成に使う llama.cpp
(`scripts/build-ruri-gguf.sh` の既定値、`internal/embed/modelspec.go` のコメント、
`THIRD_PARTY_NOTICES.md` の量子化に使った版の記述) は既存の GGUF (sha256 `2a6cb2d9…`) を
作った版の記録なので `b9437` のまま残す。生成側の版を変えるかは TASK-68 で決める。

サイドカーの版を固定している箇所:
- `.github/workflows/build.yml` の `LLAMA_RELEASE` (macOS / Windows の 2 か所)
- `scripts/build-mac-signed.sh` の `LLAMA_RELEASE` の既定値
- `THIRD_PARTY_NOTICES.md` の同梱バイナリの版 (66 行目付近) と llama.cpp の LICENSE 全文
- README.md / README.ja.md、`docs/local-generation-design.md` の同梱版の記述

b11126 にも `llama-b11126-bin-macos-arm64.tar.gz` / `llama-b11126-bin-win-cpu-x64.zip` があり、
アセット名の形式は変わっていない (確認済み)。

## 保存済みのベクトルの作り直しと TASK-73 との関係

保存済みのベクトルは古い版で計算している。内蔵モードで版を上げた後にそれらを作り直すのは、
2026-09-29 時点のコードでは設定保存時の `RebuildAll` だけになる:

- 起動時の `RunStartupTasks` の `RebuildAll` は、embedding が有効なときだけ走る。内蔵モードでは
  起動した時点でサイドカーがまだ ready になっておらず embedding が無効なので、走らない
- サイドカーが ready になったとき (`onEmbeddingReady`) に走るのは `SyncMissing` で、現行モデルの
  ベクトルが欠けた行だけを埋める。モデル名 (`ruri-v3-30m`) は版を上げても変わらないので、
  古い版のベクトルは残る
- `PUT /api/configuration` は、embedding が有効なら設定が変わっていなくても毎回 `RebuildAll` を起動する。
  いまはこれが版を上げた後の作り直しになっている

TASK-73 はこの「設定保存のたびに全件を再計算する」挙動をやめ、埋め込みに関わる欄が変わったときだけ
再計算するようにする。TASK-73 が先に入ると、設定保存でも作り直されなくなり、古い版のベクトルが残り続ける。
このため:

- TASK-66 が先なら、今の設定保存時の `RebuildAll` で作り直せる。ただし利用者が設定を保存しない限り
  作り直されないので、移行の手順 (設定を一度保存する) を記録するか、版を上げたことを検出して作り直す
- TASK-73 が先なら、TASK-73 の AC #4 の「全件を作り直す手段」を使って作り直す

どちらの場合も、版を上げた後に全件が作り直されることを AC #3 の確認に含める。版の違いでベクトルが
どれだけ変わるか (作り直しが実際に必要か) も、AC #3 の比較で分かる。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 build.yml (macOS / Windows) と build-mac-signed.sh が同梱するサイドカーの版が b11126 になっている
- [x] #2 macOS と Windows の両方で、b11126 のサイドカーが既存の GGUF (sha256 2a6cb2d9…) を読み込み、/health と 256 次元の probe を通って内蔵 embedding が ready になる
- [x] #3 ready になった後の再計算 (RebuildAll) を経て、サンプル文書での意味検索の結果が b9437 のときと比べて明らかに劣化していない
- [x] #4 THIRD_PARTY_NOTICES.md の同梱バイナリの版と llama.cpp の LICENSE 全文が b11126 のものになっている。GGUF の生成に使った版の記述は b9437 のまま
- [x] #5 README.md / README.ja.md / docs/local-generation-design.md の同梱版の記述が更新されている
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 変更
- TASK-67 以降、サイドカーの版は scripts/sidecar.mjs だけで固定している (build.yml と build-mac-signed.sh はこのスクリプトを呼ぶ)。Description が挙げる build.yml / build-mac-signed.sh の LLAMA_RELEASE はもう無いので、RELEASE と 4 つの sha256 を b11126 に書き換えた
- sha256 は GitHub の asset digest と、4 アーカイブを自分でダウンロードして計算した値が一致
- THIRD_PARTY_NOTICES.md: 同梱バイナリの版を b11126 に。b11126 タグの LICENSE は現行の全文 (Copyright (c) 2023-2026 The ggml authors) と完全一致したので本文は変更なし。GGUF の量子化に使った版 (108 行目) は b9437 のまま
- docs/local-generation-design.md の同梱版を b11126 に。README / README.ja には同梱版の記述が既に無い (TASK-67 で「scripts/sidecar.mjs で固定」に置き換え済み) ので変更なし
- sidecar.mjs の「ModernBERT を含む (>= b9437)」は最低版の条件なのでそのまま

## 保存済みベクトルの作り直し (着手時の判断)
- ユーザー判断 (2026-10-03): コードは変えず、作り直しは不要と記録する。版の検出による自動再計算や README の移行手順は入れない
- 根拠 (macOS arm64、CPU): 同じ GGUF で b9437 と b11126 の llama-server を並べ、サンプル文書 94 チャンクを埋め込んだところ、全要素がビット単位で一致 (要素差の最大 0)。古い版のベクトルが残っても検索結果は変わらない
- 作り直したい場合は、TASK-73 が入るまでは設定を一度保存すれば RebuildAll が走る

## 確認 (macOS arm64)
- AC#3: 検証用プログラム (git 管理外の _sandbox/task66) で、実際の embed.Manager / RebuildAll / RetrievalService を使って確認。サンプル文書 11 件を取り込み、b9437 で ready → RebuildAll → 検索 8 本。同じデータで b11126 に差し替えて ready → RebuildAll → 同じ検索。b11126 の再計算前後とも保存ベクトル 108 件が b9437 のものと完全一致し、8 本の検索結果 (文書・スコア・順位) も同一
- AC#2 (macOS): 同じプログラムで b11126 のとき Manager の状態が ready、dim=256。配布版も pnpm build:app → pnpm sidecar --app → 一時 DATA_DIR で .app を起動し、サイドカーの /health が ok、/v1/embeddings が 256 次元、/props の build_info が b11126-b1ff4ca23、読み込んだ dylib が 0.24.0 系 (b11126) であることを確認
- go vet / go test ./... / pnpm check:client / pnpm test:client は通過。CI と build-mac-signed.sh (署名・公証) は未実行

## 見つけたこと
- pnpm build:app は -clean しないので、手元の .app の Resources に前の版の dylib (0.13.1) が残る。読み込まれるのは symlink 先の新しい版だけ。build-mac-signed.sh は -clean 付き、CI は新しい作業ツリーで作るので配布物には入らない
- b11126 の macOS アーカイブには ggml-metal-tuning / ggml-rpc-server などの実行ファイルが増えているが、sidecar.mjs は llama-server と dylib だけを置くので影響なし

## 未確認 (Windows)
- AC#2 の Windows 分。CI (workflow_dispatch) の Windows ジョブで b11126 が取得・配置されること、その成果物か実機の pnpm sidecar + pnpm dev で内蔵 embedding が ready になること

## 確認 (Windows、2026-10-03)
- 実機 (ユーザー確認): pnpm sidecar → pnpm dev で内蔵 embedding が ready (AC#2 の Windows 分)
- CI (workflow_dispatch、run 37057977503、HEAD 6bd3ffe): Windows ジョブの pnpm sidecar --app が b11126 の win-cpu-x64.zip を取得・照合して build\bin に 30 ファイル (llama-server.exe と DLL 29 個、TASK-67 と同じ構成) を置いた。macOS ジョブも b11126 を .app の Resources に 36 ファイル置いた。両ジョブとも成功
<!-- SECTION:NOTES:END -->
