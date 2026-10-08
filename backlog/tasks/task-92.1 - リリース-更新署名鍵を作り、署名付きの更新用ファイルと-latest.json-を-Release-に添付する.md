---
id: TASK-92.1
title: 'リリース: 更新署名鍵を作り、署名付きの更新用ファイルと latest.json を Release に添付する'
status: To Do
assignee: []
created_date: '2026-10-07 22:52'
labels: []
dependencies: []
references:
  - .github/workflows/release.yml
  - .github/workflows/build.yml
  - scripts/setup-ci-signing-secrets.sh
  - scripts/build-mac-signed.sh
parent_task_id: TASK-92
priority: medium
type: feature
ordinal: 93000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
TASK-92 のリリース側。インストール済みのアプリが新しい版を見つけ、その中身が本物であることを確かめられるようにする。

## 作業

- **更新署名鍵**: Ed25519 の鍵ペアを作る。公開鍵はリポジトリに置き、Go のバイナリに埋め込む。秘密鍵は repo secret にする (名前は着手時に決める)。`prepare` ジョブの `APPLE_*` secrets の確認に並べ、欠けていたらビルド前に止める。登録手段として `scripts/setup-ci-signing-secrets.sh` に更新署名鍵だけを登録する経路を足す (今のスクリプトは .p12 を必須の引数に取るので、鍵だけを差し替えるときに証明書まで要求してしまう。mallow TASK-11.1 で同じ問題があった)。
- **macOS の更新用アーカイブ**: 今の build.yml は `.dmg` にしか公証・staple をしていない。更新では `.app` を直接置き換えるので、`.app` 自体を公証して staple し、それを `ditto -c -k --keepParent` などシンボリックリンクと拡張属性を保つ形で固める。`.dmg` はその staple 済みの `.app` から作る。
- **Windows**: NSIS インストーラ (`*-installer.exe`) をそのまま更新用ファイルにする。
- **署名**: 更新用アーカイブと Windows インストーラに更新署名鍵で署名する。署名したあと、リポジトリの公開鍵で検証してから添付する。mallow で一番危なかったのは、秘密鍵と公開鍵が食い違っていてもビルドが警告だけで通り、どのクライアントも受け付けない署名が出荷されるケースだった。
- **latest.json**: `attach` ジョブで書き、Release に添付する。中身は版と、`darwin-universal` / `windows-amd64` ごとの URL・署名。URL はタグに固定する (`releases/download/vX.Y.Z/<asset>`)。`notes` を入れるかどうかは決めて記録する (mallow は空のままにし、ダイアログを notes なしで読める形にした)。
- アセット名は今の `SNZ-Studio-$TAG-…` の命名に合わせる。SHA256SUMS.txt に新しいアセットも載せる。

## mallow と違って起きないこと

- **公開鍵を置いても、手元のビルドは止まらない。** Tauri は公開鍵があると秘密鍵なしのビルドを止めるが、ここでは公開鍵をバイナリに埋め込むだけで、署名は CI の別の手順で行う。`scripts/build-mac-signed.sh` もそのまま動くはず。着手時に確かめる。
- latest.json を書くのは `attach` ジョブ 1 つなので、同時に書き込む問題はない。build.yml の matrix を並列のまま保ってよい。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 更新署名鍵が作られ、公開鍵がリポジトリに、秘密鍵が repo secret にあり、秘密鍵はコミットされていない
- [ ] #2 secret が欠けていると、release.yml がビルド前に理由を示して止まる
- [ ] #3 公証・staple 済みの .app を固めた macOS の更新用アーカイブが Release に添付され、それを展開した .app が spctl の検査を通る
- [ ] #4 更新用アーカイブと Windows インストーラの署名が、添付の前にリポジトリの公開鍵で検証されている
- [ ] #5 Release に latest.json が添付され、darwin-universal と windows-amd64 の URL と署名を持ち、URL はタグに固定されている
- [ ] #6 latest.json の notes を入れるかどうかが決まって記録されている
- [ ] #7 scripts/setup-ci-signing-secrets.sh で更新署名鍵だけを登録でき、手元の scripts/build-mac-signed.sh が鍵なしでも最後まで動く
<!-- AC:END -->
