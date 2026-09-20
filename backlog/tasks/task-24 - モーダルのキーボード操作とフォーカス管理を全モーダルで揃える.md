---
id: TASK-24
title: モーダルのキーボード操作とフォーカス管理を全モーダルで揃える
status: To Do
assignee: []
created_date: '2026-09-20 02:10'
labels: []
milestone: m-1
dependencies: []
references:
  - frontend/src/styles/ui.tsx
  - frontend/src/components/ConfirmDialog.tsx
  - frontend/src/components/SettingsModal.tsx
  - frontend/src/pages/ProjectDetailPage.tsx
  - frontend/src/pages/ChatPage.tsx
ordinal: 24000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
TASK-16 で追加した確認ダイアログ (`ConfirmDialog.tsx`) だけが Escape でのキャンセルと
`role="dialog"` / `aria-modal` / `aria-labelledby` を持ち、他の 8 モーダルは持っていない。
PR #12 のレビュー (Codex, [P3]) ではフォーカストラップとフォーカス復帰も指摘されたが、
ダイアログ 1 つだけに入れるとモーダル層が不揃いになるため見送り、ここに分離した。

対象は `ModalOverlay` / `ModalCard` を使う 8 モーダル:

- `SettingsModal.tsx:219` 設定
- `ProjectDetailPage.tsx` の 4 つ — ドキュメント表示 (869)、タイトル編集 (944)、
  システムプロンプト編集 (976)、メモリ編集 (1008)
- `ChatPage.tsx` の 3 つ — タイトル編集 (900)、ドキュメント追加 (941)、レビュー表示 (987)

現状これらに欠けているもの:

- Escape で閉じられない (アプリ全体でキーダウンを拾っている箇所が 1 つもない)
- 支援技術からはダイアログではなく末尾の通常コンテンツに見える
- 開いている間も背後の要素へ Tab で抜けられ、閉じた後に元の要素へフォーカスが戻らない

対応方針: 個々のモーダルに同じ処理を 8 回書くのではなく、`ModalOverlay` / `ModalCard` を
使う側が挙動を受け取れる形にまとめる (共通コンポーネントか hook)。どちらにするかは
実装時に判断する。

TASK-12 (モーダル外クリックで閉じないようにする) が同じモーダル層の同じ行を触るので、
まとめて着手すると 8 ファイル分の往復が一度で済む。先に TASK-12 を済ませてからこちらに
入るのが素直だが、依存関係としては縛っていない。

表示専用モーダル (ドキュメント表示・レビュー表示) を例外にするかは TASK-12 の判断と
揃える。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 ModalOverlay / ModalCard を使う 8 モーダルすべてで Escape により閉じる (入力中の変更を破棄してよいかはモーダルごとに判断し、判断の根拠をタスクに記録する)
- [ ] #2 8 モーダルすべてがダイアログとして支援技術に認識される (role="dialog" / aria-modal / ラベル付け)
- [ ] #3 モーダルが開いている間、Tab フォーカスがモーダル内に留まる
- [ ] #4 モーダルを閉じたとき、フォーカスが開く前の要素へ戻る
- [ ] #5 同じ処理が 8 箇所へコピーされておらず、ModalOverlay / ModalCard 側の 1 箇所にまとまっている
<!-- AC:END -->
