---
id: TASK-57
title: '共通デザイン: モーダルの主操作のボタンを右寄せに揃える'
status: To Do
assignee: []
created_date: '2026-09-25 23:06'
labels:
  - design
dependencies:
  - TASK-48
ordinal: 57000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
TASK-48 (#48) のオーナーの実窓の確認 (2026-09-26) で、モーダルの主操作のボタンの位置が、画面ごとに左寄せと右寄せで混ざっていることが分かった。オーナー判断 (同日): 右寄せを基本とする。共通仕様の側は snz-design の TASK-30 が doc-9 §6.6 に書く (操作域は右寄せ、取り消し → 実行の順で主操作が右端。説明用実例 `.ex-modal__actions` は既にこの形)。snz-design は兄弟ディレクトリ `../snz-design` にある。

TASK-48 の時点の位置 (コードで確かめた):
- 右寄せ: 設定モーダル (接続の保存)、確認ダイアログ、画像文書の追加 (ImageDocumentDialog)、多人数会話の発言のメモリ保存と結論 (MultiAgentChatPage)。
- 左寄せ: プロジェクトの題名とシステムプロンプトのモーダル、プロジェクトメモリのモーダルの「メモリを保存」、ドキュメント詳細のカテゴリ保存 (ProjectDetailPage)、チャットの題名のモーダル (ChatPage)。どれも実行のボタンを `<div>` で包むだけなので左に寄っている。

区画の中のフォームの実行ボタン (プロジェクトメモリの追加、ドキュメント詳細のカテゴリ保存) をどう置くかは、snz-design TASK-30 の doc-9 §6.6 の本文に従う。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 snz_studio のすべてのモーダルで、操作域の主操作のボタンが右寄せで、取り消しがあるときは取り消し → 実行の順に並ぶ (snz-design doc-9 §6.6)
- [ ] #2 モーダルの区画の中のフォームの実行ボタンが、snz-design doc-9 §6.6 の置き方に従う
- [ ] #3 直したモーダルで、キーボードの到達と焦点の順 (Tab の並び) が見た目の並びと食い違わないことを確かめ、Implementation Notes に記録している。実窓 (WKWebView) の目視はオーナーの確認を記録する
- [ ] #4 `pnpm check:client`・`pnpm build:client` が通る
<!-- AC:END -->
