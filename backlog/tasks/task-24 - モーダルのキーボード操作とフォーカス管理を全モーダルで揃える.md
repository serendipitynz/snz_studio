---
id: TASK-24
title: モーダルのキーボード操作とフォーカス管理を全モーダルで揃える
status: In Review
assignee: []
created_date: '2026-09-20 02:10'
updated_date: '2026-09-22 10:40'
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
- [x] #1 ModalOverlay / ModalCard を使う 8 モーダルすべてで Escape により閉じる (入力中の変更を破棄してよいかはモーダルごとに判断し、判断の根拠をタスクに記録する)
- [x] #2 8 モーダルすべてがダイアログとして支援技術に認識される (role="dialog" / aria-modal / ラベル付け)
- [x] #3 モーダルが開いている間、Tab フォーカスがモーダル内に留まる
- [x] #4 モーダルを閉じたとき、フォーカスが開く前の要素へ戻る
- [x] #5 同じ処理が 8 箇所へコピーされておらず、ModalOverlay / ModalCard 側の 1 箇所にまとまっている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. components/Dialog.tsx に共通コンポーネント Dialog を作る。ModalOverlay + ModalCard を包み、role="dialog" / aria-modal / aria-labelledby (DialogTitle が useId で付ける id) を付与し、Escape で onClose・Tab のトラップ・開く前の要素へのフォーカス復帰を 1 箇所で持つ
2. 開いているダイアログをスタックで管理し、最上位だけが Escape / Tab を処理する (モーダル内から ConfirmDialog を出したとき 1 回の Escape で両方閉じないため)
3. IME 変換中の Escape (isComposing / keyCode 229) は無視する (変換取り消しのつもりでモーダルが閉じる事故を防ぐ。ChatPage の composer と同じ判定)
4. タスク記載の 8 モーダルを Dialog に置き換える。加えて、タスク作成後に入った MultiAgentChatPage の記憶保存ダイアログ (同じ処理を個別実装済み) と ConfirmDialog の Escape 処理も Dialog に寄せ、実装を 1 箇所にする (AC #5)
5. Escape の挙動はモーダルごとに「閉じるボタンと同じ操作」とし、閉じるボタンが無効な間 (記憶保存の送信中) は Escape も効かない
6. pnpm check:client / build:client、実機 (wails dev) で Escape・Tab・フォーカス復帰を確認
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
`components/Dialog.tsx` に共通コンポーネント `Dialog` / `DialogTitle` を追加し、ModalOverlay + ModalCard を直接組んでいた箇所をすべて置き換えた。hook ではなくコンポーネントにしたのは、role / aria-modal / aria-labelledby / tabIndex を付けるカード要素そのものを持つ必要があり、hook だと 9 箇所それぞれに ref と属性の受け渡しが残るため。見出しは `DialogTitle` が useId の id を付け、Dialog がそれを aria-labelledby に使う。

対象範囲: タスク記載の 8 モーダルに加え、タスク作成後に入った MultiAgentChatPage の「メモリに保存」ダイアログ (Escape / Tab トラップ / フォーカス復帰を個別実装していた) と ConfirmDialog の Escape リスナーも Dialog に寄せた。残すと同じ処理が 3 実装並び AC #5 に反するため。

AC #1 の判断 (Escape で入力中の変更を破棄してよいか): 全モーダルで「Escape = 閉じる (キャンセル) ボタンと同じ操作」とした。既存の閉じるボタンがどのモーダルでも確認なしで下書きを捨てるので、Escape が新しい損失経路を増やすわけではない。例外は 2 つ:
- 記憶保存ダイアログは送信中にキャンセルボタンが無効になるので、Escape も送信中は効かない (閉じると保存エラーの表示先がなくなる)
- IME 変換中の Escape (isComposing / keyCode 229) は無視する。日本語入力で変換取り消しのつもりの Escape がモーダルごと入力を捨てる事故を防ぐため (チャット入力欄の Enter と同じ判定)
ConfirmDialog の Escape は従来どおり false に解決する。

設計上の要点:
- 開いているダイアログをモジュール内スタックで持ち、最上位だけが Escape / Tab を処理する (モーダル内から確認ダイアログが出たとき、1 回の Escape で両方閉じないため)
- 戻り先は初回レンダー時の activeElement。effect では React の autoFocus が先に走ってダイアログ内に移っているため。非同期で開く記憶保存ダイアログだけは returnFocusTo で押したボタンを明示する
- フォーカス復帰は microtask に遅らせ、カードが DOM から外れたときだけ行う。StrictMode が effect を 1 回空打ちする際に戻り先へフォーカスを移して autoFocus を奪うのを防ぐため
- Tab の対象から display:none の要素 (ドキュメント追加の隠し file input) を除外する

検証: `pnpm check:client` (tsc --noEmit) と `pnpm build:client` が成功 (lint は package.json に定義なし)。`pnpm dev` の Wails dev サーバー (localhost:34115) をブラウザで開き、実キー入力で次を確認した:
- 設定 / プロジェクト名編集 / システムプロンプト編集 / メモリ編集 / ドキュメント表示 / チャットタイトル編集 / ドキュメント追加 / 編集レビュー / 記憶保存 / 確認ダイアログの 10 箇所すべてで role="dialog"・aria-modal="true"・aria-labelledby が見出し文言 (確認ダイアログは本文) を指す (AC #2)
- 全箇所で Tab / Shift+Tab を 5〜40 回押してもフォーカスがダイアログ外に出ない (設定では 40 回の focusin がすべて内側) (AC #3)
- 全箇所で Escape により閉じる。isComposing:true / keyCode 229 の Escape では閉じない (AC #1)
- 閉じた後、開いたボタン (設定・各編集ボタン・メモリに保存 (11 個中押した 1 個)・レビュー・プロジェクト削除) にフォーカスが戻る。記憶保存の textarea と確認ダイアログのキャンセルの autoFocus は StrictMode 下でも維持 (AC #4)

未確認・既知の制約:
- 入れ子 (モーダル内から確認ダイアログ) は実機で再現していない。該当経路はチャットのドキュメント追加で同名ファイルを上書き確認する場合だけで、テストデータの追加が要るため。スタックの判定ロジックのみで担保している
- 編集レビューをストリーム生成中に閉じると、レビューボタンが生成完了まで disabled のため focus() できず、フォーカスは body に落ちる。生成完了後に閉じた場合は戻ることを確認済み
- プロジェクト画面のドキュメント一覧の項目は onClick 付きの article でフォーカスできないため、ドキュメント表示を閉じても戻り先がない (開く前もフォーカスはなかった)
- 確認は Chromium 系のブラウザで行った。WebKit はマウスクリックでボタンにフォーカスを移さない実装なので、実アプリ (macOS WKWebView) でマウスから開いた場合は戻り先がない可能性がある (未確認)。キーボードで開いた場合は戻り先が取れる
<!-- SECTION:NOTES:END -->
