---
id: TASK-2
title: '多人数会話: ターンエンジン service'
status: To Do
assignee: []
created_date: '2026-09-08 22:28'
updated_date: '2026-09-09 03:05'
labels: []
dependencies:
  - TASK-1
references:
  - docs/multi-agent-chat-design.md
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/multi-agent-chat-design.md §4 のターンエンジンを internal/service/turnengine.go として実装する。1 リクエスト = 1 ターン。ターン進行ルール round_robin / manual、場面設定 + 役割プロンプト + 役割リマインドの system 組み立て、履歴の発言者視点への写像 (自分= assistant / 他者= user に「表示名: 本文」)、CompletionTarget による参加者ごとの接続先指定、EnsureModelLoaded / CheckConnection による事前確認、participant_id つき保存まで。既存 service/chat.go には手を入れない。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 round_robin で直近発言から次の参加者が決まる (サーバー側に進行状態を持たない)
- [ ] #2 manual で指名した参加者がターンを実行する
- [ ] #3 プロンプト写像と発言保存のユニットテストがある
- [ ] #4 chat 単位のターン実行権 (in-process 排他) により、重なったターン要求が同じ参加者を二重に発言させない。並行要求のテストがある (設計 §4.2 手順 0)
- [ ] #5 直近発言の参加者が除籍済みでも round_robin が編成の先頭から次の発言者を決められる
<!-- AC:END -->
