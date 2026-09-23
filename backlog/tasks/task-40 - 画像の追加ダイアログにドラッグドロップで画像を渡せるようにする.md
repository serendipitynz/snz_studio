---
id: TASK-40
title: 画像の追加ダイアログにドラッグ&ドロップで画像を渡せるようにする
status: To Do
assignee: []
created_date: '2026-09-23 05:37'
labels: []
dependencies:
  - TASK-38
references:
  - frontend/src/components/ImageDocumentDialog.tsx
  - frontend/src/pages/ProjectDetailPage.tsx
type: feature
ordinal: 40000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-38 で追加した「説明付きで画像を追加」ダイアログ (`ImageDocumentDialog`) は、画像を「画像を選ぶ」ボタンのファイルピッカーからしか受け取れない。ドキュメント欄の一括アップロードはドロップエリア (`DropZone`) にファイルを落とせるので、画像ダイアログだけ操作が揃っていない。

## 方針

- ダイアログ内に、既存の `DropZone` と同じ見た目のドロップ先を置き、落とした画像を「画像を選ぶ」と同じ経路 (`handleChooseFile`) で受け取る。
- 受け取るのは 1 枚だけ。複数ファイルを落とされたら先頭の画像だけを使い、そのことを表示する。画像以外のファイルは受け付けず理由を表示する。
- ドキュメント欄の一括ドロップエリアの挙動は変えない。

## スコープ外

- 一括ドロップエリアに落とした画像を、このダイアログへ振り分けること。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 画像ダイアログ内のドロップエリアに画像を 1 枚落とすと、「画像を選ぶ」で選んだときと同じくプレビュー・タイトル初期値・説明文生成の可否判定が行われる
- [ ] #2 複数ファイルを落とすと先頭の画像だけが使われ、その旨が表示される。画像以外のファイルは受け付けられず理由が表示される
- [ ] #3 ドキュメント欄の既存の一括ドロップエリアの挙動は変わらない
<!-- AC:END -->
