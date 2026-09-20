---
id: TASK-16
title: window.confirm が Wails の WebView で常に false になり、プロジェクト削除などが動かない
status: To Do
assignee: []
created_date: '2026-09-19 22:29'
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
- [ ] #3 frontend/src に window.confirm / window.alert / window.prompt の呼び出しが残っていない
<!-- AC:END -->
