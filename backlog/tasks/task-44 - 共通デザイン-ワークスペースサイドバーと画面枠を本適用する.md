---
id: TASK-44
title: '共通デザイン: ワークスペースサイドバーと画面枠を本適用する'
status: To Do
assignee: []
created_date: '2026-09-24 20:11'
labels:
  - design
dependencies:
  - TASK-43
references:
  - ../snz-design
ordinal: 44000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
snz-design の TASK-20 (snz_studio への共通デザインの本適用) が追跡する実装タスク。TASK-43 の基盤 (共通の配色・基本部品の状態) の上で、この画面の部品を snz-design の共通仕様へ寄せる。変更の出どころは snz-design doc-13 §10 と doc-14 §7 の snz_studio の行。適用の結果は snz-design の「snz_studioの共通デザイン適用記録」に記録する。snz-design は兄弟ディレクトリ `../snz-design` にある。

対象: 画面枠 (AppShell・WorkspaceShell)、ワークスペースサイドバー (プロジェクトとチャットの並び・チャットの種類のメニュー・設定ボタン)。参照: doc-9 §6.8 (ナビゲーション)・§6.11 (メニュー)、doc-8 §5.1・§5.2、doc-16 §6.3。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 サイドバーの現在地が `aria-current` を持ち、面に加えて枠と左端の帯で示される (doc-9 §6.8・doc-8 §5.2)
- [ ] #2 スクロールする並びの先頭・末尾の行でも焦点の枠が切れない (doc-16 §6.3)
- [ ] #3 900px 以下でサイドバーが畳まれた入口へ切り替わり、本体の上に積まれない (doc-9 §6.8)
- [ ] #4 チャットの種類のメニューが Tab で閉じ、Home / End を持つ (doc-9 §6.11)
- [ ] #5 この画面の無効の操作部品が焦点を受け、無効の理由を語で持つ (doc-8 §5.4)。処理中のボタンは語と幅を保つ (doc-8 §6.1・doc-9 §5.6)
- [ ] #6 4配色で doc-5 §3.2 の測定点の比を測り、キーボードだけで画面の全操作へ届くことを確かめ、測定環境 (doc-5 §5.3) とともに Implementation Notes に記録している。実窓 (WKWebView) の目視はオーナーの確認を記録する
- [ ] #7 `pnpm check:client`・`pnpm build:client` が通る (Go を変えたときは `go test ./...` も)
<!-- AC:END -->
