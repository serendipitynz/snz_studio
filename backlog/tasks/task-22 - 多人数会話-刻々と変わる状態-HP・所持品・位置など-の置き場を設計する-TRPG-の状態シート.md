---
id: TASK-22
title: '多人数会話: 刻々と変わる状態 (HP・所持品・位置など) の置き場を設計する (TRPG の状態シート)'
status: To Do
assignee: []
created_date: '2026-09-20 00:18'
labels: []
milestone: m-1
dependencies: []
references:
  - docs/multi-agent-chat-design.md
  - internal/service/turnengine.go
  - frontend/src/components/ParticipantPanel.tsx
type: spike
ordinal: 22000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TRPG では HP・所持品・現在位置・残り時間のような状態がターンごとに変わる。これは耐久性のある事実では
ないのでメモリに入れると汚れ (TASK-19 の趣旨に反する)、静的でもないのでドキュメントにも向かない。
現状の受け皿は場面設定 (`scene_prompt`) を人手で更新することだけで、履歴は直近 30 件で切られるため
古い状態変化は自然に消える。

## このタスクで決めること (実装はしない)

1. 状態の表現: 場面設定を「固定の設定」と「現在の状態」に分けるか、chat に別の自由文
   (状態シート) を 1 つ足すか、参加者ごとのシートにするか。
2. 更新の主体: 人間だけが編集する / 進行役の参加者の発言から抽出して更新する (小さいモデルでの
   信頼性が論点) / 発言の末尾に構造化した更新ブロックを書かせる。
3. prompt での位置: 背景資料 (TASK-18) と場面設定の間か、リマインダーの直前か。
4. 文脈量: 状態シートの上限文字数。

各案の利点・欠点と推奨を設計書に追記し、実装タスクを切る。検討は同梱プリセットのうち TRPG 卓
(進行役ターン規則のタスクで追加) を想定シナリオにする。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 状態の表現・更新の主体・prompt 内の位置・上限の 4 点について、案と推奨が設計書に記録されている
- [ ] #2 推奨案の実装タスクが起票されている (または実装不要と判断した根拠が記録されている)
<!-- AC:END -->
