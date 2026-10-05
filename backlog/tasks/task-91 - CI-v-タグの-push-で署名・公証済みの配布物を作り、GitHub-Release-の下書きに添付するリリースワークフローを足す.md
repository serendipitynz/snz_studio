---
id: TASK-91
title: 'CI: v タグの push で署名・公証済みの配布物を作り、GitHub Release の下書きに添付するリリースワークフローを足す'
status: In Review
assignee: []
created_date: '2026-10-05 00:13'
updated_date: '2026-10-05 00:27'
labels: []
milestone: m-1
dependencies: []
references:
  - .github/workflows/build.yml
  - scripts/build-mac-signed.sh
  - >-
    https://github.com/serendipitynz/backlog-atlas/blob/main/.github/workflows/release.yml
  - >-
    https://github.com/serendipitynz/mallow/blob/main/.github/workflows/release.yml
priority: medium
type: chore
ordinal: 91000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

serendipitynz/mallow・backlog-atlas・mermaid2pptx には `.github/workflows/release.yml` があり、`v*` タグを push すると配布物が GitHub Release に添付される。snz_studio にあるのは手動でしか動かない `build.yml` だけで、成果物は Actions の artifact に残るだけになっている。macOS 版は署名も公証もしていない (Gatekeeper の警告が出る)。署名付きの .dmg は、ローカルの `scripts/build-mac-signed.sh` でしか作れない。このリポジトリには secrets が 1 つも登録されていない。

v0.1.0 はこのワークフローで出す (TASK-90 のあと、このタスクが終わってから `v0.1.0` タグを打つ)。署名には mallow・backlog-atlas と同じ Developer ID を使う。

## 方針

- backlog-atlas の release.yml を手本にする。起動は `v*` タグの push と、タグを指定した `workflow_dispatch`。Release は下書きとして作り、ノートは前のタグ以降にマージされた PR から自動生成する (`.github/release.yml` で分類する)。公開は手で行う。
- 最初のジョブで次を確かめ、どれかが欠けていれば何もビルドせずに失敗させる。
  - `APPLE_*` secrets 6 つがそろっていること
  - タグが `vMAJOR.MINOR.PATCH` で、`package.json` の `version` と `wails.json` の `info.productVersion` に一致すること
  - そのタグの Release がすでに公開されていないこと (公開済みの資産を上書きしない)
- macOS: `build-mac-signed.sh` と同じ手順 (同梱の llama-server と dylib を含めた署名、公証、.dmg への staple) を、runner の一時 keychain に読み込んだ証明書で行う。Windows は署名しない。
- ビルドの中身 (sidecar と GGUF の配置、ネットワークなしのスモークテスト、ライセンス文書の同梱) は `build.yml` と同じにする。2 つのワークフローに同じ手順を重複させないため、共通部分を reusable workflow か composite action に切り出すかを着手時に決める。
- secrets の登録用に `scripts/setup-ci-signing-secrets.sh` と `.env.signing.example` を backlog-atlas から移す。`.gitignore` は `.env` しか除外していないので、`.env.signing` を足す。
- README / README.ja に、リリースの手順 (タグを打つ、下書きを確認して公開する) と、配布物を Releases から入手する方法を書く。

## 範囲外 (着手時に扱いを決める)

- `build.yml` の macOS 版は universal の .app に arm64 の sidecar だけを入れている (build.yml の NOTE / TODO)。Intel Mac では内蔵の埋め込みが動かない。リリースで同じ制約のまま出すか、README に書くかを決める。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 v* タグを push すると、署名・公証済みの macOS .dmg と Windows のインストーラが添付された Release の下書きができる
- [ ] #2 secrets の欠け、タグとバージョンの不一致、公開済みの Release のどれかがあると、ビルドの前に理由を示して失敗する
- [ ] #3 リリースノートが前のタグ以降の PR から自動生成され、.github/release.yml の分類で並ぶ
- [x] #4 scripts/setup-ci-signing-secrets.sh で APPLE_* secrets 6 つを登録でき、.env.signing は git の追跡から外れている
- [x] #5 README / README.ja にリリースの手順と配布物の入手方法が書かれている
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- 署名と公証は build.yml に `workflow_call` の入力 `sign` を足して入れ、release.yml はそれを呼ぶ形にした (ビルド手順を 1 か所に保つため)。手動の build.yml は未署名のまま。
- AC #1〜#3 は CI 上で `v0.1.0` タグを打った最初の実行で確かめる。下書きは非公開なので、失敗したら下書きとタグを消して直す。
- secrets 6 つは 2026-10-05 に setup-ci-signing-secrets.sh で登録済み。
<!-- SECTION:NOTES:END -->
