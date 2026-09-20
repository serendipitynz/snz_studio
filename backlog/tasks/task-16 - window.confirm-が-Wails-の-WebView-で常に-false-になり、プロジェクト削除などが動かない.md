---
id: TASK-16
title: window.confirm が Wails の WebView で常に false になり、プロジェクト削除などが動かない
status: In Review
assignee: []
created_date: '2026-09-19 22:29'
updated_date: '2026-09-20 01:06'
labels: []
milestone: m-1
dependencies: []
references:
  - frontend/src/pages/ProjectDetailPage.tsx
  - frontend/src/components/ParticipantPanel.tsx
  - frontend/src/pages/ChatPage.tsx
  - frontend/src/styles/ui.tsx
  - app.go
type: bug
ordinal: 16000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
プロジェクト画面の「プロジェクトを削除」ボタンを押しても何も起きない。

原因: `handleDeleteProject` (`ProjectDetailPage.tsx:328`) は `window.confirm` で確認してから
`api.deleteProject` を呼ぶが、Wails v2.16 の macOS 実装 (`internal/frontend/desktop/darwin/WailsContext`)
は `WKUIDelegate` を宣言しているだけで confirm パネルのデリゲートメソッドを実装していない。
WKWebView はこの場合ダイアログを出さずに `confirm` を false で返すため、ハンドラは黙って return する。
バックエンドの `DELETE /api/projects/{projectId}` と repository の `DeleteProject` は正常
(テストあり)。

同じ理由で次の箇所も常に「キャンセル」側に落ちている:

- 参加者の削除確認 (`ParticipantPanel.tsx:104`)
- ドキュメント追加時のファイル上書き確認 (`ProjectDetailPage.tsx:413`, `ChatPage.tsx:448`)。
  こちらは常に「上書きしない」になる。

対応方針: 既存の `ModalOverlay` / `ModalCard` を使ったアプリ内確認ダイアログを 1 つ作り、
4 箇所の `window.confirm` を置き換える。Wails ランタイムの `MessageDialog` を Go 側でバインドする案
(app.go の起動失敗ダイアログで既に使用) もあるが、ブラウザでの開発時に動かなくなるため採らない。
`window.alert` / `window.prompt` も同様に機能しないので、今後も使わない旨をコード上か文書に残す。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 プロジェクト画面の「プロジェクトを削除」で確認ダイアログが表示され、確認するとプロジェクトが削除されて一覧へ戻る (Wails アプリ実機で確認)
- [ ] #2 参加者の削除、ドキュメント上書き確認も同じダイアログで動作し、確認・キャンセルがそれぞれ期待どおりに効く
- [x] #3 frontend/src に window.confirm / window.alert / window.prompt の呼び出しが残っていない
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. frontend/src/components/ConfirmDialog.tsx を追加する: ConfirmProvider + useConfirm() の Promise ベース API。要求を 1 件ずつ state に保持し、ModalOverlay/ModalCard で表示、確認/キャンセルで resolve(true/false)。overlay クリックでは閉じない (TASK-12 の方針と衝突させない)。
2. main.tsx で ThemeController の内側に ConfirmProvider をマウントする (theme と t が必要なため)。
3. 4 箇所の window.confirm を await confirm(...) に置換する: ProjectDetailPage.handleDeleteProject / handleUploadFiles、ChatPage.handleUploadFiles、ParticipantPanel.handleRemoveParticipant。アップロードは既に async ループなので await をそのまま差し込める。
4. i18n に確認ダイアログ用のラベル (削除/上書きの肯定ボタン) を追加する。common.cancel は既存を使う。
5. window.confirm/alert/prompt を今後使わない根拠を ConfirmDialog.tsx のコメントに、ルールを CLAUDE.md と AGENTS.ja.md の制約に追記する。
6. 検証: pnpm check:client と pnpm build:client、frontend/src への grep。AC#1/#2 の実機確認はユーザーに依頼する。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
`frontend/src/components/ConfirmDialog.tsx` に `ConfirmProvider` + `useConfirm()` を追加し、
4 箇所の `window.confirm` を `await confirm(...)` に置き換えた。`main.tsx` で `ThemeController` の
内側 (テーマと `t` が要るため) に Provider をマウントしている。

判断したこと:

- API は Promise ベースの hook にした。ドキュメント上書き確認はファイルごとの async ループの
  途中で聞く必要があり、props で開閉する素の modal にすると両ページのアップロード処理を
  分解し直すことになるため。
- overlay クリックでは閉じない。確認/キャンセルのボタンが明示的にあるので不要で、
  overlay クリックの扱いは TASK-12 の担当範囲に残した。
- 初期フォーカスはキャンセル側。現在の呼び出しは 4 箇所とも破壊的操作 (削除・上書き) で、
  クリック直後の誤 Enter で確定させないため。
- 表示中に 2 件目の要求が来たら 1 件目を false で決着させる。ドロップゾーンは busy 中も
  受け付けるので 2 件目は実際に起こりうる経路で、放置すると 1 件目の Promise が未決着のまま
  アップロードループが busy フラグを立てたまま止まる。
- 計画では確認ダイアログ用の i18n キーを足す予定だったが、既存の `common.ok` / `common.cancel`
  がアプリ内の他の削除確認と同じラベルなので追加しなかった。
- 今後 `window.confirm` / `alert` / `prompt` を使わない根拠は ConfirmDialog.tsx の冒頭コメント
  (WKUIDelegate の実装欠落と、Wails ランタイムの MessageDialog を採らない理由) に、
  ルール自体は AGENTS.md / AGENTS.ja.md の制約一覧に書いた。

検証:

- AC#3: `grep -rn "window\.(confirm|alert|prompt)" frontend/src` の結果が
  ConfirmDialog.tsx の説明コメント 1 行のみ (呼び出しはゼロ)。
- `pnpm check:client` (tsc --noEmit) と `pnpm build:client` が成功。
- `go build ./...` と `go test ./...` は全パッケージ ok (Go 側は未変更だが退行がないことの確認)。
- AC#1 / AC#2 は Wails アプリ実機でのクリック確認が要るため、このセッションでは未検証。
  フロントエンドに自動テスト基盤がなく (テストランナー未導入)、実機操作もできないため。
  マージ前に実機で確認をお願いしたい。
<!-- SECTION:NOTES:END -->
