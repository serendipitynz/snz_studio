---
id: TASK-8
title: 'CI: build ワークフローを実走させて通ることを確認する'
status: In Review
assignee: []
created_date: '2026-09-18 23:32'
updated_date: '2026-09-20 19:51'
labels: []
milestone: m-0
dependencies:
  - TASK-7
references:
  - 'https://github.com/serendipitynz/snz_studio/pull/7'
ordinal: 8000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`.github/workflows/build.yml` は private repo の課金分 (macOS ランナーは 10x) を避けるため
`workflow_dispatch` 限定で、これまで一度も実走していない。TASK-7 でビルドステップ自体を
書き換えた (`wails build ...` → `node scripts/wails.mjs build ...`) ので、現状「CI が通る」
ことの根拠はローカルでの同等手順の再現だけで、実走の証拠が無い。TASK-7 の AC#2 は
この確認をここへ委ねる形に改めてクローズした。

実走で初めて分かる点が 2 つある:
- **Windows ジョブがランチャを通ること**。`scripts/wails.mjs` の Windows 動作は手元に
  Windows が無いため未検証で、根拠は「Node の spawn に env を渡す方式はシェル非依存」と
  いう設計上のものだけ。windows-latest ジョブがこれを実際に踏む唯一の場所。
- **setup-go が入れる floor 版 (go 1.26.3) から `toolchain` 指定の go1.27.1 への
  自動ダウンロードがランナー上で成立すること**。

実行するときは `gh workflow run build.yml` → Actions タブで両ジョブの結果を確認する。
失敗したら、ここで直すのか TASK-7 の follow-up として別に切るのかを判断する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 build ワークフローを workflow_dispatch で実走させ、macOS / Windows 両ジョブが成功する
- [x] #2 失敗した場合は原因と対処 (この場で修正 / 別タスク化) が記録されている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. main の build.yml を workflow_dispatch で実走 (gh workflow run build.yml --ref main)
2. macOS / Windows 両ジョブの結果を Actions で確認し、特に (a) Windows での scripts/wails.mjs ランチャ経由のビルド、(b) setup-go floor (go.mod の go ディレクティブ) から toolchain go1.27.1 への自動ダウンロードが成立するかを見る
3. 成功: ログから上記 2 点の証拠を抜き出し、run URL とともに Implementation Notes に記録して AC#1/#2 をクローズ
4. 失敗: 失敗ログを読み、原因が CI 設定/ランチャの軽微な修正で閉じるならこの場で直して再実走。プロダクトコード側の設計変更を要するなら別タスクを切り、その判断をユーザーに提示してから記録
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実走結果

build ワークフローを 3 回 workflow_dispatch し、最終的に macOS / Windows 両ジョブとも成功
(run 35533411413, ref `task-8-verify-ci-build`)。

### 事前に「実走でしか分からない」としていた 2 点 — どちらも 1 回目で実証済み

- **Windows での `scripts/wails.mjs` ランチャ**: 通る。pwsh 上で `node scripts/wails.mjs build ...`
  から Wails CLI が起動し、GOTOOLCHAIN の env 渡しも効いた。TASK-7 の設計上の根拠
  (「Node の spawn に env を渡す方式はシェル非依存」) が実機で裏付けられた。
- **floor 版から `toolchain go1.27.1` への自動ダウンロード**: 成立する。setup-go は
  go 1.26.3 を配置するが、両ランナーとも Set up Go の時点で `go: downloading go1.27.1` が
  走り、`GOVERSION='go1.27.1'` / GOROOT が `toolchain@v0.0.1-go1.27.1.*` を指した。
  x/tools のバージョン不整合があれば必ず落ちる `Generating bindings` も Done。

### 実走で初めて出た問題 2 件 — いずれも本タスクで修正

1. **frontend の依存が CI で一度も install されていなかった** (1 回目、両ジョブ赤)。
   `vite: command not found` / `node_modules missing`。原因は Wails v2 の
   `NpmInstallUsingCommand` が `frontend:dir` 直下に package.json が無いと即 return する
   こと (v2.16.0 `pkg/commands/build/base.go:454-460`)。本リポジトリは root 単一
   package.json なので `frontend:install` (`pnpm -C .. install`) が一度も実行されず、
   それでも "Installing frontend dependencies: Done." と表示される (ログ上 0ms)。
   ローカルでは開発者の root node_modules が既にあるため不可視だった。
   → ワークフローに `pnpm install --frozen-lockfile` を明示ステップとして追加。
   wails が install も担うと書いていたコメントも訂正した。

2. **Windows インストーラが生成されていなかったのにジョブは緑だった** (2 回目)。
   `wails build -nsis` は makensis 不在を警告に落として exit 0 を返すため
   (`Warning: Cannot create installer: makensis not found`)、成果物 4 ファイルに
   installer が無いまま成功扱いになっていた。原因は choco が NSIS を
   `C:\Program Files (x86)\NSIS` に入れてマシン PATH を書き換えても、起動済みの
   runner プロセスには伝播しないこと。
   → `$env:GITHUB_PATH` に NSIS のディレクトリを追加。あわせて
   `ls build/bin/*-installer.exe` のガードステップを置いた。警告しか出ない以上、
   成果物の実在を確かめる以外に「作られた」と「黙ってスキップされた」を区別する
   手段が無いため。3 回目で `snz-studio-amd64-installer.exe` の生成を確認、
   Windows 成果物は 4 → 5 ファイル (7.4MB → 16.4MB) になった。

どちらも CI 設定側の欠陥で、TASK-7 のランチャ差し替えとは独立 (このワークフローが
一度も走っていなかったこと自体が隠していた) ため、別タスクに切らずこの場で直した。

### 最終 run の証拠 (35533411413)

- macOS: `Built '.../snz-studio.app/Contents/MacOS/SNZ Studio'` → `created: .../SNZ-Studio.dmg`、
  成果物 `snz-studio-macOS` 24,988,015 bytes
- Windows: `Building 'amd64' installer: Done.` → ガードが
  `build/bin/snz-studio-amd64-installer.exe` を確認、成果物 `snz-studio-Windows`
  16,446,892 bytes / 5 ファイル

ローカル検証: `go vet ./...` / `go test ./...` / `pnpm run check:client` いずれもクリーン
(プロダクトコードは無変更なので回帰確認の位置づけ)。

### 未検証のまま残すもの

生成された .dmg / installer を実際にインストールして起動する確認はしていない
(手元に Windows が無く、macOS 側も未署名 artifact のため)。署名・notarization は
ワークフロー内 TODO のまま。インストーラへの LICENSE / THIRD_PARTY_NOTICES 同梱は
TASK-26 の範囲。
<!-- SECTION:NOTES:END -->
