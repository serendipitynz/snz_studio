---
id: TASK-19
title: '多人数会話: 人間が明示的にメモリを保存できるようにし、一時チャットのフラグを効かせる'
status: To Do
assignee: []
created_date: '2026-09-19 23:58'
updated_date: '2026-09-19 23:58'
labels: []
milestone: m-1
dependencies:
  - TASK-18
  - TASK-17
references:
  - internal/service/memory.go
  - internal/httpapi/multiagent.go
  - internal/service/chat.go
  - frontend/src/pages/MultiAgentChatPage.tsx
  - frontend/src/pages/ProjectDetailPage.tsx
  - docs/current-spec.ja.md
ordinal: 19000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-18 で多人数会話がプロジェクトのドキュメント・メモリを読むようになるが、書き込みは無い。
単独チャットの自動抽出 (`memory.go` `MaybeStoreFromUserMessage`) と「覚えて」抽出
(`MaybeStoreFromExplicitRequest`) を多人数会話に持ち込む案は却下した。理由:

- 人間の介入発言もロールプレイになり得る。「I am the rightful king」は semantic の cue `i am` を通り、
  演出指示の「常に」は procedural として保存される。抽出器に虚構と事実の区別は無い。
- 「覚えて」抽出は直近メッセージを role でしか区別せず、フォールバックで最新の assistant メッセージ
  (= 参加者のセリフ) を優先して保存する (`memory.go` の Heuristic fallback)。人間の発言からだけ呼んでも
  参加者のセリフがメモリになる。

## 方針

多人数会話では自動抽出を行わず、人間が保存する事実を明示的に指定する経路だけを用意する:

- 多人数会話画面から、任意のメッセージ (参加者・人間どちらでも) を選んで
  「メモリに保存」できる。保存前に内容を編集でき、kind (semantic / procedural / episodic) を選べる。
  既定の内容は選んだメッセージ本文、既定の kind は既存の `inferKindFromText` で推定。
- 保存は既存の `MemoryRepository.CreateMemory` + 埋め込み同期 (`chat.go` の自動抽出後と同じ経路)
  を使い、`source` は既存の値体系に沿って多人数会話由来と分かるものにする (organizer の対象にはなる)。
- 会話の `isTemporary` が true のときは保存操作を無効化し、理由を表示する。

ファシリテーター参加者による自律的なメモリ書き込みは行わない (帰属と方針の問題に対して初期価値が
小さい)。参加者が「これは記録に値する」と提案する仕組みも今回は入れない。

## TASK-17 との関係

TASK-17 で無効化した作成フォームの「一時チャット」チェックボックスは、このタスクで意味を持つ
(読むが書かない) ようになるので、多人数会話でも再有効化し、説明文を単独チャットと同じ趣旨に戻す。
`docs/current-spec.ja.md` §4.4 に多人数会話での一時チャットの意味を追記する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 多人数会話画面から任意のメッセージを選び、内容と kind を編集した上でプロジェクトのメモリとして保存できる
- [ ] #2 保存されたメモリは既存の埋め込み同期と organizer の対象になり、多人数会話由来であることが source から分かる
- [ ] #3 多人数会話でも自動抽出・「覚えて」抽出は動かない (参加者・人間の発言いずれからも自動保存されない)
- [ ] #4 isTemporary な多人数会話では保存操作が無効化され、理由が表示される
- [ ] #5 作成フォームの「一時チャット」チェックボックスが多人数会話でも有効に戻り、説明文と docs/current-spec.ja.md §4.4 が更新されている
<!-- AC:END -->
