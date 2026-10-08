---
id: TASK-92
title: '自動更新: アプリ内で新しい版を確認し、承認後にダウンロード・署名検証・入れ替え・再起動まで行う'
status: To Do
assignee: []
created_date: '2026-10-07 22:52'
updated_date: '2026-10-08 22:20'
labels: []
dependencies: []
references:
  - 'https://github.com/serendipitynz/mallow/tree/main/backlog/tasks'
  - 'https://github.com/serendipitynz/backlog-atlas/tree/main/backlog/decisions'
  - .github/workflows/release.yml
priority: medium
type: feature
ordinal: 92000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

v0.1.0 には更新の手段がない。新しい版が出ても、利用者が GitHub Releases を見に行かない限り気づけず、入れ替えも手作業になる。

これを、mallow の TASK-11 と同じ範囲の自動更新で解決する。アプリ内で新しい版を確認し、利用者が承認したらダウンロード・署名検証・入れ替え・再起動まで行う。

## 採らなかった案

- **新しい版の通知だけ** (backlog-atlas TASK-157 / decision-44 の方式。Releases ページを既定ブラウザで開くところまで) — 確認・設定画面・ダイアログの部分は自動更新と共通で、移行時の手戻りは小さい。ただし段階を踏むと、利用者の手動更新が 1 回増える (v0.1.0 → 通知を載せた版 → 自動更新を載せた版)。自動更新を近いうちに入れるので、直接こちらへ進む (2026-10-08 に決定)。
- **Homebrew Cask** — macOS しか覆えない。

## mallow との違い

- **Wails v2 には公式の更新機能がない。** mallow は tauri-plugin-updater が確認・署名検証・インストール・再起動を担っていたが、ここでは Go で書く。
- 署名と検証には Go 標準の `crypto/ed25519` を使い、新しい本番依存を入れない。サードパーティの selfupdate 系ライブラリを使いたくなったら、着手前に相談する。
- 対象は macOS (universal) と Windows (amd64) の 2 つだけ。mallow で一番手間がかかった Linux の deb / rpm / AppImage は無い。
- latest.json は release.yml の `attach` ジョブ 1 つで書く。mallow の「並列のビルドジョブが同時に書いて、どこかのプラットフォームが抜ける」不具合は、構造上起きない。

## 最新版の探し方

`releases/latest` には頼らない。このリポジトリではモデルも別のタグ (`ruri-v3-30m-q8_0-…`) で Releases に出していて、それを公開した時点で `latest` がモデルのリリースに移りうる。アプリは公開済み・非 prerelease のリリースの中から最大の版タグを選び、そのタグに固定した URL `releases/download/vX.Y.Z/latest.json` を読む。latest.json の中の各 URL もタグに固定する (mallow TASK-11.1 で、`/latest/download/` を指す URL が後の版の公開で 404 になる問題が見つかっている)。

## 届く範囲

更新機能を載せた最初の版より前の版 (v0.1.0 を含む) にはこの機能が無いので、何も届かない。その利用者には一度だけ手動で更新してもらう。README と、更新機能を載せた最初の版のリリースノートにそう書く (TASK-92.4)。

## 決めたこと (2026-10-08)

- **自動確認は既定で有効にし、設定でオフにできるようにする** (mallow と同じ)。利用者に頼まれずにアプリ自身がインターネットへ出るのはこれが初めてで、AGENTS.md の優先事項には「Local-only operation」がある。それでも、自動確認がオフの状態で出荷すると、新しい版があることがほぼ誰にも届かない。外へ出るのは起動ごとの GitHub への照会 1 回だけで、失敗しても何も出さない。
- **Windows は per-user インストール (`WAILS_INSTALL_SCOPE=user`、`%LOCALAPPDATA%\Programs`) に変える。** Wails の NSIS テンプレートの既定は `admin` 実行で Program Files にインストールするので、そのままだと更新のたびに UAC が出る。per-user なら更新に管理者の承認は要らない。
  - 既存の Program Files のインストールは自動では移らない。ただ、v0.1.0 には更新機能が無く、どのみち一度は手で新しい版を入れる必要がある。そのときに古いほうをアンインストールしてもらえば移行は済む (README とリリースノートに書く。TASK-92.4)。利用者はほぼいないので、インストーラ側で旧インストールを検出する処理は作らない。
  - データはインストール先ではなく `os.UserConfigDir()` の下 (`internal/bootstrap/paths.go`) にあるので、インストール先が変わってもデータは引き継がれる。

## 確認に要るもの

更新機能を載せたリリースが 2 つ続けて公開されないと、最後まで確かめられない (draft では `releases` の一覧にも latest.json の URL にも出ない)。リリース計画の中に組み込む。Windows の実機 (または VM) も必要。

## 分割

レビューの単位が違うので 4 つに分ける。リリース側 (TASK-92.1)、Go の更新処理 (TASK-92.2)、UI (TASK-92.3)、ドキュメント (TASK-92.4)。
<!-- SECTION:DESCRIPTION:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 サブタスクがすべて Done になっている
- [ ] #2 インストール済みの版から次の版へ、macOS と Windows の両方でアプリ内から更新でき、再起動後に新しい版が動く (更新機能を載せたリリースが 2 つ続けて公開された後に確かめる)
- [ ] #3 更新署名鍵の秘密鍵がこのマシン以外にも保管されていて、失くしたときに何が起きるかが書かれている
- [ ] #4 (TASK-92.1 から移した確認) 更新機能を載せた最初のリリースで、公証・staple 済みの .app を固めた macOS の更新用アーカイブ (SNZ-Studio-vX.Y.Z-macOS.app.zip) が Release に添付され、それを展開した .app が spctl の検査を通る
- [ ] #5 (TASK-92.1 から移した確認) そのリリースの実行で、prepare の鍵の一致確認 (updatesig check-key) が通り、attach の verify ステップが更新用アーカイブと Windows インストーラの署名を添付前にリポジトリの公開鍵で検証している
- [ ] #6 (TASK-92.1 から移した確認) そのリリースに latest.json が添付され、darwin-universal と windows-amd64 の URL と署名を持ち、URL はタグに固定されている
<!-- DOD:END -->
