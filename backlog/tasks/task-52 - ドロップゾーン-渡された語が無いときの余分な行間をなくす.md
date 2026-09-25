---
id: TASK-52
title: 'ドロップゾーン: 渡された語が無いときの余分な行間をなくす'
status: To Do
assignee: []
created_date: '2026-09-25 12:45'
labels:
  - design
dependencies: []
ordinal: 52000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
TASK-46 (#46) のレビュー 3 回目の [P3]。`components/FileDropZone.tsx` は、置く側から渡された語 (画像文書の追加ダイアログのファイル名と注記) を、ファイルが上にある間の語の色に合わせるため囲み (`ZoneExtra`) で包む (38a42b8)。囲むかどうかを `props.children` が真かどうかで決めているので、子の式が2つとも null の配列 (`[null, null]`) のときも、空の囲みを置いてしまう。

起きること: 画像を選ぶ前の画像文書の追加ダイアログで、ゾーンの中に高さ 0px・中身 0 の囲みが残る。そのぶん行間 (8.8px) が1つ増え、受け付ける種類の語と「画像を選ぶ」ボタンの間が1段広い (Chromium で確かめた)。3e7fdda まではこの隙間は無かった。見た目だけで、操作・読み上げ・コントラスト比には影響しない。

直し方の候補: `Children.toArray(props.children)` の長さで囲むかを決める。または `ZoneExtra` に `&:empty { display: none; }` を足す。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 画像を選ぶ前の画像文書の追加ダイアログで、ドロップゾーンの中に空の囲みが残らず、受け付ける種類の語と「画像を選ぶ」ボタンの間が他の行間と同じである
- [ ] #2 画像を選んだ後のファイル名が、ファイルが上にある間も上にある間の語の色 (onAccentSoft) で描かれる (38a42b8 の直しを保つ)
- [ ] #3 `pnpm check:client`・`pnpm build:client` が通る
<!-- AC:END -->
