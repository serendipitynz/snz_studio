---
id: TASK-4
title: '多人数会話: 編成パネルと観戦ビュー'
status: In Review
assignee: []
created_date: '2026-09-08 22:28'
updated_date: '2026-09-18 11:54'
labels: []
dependencies:
  - TASK-3
references:
  - docs/multi-agent-chat-design.md
ordinal: 4000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/multi-agent-chat-design.md §6 のフロントエンドを実装する。参加者の CRUD と接続確認・モデル選択 (POST /api/configuration/models 流用) を行う編成パネル、発言者名つきストリーミング表示の観戦ビュー、進行コントロール (1 ターン進める / 自動進行の開始・停止 = フロント側ループ / manual 時の指名)。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 UI から参加者編成〜自動進行〜停止まで操作できる (Phase B 完了条件)
- [ ] #2 発言に参加者の表示名とモデル名が表示される
- [ ] #3 自動進行の停止がターン境界で効くこと (進行中のターンは中断されない) が UI 上で分かる。実行中のターンが表示される (設計 §4.1)
- [x] #4 ターン中に SSE が切断・リロードされても、完走した発言が messages の読み直しで観戦ビューに現れる
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. api/client.ts を多人数会話に合わせて拡張する: ChatRecord に kind/turnRule/scenePrompt、MessageRecord に participantId、Participant 型、participants CRUD・updateChatSettings・createChat(kind) の各メソッド。
2. App.tsx の /chats/:chatId を ChatRoute へ差し替える。ChatRoute は chat.kind だけを読んで ChatPage / MultiAgentChatPage を出し分ける (ChatPage は無改修)。
3. MultiAgentChatPage.tsx を新設する。観戦ビュー (発言者の表示名・モデル名つき・SSE ストリーミング)、進行コントロール (1ターン進める / 自動進行の開始・停止 / manual 時の指名)、人間の介入発言コンポーザ。
4. MultiAgentPanel.tsx を新設する。編成パネル = 参加者 CRUD、base URL 入力 + POST /api/configuration/models によるモデル選択と接続確認、turnRule と scenePrompt の編集。
5. 自動進行はフロント側ループ (設計 §4.1)。停止フラグはターン境界でのみ参照し、進行中のターンは中断しない旨と実行中の発言者を UI に出す (AC #3)。
6. SSE 切断・エラー時は messages を読み直して復帰する (AC #4)。
7. ProjectDetailPage のチャット作成に多人数会話の選択肢を足し、サイドバーで多人数会話チャットを識別できるようにする。
8. i18n の en/ja 辞書に multiAgent.* を追加する。
9. pnpm check:client と go test ./... を通す。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
設計 §6 のフロントエンドを実装した。

## 画面の分け方
ChatPage (1200 行) には手を入れず、`/chats/:chatId` に振り分けコンポーネント
(`ChatRoute`) を置き、`chat.kind` で ChatPage / MultiAgentChatPage を出し分ける。
振り分けのために chat detail を 1 回余分に読むが、loopback + ローカル SQLite なので
実コストはほぼ無く、両ページが自分の state を自分で持つ構造を保てる方を採った。

## 進行ループ (§4.1)
自動進行はフロント側の while ループ。停止はフラグを倒すだけで、進行中のターンは
中断しない。`runTurn` は「ターンに必要な入力」を引数で受け「次のターンの入力」を
返す形にしてある。**Why**: ループは 1 つのクロージャの中で各ターンを await するため、
コンポーネントスコープから読むと「ループ開始時のレンダーの値」で固まる。実際、最初の
実装では 2 ターン目以降の実行中ラベルが 1 ターン目の発言者のままになるバグがあった。
ref で最新値を持つ案も試したが、`done` の setState が flush される前に次のターンが
始まりうるので React の再レンダーに依存しない形 (done フレームの chat / messages /
participants をそのまま次の入力に渡す) にした。

## 実行中の発言者の表示
round_robin の発言者はターンが保存されるまでサーバーからは分からないので、
エンジンと同じ規則 (§4.2 step 1) を `api/turnOrder.ts` の `predictNextSpeaker` に
写して実行中の発言のラベルに使い、`done` の participantId で補正する。
予測はラベルを決めるだけでリクエストの内容は変えない。

## 自動進行と manual
manual では自動進行ボタンを無効にした。manual の「次の発言者」は操作者が決めるもので、
同じ指名を繰り返すループは会話として意味を持たないため。設計 §6 の進行コントロールの
列挙 (自動進行 / (manual 時) 指名) もこの読み方に沿う。

## 接続確認
専用ルートが無いので `POST /api/configuration/models` の成否を接続確認として使う。
モデル列挙に答えるエンドポイントは、そのままターンを実行できるエンドポイントである。
成功時はその一覧がモデル選択のプルダウンになり、未確認・失敗時はテキスト入力に落ちる。

## 検証
- `pnpm check:client` (tsc --noEmit) / `pnpm build:client` / `go build ./...` /
  `go vet ./...` / `go test ./...` すべて通過。
- フロントのモジュール (api/client.ts, api/sse.ts, api/turnOrder.ts) を実際の Go API
  (httptest で実ポート起動、スタブ LM Studio 相手) に対して Node で走らせる使い捨ての
  ハーネスで 24 項目を確認した。内容: ターンの delta 受信と done の反映、予測発言者と
  エンジンの選択の一致 (round_robin 4 ターン連続 / manual 指名)、ループ中の停止要求が
  進行中の 1 ターンだけ完走させて止まること、ストリームを中断しても完走した発言が
  messages の読み直しで取れること、参加者 CRUD と論理削除、接続確認のモデル列挙、
  turnRule / scenePrompt の保存。ハーネスはリポジトリには残していない
  (フロントのテスト基盤を新設しない方針のため)。

## 未確認 (AC #1 / #2 / #3)
ブラウザ自動化ツールも LM Studio エンドポイントもこの環境に無いため、画面を実際に
操作しての確認はしていない。上記のとおりデータ経路は検証済みだが、「UI から操作できる」
「表示される」の確認はユーザーの実機実行 (wails dev + LM Studio) に委ねる。
AC #4 のみ、切断 → messages 読み直しの経路を実コードで確認したのでチェック済み。
<!-- SECTION:NOTES:END -->
