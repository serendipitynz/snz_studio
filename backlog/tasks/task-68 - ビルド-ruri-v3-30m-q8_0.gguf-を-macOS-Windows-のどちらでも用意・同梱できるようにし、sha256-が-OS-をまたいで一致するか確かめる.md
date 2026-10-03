---
id: TASK-68
title: >-
  ビルド: ruri-v3-30m-q8_0.gguf を macOS / Windows のどちらでも用意・同梱できるようにし、sha256 が OS
  をまたいで一致するか確かめる
status: In Review
assignee: []
created_date: '2026-09-28 19:48'
updated_date: '2026-10-03 20:58'
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
- [x] #1 今の手順を Windows で実行したときに sha256 2a6cb2d9… と一致するかの結果と、一致しない場合の原因がタスクに記録されている
- [x] #2 各 OS で GGUF を用意する方式 (各 OS で作り直す / Python だけで作る / 検証済みのファイルを取得する) が決まり、modelspec.go の sha256 とサイズを変えるかどうかも含めて記録されている
- [x] #3 macOS と Windows (WSL なし) の両方で、決めた方式により、開発用・配布用の場所に sha256 を照合した GGUF を置ける
- [x] #4 CI の macOS / Windows ジョブが GGUF も置いていて、作ったアプリはネットワークからダウンロードせずに、初回起動で内蔵 embedding が ready になる
- [x] #5 NSIS インストーラにサイドカーと GGUF を入れるかどうかが、同梱するか引き続き見送るかで決着している
- [x] #6 README.md / README.ja.md と THIRD_PARTY_NOTICES.md (GGUF の生成に使った版) が、決めた方式に合わせて更新されている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 検証済みの GGUF (2a6cb2d9…, 41,569,120 バイト) を、このリポジトリの GitHub Release (タグ ruri-v3-30m-q8_0-2a6cb2d9) のアセットとして公開する。Apache-2.0 の全文と出所 (HF revision、llama.cpp b9437、build-ruri-gguf.sh) を添える
2. modelspec.go の URL をそのアセットに置き換える。sha256 とサイズは変えない
3. scripts/sidecar.mjs (pnpm sidecar) で GGUF も置く。FileName / URL / SHA256 / SizeBytes は modelspec.go から読み、値を 1 か所に保つ。build/sidecar/.downloads/ に照合済みのコピーを残す (data/models/ に照合済みのコピーがあればそれを使う)。置き場所は、開発用が data/models/、--app が .app の Contents/Resources か exe の横
4. NSIS: project.nsi で、build/sidecar/windows-amd64/ のサイドカーと .downloads/ の GGUF を File /nonfatal で取り込む。CI では wails build より前に pnpm sidecar --arch amd64 を実行し、インストール先にファイルがあることを確かめる
5. CI: Windows はステージングを wails build より前に移し、成果物に GGUF を加える。両 OS で、HTTPS_PROXY を死んだプロキシに向けてアプリを起動し、サイドカーの /health が通ることを確かめる (ダウンロードせずに ready になることの確認)
6. build-mac-signed.sh: GGUF のコピー (MODEL_SRC) をやめ、sidecar.mjs --app に任せる
7. build-ruri-gguf.sh は出所を再現する手順として残す。説明を書き直し、HF のディレクトリ名がメタデータに入ることを書く
8. README.md / README.ja.md / THIRD_PARTY_NOTICES.md を更新する
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## AC#1: 今の手順を Windows で実行した結果 (一致した)
使い捨てのブランチ task-68-gguf-repro-experiment で、CI の windows-latest / macos-latest × Python 3.10 / 3.12 の 4 通りを実行した (run 37151765482)。
- f16: 4 通りとも 04635fa9… / 75,975,520 バイト
- 今の手順 (f16 → llama-quantize b9437 Q8_0): Windows の llama-quantize.exe でも 2a6cb2d9… / 41,569,120 バイトで一致。コンパイラ・SIMD・Python の版による差は出なかった
- Python だけの経路 (convert_hf_to_gguf.py --outtype q8_0): 4 通りとも 622719ed… / 41,569,120 バイト。今のファイルとはテンソル 62 個がビット単位で同じで、KV の値も同じ。違うのは KV の並び順だけ (llama-quantize は general.file_type と quantization_version を末尾に書き直す)
- 落とし穴: HF からダウンロードしたディレクトリの名前が general.name / basename / size_label に入る。1 回目の CI は hf/ に置いたので 64 バイト小さい別の sha256 になった (run 37151576776。ローカルでも再現)。build-ruri-gguf.sh は ruri-v3-30m の名前で置くので影響はない

## AC#2: 方式 (検証済みのファイルを取得する。ユーザー決定 2026-10-04)
- どの方式でも OS をまたいで再現できたので、配布の手間で選んだ。作り直す方式は、開発機と CI の毎回の実行に Python と torch が要る
- 検証済みの 2a6cb2d9… を、このリポジトリの GitHub Release ruri-v3-30m-q8_0-2a6cb2d9 のアセットとして公開した (Apache-2.0 の全文と出所を添えた)。modelspec.go の URL をこれに置き換え、sha256 とサイズは変えない (ベクトルは変わらないので RebuildAll は不要)
- build-ruri-gguf.sh は出所を再現する手順として残した (macOS 専用のまま)
- 新しい GGUF は新しいタグで出す (アセットは差し替えない)。Release の immutable 設定はリポジトリの設定なので触っていない

## AC#3: 置き方
- pnpm sidecar が GGUF も置く。開発用は data/models/、--app は .app の Contents/Resources か exe の横。FileName / URL / SHA256 / SizeBytes は modelspec.go から読む (値を 1 か所に保つため。Windows の CRLF のチェックアウトでも読めるようにした)。照合済みのコピーを build/sidecar/.downloads/ に残し、data/models/ に照合済みのコピーがあればそれを使う
- 確認 (macOS): 再利用、2 回目は何もしない、取得し直し、sha256 を 1 文字変えると中断して何も置き換えない
- 確認 (Windows): CI (run 37153059977) で pnpm sidecar --arch amd64 が data/models/ に、pnpm sidecar --app が build/bin/ に置いた

## AC#4: CI
- macOS は pnpm sidecar --app --arch arm64 が .app に GGUF も置く。Windows はステージングを wails build の前に移し、成果物に *.gguf を加えた
- 両 OS で、データディレクトリを空にし、HTTP(S)_PROXY を閉じたポートに向けてアプリを起動するステップを加えた。llama-server が /health に答えて 5 秒後も同じプロセスのままなら ready とみなす (プローブの埋め込みに失敗するとアプリがサイドカーを止めるため)。シードされた GGUF の sha256 も照合する
- run 37153059977 で両 OS とも通過 (Windows はインストール先から起動して約 11 秒)
- ローカルの macOS でも同じスクリプトが通った。GGUF を抜くと失敗し (ダウンロードもできない)、プロキシを外すとアプリが新しい URL からダウンロードして ready になった
- macOS のスモークテストは pgrep のパターンを行頭に固定している。/bin/sh の親プロセス監視ラッパーも同じパスを引数に持つため

## AC#5: NSIS (同梱する。ユーザー決定 2026-10-04)
- project.nsi で、build/sidecar/windows-${ARCH}/ の llama-server.exe と *.dll、build/sidecar/.downloads/ の GGUF を File /nonfatal で取り込む。ステージングしていない手元のビルドでは、内蔵 embedding なしのインストーラになる
- CI はサイレントインストールしたディレクトリに llama-server.exe / ggml.dll / llama.dll / GGUF があることを確かめる
- 見送っていた理由のうち「makensis の後に取得する」は解消した。「署名が無い」は、置くだけの配布と同じ状態なので理由にならないと判断した

## 測っていないこと
- Windows の実機での pnpm dev と、インストーラからの起動 (CI のランナーでは確認した)
- 署名・公証つきの build-mac-signed.sh の実行 (bash -n のみ)
- 使い捨てのブランチ task-68-gguf-repro-experiment は残してある (CI のログの参照元)

## テスト
- go vet / go test ./... / pnpm check:client / test:client は通過。gofmt -l は internal/search/model.go を挙げるが、このタスクより前からある
<!-- SECTION:NOTES:END -->
