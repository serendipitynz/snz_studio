---
id: TASK-10
title: '多人数会話: 会話内容を markdown としてエクスポートする'
status: Done
assignee: []
created_date: '2026-09-19 12:32'
updated_date: '2026-09-20 00:59'
labels: []
milestone: m-1
dependencies: []
references:
  - docs/multi-agent-chat-design.md
  - frontend/src/pages/MultiAgentChatPage.tsx
  - internal/httpapi/server.go
ordinal: 10000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
多人数会話の内容を markdown ファイルとして取り出せるようにする。いまは会話を見返す手段が
観戦ビュー (設計書 §6) のスクロールだけで、ログとして手元に残したり、外部のエディタで
整形・共有したりできない。

出力に含めるもの:

- 見出し: chat のタイトル、プロジェクト名、出力日時
- 場面設定 (`scenePrompt`) とターン進行ルール (`turnRule`)
- 編成: 参加者の表示名とモデル名。除籍済みの参加者も、過去の発言の帰属として残っている
  ものは併記する (設計書 §3)
- 発言: 話者の表示名と本文。ユーザーの発言と参加者の発言が区別できる形にする

実装の当たり:

- 生成はサーバー側に置く。messages と participants は repository が持っていて、話者名の
  解決規則も `MultiAgentChatPage.tsx:290` 付近のものをそのまま書き下せる。ルートは
  `GET /api/chats/{chatId}/export/markdown` あたりが既存の形に合う。
- 観戦ビューのヘッダ (再読み込みボタンの並び) にエクスポートの導線を足して呼ぶ。

実装時に決めること:

- ファイルの落とし先。Wails の SaveFileDialog を `app.go` にバインドして保存先を選ばせるか、
  WebView 内で Blob からダウンロードさせるか。現状の Wails バインディングは
  `GetApiBase` / `GetApiToken` の 2 つだけで、保存ダイアログは無い
  (`frontend/src/wailsjs/go/main/App.d.ts`)。
- 単一アシスタントの chat も同じルートでエクスポートできるようにするか、多人数会話だけに
  絞るか。ルートを chat 単位にするなら前者が自然だが、出力の見出しに編成や場面設定が
  無い分だけ書式が分岐する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 多人数会話の画面からエクスポートを実行すると、その会話の内容が markdown ファイルとして保存できる
- [x] #2 出力に、場面設定・ターン進行ルール・参加者一覧 (表示名とモデル名) が含まれる
- [x] #3 各発言に話者の表示名が付き、除籍済みの参加者の発言も誰の発言か分かる
- [x] #4 ユーザーの発言と参加者の発言が出力上で区別できる
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 生成はサーバー側。internal/service/export.go に BuildChatMarkdown(project, chat, participants, messages, now) を置き、turnengine.go の speakerLabels / speakerLabel (同パッケージ・未公開) をそのまま使って話者名を解決する。除籍済みは表示名に注記を付ける。
2. 出力対象は chat の kind を問わない。場面設定・ターン進行ルール・編成の 3 節は kind == multi_agent のときだけ出す。単一アシスタントは見出しと会話だけ。
3. internal/httpapi に GET /api/chats/{chatId}/export/markdown を追加。handleGetChat と同じ形で chat -> project -> participants.ListAll -> ListMessages を読み、text/markdown; charset=utf-8 で本文だけ返す。ファイル名はフロントが chat タイトルから作るので Content-Disposition は付けない。
4. 保存先はネイティブ保存ダイアログ。app.go に SaveMarkdown(suggestedName, content) を足して wailsruntime.SaveFileDialog + os.WriteFile。wailsjs のバインドは pnpm 経由の wails で再生成する。WebView 外 (束縛が無い環境) では Blob + <a download> に落とす。
5. フロントは api.exportChatMarkdown でテキストを取り、保存ヘルパを呼ぶ。MultiAgentChatPage と ChatPage のヘッダに再読み込みと並べてエクスポートボタンを置く。i18n キーは en/ja 両方に追加。
6. docs/multi-agent-chat-design.md §6 の観戦ビューにエクスポート導線を 1 行追記。
7. 検証は go test ./... / go vet ./... / pnpm check:client。AC #1 の実機保存だけは Wails アプリを起動して確認する。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 着手時に決めた 2 点

**保存先はネイティブの保存ダイアログ (app.go の SaveTextFile 束縛)、WebView の外では Blob ダウンロード。**
Blob + `<a download>` 単独を採らなかったのは、Wails v2.16 の macOS 実装
(`internal/frontend/desktop/darwin/WailsContext`) が `WKNavigationDelegate` を宣言しているだけで
ダウンロード関連のデリゲートメソッド (`didBecomeDownload`) も `WKDownloadDelegate` も実装しておらず
(module cache を grep して確認)、TASK-16 の `window.confirm` と同じ「宣言だけあってメソッドが無い →
WebKit の既定動作で無言に落ちる」形になるため。TASK-16 は Go 側ダイアログを「ブラウザ開発時に
動かなくなる」として退けているが、あちらは HTML モーダルという完全な代替があるのに対し、
ファイル保存には WebView 内で効く代替が無いので判断が逆になる。Blob 側は `window.go` が無い環境
(README が廃止した localhost 直開きなど) の保険として残した。束縛の失敗は Blob に落とさずそのまま
投げる — 書き込み失敗を「保存できなかった」と報告させるため。

**エクスポートは chat 単位で単独アシスタントの chat にも効かせた。**
chats は 1 テーブルで `kind` 列だけが違うため、ルートを多人数会話に絞ると単独アシスタントの会話だけ
取り出せないまま残る。場面設定・ターン進行ルール・編成の 3 節は `kind == multi_agent` のときだけ出す。

## 実装

- 生成は `internal/service/export.go` の `BuildChatMarkdown`。話者名の解決は turnengine の
  `speakerLabel` ではなく観戦ビュー (`MultiAgentChatPage.speakerLabel`) の規則に寄せた
  — 画面から離れて読む transcript には、どのモデルが話したかと除籍済みであることの両方が要る。
  発言のモデル名は message が記録した値を優先し、無いときだけ参加者の現在の設定に落とす
  (ターンが実際に走ったモデルと現在の設定は食い違いうる)。
- ルートは `GET /api/chats/{chatId}/export/markdown`。参加者は `ListAll` で読む (除籍済みの帰属、§3)。
  `Content-Disposition` は付けない — 唯一の呼び出し元は認証ヘッダ付きの fetch で、ファイル名は
  手元の chat タイトルから作るため、非 ASCII を RFC 5987 で通す読み手がいない。
- フロントは `api.exportChatMarkdown` (JSON ではないので `requestText` を新設) → `saveTextFile`。
  ボタンは `ExportChatButton` に切り出して観戦ビューと ChatPage の両方のヘッダに置いた。
  `window.go` の有無は `global.d.ts` に型を足して明示的に見る (try/catch だと本物の保存エラーを
  ホスト未対応と取り違える)。
- `docs/multi-agent-chat-design.md` §6 にエクスポートの節を追記。

## 検証

- `go test ./...` 全パッケージ green。`go vet ./...` 指摘なし。`pnpm check:client` / `pnpm build:client` 通過。
- AC #2 / #3 / #4 は `internal/service/export_test.go` と `internal/httpapi/export_test.go` で確認。
  後者は実際に chat を作り、参加者 Alice にターンを 1 回話させ、人間の割り込みを投稿し、Alice を
  除籍してから export を叩いて「Alice（除籍済み）」「ユーザー」の両方が transcript に残ることを見ている。
- AC #1 は未チェック。HTTP ルートが会話内容を返すところまでは上のテストで押さえたが、
  ネイティブ保存ダイアログを開いてファイルが書かれるところは実機で人が操作しないと確かめられない。
  Wails アプリを起動しての確認をお願いしたい。

AC #1 は wails dev で実機確認済み (オーナー、2026-09-20)。保存ダイアログが開き、markdown ファイルが書き出されることを確認。
<!-- SECTION:NOTES:END -->
