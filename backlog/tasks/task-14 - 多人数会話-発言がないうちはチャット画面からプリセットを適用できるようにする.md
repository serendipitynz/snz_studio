---
id: TASK-14
title: '多人数会話: 発言がないうちはチャット画面からプリセットを適用できるようにする'
status: To Do
assignee: []
created_date: '2026-09-19 22:29'
labels: []
milestone: m-1
dependencies: []
references:
  - frontend/src/components/ParticipantPanel.tsx
  - frontend/src/pages/MultiAgentChatPage.tsx
  - internal/httpapi/multiagent.go
  - internal/preset
  - docs/multi-agent-chat-design.md
ordinal: 14000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
プリセット (参加者 / ターンルール / 場面設定を一括で埋めるもの) は、現状チャット作成時にしか
適用できない (`POST /api/projects/{id}/chats` の `presetId` / `preset`)。サイドバーの「＋」から
空の名簿で多人数会話を作れるようになると、作成後にプリセットを選びたくなる。

対応方針: 発言 (メッセージ) が 1 件も無い多人数会話に限り、チャット画面 (編成パネル) から
同梱プリセットまたはファイルから読んだプリセットを適用できるようにする。適用は作成時と同じく
参加者・ターンルール・場面設定を置き換える。発言がある会話への適用はサーバー側で拒否し、
UI でもプリセット選択を出さない (途中で名簿と場面を丸ごと差し替えると会話の一貫性が壊れるため)。
既存の参加者が居る場合に置き換えるか追記するかは実装時に決め、根拠を記録する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 発言が 0 件の多人数会話では、チャット画面からプリセットを選んで適用できる (同梱・インポート両方)
- [ ] #2 適用後の参加者・ターンルール・場面設定が、作成時に同じプリセットを指定した場合と一致する
- [ ] #3 発言が 1 件以上ある会話ではプリセット適用が UI に出ず、API も拒否する
- [ ] #4 既存参加者がいるときの扱い (置換 / 追記) とその根拠がタスクに記録されている
<!-- AC:END -->
