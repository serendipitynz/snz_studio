---
id: TASK-59
title: 'レイアウト: 画面外の隠しテキストで body がスクロールし、固定のペインがずれる不具合を直す'
status: Done
assignee: []
created_date: '2026-09-26 11:56'
updated_date: '2026-09-26 12:24'
labels:
  - design
dependencies: []
ordinal: 59000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
TASK-57 (#55) の実窓の確認 (2026-09-26) で、オーナーが見つけた。`data/app.sqlite` のプロジェクト「トーク with doc」の詳細画面を開くと、ページ全体 (body) が縦にスクロールできるようになり、固定のはずの左サイドバーやペインがまとめてずれる。ほかのプロジェクトでは起きない。TASK-57 の変更とは関係なく、main に以前からある。

コードとデータから推定した原因 (実窓での再現と計測はまだ):
- インスペクタのチャット一覧では、一時チャットの行に、読み上げ用の隠しテキスト「一時チャット」(`VisuallyHidden`、`frontend/src/styles/ui.tsx`) を置く (`ProjectDetailPage.tsx` のチャット一覧)。
- `VisuallyHidden` は `position: absolute`。祖先の `InspectorPane`・`Card`・`Item` はどれも位置の指定を持たないので、位置の基準はペインではなく画面全体になる。この要素はインスペクタの中の本来の位置に置かれるが、インスペクタの `overflow: auto` では切り取られない。インスペクタの内容が長く、その位置が画面の下端より下にあると、ページの高さがそこまで伸びる。
- `data/app.sqlite` で一時チャットを持つプロジェクトは「トーク with doc」だけ (5 件中 2 件)。ほかのプロジェクトは 0 件なので、このプロジェクトでだけ起きることと合う。

直し方の案: スクロールするペイン (`InspectorPane`・`SidebarPane`・`PaneBody`) に `position: relative` を持たせ、中の `position: absolute` の要素 (`VisuallyHidden`、Checkbox の隠した入力、ReorderList の読み上げなど) の基準をペインにする。`InspectorPane` の重ねる表示 (`$overlay`) の `position: fixed` は、後に書かれた宣言として残す。同じ形の隠しテキストは、サイドバー・チャット画面・多人数会話の画面にもある。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 一時チャットを持つプロジェクトの詳細画面で、ページ全体 (body) がスクロールせず、サイドバー・本文・インスペクタの位置がほかのプロジェクトと同じになる
- [x] #2 原因を実際の画面で確かめ (ページの高さが画面の高さを超える要素を特定する)、Implementation Notes に記録している
- [x] #3 スクロールするペインの中の `position: absolute` の要素が、ペインの外へ出てページの高さを伸ばさないことを、サイドバー・チャット画面・多人数会話の画面でも確かめている
- [x] #4 インスペクタの重ねる表示 (1180px 以下) が、これまでどおり画面の端から重なって出る
- [x] #5 `pnpm check:client`・`pnpm build:client` が通る
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. pnpm dev + Playwright (Chromium/WebKit) で実データの各画面を計測し、ページ高を伸ばす要素と基準 (containing block) を特定する
2. SidebarPane・InspectorPane・PaneBody に position: relative を足す。InspectorPane の $overlay の position: fixed は後の宣言として残す
3. 同じ計測で修正後を確認 (詳細画面・チャット画面・多人数会話・サイドバーの長い一覧・1180px 以下の重ねる表示)
4. pnpm check:client・pnpm build:client
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 原因 (AC#2)
`pnpm dev` (実データ `data/app.sqlite`) を Playwright の Chromium / WebKit から開き、`document.scrollingElement.scrollHeight` と、画面の下端より下にある `position: absolute` の要素・その基準 (offsetParent)・いちばん近いスクロール領域を計測した。
- 「トーク with doc」の詳細画面 (1280×800): scrollHeight 1043 (WebKit 1001) で、画面の高さ 800 を超える。超えているのはインスペクタ (`aside`) の一時チャット2行の隠しテキスト「一時チャット」(bottom 943・1043)。基準は body で、インスペクタの `overflow: auto` の切り取りを受けていない。推定どおり。
- 同じ形がサイドバーにもあった: 画面の高さ 500 では、チャットが9件ある「対話」の詳細画面・チャット画面で、サイドバーのナビ (`nav`「プロジェクトとチャット」) の多人数会話の行の隠しテキスト「多人数会話」(bottom 544・600、基準 body) がページの高さを 600 に伸ばしていた。チャットが多いプロジェクトや低い窓で起きる。
- チャット画面・多人数会話の画面の本文 (MessageScroller) の中の隠しテキストは、基準が `MainPane` (position: relative・overflow: hidden) なので、もともとページの高さを伸ばさない。

## 直し方
`SidebarPane`・`InspectorPane`・`PaneBody` に `position: relative` を足した。スクロール領域そのもの (サイドバーの nav など) ではなくペインに持たせたのは、ペインの中のどこに隠しテキストが増えても同じ規則で閉じ込められるから。`InspectorPane` の重ねる表示の `position: fixed` は後の宣言なのでそのまま勝つ。
ほかの `position: absolute` の部品 (ReorderList の DropPosition、Hint の Body、Checkbox の NativeBox、サイドバーの KindMenu・CollapsedPanel、RegionCloseButton) は、それぞれ自前の位置指定の親を持つので基準は変わらない。

## 確認 (修正後)
- AC#1・#3: 詳細画面 (トーク with doc・対話・翻訳)・一時チャットの単独チャット画面・一時チャットの多人数会話の画面・サイドバーに9件並ぶ多人数会話の画面・プロジェクト一覧を、1280×800・1440×900・1100×800・1280×500 の各サイズで計測し、Chromium・WebKit とも scrollHeight が画面の高さと一致 (修正前は 8 件が超過)。隠しテキストの基準は `aside` (インスペクタ・サイドバー) や `PaneBody` に変わった。1280×800 で `window.scrollTo(0, 1e6)` 後も scrollY 0、3つのペインの位置 (x=9/298/931、y=9、高さ 782) はトーク with doc と他の画面で同じ。
- AC#4: 1100×800 でトリガーを押すと、詳細画面・チャット画面・多人数会話の画面とも、インスペクタは position: fixed、右端・上下から 9px、幅 360、スライドインのアニメーションで出る。閉じるボタンはペインの右上 (左から 316・上から 6)。
- AC#5: `pnpm check:client`・`pnpm build:client` 成功。

## 計測環境と残したこと
Playwright 1.62.1 の Chromium / WebKit (headless、WKWebView ではない)、`pnpm dev` の localhost:34115、倍率 1、表示言語 ja、2026-09-27。実窓 (WKWebView) での目視はしていないので、オーナーの確認に残す。
<!-- SECTION:NOTES:END -->
