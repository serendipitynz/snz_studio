---
id: TASK-17
title: '多人数会話: 作成フォームの「一時チャット」チェックボックスを無効化し、メモリを書き込まない旨を示す'
status: Done
assignee: []
created_date: '2026-09-19 22:29'
updated_date: '2026-09-20 11:12'
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
- [x] #1 種別を多人数会話にすると「一時チャット」チェックボックスが無効化され、チェック済みだった場合は外れる
- [x] #2 無効化の理由 (多人数会話はプロジェクトのメモリを書き込まない) が短い説明として表示され、TASK-18 導入後も文面が正しい
- [x] #3 多人数会話の作成リクエストに isTemporary が含まれない
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 作成フォームの種別 Select で multi_agent を選んだ時点で newChatIsTemporary を false に落とす (AC#1 のチェック解除)
2. チェックボックスを multi_agent のとき disabled にし、既存の presetInput() と同じ形の temporaryInput() で isTemporary をリクエストから省く (AC#3)
3. i18n に project.temporaryChatMultiAgentNote を EN/JA で追加し、multi_agent のときだけ Subtle で表示。文面は「メモリを書き込まない」とし、TASK-18 (読む機能) 後も正しいままにする (AC#2)
4. pnpm check:client と go build/test で検証
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 変更点

- `ProjectDetailPage.tsx`: 種別 Select の onChange で multi_agent を選んだ時点で
  `setNewChatIsTemporary(false)` を呼び、チェック済みの状態を解除する。あわせて
  チェックボックスを `disabled={newChatKind === "multi_agent"}` にした (AC#1)。
- 同ファイル: 既存の `presetInput()` と同じ形の `temporaryInput()` を追加し、
  multi_agent のときは空オブジェクトを返してリクエストボディから `isTemporary` を
  落とす (AC#3)。`handlers.go` の `bodyBool` はキー欠落時 false を返すため、
  サーバ側の `is_temporary` は従来どおり false になる。
- `i18n/index.tsx`: `project.temporaryChatMultiAgentNote` を EN/JA に追加し、
  multi_agent のときだけ `Subtle` で表示する (AC#2)。

## 判断

- チェックボックスを非表示ではなく disabled + 説明文にした。TASK-19 で再有効化する
  予定の項目なので、存在ごと消すと「なぜ多人数会話だけ設定がないのか」が UI から
  読み取れなくなる。
- 説明文は「メモリを書き込まない」とだけ書き、「読まない」には触れていない。
  TASK-18 でドキュメント・メモリを読むようになっても文面が正しいままになる (AC#2)。
- `isTemporary: false` を送るのではなく、キーごと省く方式にした。false を送ると
  「一時チャットでないことを明示的に指定した」という記録になり、TASK-19 で
  再有効化するときに既存データの解釈が変わる。

## 検証

- `pnpm check:client` (tsc --noEmit) 成功。`ja` は `Record<MessageKey, string>` で
  `en` のキー集合に型で縛られているため、これが通ることが新キーの両ロケール実装の証拠。
- `pnpm build:client` 成功 (347 modules)。
- `go build ./...` / `go test ./...` 成功 (全パッケージ ok)。

## 未検証 (人間の目で見てほしい点)

- 実機 (Wails ウィンドウ) での見た目は確認していない。フロントにテスト基盤がなく
  (vitest 等の devDependency なし)、導入はこのタスクのスコープ外と判断した。
  説明文の折り返しと Subtle の余白は実機で見てほしい。
- ChatPage の「チャット設定」モーダルにも同じ一時チャットのチェックボックスがあり、
  多人数会話でも操作できる状態のまま。AC は作成フォームのみを対象にしているので
  今回は触っていない。TASK-19 で再有効化を扱う際に一緒に見るのが自然だと思う。
<!-- SECTION:NOTES:END -->
