---
id: TASK-97
title: 'Windows: arm64 版を作り、リリースと自動更新で amd64 版と並べて配る'
status: In Review
assignee: []
created_date: '2026-10-10 21:14'
updated_date: '2026-10-12 01:50'
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
- [x] #3 windows/arm64 のアプリで updater.Platform() が windows-arm64 を返し、更新の確認が arm64 のインストーラを選ぶ (テストで確かめる)
- [ ] #4 arm64 のインストーラで入れたアプリが Windows on Arm で起動し、内蔵の埋め込みが動く
- [x] #5 README.md / README.ja.md の配布物と latest.json のキーの記述が arm64 版を含む形に直っている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
着手時の判定 (2026-10-12, オーナー確認済み):
- runner: windows-11-arm で native ビルド (arm64 の llama-server を CI で実際に動かすため)
- インストーラ: arch ごとに別。project.nsi の OutFile が既に ${ARCH} 付き
- エミュレーションで amd64 版を使っている人: 自動移行はせず、README で arm64 版の入れ直しを案内
- AC #4: 実機が無いので、windows-11-arm runner 上で今の Windows の確認 (サイレントインストール、/UPDATE 後の起動、ネットワーク無しでの埋め込み) を arm64 インストーラに掛けたものを証拠とする

1. updatesig に PlatformWindowsARM64 を足し Platforms に含める (verify が 3 キーを要求する)
2. updater: Platform() を platformFor(goos, goarch) に分け windows/arm64 → windows-arm64。updateFileName は Windows 両 arch で installer 名。テストで platformFor と releaseFor の arm64 選択を確かめる
3. build.yml: matrix に Windows (arm64) / windows-11-arm を足し、arch と artifact 名を matrix から取る (snz-studio-Windows-amd64 / -arm64)。sidecar とビルドの arch を matrix.arch に
4. release.yml: attach が両 arch のインストーラを集め、windows-arm64 を署名・latest.json・SHA256SUMS に載せる
5. README.md / README.ja.md、tools/updatesig のコメントを直す
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装 (2026-10-12)

- ビルドは windows-11-arm runner で native に行う。クロスビルドにすると arm64 の llama-server を CI で動かせず、AC #4 を確かめる手段が無くなる (オーナーの手元に Arm 機が無い)。runner-images の Windows11-Arm64-Readme で Chocolatey 2.7.4 / NSIS 3.10 / Node / Go が入っていることを確認し、NSIS の手順は amd64 と共通のまま。フロントエンドの依存は lockfile に @esbuild/win32-arm64 と @rollup/rollup-win32-arm64-msvc が載っている。
- インストーラは arch ごとに別。project.nsi の OutFile が既に ${ARCH} 付きなので、インストーラ側は手を入れていない。artifact 名は matrix から取り、snz-studio-Windows-amd64 / snz-studio-Windows-arm64 にした (両 Windows ジョブが runner.os で同名になり衝突するため)。
- Arm の runner は x64 のバイナリもエミュレーションで動かすので、スモークテストが通っても arm64 だとは言えない。インストール先の exe / dll の PE ヘッダの Machine (arm64 は 0xAA64、amd64 は 0x8664) を確かめる手順を足した (uninstall.exe は NSIS の x86 stub なので除外)。
- updater: Platform() を platformFor(goos, goarch) に分けて表で試験できるようにした。updateFileName は darwin 以外をインストーラ扱いに変えた (以前は amd64 だけがインストーラ名で、arm64 は .app.zip 名で保存されるところだった)。
- エミュレーションで amd64 版を使っている人は自動移行しない。README に、arm64 版のインストーラを上から実行すれば移れることを書いた。

## 確認したこと

- AC #3: go test ./... がすべて通った。TestPlatformForEachBuild が windows/arm64 → windows-arm64 を確かめる。TestReleaseForPicksEachWindowsArchInstaller は、3 キーの latest.json から各 arch のインストーラを選ぶこと、両 arch とも installer 名で保存することを確かめる。GOOS=windows GOARCH=arm64 で go vet、テストバイナリのビルド、アプリのビルドが通った。
- AC #5: README.md / README.ja.md の配布物、ビルド手順、リリース手順、latest.json のキーの記述を直した。
- release.yml の収集処理を、ダミーの dist (snz-studio-macOS / -Windows-amd64 / -Windows-arm64) で手元で実行した。4 ファイルが期待どおりの名前で out/ に揃った。一時鍵での sign では、latest.json に darwin-universal / windows-amd64 / windows-arm64 の 3 キーが載った。
- actionlint v1.7.7 の指摘は windows-11-arm を知らないラベルとするものだけ (このバージョンのラベル一覧が古いため)。

## 未確認 (CI / リリースの実行待ち)

- AC #1・#4: arm64 runner での build.yml の実行結果を待つ。インストーラの生成、ライセンス文書と sidecar の同梱、PE Machine の確認、/UPDATE 後の起動、ネットワーク無しでの埋め込みのスモークテストがここで回る。
- AC #2: 本物の UPDATE_SIGNING_KEY で attach ジョブが通るかは、リリースを実行するまで分からない (verify は updatesig.Platforms の 3 キーすべてを要求する)。
- 画面の見た目は、amd64 と同じく CI では確かめていない。runner はサーバー向けの Arm VM で、Snapdragon 機ではない。
<!-- SECTION:NOTES:END -->
