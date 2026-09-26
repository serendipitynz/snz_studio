---
id: TASK-59
title: 'レイアウト: 画面外の隠しテキストで body がスクロールし、固定のペインがずれる不具合を直す'
status: To Do
assignee: []
created_date: '2026-09-26 11:56'
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
- [ ] #1 一時チャットを持つプロジェクトの詳細画面で、ページ全体 (body) がスクロールせず、サイドバー・本文・インスペクタの位置がほかのプロジェクトと同じになる
- [ ] #2 原因を実際の画面で確かめ (ページの高さが画面の高さを超える要素を特定する)、Implementation Notes に記録している
- [ ] #3 スクロールするペインの中の `position: absolute` の要素が、ペインの外へ出てページの高さを伸ばさないことを、サイドバー・チャット画面・多人数会話の画面でも確かめている
- [ ] #4 インスペクタの重ねる表示 (1180px 以下) が、これまでどおり画面の端から重なって出る
- [ ] #5 `pnpm check:client`・`pnpm build:client` が通る
<!-- AC:END -->
