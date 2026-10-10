---
id: TASK-96
title: 'README: リリースの下書きのノートを API で直すと tag_name が外れることと、その避け方をリリース手順に書く'
status: To Do
assignee: []
created_date: '2026-10-10 10:39'
labels: []
milestone: m-3
dependencies: []
references:
  - README.md
  - README.ja.md
  - .github/workflows/release.yml
priority: medium
type: docs
ordinal: 100000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

v0.2.0 のリリース (TASK-95) で、release.yml が作った下書きのノートを `gh api -X PATCH repos/…/releases/<id> -F body=@…` で差し替えた。body だけを送るこの PATCH で、下書きの tag_name が `v0.2.0` から `untagged-…` に置き換わった。そのまま公開したところ、GitHub はリリースの target_commitish (`main`) の先頭にその `untagged-…` タグを作った。結果は次のとおり。

- アプリは `vX.Y.Z` の形のタグを持つリリースしか探さないので、このリリースを更新先として見つけられない。
- latest.json の `releases/download/v0.2.0/…` の URL は 404 になる。

このリポジトリでは immutable releases が有効なので、公開後は tag_name を変えられない (`tag_name cannot be changed when release is immutable`)。リリースとタグを消し、release.yml を手動で実行し直して作り直した。作り直した下書きで同じ PATCH をすると再び `untagged-…` になったので、原因はこの PATCH で確定している。

v0.1.0 ではノートを GitHub の画面で書き直していて、タグは外れていない。

## 方針

README.md / README.ja.md の「To cut a release」の手順 4 (ノートと添付を確かめて公開する段) に、次を書き足す。

- 下書きのノートは GitHub の画面で直す。API で直すときは、body と一緒に `tag_name` も送る。
- 公開する前に、下書きの tag_name がリリースのタグのままかを確かめる。
- 公開すると immutable になり、タグを付け替えられない。間違えたときは、リリースを消して作り直すしかない。そのとき消したタグの名前は二度と使えない。

release.yml 側の確認 (attach の最後に tag_name を確かめるなど) は入れない。タグが外れるのはワークフローが終わったあとのノートの手直しなので、ワークフローでは防げない。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 README.md の「To cut a release」に、下書きのノートを API で直すときは tag_name も送ること、公開前に tag_name を確かめること、公開後は immutable で付け替えられないことが書かれている
- [ ] #2 README.ja.md の同じ段に、同じ内容が日本語で書かれている
<!-- AC:END -->
