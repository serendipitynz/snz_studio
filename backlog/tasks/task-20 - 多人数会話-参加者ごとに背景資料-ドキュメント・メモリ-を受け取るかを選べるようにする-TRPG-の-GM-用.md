---
id: TASK-20
title: '多人数会話: 参加者ごとに背景資料 (ドキュメント・メモリ) を受け取るかを選べるようにする (TRPG の GM 用)'
status: To Do
assignee: []
created_date: '2026-09-20 00:18'
updated_date: '2026-09-21 01:14'
labels: []
milestone: m-1
dependencies:
  - TASK-18
references:
  - docs/multi-agent-chat-design.md
  - internal/model/model.go
  - internal/repository/participant.go
  - internal/service/turnengine.go
  - frontend/src/components/ParticipantPanel.tsx
  - internal/db/schema.go
ordinal: 20000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

多人数会話の目標の一つに AI による TRPG の再現がある。TASK-18 はプロジェクトのドキュメント・メモリを
背景資料としてターンごとに 1 回組み立て、全参加者に同じものを渡す。共有世界の文脈としては妥当だが、
TRPG では GM だけがシナリオの中身 (ダンジョンの構造、NPC の真意、伏線) を知っている必要があり、
プレイヤー役の参加者が背景資料を読むと答えを知った上で行動してしまう。

## 方針

参加者に「背景資料を受け取る」の真偽値を持たせ (既定は true、既存の参加者も true)、false の参加者の
ターンでは背景資料を system prompt に入れず、参照も保存しない。編成パネルに参加者ごとの
チェックボックスを 1 つ足す。プリセットの participants にも同じ項目を任意で持たせる (省略時 true)。

背景資料の組み立て自体は TASK-18 のままターンごとに 1 回で、受け取らない話者のターンでは検索を
省略する (性能面でも自然)。ドキュメント側に「GM 専用」などの分類を付けて参加者ごとに出し分ける案は、
このフラグで足りないことが確認されてから検討する。

足りるかどうかは 2 つの用途で見る。TRPG の GM (シナリオを GM だけが読む) は真偽値で足りる。
役割別レビュー (設計者は要件と設計書、レビュアーはそれに加えてコーディング規約、QA は要件と既知の問題、のように
参加者ごとに異なる資料の部分集合を読む) は真偽値では表現できない。後者を実現するときは、参加者 → ドキュメント ID の
集合を持たせる実装タスクを、TASK-18 の組み立て関数の話者引数 (TASK-18 項目 10) を前提に切る。
メモリを参加者ごとに絞る必要は、その時点で用途が見えてから判断する。

## 付随

設計書 `docs/multi-agent-chat-design.md` に、TRPG 再現を目標の一つとして明記し、
「背景資料は既定で全参加者に見える」ことを既知の制約として記録する (TASK-18 の §4.4 改訂と整合させる)。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 参加者ごとに背景資料を受け取るかを編成パネルで切り替えられ、既定と既存参加者は受け取る
- [ ] #2 受け取らない参加者のターンでは背景資料が system prompt に入らず、参照も保存されず、検索も実行されない
- [ ] #3 プリセット JSON で参加者ごとにこの項目を指定でき、省略時は受け取る
- [ ] #4 設計書に TRPG 再現の目標と、背景資料が既定で全参加者に見えるという制約が記録されている
<!-- AC:END -->
