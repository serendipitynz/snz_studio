---
id: TASK-92.1
title: 'リリース: 更新署名鍵を作り、署名付きの更新用ファイルと latest.json を Release に添付する'
status: In Review
assignee: []
created_date: '2026-10-07 22:52'
updated_date: '2026-10-08 19:52'
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
- [x] #1 更新署名鍵が作られ、公開鍵がリポジトリに、秘密鍵が repo secret にあり、秘密鍵はコミットされていない
- [x] #2 secret が欠けていると、release.yml がビルド前に理由を示して止まる
- [ ] #3 公証・staple 済みの .app を固めた macOS の更新用アーカイブが Release に添付され、それを展開した .app が spctl の検査を通る
- [ ] #4 更新用アーカイブと Windows インストーラの署名が、添付の前にリポジトリの公開鍵で検証されている
- [ ] #5 Release に latest.json が添付され、darwin-universal と windows-amd64 の URL と署名を持ち、URL はタグに固定されている
- [x] #6 latest.json の notes を入れるかどうかが決まって記録されている
- [x] #7 scripts/setup-ci-signing-secrets.sh で更新署名鍵だけを登録でき、手元の scripts/build-mac-signed.sh が鍵なしでも最後まで動く
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. internal/updatesig: 公開鍵 (update-signing-key.pub, base64) を go:embed し、署名対象メッセージ (版・プラットフォーム・ファイル全体の SHA-256) の組み立てと Verify を置く。92.2 のクライアントもこれを使う
2. tools/updatesig: keygen / check-key / sign-release (署名 → 公開鍵で検証 → latest.json 生成)。標準ライブラリのみ
3. 鍵ペアを生成し、公開鍵をコミット、秘密鍵は repo 外のファイルに置き UPDATE_SIGNING_KEY として登録
4. setup-ci-signing-secrets.sh に --update-key PATH (鍵だけ登録、公開鍵との一致を先に確認)
5. release.yml prepare: UPDATE_SIGNING_KEY の欠落チェックと鍵の一致確認をビルド前に
6. build.yml macOS: .app を公証・staple → ditto で .app.zip → staple 済み .app から .dmg
7. release.yml attach: 署名・検証・latest.json (notes なし、URL はタグ固定) を作り、SHA256SUMS に含めて添付
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- 署名の対象はファイル全体ではなく「版・プラットフォーム・ファイル全体の SHA-256」を並べた短いメッセージ (`internal/updatesig` の `message`)。更新用ファイル (.app.zip 約 64MB、モデル込み) を丸ごとメモリに載せずに済み、版を署名に含めることで古い署名済みファイルを新しい版番号で配るダウングレードを防ぐ。frontend/dist は Go バイナリに go:embed されているので SHA-256 の範囲に入っている (2026-10-09 にユーザーと合意)。
- 形式を 1 か所に置くため、latest.json の型・署名メッセージ・埋め込み公開鍵を `internal/updatesig` にまとめ、CI の署名 (`tools/updatesig`) と TASK-92.2 のクライアントの両方がこれを使う。標準ライブラリのみで本番依存は増えていない。
- secret 名は `UPDATE_SIGNING_KEY` (Ed25519 seed の base64、パスワードなし)。秘密鍵の手元の保管先は `~/.config/snz-studio/update-signing.key` (600)。公開鍵は `internal/updatesig/update-signing-key.pub`。別の場所への保管は TASK-92 の DoD #3 で行う。
- AC #6: latest.json に notes は入れない。attach の時点ではリリースはまだ下書きで、ノートは公開前に手で直すため、入れると直す前の文面が出荷される。ダイアログからはタグのリリースページに案内する想定 (TASK-92.3)。
- 鍵の食い違い対策: prepare で `updatesig check-key` を走らせ、秘密鍵がタグの公開鍵と対でなければビルド前に止める。attach では署名のあと secret を持たないステップで `updatesig verify` を走らせ、ディスク上の latest.json とファイルをタグの公開鍵で検証してから添付する。
- 確認したこと:
  - AC #1: keygen で生成し公開鍵をコミット、秘密鍵は repo 外。`setup-ci-signing-secrets.sh --update-key` で登録済み (2026-10-09、`gh secret list` に UPDATE_SIGNING_KEY)。
  - AC #2: secret 登録前にブランチから release.yml を手動実行 (run 37835129300, tag v0.0.99)。prepare の「Verify the signing secrets are registered」が `Missing repository secret UPDATE_SIGNING_KEY` と登録手順を出して失敗し、build / attach は skipped、下書きは作られなかった。
  - AC #7: 対でない鍵を `--update-key` に渡すと `gh secret set` の前に止まる。手元の `build-mac-signed.sh` は UPDATE_SIGNING_KEY なしで最後まで通った (公証 Accepted、spctl source=Notarized Developer ID)。
  - CI の .app 手順を手元のビルドで再現: .app の zip を公証 (Accepted) → staple → ditto で固め展開 → stapler validate / spctl --assess --type execute (Notarized Developer ID) / codesign --verify --strict --deep が通った。
  - 署名ツール: 本物の鍵で sign → verify が通り、ファイル改ざん・版の不一致・対でない鍵・空の鍵はどれも失敗した。`go vet ./...` と `go test ./...` は通過。
- 未確認 (次の正式リリースの下書きで確かめる): AC #3 (Release 上の .app.zip が spctl を通る)、AC #4 (attach の verify ステップの実行)、AC #5 (添付された latest.json の中身)。prepare の check-key ステップも CI ではまだ一度も走っていない (v0.0.99 はその前のステップで止まるため)。
- このタスクより前のコミットを指すタグでは release.yml が `tools/updatesig` を見つけられず失敗する。v0.1.0 などの再ビルドはしない前提。
<!-- SECTION:NOTES:END -->
