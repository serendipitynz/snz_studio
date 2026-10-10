---
id: TASK-97
title: 'Windows: arm64 版を作り、リリースと自動更新で amd64 版と並べて配る'
status: To Do
assignee: []
created_date: '2026-10-10 21:14'
labels: []
milestone: m-3
dependencies: []
references:
  - .github/workflows/build.yml
  - .github/workflows/release.yml
  - build/windows/installer/project.nsi
  - internal/updater/updater.go
  - internal/updatesig/updatesig.go
  - scripts/sidecar.mjs
priority: medium
type: feature
ordinal: 101000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

オーナーの要望 (2026-10-11)。v0.3.0 で arm64 版 Windows (Windows 11 on Arm) に対応したい。

今の配布は Windows を amd64 しか持たない。

- build.yml の matrix は `windows/amd64` の 1 行だけで、サイドカーも `pnpm sidecar --arch amd64` で置いている。
- release.yml の `attach` ジョブは Windows のインストーラを 1 つだけ集め、`SNZ-Studio-$TAG-Windows-amd64-installer.exe` として添付する。latest.json のキーは `darwin-universal` と `windows-amd64` の 2 つ。
- `internal/updater` の `Platform()` は windows/arm64 で "" を返す。arm64 で動かすと更新の確認がエラーになる。

一方で、部品の一部はすでに arm64 を見込んでいる。

- `scripts/sidecar.mjs` は `windows-arm64` (llama.cpp `llama-b11126-bin-win-cpu-arm64.zip`) の sha256 を固定済み。
- `build/windows/installer/project.nsi` はサイドカーを `build/sidecar/windows-${ARCH}/` から取り、Wails の雛形どおり ARM64 のバイナリも受け取れる。

Windows 11 on Arm では今の amd64 版も x64 エミュレーションで動く。その場合アプリの GOARCH は amd64 なので、更新の確認は `windows-amd64` を取り続ける。

## 作業

- build.yml に windows/arm64 のビルドを足す。インストーラにはその arch のサイドカーと GGUF を入れる。今 Windows で走らせている確認 (インストーラができたこと、ライセンス文書を入れたことなど) を arm64 にも掛ける。
- release.yml で arm64 のインストーラを集めて署名し、latest.json に `windows-arm64` を足す。SHA256SUMS.txt にも載せる。
- `updatesig` に `windows-arm64` の定数を足し、`updater.Platform()` が windows/arm64 でそれを返すようにする。
- README.md / README.ja.md の配布物の説明と、「Rules for changing the release workflow」の latest.json のキーの記述を直す。

## 着手時に決めること

- **ビルドする runner**: GitHub の arm64 Windows runner (`windows-11-arm`) で native にビルドするか、`windows-latest` (x64) から `-platform windows/arm64` でクロスビルドするか。x64 の runner では arm64 の llama-server を実行できないので、クロスビルドにするとインストール後の確認や埋め込みの確認を arm64 で回せない。
- **インストーラの形**: arch ごとに別のインストーラにするか、NSIS の雛形が持つ両 arch 入りの 1 本にするか。更新はインストーラそのものを落とすので、1 本にすると更新のダウンロードが倍近くになる。
- **今 amd64 版をエミュレーションで使っている人の移行**: amd64 版の更新で arm64 版へ移すか、手で入れ直してもらうか。移すなら、amd64 のアプリが自分が Arm 機の上で動いていることを知る手段 (`IsWow64Process2` など) が要る。
- **実機での確認**: オーナーの手元に Windows on Arm の実機があるか。無ければ、AC #4 をどう確かめるかを決める。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 build.yml が windows/arm64 のインストーラを作り、そのインストーラに arm64 の llama-server と GGUF が入っている
- [ ] #2 release.yml が arm64 のインストーラを Release に添付し、latest.json に windows-arm64 の URL と署名が載り、公開鍵での検証を通る
- [ ] #3 windows/arm64 のアプリで updater.Platform() が windows-arm64 を返し、更新の確認が arm64 のインストーラを選ぶ (テストで確かめる)
- [ ] #4 arm64 のインストーラで入れたアプリが Windows on Arm で起動し、内蔵の埋め込みが動く
- [ ] #5 README.md / README.ja.md の配布物と latest.json のキーの記述が arm64 版を含む形に直っている
<!-- AC:END -->
