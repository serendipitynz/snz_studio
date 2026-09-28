---
id: TASK-66
title: '同梱: 内蔵 embedding のサイドカー llama.cpp を b9437 から b11126 に上げる'
status: To Do
assignee: []
created_date: '2026-09-28 19:47'
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

保存済みのベクトルは古い版で計算しているが、embedding が有効な状態での起動時と設定保存時に
`RebuildAll` が全件を計算し直すので、移行処理は要らない。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 build.yml (macOS / Windows) と build-mac-signed.sh が同梱するサイドカーの版が b11126 になっている
- [ ] #2 macOS と Windows の両方で、b11126 のサイドカーが既存の GGUF (sha256 2a6cb2d9…) を読み込み、/health と 256 次元の probe を通って内蔵 embedding が ready になる
- [ ] #3 ready になった後の再計算 (RebuildAll) を経て、サンプル文書での意味検索の結果が b9437 のときと比べて明らかに劣化していない
- [ ] #4 THIRD_PARTY_NOTICES.md の同梱バイナリの版と llama.cpp の LICENSE 全文が b11126 のものになっている。GGUF の生成に使った版の記述は b9437 のまま
- [ ] #5 README.md / README.ja.md / docs/local-generation-design.md の同梱版の記述が更新されている
<!-- AC:END -->
