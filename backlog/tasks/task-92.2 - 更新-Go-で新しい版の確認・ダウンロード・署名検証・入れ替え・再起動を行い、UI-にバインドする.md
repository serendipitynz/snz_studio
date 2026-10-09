---
id: TASK-92.2
title: '更新: Go で新しい版の確認・ダウンロード・署名検証・入れ替え・再起動を行い、UI にバインドする'
status: Done
assignee: []
created_date: '2026-10-07 22:53'
updated_date: '2026-10-09 11:47'
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
- [x] #1 実行中の版が wails.json の productVersion からビルド時に入り、Go から読める
- [x] #2 最大の版タグを選ぶ処理が、モデルのタグ・draft・prerelease を無視し、実行中の版以下なら「更新なし」を返す
- [x] #3 署名が合わない、または latest.json の版がタグと食い違う更新用ファイルは、何も置き換えずに失敗として返す
- [x] #4 macOS で、インストール済みの .app が新しい版に入れ替わり、llama-server を残さずに新しい版が起動する
- [x] #5 macOS で .app の親に書き込めない、または .dmg や App Translocation から動いているときは、昇格せずに理由を返す
- [x] #6 版の比較・版タグの選別・latest.json の読み込み・署名検証に Go のテストがあり、go test ./... が通る
- [x] #7 Windows のインストーラが per-user (%LOCALAPPDATA%\Programs、HKCU) でインストールし、インストールにも更新にも UAC が出ない
- [x] #8 Windows で、署名を検証したインストーラが起動してアプリが終了し、更新後の再起動をどうするかが決まって実装されている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 版の埋め込み: scripts/wails.mjs が build / dev のとき wails.json の info.productVersion を -ldflags "-X main.appVersion=…" で渡す。GetVersion バインドで UI に返す
2. internal/updater (OS 非依存): 版の比較 (vMAJOR.MINOR.PATCH のみ)、releases 一覧から公開済み・非 prerelease の最大の版タグを選ぶ、タグ固定 URL の latest.json の読み込みと版の一致確認、ダウンロード (進捗コールバック・SHA-256 を同時に計算) と updatesig.Verify。失敗は Check の結果値として返す。配布元は既定で GitHub、SNZ_UPDATE_BASE_URL で差し替え可 (署名は常に埋め込み鍵で検証)
3. macOS の適用: os.Executable から .app を求め、App Translocation・読み取り専用ボリューム・親か .app に書き込めない場合は理由コードを返す。.app の親に作った一時ディレクトリへ ditto -x -k で展開 → llama-server 停止 → 旧 .app を退避して新 .app を置く (失敗時は戻す) → 終了処理の最後に open -n で新しい版を起動
4. Windows の適用: インストール先に書き込めるか確かめ、署名を検証したインストーラを終了処理の最後に /S /UPDATE /D=<インストール先> で起動。project.nsi は per-user (WAILS_INSTALL_SCOPE / REQUEST_EXECUTION_LEVEL = user) にし、/UPDATE のときだけ exe のロックが外れるまで待ち、成功後に新しい版を起動する (2026-10-09 ユーザーと決定)
5. app.go に GetVersion / CheckForUpdate / InstallUpdate をバインド。進捗は Wails イベント update:progress。承認後の失敗は status=failed と理由コードで返し、文面で分岐させない
6. build.yml の Windows ジョブで per-user のインストール先と HKCU の登録を検査。go test ./... と GOOS=windows の go vet。macOS は本物の鍵で署名したテスト用 zip をローカル配布元から配って e2e を確認 (2026-10-09 ユーザー了承)
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- 版の埋め込み: scripts/wails.mjs が build / dev のとき `-ldflags "-X main.appVersion=<info.productVersion>"` を渡す (利用者が -ldflags を渡したときは先頭に足す)。launcher を通さない `go build` の版は空で、その場合は確認を常に失敗として返す (版を推測しない)。
- 新しいパッケージ `internal/updater`: 確認・ダウンロード・署名検証は OS に依存しない updater.go、入れ替えは install_darwin.go / install_windows.go (Linux はビルドだけ通す install_other.go)。署名の形式と公開鍵は TASK-92.1 の `internal/updatesig` をそのまま使い、本番依存は増やしていない。
- バインド (app_update.go): `GetVersion` / `CheckForUpdate` / `InstallUpdate(version)`。結果は status (`upToDate` / `available` / `failed` / `restarting` / `manual`) と reason (`translocated` / `readOnlyVolume` / `notWritable` / `notInstalled` / `unsupportedPlatform` / `devBuild`) で返し、UI が文面で分岐しないようにした。承認後の失敗は理由を返さず `failed` だけにした (利用者が次にとる行動はどこで失敗しても同じなので、理由はログに残す)。進捗はイベント `update:progress` ({downloaded, total}、total は不明なら 0)。InstallUpdate は直前の確認で見つかった版と一致するときだけ動き、同時に 2 回は走らない。
- 新しい版の起動は終了処理 (shutdown) の最後に行う。DB を閉じ、サイドカーを止めたあとに起動するため。macOS は `open -n`、Windows はインストーラを `/S /UPDATE /D=<インストール先>` で起動する。/D= は NSIS がクォートなしで行末まで読むので、exec のクォートを避けて CmdLine を手で組んでいる。
- macOS の入れ替え: .app の親に一時ディレクトリを作って ditto で展開し、rename 2 回で入れ替える (2 回目が失敗したら元に戻す)。利用者が変えた .app 名はそのまま保つ。llama-server は入れ替えの前に止め、入れ替えが失敗したら internal モードのときだけ起動し直す。App Translocation はパス (`/AppTranslocation/`) で判定する (cgo なしで使える公開 API がないため)。読み取り専用ボリュームは statfs の MNT_RDONLY、書き込み権限は親と .app の両方を access(W_OK) で確かめる。
- Windows の再起動 (2026-10-09 ユーザーと決定): 再起動する。project.nsi は `/UPDATE` が付いたときだけ、実行中の exe が書き込み可能になるまで最大 30 秒待ってから書き込み、成功したら新しい版を起動する。/UPDATE のない silent インストールは今までどおり起動しない。インストーラを使っていない portable の配置 (uninstall.exe がない) は `notInstalled` として入れ替えない。
- 確かめたこと:
  - AC #1 / #4 (macOS の e2e、2026-10-09): 0.1.1 の .app を ditto で固め、本物の更新署名鍵で署名し (ユーザー了承済み。鍵は表示していない)、`tools/updatesig verify` を通した。ローカルの偽の配布元 (`SNZ_UPDATE_BASE_URL`。リリース一覧にはモデルのタグも混ぜた) から配り、スクラッチの場所に置いた 0.1.0 の .app に、確認とインストールを自動で呼ぶ一時ファイル (コミットしていない) を入れて起動した。ログは `app: version "0.1.0"` → check `available 0.1.1` → install `restarting`。旧版の本体 (PID 45822) と llama-server (45828) はどちらも終了し、同じパスに 0.1.1 の .app (署名した zip の中身とバイナリが一致し、Info.plist も 0.1.1) が入り、新しい本体と新しい llama-server がそこから起動した。退避用のディレクトリは残っていない。データは DATA_DIR でスクラッチに向け、`launchctl setenv` は終了後に戻した。
  - AC #2 / #3 / #6: `go test ./...` が通過 (`internal/updater` のテスト: 版の解析と比較、モデル・draft・prerelease・不正なタグの除外、実行中の版以下なら更新なし、latest.json の版とタグの食い違い、別の版に署名されたファイル・署名後に差し替えたファイル・別の鍵で署名したファイルを拒否して何も残さないこと)。`go vet ./...` は macOS と GOOS=windows の両方で通過。`pnpm check:client` と `pnpm test:client` (18 件) も通過。
  - AC #5: `.app` でないパス・`/AppTranslocation/` の下・親が書き込み不可、の 3 つを単体テストで確かめた。読み取り専用ボリュームは、テストの中で hdiutil で .dmg を作って読み取り専用でマウントして確かめた。どの場合も昇格の経路はなく、理由コードを返す。
  - AC #7: ブランチで build.yml を手動実行 (run 37919867340、macOS と Windows の両方が success)。新しいステップで、インストーラのマニフェストが asInvoker であること、既定のインストール先が `%LOCALAPPDATA%\Programs\SNZ Studio` であること、アンインストール情報が HKCU にあって HKLM にないこと、/UPDATE のない silent インストールではアプリが起動しないことを確かめた。runner は管理者アカウントなので「UAC が出ない」ことは画面では確かめられない。asInvoker のマニフェストは昇格を求めないので、これを根拠にチェックした。
  - 最初の run (37914102587) は、`Start-Process -Wait` が子孫プロセスまで待つため、/UPDATE で起動したアプリを待ち続けて止まった。インストーラ本体だけを待つ形に直し、ステップに 10 分の上限を付けた。
- 未確認 (AC #8 は未チェック): Windows で動いているアプリから InstallUpdate を実行し、インストーラが起動してアプリが終了し、exe のロック待ちのあとに新しい版が起動する流れは、まだ一度も動かしていない (CI で確かめたのは、アプリが動いていない状態での /UPDATE の再起動だけ)。Windows の実機か VM で確かめる必要がある。標準ユーザーのアカウントで UAC が出ないことの目視も同じ。
- 手元の makensis (Homebrew 3.12) は空のスクリプトでも bad_alloc で落ちたので、project.nsi のコンパイル確認は CI に頼った。
- build/bin の以前の成果物 (.dmg など) は、e2e のビルド (-clean) で消えている。最後に一時ファイルを含まない .app をビルドし直してある。

- レビュー 1 回目 (Codex) の [P2] への対応: 途中で送信が止まり接続だけ開いたままのサーバに対して、ダウンロードが永久に待ち、二重インストール防止のフラグも立ったままで再試行できなかった。60 秒何も届かなければ中断する watchdog を入れた (応答待ちの間も含む)。全体の期限にしなかったのは、遅くても動いている回線を切らないため。止まった転送が中断されてファイルが残らず、再試行が成功すること、遅くても途切れない転送は切られないことをテストした。[P3] (リリース一覧の 2 ページ目を見ない) は対応しない。GitHub の一覧は作成日時の新しい順で、新しい安定版が 2 ページ目に回るのは、それより後に作られた 100 件がすべてそれより低い版のときだけだから。

- AC #8 (Windows 実機での e2e、2026-10-10、ユーザーが実施): この PR のコードを CI でビルドしたテスト用インストーラを 2 つ用意した。旧版は版 0.1.0 で devtools 付き (run 37921737909)、新版は版 0.1.1 (run 37921742253)。新版は本物の更新署名鍵で署名し、latest.json と一緒にローカルの偽の配布元 (SNZ_UPDATE_BASE_URL) から配った。旧版をインストールし、開発者ツール (Ctrl+Shift+F12) から CheckForUpdate → InstallUpdate("0.1.1") を実行した。配布元への GET (一覧 → latest.json → インストーラ) は 00:38:06、新しい本体と新しい llama-server の起動は 3 秒後の 00:38:09 で、インストール先から動いているのはその 2 つだけだった (古い本体と古い llama-server は残っていない)。HKCU の DisplayVersion は 0.1.1、画面の版表示も v0.1.1。旧版のインストールで UAC は出なかった。捨てるブランチ e2e/windows-update-old と e2e/windows-update-new はこの確認のためだけに作った。
<!-- SECTION:NOTES:END -->
