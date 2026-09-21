---
id: TASK-29
title: '多人数会話: 発言ごとにコピーボタンを付ける'
status: In Review
assignee: []
created_date: '2026-09-21 09:05'
updated_date: '2026-09-21 09:28'
labels: []
milestone: m-1
dependencies: []
references:
  - frontend/src/pages/MultiAgentChatPage.tsx
  - frontend/src/pages/ChatPage.tsx
ordinal: 29000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

単独アシスタントのチャット画面 (`ChatPage.tsx`) には発言ごとに 24px のアイコンボタンでコピー (とレビュー) があるが、多人数会話画面 (`MultiAgentChatPage.tsx`) の発言には無い。TASK-19 で「メモリに保存」を同じ形のアイコンボタンとして発言の右下 (時刻の左) に置いたので、コピーもその列に並べる。

## 方針

- `ChatPage.tsx` の `handleCopyMessage` / `CopyIcon` と同じ挙動・見え方 (クリップボードへ本文をコピー、押した直後だけ title を「コピーしました」に変え、失敗時はエラー表示)。
- 参加者の発言・人間の介入発言のどちらにも付ける。
- 配置は「メモリに保存」ボタンの左、時刻の左。
- 単独チャットとの重複実装は、必要なら小さな共通コンポーネントに寄せてよいが、まず動く最小で始める。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 多人数会話画面の各発言 (参加者・人間) にコピーボタンがあり、押すと本文がクリップボードにコピーされる
- [x] #2 ボタンの大きさ・見え方が単独チャットのコピーボタンと揃っている (24px のアイコンボタン、メモリ保存ボタンと同じ列)
- [x] #3 コピー直後のフィードバック (title の切り替え) と失敗時のエラー表示が単独チャットと同じ
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. frontend/src/components/CopyMessageButton.tsx を追加する。24px の素の IconButton + CopyIcon で、クリップボード書き込み・押下直後 1400ms だけ title を「コピーしました」に切り替える copied 状態を内部に持ち、失敗は ExportChatButton と同じく onError コールバックでページのエラーバナーに渡す。CopyIcon も同ファイルから export する。
2. MultiAgentChatPage の各発言 (参加者・人間の両方) のアクション列で、メモリ保存ボタンの左に CopyMessageButton を置く。
3. ChatPage のインラインのコピーボタン (copiedMessageId state / handleCopyMessage / ローカル CopyIcon) を共通コンポーネントに差し替える。レビュー結果のコピーボタン 2 箇所は挙動が違う (copied フィードバックなし) ので IconButton + import した CopyIcon のまま残す。
4. i18n キーは既存の chat.copy / chat.copied / chat.copyMessage / chat.copyMessageError をそのまま使う (単独チャットと文言を揃えるため新規キーは足さない)。
5. tsc + build + go test で検証する。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装

`frontend/src/components/CopyMessageButton.tsx` を新設し、単独チャットと多人数会話の両方から使う形にした。

- 最小実装として MultiAgentChatPage 側にコピー処理を書き足すこともできたが、AC#2 / AC#3 は「単独チャットと揃っていること」を要求している。コピペで揃えると以後どちらか一方を触ったときに静かにずれるので、同一コンポーネントを共有して構造的に揃うようにした。ChatPage 側の `copiedMessageId` state・`handleCopyMessage`・ローカルの `CopyIcon` は削除し、コンポーネント内の state に寄せた (差分は ChatPage -45 行 / MultiAgent +2 行)。
- 失敗時の扱いは既存の `ExportChatButton` と同じく `onError` コールバックでページのエラーバナー (`setError`) に渡す形にした。コンポーネント内でエラーを描画するとアクション列の中に出てしまうため。
- `copied` を戻す 1400ms のタイマーは unmount 時に clearTimeout する。元の ChatPage 実装はページ単位の state だったので解除不要だったが、発言ごとにマウント/アンマウントするコンポーネントに移したことで、消えた発言のタイマーが残ると unmount 後の setState になる。
- i18n キーは既存の `chat.copy` / `chat.copied` / `chat.copyMessage` / `chat.copyMessageError` をそのまま流用した。文言を揃えるのが AC なので `multiAgent.*` に別キーを作る理由がない。
- コピーボタンは発言の role で分岐しない位置 (`state.messages.map` 共通のアクション列) に置いたので、参加者発言・人間の介入発言の両方に付く。配置はメモリ保存ボタンの左、時刻の左。

## 検証

- `pnpm check:client` (tsc --noEmit) 通過。
- `pnpm build:client` (vite build) 通過。
- `go build ./...` / `go test ./...` 通過 (今回は Go 側を触っていないが回帰確認として実施)。
- AC#1 のボタン存在は描画パスを読んで確認した (role で分岐しない単一の map 上にある)。コピー動作そのもの・AC#2 の見え方・AC#3 のフィードバックは、単独チャットで既に動いている実装をそのまま共有コンポーネント化したもので、両画面が同一コードを通ることをもって満たしているとみなした。
- 未実施: Wails の WebView で実際にボタンを押してクリップボードとフィードバックを目視する確認。README の通りブラウザ単体 (localhost:5173) での開発が廃止されており、この環境から実アプリのクリックを駆動できないため。オーナー側での目視確認を残す。
<!-- SECTION:NOTES:END -->
