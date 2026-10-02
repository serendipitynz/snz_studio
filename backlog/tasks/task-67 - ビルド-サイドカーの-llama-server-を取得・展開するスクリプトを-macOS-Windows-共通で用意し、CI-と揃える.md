---
id: TASK-67
title: 'ビルド: サイドカーの llama-server を取得・展開するスクリプトを macOS / Windows 共通で用意し、CI と揃える'
status: Done
assignee: []
created_date: '2026-09-28 19:47'
updated_date: '2026-10-02 01:11'
labels: []
dependencies: []
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
- [x] #1 macOS と Windows (PowerShell、WSL なし) の両方で、同じ pnpm スクリプト 1 本で、実行中の OS と CPU に合うサイドカーを取得して展開できる。固定した版が展開済みなら何もしない
- [x] #2 開発用の場所 (build/sidecar/<GOOS>-<GOARCH>/) に展開した後、SNZ_LLAMA_SERVER_BIN を指定しなくても、pnpm dev で内蔵 embedding が ready になる (macOS / Windows)
- [x] #3 配布用の場所 (macOS は .app の Contents/Resources、Windows は exe の横) に展開できる。pnpm build:app で作ったアプリで内蔵 embedding が ready になる (macOS / Windows)
- [x] #4 ダウンロードしたアーカイブの sha256 を、固定した値と照合してから展開する
- [x] #5 llama.cpp の版の固定が 1 か所にまとまっていて、build.yml と build-mac-signed.sh はこのスクリプト経由で配置する (curl で直接取得する処理が残っていない)
- [x] #6 README.md / README.ja.md のビルド手順に、このスクリプトの使い方が書かれている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. scripts/sidecar.mjs を Node で書き、package.json に pnpm sidecar として登録する。llama.cpp の版と、OS・CPU ごとのアーカイブ名と sha256 はこのスクリプトの中の 1 か所に置く (darwin-arm64 / darwin-amd64 / windows-amd64 / windows-arm64)
2. 既定の展開先は開発用の build/sidecar/<GOOS>-<GOARCH>/。--app で配布用 (macOS は build/bin/*.app/Contents/Resources、Windows は build/bin/)、--arch で CPU を指定できる
3. ダウンロードは build/sidecar/.downloads/ に置き、sha256 を照合してから OS の tar で展開する (Windows は Git の GNU tar を避けて System32\tar.exe を使う)。照合済みのアーカイブがあれば取得し直さない
4. 展開先には llama-server と共有ライブラリ (*.dylib / *.dll) だけを置く (macOS の公証で、署名していない実行ファイルを残さないため)。LICENSE はアプリの LICENSE を上書きしないよう置かない。展開先に版の記録を残し、同じ版が展開済みなら何もしない
5. build.yml (macOS / Windows) と build-mac-signed.sh の curl による取得をこのスクリプトの呼び出しに置き換える。build.yml の成果物に build/bin/*.dll を加える (今は DLL が抜けていて、ポータブル版の llama-server.exe が動かない)
6. README.md / README.ja.md のビルド手順に使い方を書く
7. 確認: macOS で取得・冪等性・sha256 不一致での中断・pnpm dev と pnpm build:app のアプリで ready。Windows は CI で取得と配置、CI の成果物を実機で起動して ready、実機で pnpm dev を 1 回
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 依存の変更
- 依存から TASK-66 を外した (2026-10-02、ユーザー了承)。スクリプトは版に依存せず、b9437 と b11126 でアセット名の形式も同じ。先に入れれば、TASK-66 の版上げは scripts/sidecar.mjs の RELEASE と sha256 の書き換えだけになり、Windows での b11126 の確認もこのスクリプトで置ける

## 方針
- scripts/sidecar.mjs (Node、npm 依存なし) を pnpm sidecar として登録。版と OS・CPU ごとのアーカイブ名・sha256 はこのファイルの ASSETS だけに置く。darwin-arm64 / darwin-amd64 / windows-amd64 / windows-arm64 を固定 (sha256 は GitHub の asset digest と、自分でダウンロードして計算した値が一致)
- 展開は OS の bsdtar。Windows は %SystemRoot%\System32\tar.exe を明示する。Git for Windows の GNU tar が PATH にあると zip を読めず、C: をリモートホストと解釈するため
- 置くのは llama-server(.exe) と *.dylib / *.dll だけ (許可リスト)。従来の macOS 処理は「Mach-O 実行ファイルを消す」で LICENSE も残していたが、build/bin に置くとアプリの LICENSE を上書きするので置かない (llama.cpp の LICENSE は THIRD_PARTY_NOTICES.md にある)。dylib の相対 symlink はそのまま作り直す
- 展開先に .llama-server-release (版・対象・sha256) を書き、一致すれば何もしない。アーカイブは build/sidecar/.downloads/ に残し、照合が通れば取得し直さない
- 開発用の場所は前の版の dylib を残さないよう消してから置く。配布用の場所はアプリのファイルがあるので消さない (wails build -clean が作り直す)

## 確認 (macOS arm64)
- pnpm sidecar: 取得・展開 (36 ファイル、実行ファイルは llama-server のみ)。2 回目は「already staged」で何もしない
- sha256 を 1 文字変えたコピーで実行: mismatch で exit 1、アーカイブは捨てられ、展開先は変わらない。キャッシュを壊した場合は取得し直す
- AC#2 (macOS): build/bin を退避して env -u SNZ_LLAMA_SERVER_BIN pnpm dev → サイドカーは build/sidecar/darwin-arm64/llama-server から起動し、/api/embedding/status が state=ready, dim=256
- AC#3 (macOS): pnpm build:app → pnpm sidecar --app (2 回目は何もしない) → DATA_DIR を一時ディレクトリにしてアプリを起動。サイドカーは .app の Contents/Resources/llama-server から起動し、/health が ok、/v1/embeddings が 256 次元 (manager が ready にする条件と同じ。配布版はトークンを外から取れないので status API ではなくこの 2 つで確認)
- pnpm check:client / test:client / go test ./... は通過。build-mac-signed.sh は bash -n のみ (署名と公証は未実行)

## 見つけたこと
- build.yml の成果物に build/bin/*.dll が入っておらず、ポータブル版の llama-server.exe が DLL なしで上がっていた。*.dll を加えた
- macOS の wails dev は build/bin/snz-studio.app から起動するので、その .app にサイドカーがあると build/sidecar より先に使われる (resolveServerBinary の順序)。README に注意として書いた。コードは変えていない
- README の「SNZ Studio.app」は、実際の成果物 build/bin/snz-studio.app と名前が違う (既存の記述。このタスクでは直していない)

## 未確認 (Windows)
- AC#1 / #2 / #3 の Windows 分。CI (workflow_dispatch) の Windows ジョブで pnpm sidecar --app (PowerShell) を確かめ、その成果物を実機で起動して ready を確認、実機で pnpm sidecar と pnpm dev を 1 回

## 確認 (Windows、2026-10-02)
- CI (workflow_dispatch、run 36936836537): Windows ジョブの pnpm sidecar --app は pwsh で実行され、windows-amd64 のアーカイブを取得・照合して build\bin に 30 ファイル (llama-server.exe と DLL 29 個) を置いた。成果物 snz-studio-Windows に DLL 29 個が入っていることも確認。macOS ジョブも同じスクリプトで 36 ファイルを置いて成功
- 実機 (ユーザー確認): pnpm sidecar を 2 回実行し、2 回目は何もしない。pnpm dev で内蔵 embedding が ready (AC#1 / #2)
- 実機 (ユーザー確認): CI 成果物を展開したフォルダーの SNZ Studio.exe を起動し、設定画面に「同梱の埋め込みモデルの準備が整いました」と表示された (AC#3)。インストーラーにはサイドカーが入っていない (TASK-26) ので、展開したフォルダーで確認した
- 配布版の起動時に、llama-server.exe 用の空のコンソールウィンドウが開いていた。コンソールを持たない GUI アプリからコンソールプログラムを起動したため。sidecar_windows.go で llama-server と taskkill に CREATE_NO_WINDOW を付けた (03ca226)。pnpm dev では開発用ターミナルのコンソールを引き継ぐので出ない
- ダッシュボードの接続カードは内蔵 embedding の状態を表示時に 1 回だけ読むので、起動直後に開くと ready になっても「準備中」のまま変わらない。設定画面は 2 秒ごとに読み直すので ready と出る。このタスクの範囲外 (画面側の不具合)
<!-- SECTION:NOTES:END -->
