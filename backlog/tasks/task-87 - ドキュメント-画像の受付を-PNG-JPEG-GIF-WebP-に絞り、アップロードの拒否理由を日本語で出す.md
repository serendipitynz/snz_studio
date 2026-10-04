---
id: TASK-87
title: 'ドキュメント: 画像の受付を PNG / JPEG / GIF / WebP に絞り、アップロードの拒否理由を日本語で出す'
status: Done
assignee: []
created_date: '2026-10-02 10:12'
updated_date: '2026-10-04 05:00'
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
- [x] #1 ドロップゾーンと画像追加ダイアログは PNG / JPEG / GIF / WebP 以外の画像 (SVG / HEIC など) を送信せず、名前を挙げて取り込まなかったことを示す
- [x] #2 受け付ける種類の表示に 4 形式が明記されている
- [x] #3 サーバーが非対応形式 (400) または上限超過 (413) で拒否したとき、UI には日本語の理由が表示される
- [x] #4 PNG / JPEG / GIF / WebP と .md / .markdown / .txt の追加は今までどおりできる
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. frontend/src/components/documentFiles.ts を新設し、ProjectDetailPage / ChatPage に重複していた detectDocumentType・isDocumentMime と inputAccept を移す。画像は image/png・image/jpeg・image/gif・image/webp だけを受け付ける。ImageDocumentDialog も同じ判定を使う。
2. サーバーの拒否本文に機械可読な値を足す: 非対応画像の 400 に code: "unsupported_image"、413 に limit (バイト数) を載せる。英語メッセージの文字列一致で判定しないため。状態コードと error 文字列は変えない。
3. documentFiles.ts に、拒否 (400 unsupported_image / 413) をファイル名付きの日本語 (en 辞書は英語) の理由に置き換える関数を置き、3 か所のアップロード経路で使う。
4. 受け付ける種類の文言 (project.dropAccept / imageDialog.dropAccept) に 4 形式を明記する。
5. テスト: Go の upload_test に本文の code / limit を確認する検査を足す。frontend/test に判定関数と拒否文言の単体テストを足す。pnpm check:client / test:client / go test / wails dev での確認。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装

- 受付判定 (detectDocumentType / isDocumentMime / input の accept) を `frontend/src/components/documentFiles.ts` にまとめ、ProjectDetailPage・ChatPage の重複定義を消した。画像は image/png・image/jpeg・image/gif・image/webp だけを受け付ける。ImageDocumentDialog も同じ判定を使う。
- サーバーの拒否本文に機械可読な値を足した: 非対応画像の 400 に `code: "unsupported_image"`、413 に `limit` (バイト数)。状態コードと `error` の英語文言は変えていない。UI が英語文言の一致で判定すると、文言を直したときに黙って生の英語表示へ戻るため、こちらを選んだ。上限の 20MB も UI に書かず `limit` から出す。
- `describeUploadFailure` が 2 つの拒否をファイル名付きの文に置き換える。それ以外の失敗は従来どおりサーバーの文言を出す。ApiError を instanceof でなく形で見るのは、node --test が拡張子なし import の client.ts を読み込めないため。

## 確認

- `pnpm check:client` / `pnpm test:client` (新規 5 件を含む 14 件) / `go test ./...` / `go vet ./...` / `pnpm build:client` すべて通過。Go の upload_test に code と limit の検査を追加。
- wails dev をスクラッチの DATA_DIR で起動し、ドロップイベントを発火して確認した。
  - プロジェクト画面: SVG・HEIC を含む 9 件のドロップで「icon.svg、photo.heic は取り込めません。受け付ける種類: …画像 (PNG・JPEG・GIF・WebP)…」と表示。残り 7 件 (png/jpg/gif/webp/md/markdown/txt) は保存された (AC#1, #2, #4)。
  - 中身が HEIC の disguised.png → 「disguised.png を保存できませんでした。中身が PNG・JPEG・GIF・WebP のどれでもありません。」、21MB の huge.txt → 「huge.txt を保存できませんでした。上限の 20MB を超えています。」(AC#3)。
  - 画像追加ダイアログ: SVG は送信前に拒否、accept は 4 形式。disguised.png と 21MB の huge.png で同じ日本語の理由を表示し、正しい PNG は保存できた。
  - チャット画面のドキュメント追加モーダル: SVG の送信前拒否、disguised.webp の拒否理由、PNG・md の保存を確認。
- en 辞書の文言は型検査のみで、画面では見ていない。

## 残したもの

- 一括追加は今も 1 件の拒否で残りの取り込みを止める (Description 背景の 3 点目の後半)。方針と AC に含まれないため変えていない。送信前判定で 400 はほぼ起きなくなったが、20MB 超のファイルが途中にあると残りは取り込まれない。
- 同名ドキュメントを置き換えるとき、既存を削除してから新規作成するため、新規作成が拒否されると既存は消えたままになる (今回の変更前からの動作)。
<!-- SECTION:NOTES:END -->
