---
id: TASK-48
title: '共通デザイン: 設定モーダルと確認ダイアログを本適用する'
status: To Do
assignee: []
created_date: '2026-09-24 20:12'
labels:
  - design
dependencies:
  - TASK-43
references:
  - ../snz-design
ordinal: 48000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
snz-design の TASK-20 (snz_studio への共通デザインの本適用) が追跡する実装タスク。TASK-43 の基盤 (共通の配色・基本部品の状態) の上で、この画面の部品を snz-design の共通仕様へ寄せる。変更の出どころは snz-design doc-13 §10 と doc-14 §7 の snz_studio の行。適用の結果は snz-design の「snz_studioの共通デザイン適用記録」に記録する。snz-design は兄弟ディレクトリ `../snz-design` にある。

対象: 設定モーダル (SettingsModal。配色系統と明暗・言語・接続)、確認ダイアログ (ConfirmDialog)。参照: doc-7 §5.1・§5.3、doc-9 §6.6・§6.10・§5.7、doc-8 §6.5 (バッジ)・§6.7.1 (進捗の表示)。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 保存値が収録外のとき、その値が無いことを設定モーダルで述べ、保存値は書き換えない (doc-7 §5.1)
- [ ] #2 配色の選択を保存できなかったとき、その起動の間は効くことと保存できなかったことを述べる (doc-7 §5.3)
- [ ] #3 接続の点が色以外の手がかり (語か図形) を持つ (doc-8 §6.5・1.4.1)
- [ ] #4 接続の設定の書きかけを持って閉じるとき、破棄前確認を出す (doc-9 §5.7)
- [ ] #5 埋め込みモデルの取得が、量の分かる進捗の表示で描かれる (doc-8 §6.7.1)
- [ ] #6 確認ダイアログの実行のボタンが「OK」ではなく操作の語を持つ。区画の見出しをバッジで代用しない (doc-9 §6.6・§6.3)
- [ ] #7 この画面の無効の操作部品が焦点を受け、無効の理由を語で持つ (doc-8 §5.4)。処理中のボタンは語と幅を保つ (doc-8 §6.1・doc-9 §5.6)
- [ ] #8 4配色で doc-5 §3.2 の測定点の比を測り、キーボードだけで画面の全操作へ届くことを確かめ、測定環境 (doc-5 §5.3) とともに Implementation Notes に記録している。実窓 (WKWebView) の目視はオーナーの確認を記録する
- [ ] #9 `pnpm check:client`・`pnpm build:client` が通る (Go を変えたときは `go test ./...` も)
<!-- AC:END -->
