---
id: TASK-26
title: Windows インストーラ本体に LICENSE / THIRD_PARTY_NOTICES を同梱する
status: In Review
assignee: []
created_date: '2026-09-20 11:47'
updated_date: '2026-09-20 23:47'
labels: []
milestone: m-1
dependencies: []
type: chore
ordinal: 26000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
TASK-25 の PR #19 レビューで残った [P2]。

現状 (TASK-25 で入れた範囲):
- macOS は `scripts/build-mac-signed.sh` と CI の macOS ジョブで `Contents/Resources` へ
  署名前にステージ済み。`.app` 署名が封をし公証の対象にも入る
- Windows は exe の隣に置いて artifact に含めるところまで。Actions の artifact は zip 化されるので、
  artifact 経由で受け取る限り表示は届く

残る穴:
NSIS インストーラ単体を配ると、payload にもインストール後のディレクトリにも表示が入らない。
TinySegmenter の修正 BSD は binary 形式の再頒布に表示の付随を求めるので、インストーラを
単独配布する経路を開くならその前に塞ぐ必要がある。

なぜ TASK-25 で見送ったか:
- `build/windows/installer/project.nsi` はこのリポジトリに未コミットで、`wails build -nsis` が
  生成する側。テンプレートを起こす作業になる
- 手元に Windows 環境が無く、書いても検証できない。未検証のインストーラテンプレートを
  リリース経路に入れるのは、現時点で塞ごうとしている穴より risky と判断した
- Windows ビルドは現在未署名で、実配布の経路が無い

やること:
1. NSIS テンプレート (`build/windows/installer/project.nsi` + `wails_tools.nsh`) を生成してコミットする
2. `File` ディレクティブで `LICENSE` と `THIRD_PARTY_NOTICES.md` を payload に入れ、
   アプリと同じ場所へインストールする
3. ビルドしたインストーラに両ファイルが入っていることを確認する

同じテンプレート改修は `.github/workflows/build.yml` の「サイドカーを NSIS に入れる」TODO でも
必要になるので、まとめて片付けるのが合理的。着手は CI を実走できる状態 (TASK-8) が前提。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 build/windows/installer のテンプレートがコミットされ、LICENSE と THIRD_PARTY_NOTICES.md が NSIS payload に入っている
- [x] #2 インストール後のディレクトリに両ファイルが存在することを、ビルドしたインストーラで確認できている
- [x] #3 サイドカー (llama-server + DLL) を NSIS に入れる TODO の扱いが、同梱するか引き続き先送りするかで決着している
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. build/windows/installer/project.nsi を Wails v2.16.0 の埋め込みテンプレートから起こしてコミットし、
   インストール Section (SetOutPath $INSTDIR の後) に LICENSE / THIRD_PARTY_NOTICES.md の File を追加する
2. wails_tools.nsh はコミットしない — ReadOriginalFileWithProjectDataAndSave で毎ビルド無条件に
   再生成される生成物。build/windows/ 配下 (icon.ico / info.json / wails.exe.manifest / installer/tmp) も同様なので
   .gitignore で除外し、project.nsi だけ追跡する
3. .github/workflows/build.yml に Windows ジョブの検証ステップを追加: 生成された installer.exe を
   /S /D=<RUNNER_TEMP> でサイレント実行し、インストール先に両ファイルが存在することを assert する
4. サイドカー (llama-server.exe + DLL) の NSIS 同梱は先送りで決着 (AC #3)。理由を project.nsi と
   build.yml のコメントに残す
5. ブランチを push し、gh workflow run で build ワークフローを実走させて 3 の assert が通ることを AC #2 の証跡にする
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装 (TASK-26)

### コミットしたのは project.nsi のみ (タスク記述からの変更点)
タスクは `project.nsi` と `wails_tools.nsh` の両方をコミットと書いていたが、Wails v2.16.0 の
実装を読むと両者の扱いが違う:

- `project.nsi` は `buildassets.ReadFile` — ディスクにあればそれを使い、無いときだけ埋め込み
  テンプレートを書き出す。つまり編集してコミットする意味がある
- `wails_tools.nsh` は `ReadOriginalFileWithProjectDataAndSave` — 埋め込みテンプレートを毎回
  読み直して解決し、ディスクへ**無条件に上書き**する。コミットしても毎ビルド再生成され、
  作業ツリーが汚れるだけ

`build/windows/` 配下は icon.ico (appicon.png から生成)・info.json・wails.exe.manifest・
installer/tmp の WebView2 bootstrapper もすべて生成物なので、.gitignore で `/build/windows/*` を
落とし `project.nsi` だけ `!` で戻す形にした。

### AC #1 の証跡
`File "..\..\..\LICENSE"` / `File "..\..\..\THIRD_PARTY_NOTICES.md"` を install Section の
`SetOutPath $INSTDIR` 後に追加。手元 (macOS, makensis 3.12) で wails と同じ配置を scratch に
再現して実コンパイルし、警告なしで通ることを確認した:

    Processed 1 file, writing output (x86-unicode)
    Output: "../../bin/snz-studio-amd64-installer.exe"

さらに negative control として THIRD_PARTY_NOTICES.md を退避して再実行し、

    File: "..\..\..\THIRD_PARTY_NOTICES.md" -> no files found.
    Error in script "project.nsi" on line 104 -- aborting creation process

で hard fail することを確認。相対パスがリポジトリルートの実ファイルに解決していること、
取りこぼしたら黙って抜けるのではなくビルドが落ちることの両方が取れている。
`-clean` は `cleanBinDirectory` で build/bin しか消さないので project.nsi は生き残る。

### AC #2 の検証手段
手元に Windows 機が無いので、CI 側に恒久的な検証を置いた (build.yml
"Verify the installer ships the license notices"): 生成された installer.exe を
`/S /D=$env:RUNNER_TEMP\installed-check` でサイレント実行し、インストール先に両ファイルが
あることを assert する。`/D=` は最後・クォート無しが NSIS の要件なので Start-Process の
ArgumentList を分けている。ワークフロー実走の結果が出るまで AC #2 は未チェックのまま。

### AC #3: サイドカーは引き続き先送りで決着
理由 (project.nsi と build.yml のコメントに記録):

- makensis は `wails build` の内側で走るので、CI がサイドカーを curl するステップより先に
  終わっている。同梱するには CI のステップ順の再構成が要る
- サイドカーは素のチェックアウトに存在しないため、無条件の `File` はローカルの
  `wails build -nsis` を壊す。LICENSE / NOTICES は常にリポジトリにあるので無条件で安全、という
  非対称がある
- Windows ビルドは未署名で実配布経路が無く、塞ぐ価値が現時点で低い

ゆるい fallback として、loose なサイドカー exe + DLL 自体もバイナリ再頒布なので、exe の隣への
LICENSE / NOTICES 配置 (既存ステップ) は残してある。

### 検証
`go vet ./...` / `go test ./...` / `pnpm run check:client` いずれも通過 (Go/TS は無変更)。
build.yml は PyYAML でパースし、ステップ順を目視確認。actionlint / shellcheck は未インストールのため未実行。

### AC #2 の証跡 (CI 実走)
build ワークフローを task-26-nsis-license-notices ブランチで dispatch
(run 35545446747)。macOS / Windows(amd64) とも success。
"Verify the installer ships the license notices (Windows)" のログで、サイレント
インストール先 `D:\a\_temp\installed-check` の中身を確認:

    LICENSE                   1090
    snz-studio.exe        17955840
    THIRD_PARTY_NOTICES.md   16860
    uninstall.exe            82226

サイズがリポジトリ上 (1069 / 16542) と違うのは Windows チェックアウトの CRLF 変換による
もので、内容の差ではない。インストーラ単体を配っても表示がアプリと同じディレクトリに届く。
<!-- SECTION:NOTES:END -->
