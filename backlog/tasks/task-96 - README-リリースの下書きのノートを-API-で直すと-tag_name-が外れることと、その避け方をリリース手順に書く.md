---
id: TASK-96
title: 'README: リリースの下書きのノートを API で直すと tag_name が外れることと、その避け方をリリース手順に書く'
status: In Review
assignee: []
created_date: '2026-10-10 10:39'
updated_date: '2026-10-10 21:14'
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
- [x] #1 README.md の「To cut a release」に、下書きのノートを API で直すときは tag_name も送ること、公開前に tag_name を確かめること、公開後は immutable で付け替えられないことが書かれている
- [x] #2 README.ja.md の同じ段に、同じ内容が日本語で書かれている
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- README.md / README.ja.md の「To cut a release」/「リリースの手順」の手順 4 に 3 段を足した: (1) ノートは GitHub の画面で直し、API なら body と一緒に tag_name を送る (理由として v0.2.0 で起きたことと、アプリが見つけられず latest.json が 404 になることを書いた)、(2) 公開前に下書きの tag_name を確かめるコマンド、(3) immutable releases なので公開後は付け替えられず、消して作り直すしかなく、消したタグ名は再利用できない。
- 確かめるコマンドは `gh api 'repos/{owner}/{repo}/releases' --jq '.[] | select(.draft) | .tag_name'` にした。`gh release view vX.Y.Z` はタグが外れた下書きを名前で引けず「見つからない」で終わるので、外れたことを直接は示さない。下書きを列挙して tag_name を出すほうが、`untagged-…` をそのまま見せられる。
- 確認: 今は下書きが無いので上のコマンドは空を返した。select を反転した (`select(.draft|not)`) 同じコマンドが v0.2.0 / v0.1.0 / ruri-… を返すことで、プレースホルダの解決と jq の形が正しいことを確かめた。下書きがある状態での出力は次のリリースで見ることになる。
- docs のみの変更なので Go / フロントのテストは走らせていない。
<!-- SECTION:NOTES:END -->
