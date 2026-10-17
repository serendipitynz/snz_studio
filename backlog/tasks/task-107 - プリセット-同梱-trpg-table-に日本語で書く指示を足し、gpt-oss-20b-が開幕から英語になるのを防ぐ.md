---
id: TASK-107
title: 'プリセット: 同梱 trpg-table に日本語で書く指示を足し、gpt-oss-20b が開幕から英語になるのを防ぐ'
status: To Do
assignee: []
created_date: '2026-10-17 23:35'
labels: []
dependencies: []
references:
  - internal/preset/bundled/25-trpg-table.json
  - docs/multi-agent-chat-design.md
  - docs/multi-agent-presets.md
type: bug
ordinal: 111000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-103 の測定（設計書 §4.9.7 の「条件」）で、同梱 `trpg-table` をそのまま gpt-oss-20b に読ませると、開幕の GM の発言から英語で書いた（5 回中 5 回）。
場面設定も役割プロンプトも日本語だが、言語の指示は無い。ユーザーが実際に使っている TRPG の会話は、場面設定の「進め方」に次の 1 行を手で足しており、
これを足すと gpt-oss も日本語で進んだ。

- 発言はすべて日本語で書きます。他の参加者が別の言語で話しても、日本語で続けます。

gemma-4-e4b は同梱のままでも日本語で進んだ。

## 決めること

- 足す場所: `25-trpg-table.json` の場面設定（上の 1 行）にするか、ほかの同梱プリセットにも同じ指示が要るか。
  役割リマインド（§4.3、`buildTurnSystemPrompt`）に足すと全会話の prompt が変わるので、その場合は着手時にユーザーに確認する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 同梱 trpg-table を gpt-oss-20b で開幕から 5 ターン進めて、英語の発言が出ないことを確かめた結果が記録されている
- [ ] #2 指示を足した場所と、ほかの同梱プリセットに同じ指示が要るかの判断が記録されている
- [ ] #3 プリセットの検証テスト（go test ./internal/preset/...）が通る
<!-- AC:END -->
