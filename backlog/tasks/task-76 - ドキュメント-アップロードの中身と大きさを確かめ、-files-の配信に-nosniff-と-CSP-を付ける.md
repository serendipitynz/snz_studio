---
id: TASK-76
title: 'ドキュメント: アップロードの中身と大きさを確かめ、/files の配信に nosniff と CSP を付ける'
status: In Review
assignee: []
created_date: '2026-09-28 20:23'
updated_date: '2026-10-02 08:54'
labels:
  - security
dependencies: []
references:
  - internal/httpapi/handlers.go
  - internal/httpapi/server.go
  - internal/httpapi/imagedescription.go
priority: low
type: bug
ordinal: 76000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F10 (Low)。2026-09-29 に `756d32b` で未修正を確認した。

- `handleCreateDocument` (`handlers.go`) は `ParseMultipartForm` の前に `MaxBytesReader` をかけていない (`maxMultipartMemory` の 32MB はメモリに置く量で、超えた分は一時ファイルに書かれる)。テキスト文書も `io.ReadAll` で上限なく読む。
- 画像の種別でも中身を検証せず、拡張子 (`filepath.Ext(header.Filename)`) とクライアントが申告した Content-Type をそのまま保存する。
- `/files` は `http.ServeFile` で拡張子から Content-Type を決めるため、`.html` / `.svg` はそのまま HTML / SVG として配信され、`X-Content-Type-Options: nosniff` も付かない。
- 影響: 今の UI は `<img>` でしか表示しないので、HTML / SVG の中のスクリプトは実行されない。ただし API のオリジン (`127.0.0.1:<port>`) で HTML を直接開かせる経路ができると、URL の `?t=` から API トークン (起動ごとに生成され `/api` と `/files` の全リクエストで照合される 256bit のランダム値) を読めるので、API 全体を操作できる (潜在)。サイズの上限が無いことは、自分のディスク・メモリを使い切るだけに留まる。

## 方針

- `http.DetectContentType` で `image/png` / `image/jpeg` / `image/gif` / `image/webp` に限り、保存する拡張子はサーバーが中身から決める。
- `/files` の応答に `X-Content-Type-Options: nosniff` と `Content-Security-Policy: default-src 'none'` を付け、画像以外は `Content-Disposition: attachment` にする。
- `MaxBytesReader` で上限 (例: 20MB) を設ける。画像の説明生成 (`imagedescription.go`) の上限と揃える。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 画像として送られた .html / .svg や、画像でない中身は 400 で拒否され、保存されない。テストがある
- [x] #2 保存する拡張子と MIME 型は、サーバーが中身から決める
- [x] #3 上限を超えるアップロード (画像・テキストとも) が 413 で拒否される。上限値とその理由が記録されている
- [x] #4 /files の応答に nosniff と CSP が付く。テストがある
- [ ] #5 既存の画像ドキュメント (png / jpeg / gif / webp) の追加と表示が変わらない
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. handlers.go handleCreateDocument: Content-Length 事前判定 + MaxBytesReader で multipart 全体を 20MB(+枠分) に制限し、超過は 413。上限値と理由は定数のコメントに記録
2. 画像: 先頭 512B を http.DetectContentType で判定し png/jpeg/gif/webp 以外は 400。拡張子と mime_type は判定結果から決め、クライアントの Content-Type と filename の拡張子は使わない
3. fileHandler: X-Content-Type-Options: nosniff と CSP default-src 'none' を全応答に付け、許可した画像拡張子以外は Content-Disposition: attachment
4. service/imagedescription.go の「保存は無制限」コメントを更新
5. handlers_test / auth_test にテスト追加 (html/svg/非画像 400、4 形式の追加と配信、413 画像・テキスト、/files ヘッダ)。go test ./... と go vet
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装
- `internal/httpapi/upload.go` に上限・形式判定・`/files` ヘッダをまとめ、`handleCreateDocument` と `fileHandler` から呼ぶ。
- 画像: 先頭 512B を `http.DetectContentType` で判定し、png / jpeg / gif / webp 以外は 400 (`image must be PNG, JPEG, GIF or WebP`)。保存拡張子 (.png / .jpg / .gif / .webp) と mime_type は判定結果から決め、ファイル名の拡張子とクライアント申告の Content-Type は使わない。判定は保存前なので、拒否時はファイルも行も残らない。
- 上限: リクエスト本文に 20MB + 1MB (フォーム枠と note / derivedText などのテキスト欄の分) を `MaxBytesReader` でかけ、Content-Length が上限を超えると申告された場合は読む前に 413。画像・テキスト共通の 1 つの値にした。
- 20MB の理由 (定数 `maxDocumentUploadBytes` のコメントに記録): 保存するのは元画像で (UI が縮小するのは説明生成に送る複製だけ)、スマホやカメラの写真を保存できる大きさが要る。テキストとしては文脈に使える量を大きく超える。上限はセキュリティ境界ではなく、巨大ファイルを誤ってドロップしたときにメモリへの読み込みや保存が起きないようにするための防護。
- 説明生成の上限 (10MB) とは値ではなく仕組み (Content-Length の事前判定 → MaxBytesReader → 413) を揃えた。10MB は縮小後の画像を base64 で JSON に載せる経路の上限で、元画像の保存上限とは別物のため。`service/imagedescription.go` にあった「保存は無制限」のコメントは書き直した。
- `/files`: 全応答に `X-Content-Type-Options: nosniff` と `Content-Security-Policy: default-src 'none'` を付ける。拡張子が .png / .jpg / .jpeg / .gif / .webp 以外 (今回の判定より前に保存された .html / .svg など) は `Content-Disposition: attachment`。.jpeg は以前の保存分のために許可している。

## 検証
- `go test ./... -count=1` 全パス、`go vet ./...` 指摘なし。
- AC1: `TestCreateImageDocumentRejectsNonImages` — .html / .svg / 画像名のテキスト / HEIC / 空ファイルが 400。uploads ディレクトリが空で、文書も作られていないことを確認。
- AC2: `TestCreateImageDocumentTakesFormatFromContent` — ファイル名 `pic.html`・Content-Type `text/html` で送っても、4 形式それぞれの拡張子と mime_type が中身どおりになる。
- AC3: `TestCreateDocumentRefusesOversizeUploads` — 画像・テキストの両方について、Content-Length を申告した場合としない場合 (MaxBytesReader の経路) の両方で 413。保存物も文書も残らない。
- AC4: `TestCreateImageDocumentTakesFormatFromContent` (画像は inline 配信で nosniff と CSP 付き)、`TestFilesServesNonImagesAsAttachments` (.html / .svg / .txt は attachment で nosniff と CSP 付き)。
- AC5 は未チェック: テストで確かめたのは、4 形式の追加と `/files` からの配信 (バイト列一致・Content-Type・inline) まで。実アプリの WebView で `<img>` 表示が変わらないことは見ていないので、ユーザーの確認待ち。

## 挙動の変化 (UI 側は未変更)
- 画像の入力欄は今も `image/*` を受け付けるため、SVG / HEIC / BMP などは送信したあとにサーバーの 400 で失敗する (以前は保存できていた)。UI の受付形式を 4 形式に絞る対応は別タスクの候補。
<!-- SECTION:NOTES:END -->
