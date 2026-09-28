---
id: TASK-76
title: 'ドキュメント: アップロードの中身と大きさを確かめ、/files の配信に nosniff と CSP を付ける'
status: To Do
assignee: []
created_date: '2026-09-28 20:23'
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
- [ ] #1 画像として送られた .html / .svg や、画像でない中身は 400 で拒否され、保存されない。テストがある
- [ ] #2 保存する拡張子と MIME 型は、サーバーが中身から決める
- [ ] #3 上限を超えるアップロード (画像・テキストとも) が 413 で拒否される。上限値とその理由が記録されている
- [ ] #4 /files の応答に nosniff と CSP が付く。テストがある
- [ ] #5 既存の画像ドキュメント (png / jpeg / gif / webp) の追加と表示が変わらない
<!-- AC:END -->
