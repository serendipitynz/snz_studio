---
id: TASK-47
title: '共通デザイン: チャットと複数エージェントチャットを本適用する'
status: To Do
assignee: []
created_date: '2026-09-24 20:12'
labels:
  - design
dependencies:
  - TASK-43
references:
  - ../snz-design
ordinal: 47000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
snz-design の TASK-20 (snz_studio への共通デザインの本適用) が追跡する実装タスク。TASK-43 の基盤 (共通の配色・基本部品の状態) の上で、この画面の部品を snz-design の共通仕様へ寄せる。変更の出どころは snz-design doc-13 §10 と doc-14 §7 の snz_studio の行。適用の結果は snz-design の「snz_studioの共通デザイン適用記録」に記録する。snz-design は兄弟ディレクトリ `../snz-design` にある。

対象: チャット (ChatPage)、複数エージェントチャット (MultiAgentChatPage)、参加者パネル (ParticipantPanel)、会話 (MessageBubble・Composer・最新へ戻るボタン)。会話はアプリ固有の例外だが、共通の値と横断規則は当てる (doc-16 §9)。参照: doc-4 §5.3 (会話の保持条件)、doc-9 §6.3・§6.3.1 (区画の出し入れ)・§6.9、doc-8 §6.7・§6.9。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 インスペクタ・参加者列の出し入れのトリガーが、出ているかどうかを読み上げ (aria-expanded) と見え方で述べ、1180px 以下でも残る (doc-9 §6.3.1)
- [ ] #2 user と assistant の発言が、7配色系統 × 明暗のすべてで見分けられ、生成中の発言が確定済みと同じ形で発言者名とともに描かれる (doc-4 §5.3)
- [ ] #3 処理中の図形の回転が、動きを減らす設定で遅くなる (doc-8 §6.7)
- [ ] #4 参加者パネルの折り畳み区画・上下移動・補助表示が doc-9 §6.3・§6.9、doc-8 §6.9 の状態とキー操作を持つ
- [ ] #5 失敗の語が、失敗した操作の対象の近くに失敗の段の図形とともに出る (doc-9 §5.5・§6.4)
- [ ] #6 この画面の無効の操作部品が焦点を受け、無効の理由を語で持つ (doc-8 §5.4)。処理中のボタンは語と幅を保つ (doc-8 §6.1・doc-9 §5.6)
- [ ] #7 4配色で doc-5 §3.2 の測定点の比を測り、キーボードだけで画面の全操作へ届くことを確かめ、測定環境 (doc-5 §5.3) とともに Implementation Notes に記録している。実窓 (WKWebView) の目視はオーナーの確認を記録する
- [ ] #8 `pnpm check:client`・`pnpm build:client` が通る (Go を変えたときは `go test ./...` も)
<!-- AC:END -->
