---
id: TASK-26
title: Windows インストーラ本体に LICENSE / THIRD_PARTY_NOTICES を同梱する
status: To Do
assignee: []
created_date: '2026-09-20 11:47'
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
- [ ] #1 build/windows/installer のテンプレートがコミットされ、LICENSE と THIRD_PARTY_NOTICES.md が NSIS payload に入っている
- [ ] #2 インストール後のディレクトリに両ファイルが存在することを、ビルドしたインストーラで確認できている
- [ ] #3 サイドカー (llama-server + DLL) を NSIS に入れる TODO の扱いが、同梱するか引き続き先送りするかで決着している
<!-- AC:END -->
