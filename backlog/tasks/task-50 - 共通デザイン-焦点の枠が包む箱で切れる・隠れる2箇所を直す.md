---
id: TASK-50
title: '共通デザイン: 焦点の枠が包む箱で切れる・隠れる2箇所を直す'
status: Done
assignee: []
created_date: '2026-09-25 03:04'
updated_date: '2026-09-25 03:18'
labels:
  - design
dependencies: []
references:
  - ../snz-design
ordinal: 50000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
TASK-43 (#42) で部品に焦点の枠 (外側 2px・offset 1px) を持たせたが、オーナーの実窓の確認 (2026-09-25) で、包む箱のせいで枠が見えない箇所が2つ残った。snz-design doc-16 §6.3 と doc-14 §4.2 が挙げていた箇所で、TASK-44・TASK-45 から該当の AC をこのタスクへ移した。

- サイドバーのプロジェクト・チャットの並び: 並びを包む `Stack` がスクロールする箱 (`overflow: auto`) で、先頭の行の枠の上と左右が切れる。
- ダッシュボードのプロジェクトの行: 行 (`Item`) が `overflow: hidden` で、行いっぱいに広がるリンクの外側の枠がすべて隠れる。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 サイドバーのスクロールする並びの先頭・末尾・左右の行で、焦点の枠が切れずに描かれる
- [x] #2 ダッシュボードのプロジェクトの行にキーボードで焦点が来たとき、行の外周に焦点の枠が見える。ポインタで押したときは出ない (snz-design doc-8 §5.1、doc-9 §6.2 の押せるカードの焦点)
- [x] #3 `pnpm check:client`・`pnpm build:client` が通る
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 直したこと
- サイドバー: 並びを包むスクロールする箱 (`Stack`、`overflow: auto`) は padding の端で描画を切る。箱に焦点の枠が届く幅 (`FOCUS_RING_REACH` = 枠 2px + offset 1px。共通寸法から計算) の padding を持たせ、同じ幅の負の margin で打ち消して、行の位置は変えていない。snz-design doc-16 §6.3 の「箱に枠の幅の余白を持たせる」。
- ダッシュボードの行: 焦点を受けるのは行いっぱいに広がるリンクで、行が `overflow: hidden` なので枠が全部隠れていた。`$interactive` の行は、中のリンクに焦点があるとき (`:has(a:focus-visible)`) 行そのものの外周に枠を描き、リンク自身の枠は消す。説明用実例の押せるカード (snz-design doc-9 §6.2、`card.css` の `:has(.ex-card__open:focus-visible)`) と同じ形。「枠を内側に描く」は角丸の行で四角い枠が浮くので採らなかった。

## 確かめたこと
Chromium 152、macOS 26.6.2、1280×800、標準 Light、固定データの API。ダッシュボードで実際に Tab を 11 回送り、焦点を受けた各要素について、枠を描く要素の外周に枠の届く幅を足した矩形が、`overflow` を持つ祖先の要素の内側に収まるかを計算した。11 か所すべてで切れる祖先は無かった。ダッシュボードの行3つは行 (`article`) に 2px の枠、サイドバーのプロジェクトの先頭の行も切れなかった。`:focus-visible` は全箇所で真。`pnpm check:client`・`pnpm build:client` が通った。

## 残したこと
- 内蔵ブラウザの画面の記録には焦点の枠が描かれないので、見え方は計算で確かめた。WKWebView の実窓での見え方はオーナーの確認を待つ。

- レビュー (PR #43 の第1ラウンド、[P3] 3件) を受けて: 行いっぱいのリンクに `data-row-link` を付け、行の枠と、枠を消す規則をそのリンクに限った (行に別のリンクが足されたとき、それが枠を失わないように)。サイドバーの負の margin を横だけにした (縦にも掛けると、スクロールの切り口が区切り線の下の隙間へ 3px 出て、スクロールで上へ出た行がそこに残って見える)。修正後も、Tab 11 回の止まりで枠を切る祖先は無く、並びの上端は区切り線の下 16px (gap) のまま、行の左端は設定のボタンと揃っていることを確かめた。適用記録 (snz-design doc-17) へ、選んだ直し方を書くこと (doc-16 §6.3) は snz-design の側で行う。

- オーナーの実窓 (WKWebView) の確認 (2026-09-25): サイドバーの先頭の行、ダッシュボードのプロジェクトの行とも、焦点の枠が切れずに描かれることを確かめた (OK)。
<!-- SECTION:NOTES:END -->
