---
id: TASK-12
title: '設定モーダル: モーダル外クリックで閉じないようにする'
status: Done
assignee: []
created_date: '2026-09-19 22:29'
updated_date: '2026-09-20 09:10'
labels: []
milestone: m-1
dependencies: []
references:
  - frontend/src/components/SettingsModal.tsx
  - frontend/src/styles/ui.tsx
  - frontend/src/pages/ProjectDetailPage.tsx
  - frontend/src/pages/ChatPage.tsx
ordinal: 12000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
設定モーダルは `ModalOverlay` に `onClick={onClose}` を渡しているため (`SettingsModal.tsx:219`)、
モーダル外をクリックすると閉じる。設定のテキストフィールドで範囲選択した際にドラッグが
カードの外で終わるなど、意図せず閉じてしまうことがある。閉じる操作は「閉じる」ボタンだけに
限定したい。

同じ overlay クリックで閉じるパターンは ProjectDetailPage の 4 モーダル (ドキュメント表示・
タイトル編集・システムプロンプト編集・メモリ編集) と ChatPage の 3 モーダルにもある。
入力欄を持つモーダルは同じ事故が起きるので、設定モーダルだけ直すのではなく、
overlay クリックで閉じる挙動を全モーダルから外して統一する方針を第一候補とする。
表示専用のモーダル (ドキュメント表示) を例外にするかは実装時に判断し、結果を記録する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 設定モーダルは、モーダル外をクリックしても閉じず、「閉じる」ボタンでだけ閉じる
- [x] #2 設定用テキストフィールドの範囲選択でドラッグがモーダル外で終わっても閉じない (実機で確認)
- [x] #3 他のモーダルについて、同じ挙動に統一したか、例外とした場合はその根拠がタスクに記録されている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. ModalOverlay を使う全モーダル (SettingsModal 1, ProjectDetailPage 4, ChatPage 3) を洗い出す
2. 各 ModalOverlay の onClick (外側クリックで閉じる) を削除し、あわせて不要になった ModalCard の stopPropagation も外す
3. 表示専用のドキュメント表示モーダルも例外にせず統一する (本文のドラッグ選択で閉じる事故は同じ、かつモーダルごとに閉じ方が違う状態を避ける)
4. ConfirmDialog は元から overlay クリックで閉じないため変更なし
5. tsc/build と lint を実行し、実機 (wails dev) でドラッグ選択が外で終わっても閉じないことを確認
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
ModalOverlay を使う 8 モーダル (SettingsModal 1 / ProjectDetailPage 4: ドキュメント表示・タイトル編集・システムプロンプト編集・メモリ編集 / ChatPage 3: タイトル編集・ドキュメント追加・編集レビュー) から overlay の onClick を削除し、閉じる操作を「閉じる」ボタンだけに統一した。overlay ハンドラがなくなり不要になった ModalCard 側の `onClick={(event) => event.stopPropagation()}` も同時に外している (残すと、閉じない overlay を打ち消すためのガードに見えて誤読される)。ConfirmDialog は元から overlay クリックで閉じないため変更なし。

AC #3 の判断: 表示専用のドキュメント表示モーダルも例外にせず統一した。本文をドラッグで範囲選択してカード外でドロップする事故は入力欄と同じように起き、かつモーダルごとに閉じ方が違うほうがコストが高いため。overlay クリックの代替となる閉じ方 (Esc キー) はこのタスクの範囲外で、TASK-24 (モーダルのキーボード操作とフォーカス管理の統一) が受け持つ。

検証: `pnpm check:client` (tsc --noEmit) と `pnpm build:client` がともに成功。AC #1 / #2 は実機 (wails dev) でのドラッグ選択確認が必要なため未チェックのまま残している。

実機 (wails dev) で確認: 設定モーダルはモーダル外クリックで閉じず、テキストフィールドの範囲選択でドラッグがモーダル外で終わっても閉じない (AC #1 / #2)。
<!-- SECTION:NOTES:END -->
