---
id: TASK-29
title: '多人数会話: 発言ごとにコピーボタンを付ける'
status: To Do
assignee: []
created_date: '2026-09-21 09:05'
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
- [ ] #1 多人数会話画面の各発言 (参加者・人間) にコピーボタンがあり、押すと本文がクリップボードにコピーされる
- [ ] #2 ボタンの大きさ・見え方が単独チャットのコピーボタンと揃っている (24px のアイコンボタン、メモリ保存ボタンと同じ列)
- [ ] #3 コピー直後のフィードバック (title の切り替え) と失敗時のエラー表示が単独チャットと同じ
<!-- AC:END -->
