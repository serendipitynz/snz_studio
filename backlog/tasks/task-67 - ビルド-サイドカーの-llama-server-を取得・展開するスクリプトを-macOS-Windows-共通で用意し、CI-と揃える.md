---
id: TASK-67
title: 'ビルド: サイドカーの llama-server を取得・展開するスクリプトを macOS / Windows 共通で用意し、CI と揃える'
status: To Do
assignee: []
created_date: '2026-09-28 19:47'
labels: []
dependencies:
  - TASK-66
references:
  - .github/workflows/build.yml
  - scripts/build-mac-signed.sh
  - scripts/wails.mjs
  - internal/embed/sidecar_windows.go
  - internal/embed/sidecar_unix.go
  - internal/embed/manager.go
type: chore
ordinal: 67000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
サイドカー (`llama-server` と dylib / DLL) の取得と展開は、今は CI (`.github/workflows/build.yml`) と
`scripts/build-mac-signed.sh` にそれぞれ直接書かれている。ローカルでは macOS の署名ビルド以外に
手段が無い。Windows では zip を手で展開して exe の横に置くしかなく、`pnpm dev` 用の
`build/sidecar/<os>-<arch>/` も手で用意している。

macOS と Windows のどちらでも動くスクリプトを 1 本用意し、CI と `build-mac-signed.sh` もそれを呼ぶようにして、
3 か所で手順がずれないようにする。

前提と候補 (着手時に確かめる):
- WSL なしで Windows (PowerShell) から動かすため、Node で書く (`scripts/wails.mjs` と同じ理由。
  Node はビルドに必須のツールで、POSIX シェルは Windows で使えない)
- 展開には npm 依存を増やさず OS の `tar` を使う案がある。macOS の bsdtar も Windows 10 以降の
  `tar.exe` も zip を展開できる。新しい依存が必要になる場合は、先にユーザーに確認する
- 展開先:
  - 開発用: `build/sidecar/<GOOS>-<GOARCH>/` (`devServerBinaryPath`、`wails dev` が cwd 相対で探す)
  - macOS の配布用: `<app>.app/Contents/Resources/` (`defaultServerBinaryPath`)
  - Windows の配布用: exe の横 (`build/bin/`)
- macOS では、実行ファイルは `llama-server` 以外を取り除く (署名していない実行ファイルがあると
  公証に通らない。CI と build-mac-signed.sh で既にやっている処理)
- llama.cpp の版の固定は今 3 か所 (build.yml の 2 か所と build-mac-signed.sh) に散っている。
  スクリプト、CI、build-mac-signed.sh が同じ 1 か所を読むようにする

GGUF の同梱は TASK-68 で扱う。このスクリプトの延長で置けるようにしておく。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 macOS と Windows (PowerShell、WSL なし) の両方で、同じ pnpm スクリプト 1 本で、実行中の OS と CPU に合うサイドカーを取得して展開できる。固定した版が展開済みなら何もしない
- [ ] #2 開発用の場所 (build/sidecar/<GOOS>-<GOARCH>/) に展開した後、SNZ_LLAMA_SERVER_BIN を指定しなくても、pnpm dev で内蔵 embedding が ready になる (macOS / Windows)
- [ ] #3 配布用の場所 (macOS は .app の Contents/Resources、Windows は exe の横) に展開できる。pnpm build:app で作ったアプリで内蔵 embedding が ready になる (macOS / Windows)
- [ ] #4 ダウンロードしたアーカイブの sha256 を、固定した値と照合してから展開する
- [ ] #5 llama.cpp の版の固定が 1 か所にまとまっていて、build.yml と build-mac-signed.sh はこのスクリプト経由で配置する (curl で直接取得する処理が残っていない)
- [ ] #6 README.md / README.ja.md のビルド手順に、このスクリプトの使い方が書かれている
<!-- AC:END -->
