---
id: TASK-17
title: '多人数会話: 作成フォームの「一時チャット」チェックボックスを無効化し、メモリを書き込まない旨を示す'
status: To Do
assignee: []
created_date: '2026-09-19 22:29'
updated_date: '2026-09-19 23:58'
labels: []
milestone: m-1
dependencies: []
references:
  - internal/httpapi/multiagent.go
ordinal: 17000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
プロジェクト画面の新規チャット作成フォームは、種別を多人数会話にしても「一時チャット」
チェックボックスが有効のままで、`isTemporary: true` で作成できる (`ProjectDetailPage.tsx:611`)。

一時チャットの効果 (自動メモリ抽出をしない、「覚えて」を無効化する、organizer の対象外にする。
`docs/current-spec.ja.md` §4.4) は ChatService 側の分岐で実現されており、多人数会話のターンエンジン
(`turnengine.go`) と人間の介入発言の保存経路 (`multiagent.go` の `storeHumanMessage`) は
メモリを一切書き込まない。したがってフラグを立ててもサイドバーの ⏱️ 表示が変わるだけで挙動は
変わらない。

多人数会話でプロジェクトのドキュメント・メモリを読む機能 (TASK-18) が入っても、この状況は変わらない
(読むだけで書かない)。人間が明示的にメモリを保存する経路 (TASK-19) が入る時点で、一時チャットは
「読むが書かない」の意味を持つようになるので、チェックボックスの再有効化は TASK-19 の中で行う。

対応方針: 種別が多人数会話のときはチェックボックスを disabled にし、チェック済みなら外す。
横に「多人数会話はプロジェクトのメモリを書き込まないため、一時チャットの設定は不要」という趣旨の
短い説明を出す。作成リクエストでも `isTemporary` を送らない。
説明文は TASK-18 でドキュメント・メモリを読むようになっても正しいまま (「読まない」とは書かない)。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 種別を多人数会話にすると「一時チャット」チェックボックスが無効化され、チェック済みだった場合は外れる
- [ ] #2 無効化の理由 (多人数会話はプロジェクトのメモリを書き込まない) が短い説明として表示され、TASK-18 導入後も文面が正しい
- [ ] #3 多人数会話の作成リクエストに isTemporary が含まれない
<!-- AC:END -->
