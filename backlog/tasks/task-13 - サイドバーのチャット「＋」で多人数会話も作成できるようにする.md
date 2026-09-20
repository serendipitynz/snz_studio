---
id: TASK-13
title: サイドバーのチャット「＋」で多人数会話も作成できるようにする
status: Done
assignee: []
created_date: '2026-09-19 22:29'
updated_date: '2026-09-20 10:12'
labels: []
milestone: m-1
dependencies: []
references:
  - frontend/src/components/WorkspaceSidebar.tsx
  - frontend/src/pages/ProjectDetailPage.tsx
  - frontend/src/api/client.ts
  - docs/multi-agent-chat-design.md
ordinal: 13000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
左サイドバーのチャット欄の「＋」は `api.createChat(projectId, { title: "" })` を呼ぶだけで
(`WorkspaceSidebar.tsx:41`)、kind 未指定のため常に単独アシスタントのチャットになる。
多人数会話を作れるのはプロジェクト画面の作成フォームだけ。

対応方針: 「＋」を押したときに種別を選ぶ小メニュー (単独アシスタント / 多人数会話) を表示する。
多人数会話を選んだ場合は `kind: "multi_agent"` で空の名簿のチャットを作成し、そのチャット画面へ
遷移する。編成は既存の参加者パネルで行う。プリセットの適用は現状チャット作成時にしかできないため
(適用 API が無い)、この小メニューではプリセットを扱わない。作成後にプリセットを選べるようにする
件は別タスクとする。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 サイドバーの「＋」を押すと、単独アシスタントか多人数会話かを選ぶメニューが表示される
- [x] #2 多人数会話を選ぶと kind=multi_agent の空の名簿のチャットが作成され、そのチャット画面に遷移する
- [x] #3 単独アシスタントを選んだときの挙動は従来と同じ (即時作成して遷移)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. WorkspaceSidebar の「＋」を、押下で種別メニューを開くトリガーに変える (role=menu / menuitem, aria-haspopup, aria-expanded)。
2. メニュー項目は既存 i18n キー multiAgent.kindAssistant / multiAgent.kindMultiAgent を再利用。プリセットは扱わない。
3. 選択時に api.createChat(projectId, { title: "", kind }) を呼び、作成されたチャットへ遷移。単独アシスタントは従来と同じ即時作成・遷移 (kind 明示のみ)。
4. メニューは Escape・外側 pointerdown・項目選択で閉じる。開いたら先頭項目にフォーカスし、ArrowUp/Down で移動、閉じたらトリガーへフォーカスを戻す。
5. ポップオーバーの見た目は styles/ui のテーマトークン (surfaceCard / lineMedium / shadow) で Emotion styled として WorkspaceSidebar 内に置く。サーバ側は preset 無し multi_agent を既に受理するため変更不要。
6. 検証は pnpm check:client と pnpm build:client。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
サイドバーの「＋」を、種別を選ぶ小メニューのトリガーに変更 (frontend/src/components/WorkspaceSidebar.tsx)。

- 種別の選択肢は既存 i18n キー multiAgent.kindAssistant / multiAgent.kindMultiAgent を再利用した。
  サイドバー専用のラベルを新設すると、同じ概念の訳語がプロジェクト画面の作成フォームと二重管理になるため。
- 選択時に api.createChat(projectId, { title: "", kind }) を呼ぶ。単独アシスタントは従来 kind 未指定で
  サーバ既定の "assistant" になっていたところを明示送信に変えただけで、作成されるチャットは同一
  (internal/httpapi/handlers.go:369 で kind 空文字は ChatKindAssistant に既定化)。
- メニューは Escape・メニュー外 pointerdown・項目選択で閉じる。role="menu" を名乗る以上 Tab だけでは
  約束を果たせないため ArrowUp/Down での移動も実装し、開いたら先頭項目へ、Escape ではトリガーへ
  フォーカスを戻す。外側クリックでの close だけはフォーカスを戻さない (ポインタが行き先を決めているため)。
- プリセットは扱わない (task の方針どおり)。理由をコード内コメントに残した。
- サーバ側は変更なし。preset 無しの kind=multi_agent 作成は internal/httpapi/multiagent_test.go:46 で
  既にカバーされており、空の名簿で作成されることはこのテストが担保している。

検証:
- pnpm check:client / pnpm build:client いずれも成功。
- go test ./... 全パス (Go 側は無変更、回帰確認のため実行)。
- AC は 3 件とも未チェックのまま。フロントにテストハーネスが無く、README のとおりブラウザ直開きでの
  開発が廃止されている (非 GET が API に届かない) ため、メニュー表示・遷移の実挙動を自動で確認する
  手段がこのリポジトリに無い。pnpm dev のネイティブ WebView 上での目視確認が必要で、それは
  レビュー/マージ時にユーザーの手元で行う。

AC 3 件はユーザーが pnpm dev のネイティブ WebView 上で目視確認し、いずれも OK (2026-09-20)。自動検証手段が無い分をここで補った。
<!-- SECTION:NOTES:END -->
