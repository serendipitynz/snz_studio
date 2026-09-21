---
id: TASK-30
title: コピー完了のフィードバックが実アプリで視認できない
status: In Review
assignee: []
created_date: '2026-09-21 09:57'
updated_date: '2026-09-21 09:58'
labels: []
milestone: m-1
dependencies: []
references:
  - frontend/src/components/CopyMessageButton.tsx
ordinal: 30000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-29 のマージ後、オーナーが Wails の実アプリで確認したところ、クリップボードへのコピーは機能していたが、コピー完了のフィードバックが視認できなかった。

原因はフィードバックが `title` 属性 (OS ネイティブのツールチップ) であること。ツールチップはクリック (mousedown) で消え、ポインタが要素から出て入り直すまで再表示されず、再表示にも hover の待ち時間 (macOS の WKWebView でおおむね 1〜2 秒) がかかる。`title` を「コピーしました」にしている時間は 1400ms なので、消えている間に書き換わり、再表示できる頃には元に戻っており、実質的に見る機会がない。同時に変わる `opacity` 0.82 → 1 は目視できる差ではない。

これは TASK-29 が持ち込んだ不具合ではない。同じ仕組みが単独アシスタント画面に元からあり、TASK-29 はそれを共有コンポーネントに移設しただけなので、両画面とも以前から見えていなかった。TASK-29 の AC#3 は「単独チャットと同じ」という同等性の要求だったため、効いていない実装を追認する形になっていた。

## 方針

`title` の切り替えに頼らず、アイコン自体を変える。コピー直後の一定時間だけ `CopyIcon` をチェックマークに差し替える。TASK-29 で共通コンポーネントに寄せてあるので、`components/CopyMessageButton.tsx` の変更だけで単独チャット・多人数会話の両画面に効く。`title` の切り替えは、ホバーし直したときの表示とアクセシビリティ上の情報として残してよい。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 コピー直後、ボタンのアイコンが一定時間チェックマークなど「コピーできた」と分かる見た目に変わり、実アプリ (Wails の WebView) で視認できる
- [x] #2 単独アシスタント画面と多人数会話画面の両方で同じフィードバックが出る
- [x] #3 一定時間後に元のコピーアイコンに戻り、連続してコピーしても状態が壊れない
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. components/CopyMessageButton.tsx に CheckIcon を追加し、copied が true の間だけ CopyIcon の代わりに描画する。アイコンの差し替えはツールチップと違いクリック後の hover 状態に依存しないので、1400ms でも確実に視認できる。
2. title の切り替え (chat.copy / chat.copied) はそのまま残す。ホバーし直したときの表示として、またアイコンだけでは伝わらない利用者向けの情報として意味があるため。
3. 連続コピーは既存実装がタイマーを張り替える形 (新しいタイマーを張る前に clearTimeout) になっているので、そのまま。
4. 新しい i18n キーは足さない。アイコンに文言はなく、title は既存キーで足りる。
5. tsc + vite build で検証し、実アプリでの視認はオーナーに依頼する (この環境から Wails の WebView のクリックを駆動できないため)。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装

`components/CopyMessageButton.tsx` に `CopiedIcon` (チェックマーク) を足し、`copied` が true の間だけ `CopyIcon` と差し替えるようにした。変更はこの 1 ファイルのみ。TASK-29 で両画面が同じコンポーネントを通る形にしてあるので、単独チャットと多人数会話の両方に同時に効く。

- `title` の切り替え (`chat.copy` / `chat.copied`) は残した。消しても見た目は変わらないが、ホバーし直したときの表示とアクセシビリティ上の情報として意味があるため。効いていなかったのは「title だけに頼っていたこと」であって title 自体ではない。
- 新しい i18n キーは足していない。アイコンに文言はなく、title は既存キーで足りる。
- 連続コピー時の挙動は既存のまま。新しいタイマーを張る前に前のタイマーを clearTimeout しているので、2 回目のクリックで 1 回目のタイマーが先に発火してアイコンが早戻りすることはない。
- アイコンは既存の house style (16x16 viewBox / currentColor / strokeWidth 1.2 / 丸いキャップ) に揃えた。

## 検証

- `pnpm check:client` (tsc --noEmit) 通過。
- `pnpm build:client` (vite build) 通過。
- AC#1 のうち「チェックマークに見えるか」は、同じ path を rsvg-convert で PNG に起こして目視確認した (チェックマークとして読める)。
- AC#2 は両画面が同一コンポーネントを通ることによる。AC#3 は既存のタイマー張り替え処理を読んで確認した。
- 未実施: Wails の WebView で実際に押してアイコンが切り替わるのを見る確認。ブラウザ単体での開発が廃止されておりこの環境から駆動できない。ただし今回の切り替えは DOM の描画そのもので、ネイティブのツールチップのようにクリック後のホバー状態に依存しないため、TASK-29 のときと違って実機依存の不確かさは小さい。
<!-- SECTION:NOTES:END -->
