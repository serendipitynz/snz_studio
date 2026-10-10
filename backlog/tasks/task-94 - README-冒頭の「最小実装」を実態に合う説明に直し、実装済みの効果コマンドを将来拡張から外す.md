---
id: TASK-94
title: 'README: 冒頭の「最小実装」を実態に合う説明に直し、実装済みの効果コマンドを将来拡張から外す'
status: Done
assignee: []
created_date: '2026-10-09 21:32'
updated_date: '2026-10-10 03:19'
labels: []
dependencies: []
references:
  - README.md
  - README.ja.md
  - AGENTS.md
priority: low
type: docs
ordinal: 98000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

README の冒頭は `A minimal local-LLM project workspace for personal use.`（README.ja.md は「個人用途向けのローカル LLM プロジェクト管理ツールの最小実装です。」）と書いている。これは AGENTS.md の Goal にある初期目標の言い回しが残ったもので、2026-10-10 時点の実態と合わない。

- 構成は今も小さい: 外部インフラなし (SQLite + ローカルファイル)、Go の直接依存は 4 つ、層は httpapi → service → repository の 3 段。
- 機能の範囲は最小ではない: Go 本体が約 20,500 行 (テスト別に約 15,200 行)、TS/TSX が約 15,000 行。マルチエージェントチャット (ターンルール 4 種、TRPG の状態シート・ダイス・効果コマンド、プリセット 25 個)、埋め込みサイドカー同梱、日本語トークナイザの移植、自動アップデータと署名検証、12 テーマ、i18n などを持つ。

あわせて、「Future extension points」(README.ja.md では「将来拡張ポイント」) が「状態シートを更新する効果コマンド」を未実装として挙げているが、Features には `/add` / `/use` / `/set` が実装済みとして書かれている。進行役モデルによる発言者指名と生成のキャンセルは、まだ未実装 (docs/multi-agent-chat-design.md §7)。

## 作業

- README.md と README.ja.md の冒頭の一文を、「ローカルで完結する軽量な」ワークスペースという実態に合う説明に直す。案: EN `A lightweight, local-only LLM project workspace for personal use.` / JA「個人用途向けの、ローカルで完結する LLM プロジェクトワークスペースです。」
- 「最小」と言える性質 (外部インフラなし・依存が少ない・層が浅い) を残したい場合は、冒頭ではなく構成の説明 (Structure / Implementation approach) に書く。
- 両 README の将来拡張の項目から効果コマンドを外し、残りの 2 項目 (進行役モデルによる指名、生成のキャンセル) はそのまま残す。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 README.md と README.ja.md の冒頭が、機能範囲を「最小実装」と言わない説明になっていて、両者の意味が揃っている
- [x] #2 両 README の将来拡張の項目に、実装済みの効果コマンド (/add・/use・/set) が残っていない
- [x] #3 将来拡張の項目に、進行役モデルによる発言者指名と生成のキャンセルが残っている
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- 冒頭: EN は案どおり `A lightweight, local-only LLM project workspace for personal use.`。JA は案に「軽量な」を足して「個人用途向けの、ローカルで完結する軽量な LLM プロジェクトワークスペースです。」とし、EN の lightweight と意味を揃えた (AC #1)。
- 「最小」と言える性質を構成の説明へ移すかは、追記しないと判断した。冒頭 2 文目が「Wails + React + SQLite + ローカル filesystem だけで構成」と既に書き、Caveats / 注意にも「最小構成」が残っていて、構成の小ささは伝わっているため。Features の「persistent memory の最小実装」は memory 抽出が rule-based である実態どおりなので触っていない。
- 将来拡張: docs/multi-agent-chat-design.md §7 で TRPG 対応 (状態シート TASK-35・ダイス TASK-37・効果コマンド TASK-36) がすべて実装済みと確認できたので、「TRPG 対応の残り」という書き出しごと外し、進行役モデルによる指名と生成中断の 1 項目にまとめた (AC #2, #3)。
- 確認: `grep -n -i 'effect command\|効果コマンド' README.md README.ja.md` の該当は Features の実装済みの記述 1 件ずつだけ。
- §7 の「将来」にある retrieval 統合は README の将来拡張に元から載っておらず、本タスクの範囲外として足していない。
- docs のみの変更で、Markdown lint はリポジトリにないためテスト・lint は実行していない。
<!-- SECTION:NOTES:END -->
