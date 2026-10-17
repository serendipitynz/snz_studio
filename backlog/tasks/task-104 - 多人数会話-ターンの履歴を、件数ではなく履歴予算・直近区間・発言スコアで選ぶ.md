---
id: TASK-104
title: '多人数会話: ターンの履歴を、件数ではなく履歴予算・直近区間・発言スコアで選ぶ'
status: To Do
assignee: []
created_date: '2026-10-17 19:24'
labels: []
dependencies:
  - TASK-103
references:
  - docs/multi-agent-chat-design.md
  - internal/service/turnengine.go
  - internal/service/turncontext.go
  - internal/service/llmclient.go
  - internal/httpapi/multiagent.go
  - frontend/src/pages/MultiAgentChatPage.tsx
type: feature
ordinal: 108000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

設計書 §4.9 の履歴の選び方を実装する。値 (参加者 1 人あたりの予算・R・換算係数・加点) は TASK-103 の測定で決めたものを使う。
ピン留め (TASK-105) と選抜発言の参照表示 (TASK-106) は別タスクで、このタスクではピン留めの加点を入れない。

## やること

- 窓上限: 話者のモデルのロード時のコンテキスト長から system prompt と生成分を引いて文字数に換算する。取れないときは既定値 (§4.9.2 手順 1)
- 履歴予算 = min (参加者 1 人あたりの予算 × 参加者数, 窓上限)。参加者数は現在の名簿の人数 (§4.9.2 手順 2)
- 直近区間: 末尾から参加者の発言が参加者数 × R 件含まれるまで。予算を超えたら新しい側から収まるだけ入れ、選抜発言は入れない。末尾の発言は必ず入れる (手順 3)
- 発言スコア: 人間の介入・反響・今のターンとの関連・話者との関わり・会話の先頭 (§4.9.3)。同点は新しい発言を優先
- 選抜発言: スコア順に応答先とペアで予算に収まるものを足す。時系列に並べ、連続しない箇所に省略行 (独立した user メッセージ) を挟む (手順 4・5)
- `TurnHistoryLimit` を廃し、`GET /api/chats/{chatId}/turns/next` の `historyLimit` を R を返す欄に替える。観戦ビューの `(?)` の説明を直す
- §4.9.8 の既存記述 (設計書 §4.3・§4.4・§5・§6・§8.1、current-spec (en/ja)、multi-agent-presets.md) を実装に合わせて書き換える
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 話者の prompt の履歴が、履歴予算の範囲で直近区間と選抜発言から組み立てられ、連続しない箇所に省略行が入る (テストで確認)
- [ ] #2 参加者数を変えると直近区間の件数が参加者数 × R に追随し、履歴予算は窓上限を超えない (テストで確認)
- [ ] #3 直近区間が履歴予算を超えるとき、選抜発言が入らず、末尾の発言は必ず入る (テストで確認)
- [ ] #4 発言スコアの各加点が §4.9.3 どおりに効き、発言の長さが加点されない (テストで確認)
- [ ] #5 観戦ビューの説明と turns/next の応答が新しい方式を述べ、§4.9.8 に挙げた既存の記述が実装に合わせて更新されている
<!-- AC:END -->
