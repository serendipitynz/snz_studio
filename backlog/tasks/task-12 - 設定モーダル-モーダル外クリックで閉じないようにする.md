---
id: TASK-12
title: '設定モーダル: モーダル外クリックで閉じないようにする'
status: To Do
assignee: []
created_date: '2026-09-19 22:29'
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
- [ ] #1 設定モーダルは、モーダル外をクリックしても閉じず、「閉じる」ボタンでだけ閉じる
- [ ] #2 設定用テキストフィールドの範囲選択でドラッグがモーダル外で終わっても閉じない (実機で確認)
- [ ] #3 他のモーダルについて、同じ挙動に統一したか、例外とした場合はその根拠がタスクに記録されている
<!-- AC:END -->
