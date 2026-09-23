---
id: TASK-40
title: 画像の追加ダイアログにドラッグ&ドロップで画像を渡せるようにする
status: In Review
assignee: []
created_date: '2026-09-23 05:37'
updated_date: '2026-09-23 10:49'
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
- [x] #1 画像ダイアログ内のドロップエリアに画像を 1 枚落とすと、「画像を選ぶ」で選んだときと同じくプレビュー・タイトル初期値・説明文生成の可否判定が行われる
- [x] #2 複数ファイルを落とすと先頭の画像だけが使われ、その旨が表示される。画像以外のファイルは受け付けられず理由が表示される
- [x] #3 ドキュメント欄の既存の一括ドロップエリアの挙動は変わらない
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. ImageDocumentDialog の「画像を選ぶ」行とファイル名を DropZone で囲み、ドロップのヒント文を添える。dragover/leave は ProjectDetailPage と同じ扱い
2. ドロップしたファイル群から先頭の image/* を選ぶ。画像が無ければ ErrorText で理由を出し、複数・非画像混在なら先頭画像を使った旨を Subtle で出す。選んだ画像は handleChooseFile に渡す
3. busy 中のドロップは無視する (ボタンの disabled と同じ)
4. i18n (en/ja) にヒント・通知・エラー文言を追加
5. pnpm check:client / build:client と、ブラウザでのドロップ操作で確認
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- 「画像を選ぶ」ボタン行をダイアログ内の DropZone で囲み、ドロップを同じ handleChooseFile に渡す形にした。ドロップ先をダイアログ全体にしなかったのは、Description が「DropZone と同じ見た目のドロップ先を置く」と定めており、どこに落とせばよいかが見える方が一括ドロップエリアと操作が揃うため。
- 先頭の画像は dataTransfer.files のうち type が image/* の最初のもの。1 件も無ければ ErrorText で理由を出し、既に選んだ画像は残す。複数ファイル時の通知は Subtle で出し、次にファイルを選ぶと消える。生成中・保存中のドロップはボタンの disabled と同じく無視する。
- 検証 (pnpm dev の http://localhost:34115 で、DataTransfer を合成した dragover/drop をダイアログのドロップ先に送出):
  - AC#1: png 1 枚 → プレビュー表示・タイトル初期値 single.png・「説明文を生成」が有効 (画像説明モデル設定済み環境)。ファイルピッカー経路と同じ handleChooseFile を通る。
  - AC#2: a.png, b.png, c.txt → a.png を採用し「3 件のファイルがドロップされたため、先頭の画像 a.png だけを使います。」を表示。notes.txt, z.png → z.png を採用。notes.txt のみ → 画像ではない旨のエラー、ファイル未選択のまま。
  - AC#3: ProjectDetailPage.tsx と DropZone のスタイルは差分なし。
  - pnpm check:client / pnpm build:client 通過。
- 未確認: macOS の Finder から Wails の WebView へ実ファイルをドラッグする操作 (合成イベントでの確認のみ)。
<!-- SECTION:NOTES:END -->
