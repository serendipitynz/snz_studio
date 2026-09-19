---
id: TASK-10
title: '多人数会話: 会話内容を markdown としてエクスポートする'
status: To Do
assignee: []
created_date: '2026-09-19 12:32'
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
- [ ] #1 多人数会話の画面からエクスポートを実行すると、その会話の内容が markdown ファイルとして保存できる
- [ ] #2 出力に、場面設定・ターン進行ルール・参加者一覧 (表示名とモデル名) が含まれる
- [ ] #3 各発言に話者の表示名が付き、除籍済みの参加者の発言も誰の発言か分かる
- [ ] #4 ユーザーの発言と参加者の発言が出力上で区別できる
<!-- AC:END -->
