---
id: TASK-87
title: 'ドキュメント: 画像の受付を PNG / JPEG / GIF / WebP に絞り、アップロードの拒否理由を日本語で出す'
status: To Do
assignee: []
created_date: '2026-10-02 10:12'
labels: []
dependencies:
  - TASK-76
references:
  - frontend/src/pages/ProjectDetailPage.tsx
  - frontend/src/pages/ChatPage.tsx
  - frontend/src/components/ImageDocumentDialog.tsx
  - frontend/src/i18n/index.tsx
  - internal/httpapi/upload.go
priority: low
type: bug
ordinal: 87000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-76 (#71) で、サーバーは画像ドキュメントを中身から判定して PNG / JPEG / GIF / WebP だけを保存するようになり、それ以外は 400 (`image must be PNG, JPEG, GIF or WebP`) で拒否し、20MB を超えるアップロードは 413 (`upload is too large`) で拒否する。UI 側は変えていないため、次のずれが残っている (2026-10-02 に wails dev で SVG を追加して確認)。

- ドキュメント一覧のドロップゾーン (`ProjectDetailPage.tsx` / `ChatPage.tsx` の `detectDocumentType` と `isDocumentMime`、`inputAccept=".md,.markdown,.txt,image/*"`) と画像追加ダイアログ (`ImageDocumentDialog.tsx`、`image/*`) は、どの画像 MIME 型でも受け付ける。SVG / HEIC / BMP などは送信してからサーバーに拒否される。
- 受け付ける種類の表示 (`project.dropAccept`「…・画像」、`imageDialog.dropAccept`「画像ファイル」) が、実際に受け付ける形式を述べていない。`docs/current-spec.ja.md` のドキュメント追加モーダルは「受け付ける種類を述べ、それ以外は名を挙げて取り込まない」としている。
- 拒否されたとき、サーバーの英語メッセージがそのまま表示される。一括追加では 1 件の拒否で残りの取り込みも止まる。

## 方針

- UI の受付判定を `image/png` / `image/jpeg` / `image/gif` / `image/webp` に絞り、それ以外の画像は送信前に、ほかの非対応ファイルと同じ扱いで名前を挙げて取り込まない。
- 受け付ける種類の文言に 4 形式を明記する。
- サーバーの 400 (非対応の画像形式) と 413 (上限超過、20MB) を UI で日本語の理由に置き換えて表示する。
- UI の判定は案内にすぎず、拒否の判定はサーバーが持つ (TASK-76) ことは変えない。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 ドロップゾーンと画像追加ダイアログは PNG / JPEG / GIF / WebP 以外の画像 (SVG / HEIC など) を送信せず、名前を挙げて取り込まなかったことを示す
- [ ] #2 受け付ける種類の表示に 4 形式が明記されている
- [ ] #3 サーバーが非対応形式 (400) または上限超過 (413) で拒否したとき、UI には日本語の理由が表示される
- [ ] #4 PNG / JPEG / GIF / WebP と .md / .markdown / .txt の追加は今までどおりできる
<!-- AC:END -->
