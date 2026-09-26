---
id: TASK-61
title: '共通デザイン: モーダルの見出し域を残し、本体域だけをスクロールさせる'
status: To Do
assignee: []
created_date: '2026-09-26 21:59'
labels:
  - design
dependencies: []
ordinal: 61000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
snz-design TASK-20 の確認 (2026-09-27) で分かったこと。共通の `Dialog` (`components/Dialog.tsx`) が包む面 `ModalCard` (`styles/ui.tsx`) は、面そのものに `max-height: calc(100vh - 48px); overflow: auto` を持つ。どのモーダルも題名と閉じるボタンの行を面の中の先頭に置いているので、面の高さが画面を超えると、題名と閉じるボタンも中身と一緒に流れて見えなくなる。閉じるには上へ戻るか Escape を押すしかない。背後の画面は動かない。

snz-design doc-9 §6.6 (モーダル) は、面を見出し域・本体域・操作域に分け、「高さが画面を超えたら本体域だけがスクロールし、見出し域と操作域は残る」と求めている。snz-design は兄弟ディレクトリ `../snz-design` にある。

オーナー判断 (2026-09-27): `Dialog` を見出し域・本体域・操作域に分け、本体域だけをスクロールさせる。何を見出し域に置き、何を本体域に置き、操作域をどう持つかはアプリ側で決めてよい。今は全モーダルが部品を本体域に置く形で揃っている。閉じるボタンが流れるのは使い勝手が悪いので、見出し域を設ける。

`Dialog` を使う箇所 (2026-09-27 時点): 設定 (SettingsModal)、確認ダイアログ (ConfirmDialog)、プロジェクト作成 (CreateProjectDialog)、画像文書の追加 (ImageDocumentDialog)、チャットの題名・ドキュメント・編集レビュー (ChatPage)、発言のメモリ保存と結論 (MultiAgentChatPage)、ドキュメント詳細・題名・システムプロンプト・プロジェクトメモリ (ProjectDetailPage)。高さが出やすいのは設定・プロジェクトメモリ・ドキュメント・編集レビューのモーダル。

snz-design の適用記録 (doc-17) は、このタスクを §5 (未適用) の引き受け手として記録している。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 `Dialog` を使うすべてのモーダルで、面の高さが画面を超えたとき、題名と閉じるボタンが見出し域として残り、本体域だけがスクロールする (snz-design doc-9 §6.6 の「狭い画面」)
- [ ] #2 モーダルごとに見出し域・本体域・操作域に何を置いたかを Implementation Notes に一覧で記録している。操作域を本体域の末尾に置くモーダル (区画ごとに保存する設定モーダルなど) は、そう置いた理由を書いている
- [ ] #3 本体域の中で焦点を受けた部品が、見出し域・操作域に完全に隠れず、焦点の枠がスクロールする箱に切られない (snz-design doc-5 の 2.4.11、doc-16 §6.3)
- [ ] #4 焦点の閉じ込め・Escape・焦点の返却・破棄前確認の振る舞いが今までどおりである。360px 幅と低い画面の高さで、横スクロールが出ないことを確かめている
- [ ] #5 変えた箇所の比を4配色で測り、測定環境 (snz-design doc-5 §5.3) とともに Implementation Notes に記録している。実窓 (WKWebView) の目視はオーナーの確認を記録する
- [ ] #6 `pnpm check:client`・`pnpm build:client` が通る
<!-- AC:END -->
