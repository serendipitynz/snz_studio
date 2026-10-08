---
id: TASK-92.2
title: '更新: Go で新しい版の確認・ダウンロード・署名検証・入れ替え・再起動を行い、UI にバインドする'
status: To Do
assignee: []
created_date: '2026-10-07 22:53'
updated_date: '2026-10-07 22:57'
labels: []
dependencies:
  - TASK-92.1
references:
  - app.go
  - main.go
  - scripts/wails.mjs
  - internal/embed/manager.go
  - build/windows/installer/project.nsi
parent_task_id: TASK-92
priority: medium
type: feature
ordinal: 94000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
TASK-92 の Go 側。TASK-92.1 が添付する latest.json と更新用ファイルを使って、確認・ダウンロード・署名検証・入れ替え・再起動を行い、Wails のバインドで UI (TASK-92.3) に渡す。

## 作業

- **実行中の版を Go から読めるようにする。** 今の版は `wails.json` の `info.productVersion` にしかなく、macOS の Info.plist と Windows のリソースに書き込まれるだけで、Go からは読めない。`scripts/wails.mjs` で `-ldflags -X` を渡すなど、ビルドのたびに wails.json から入る形にする (release.yml はタグと wails.json の一致を既に確かめている)。
- **確認**: 公開済み・非 prerelease のリリースから最大の版タグを選び、実行中の版より新しければ、そのタグの latest.json を読む。モデルのタグなど版タグでないものは無視する。api.github.com の未認証の上限は 60 回/時で、起動ごとに 1 回なら足りる。タイムアウトを付ける。
- **ダウンロード**: 一時ディレクトリへ保存し、進捗を Wails のイベントで UI に送る。
- **署名検証**: バイナリに埋め込んだ公開鍵で検証し、通らなければ何も置き換えない。latest.json の版が版タグと一致することも確かめる。
- **macOS の入れ替え**: `os.Executable()` から実行中の `.app` を求め、同じボリューム上に展開する。内蔵 embedding の llama-server (`internal/embed`) を止めてから `.app` を差し替え、`open -n` で新しい版を起動して自分は終了する。
  - `.app` の親ディレクトリに書き込めないとき (管理者でないアカウントなど) は、管理者権限に昇格しない。mallow の AppleScript による昇格のような経路は持たず、Releases ページを開く形に落とす。
  - `.dmg` から直接起動している、または App Translocation で読み取り専用の場所から動いているときは、入れ替えずに理由を返す。
- **Windows を per-user インストールにする** (TASK-92 で決定)。`build/windows/installer/project.nsi` で `WAILS_INSTALL_SCOPE` と `REQUEST_EXECUTION_LEVEL` をどちらも `user` に define する。インストール先が `%LOCALAPPDATA%\Programs`、アンインストール情報が HKCU、スタートメニューのショートカットが現在のユーザーのものになる (`wails_tools.nsh` の `wails.setShellContext` は `REQUEST_EXECUTION_LEVEL` を見て切り替える)。片方だけ変えると、昇格しないまま Program Files や HKLM に書こうとして失敗するので、両方をそろえる。
- **Windows の更新**: 署名を検証したインストーラを `/S` で起動し、アプリは終了する。per-user なので昇格 (ShellExecute の `runas`) は要らない。Wails の NSIS テンプレートは silent インストールの後にアプリを起動しないので、再起動させるかどうかを決める。させるなら `project.nsi` に手を入れる。
- **失敗の扱い**: 自動確認の失敗 (オフライン・上限超過・リリースなし) は値として返し、UI は何も出さない。承認後の失敗 (ダウンロード・署名検証・書き込みのどれで失敗しても) は「更新は行われなかった」として返す。エラーメッセージの文面で分岐しない。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 実行中の版が wails.json の productVersion からビルド時に入り、Go から読める
- [ ] #2 最大の版タグを選ぶ処理が、モデルのタグ・draft・prerelease を無視し、実行中の版以下なら「更新なし」を返す
- [ ] #3 署名が合わない、または latest.json の版がタグと食い違う更新用ファイルは、何も置き換えずに失敗として返す
- [ ] #4 macOS で、インストール済みの .app が新しい版に入れ替わり、llama-server を残さずに新しい版が起動する
- [ ] #5 macOS で .app の親に書き込めない、または .dmg や App Translocation から動いているときは、昇格せずに理由を返す
- [ ] #6 版の比較・版タグの選別・latest.json の読み込み・署名検証に Go のテストがあり、go test ./... が通る
- [ ] #7 Windows のインストーラが per-user (%LOCALAPPDATA%\Programs、HKCU) でインストールし、インストールにも更新にも UAC が出ない
- [ ] #8 Windows で、署名を検証したインストーラが起動してアプリが終了し、更新後の再起動をどうするかが決まって実装されている
<!-- AC:END -->
