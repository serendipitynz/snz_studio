---
id: TASK-68
title: >-
  ビルド: ruri-v3-30m-q8_0.gguf を macOS / Windows のどちらでも用意・同梱できるようにし、sha256 が OS
  をまたいで一致するか確かめる
status: To Do
assignee: []
created_date: '2026-09-28 19:48'
labels: []
dependencies:
  - TASK-67
references:
  - scripts/build-ruri-gguf.sh
  - internal/embed/modelspec.go
  - internal/embed/downloader.go
  - internal/embed/sidecar_windows.go
  - build/windows/installer/project.nsi
  - .github/workflows/build.yml
type: task
ordinal: 68000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
内蔵 embedding のモデル `ruri-v3-30m-q8_0.gguf` を、macOS と Windows のどちらでも用意して同梱できるようにする。
あわせて、Windows と macOS で同じ sha256 の GGUF を作れるかを確かめる。

現状の問題:
- `scripts/build-ruri-gguf.sh` は macOS でしか動かない。`llama-${LLAMA_RELEASE}-bin-macos-*.tar.gz` の
  `llama-quantize` を使い、BSD の `stat -f%z`、`shasum`、`DYLD_LIBRARY_PATH` に依存している。
  WSL (Linux) でも動かない
- CI (`build.yml`) は macOS / Windows のどちらにも GGUF を置いていない。`modelspec.go` の URL は
  仮の値 (`REPLACE_OWNER`) のままでダウンロードにも頼れないので、CI で作ったアプリでは内蔵 embedding が起動しない
- Windows 版は exe の横の GGUF を初回起動時に `%AppData%\snz-studio\models\` へコピーする
  (`bundledModelPath`)。そこに置く手順が無い

アプリは `verifyFile` で GGUF の sha256 とサイズを `modelspec.go` の値と照合し、一致しないファイルは使わない。
このため、sha256 が OS をまたいで一致するかで、取れる方式が変わる。

確かめること (結論はタスクに記録する):
1. 今の手順 (HF → f16 → `llama-quantize` で Q8_0) を Windows の `llama-quantize.exe` (b9437) で実行して、
   `2a6cb2d9…` / 41,569,120 バイトと一致するか。f16 への変換は Python (numpy) で決定的に動くはず。
   差が出るとすれば、量子化の C 実装のコンパイラや SIMD の違い、Windows で入る Python 依存の版の違いが疑わしい
2. Python だけで完結する経路: b9437 の gguf-py の `Q8_0` は「ggml-quants.c の参照実装とビット単位で同じ結果」を
   うたっており、`convert_hf_to_gguf.py` は `--outtype q8_0` を受け付ける。この経路なら OS に依存するバイナリが
   消える。ただし f16 を経由せずに量子化するため、今の sha256 とは一致しない見込みで、`modelspec.go` の値を
   更新することになる
3. OS ごとに作り直さない方式: 検証済みの GGUF を改変されない場所 (Hugging Face の固定 revision や
   GitHub Release のアセット) に置き、sha256 を照合してから取得する。`modelspec.go` の URL の TODO を
   埋めることにもなる。`build-ruri-gguf.sh` は出所を再現する手順として残す。
   公開の場所に置くことになるので、採るかはユーザーが決める (ruri-v3-30m は Apache-2.0 で再配布は許されている)

方式が決まったら、TASK-67 のスクリプトと同じやり方で GGUF を置けるようにする。置き場所:
- 開発用: `data/models/` (`wails dev` のデータディレクトリの配下)
- macOS の配布用: `.app/Contents/Resources/`
- Windows の配布用: exe の横

ステージングを `wails build` より前にできるなら、NSIS インストーラにサイドカーと GGUF を `File` で入れられる
可能性がある (`build/windows/installer/project.nsi` のコメントが挙げている、見送りの前提が崩れる)。
同梱するか引き続き見送るかも、このタスクで決める。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 今の手順を Windows で実行したときに sha256 2a6cb2d9… と一致するかの結果と、一致しない場合の原因がタスクに記録されている
- [ ] #2 各 OS で GGUF を用意する方式 (各 OS で作り直す / Python だけで作る / 検証済みのファイルを取得する) が決まり、modelspec.go の sha256 とサイズを変えるかどうかも含めて記録されている
- [ ] #3 macOS と Windows (WSL なし) の両方で、決めた方式により、開発用・配布用の場所に sha256 を照合した GGUF を置ける
- [ ] #4 CI の macOS / Windows ジョブが GGUF も置いていて、作ったアプリはネットワークからダウンロードせずに、初回起動で内蔵 embedding が ready になる
- [ ] #5 NSIS インストーラにサイドカーと GGUF を入れるかどうかが、同梱するか引き続き見送るかで決着している
- [ ] #6 README.md / README.ja.md と THIRD_PARTY_NOTICES.md (GGUF の生成に使った版) が、決めた方式に合わせて更新されている
<!-- AC:END -->
