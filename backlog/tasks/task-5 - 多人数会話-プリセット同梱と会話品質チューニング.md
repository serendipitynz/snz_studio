---
id: TASK-5
title: '多人数会話: プリセット同梱と会話品質チューニング'
status: To Do
assignee: []
created_date: '2026-09-08 22:28'
labels: []
dependencies:
  - TASK-4
references:
  - docs/multi-agent-chat-design.md
ordinal: 5000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/multi-agent-chat-design.md §6〜§7 Phase C。ディベート用・即興劇用のプリセット (参加者一式 + ターン進行ルール + 場面設定の雛形) を同梱し、新規作成時に選択できるようにする。履歴圧縮 (summary サービス流用の要否 = §8 未決) と役割リマインド文のチューニングを行う。保存形式 (ハードコード / 同梱 JSON / ユーザー編集可能) は着手時に決めて設計書へ反映する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 プリセット選択だけでディベート / 即興劇を開始できる (Phase C 完了条件)
- [ ] #2 長い会話 (20 ターン以上) で役割崩れ・同調収束が起きにくいことを実機の LM Studio で確認した
<!-- AC:END -->
