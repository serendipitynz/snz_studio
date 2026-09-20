---
id: TASK-14
title: '多人数会話: 発言がないうちはチャット画面からプリセットを適用できるようにする'
status: In Review
assignee: []
created_date: '2026-09-19 22:29'
updated_date: '2026-09-20 10:24'
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
- [x] #2 適用後の参加者・ターンルール・場面設定が、作成時に同じプリセットを指定した場合と一致する
- [ ] #3 発言が 1 件以上ある会話ではプリセット適用が UI に出ず、API も拒否する
- [x] #4 既存参加者がいるときの扱い (置換 / 追記) とその根拠がタスクに記録されている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 用語の固定 (対応表): 新語は導入しない。既存の設計書 §2 の語をそのまま使う —
   「プリセットの適用」= turnRule / scenePrompt / participants[] を chat に入れること、
   「編成」= deleted_at IS NULL の参加者集合、「発言」= messages の 1 行。
   新しく書き足す条件は「発言が 1 件も無い多人数会話」と文で書き下し、ラベル化しない。
2. AC #4 の判定: 既存参加者は「置換」。しかも論理削除ではなく物理削除で消す。
   根拠: (a) AC #2 が「作成時に同じプリセットを指定した場合と一致」を要求するので、追記では
   名簿が一致しない。(b) 論理削除だと GET /participants に除籍行が残り、CreateParticipant の
   sort_order が max+1 起点になるため、作成時 (0 起点・除籍行なし) と一致しない。
   (c) 論理削除の理由 (設計書 §3: 過去発言の話者解決と round_robin の巡回起点) は発言 0 件では
   成立しないので、物理削除しても壊れるものが無い。
3. サーバ: POST /api/chats/{chatId}/preset を追加。multi_agent 以外は 400、未知 presetId は 404、
   presetId と preset の同時指定は 400 (既存 presetFromBody を流用)、発言が 1 件以上なら 409。
   適用は 編成の全行を削除 → turnRule / scenePrompt を更新 → title が空ならプリセットの title →
   participants を登録順に作成。応答は { chat, participants }。
   ParticipantRepository に DeleteRoster(chatID) を追加。
4. フロント: ProjectDetailPage に埋め込まれているプリセット選択 (一覧取得・群分け・ファイル読み込み・
   要約表示) を components/PresetPicker.tsx に抽出し、作成フォームと編成パネルの両方から使う。
   編成パネルは発言 0 件のときだけプリセットカードを出し、名簿が空でないときは ConfirmDialog で確認。
   観戦ビューは messages.length === 0 を渡し、適用後は chat と participants を差し替える。
5. i18n (en / ja) に適用まわりのキーを追加。WorkspaceSidebar の「プリセットは作成時のみ」コメントを訂正。
6. 文書: multi-agent-chat-design.md (§2 プリセットの定義・§5 ルート表・§6 プリセット節)、
   current-spec.md / current-spec.ja.md、presets/multi-agent/README.md を実装に追随させる。
7. 検証: go test ./... / pnpm check:client / pnpm build:client。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装 (2026-09-20)

`POST /api/chats/{chatId}/preset` を追加し、発言が 1 件も無い多人数会話に限ってプリセットを
適用できるようにした。フロントは作成フォームに埋め込まれていたプリセット選択 UI を
`frontend/src/components/PresetPicker.tsx` に抽出し、編成パネルの先頭でも同じ UI を使う。

### AC #4 の判定 — 既存参加者は「置換」、しかも物理削除

- 追記にしなかった理由: AC #2 が「作成時に同じプリセットを指定した場合と一致」を要求する。
  追記すると編成が (既存 + プリセット) になって一致しないうえ、プリセットの場面設定は
  プリセットの配役に向けて書かれているので、混ざった編成はその場面設定で進むことになる。
- 論理削除 (`deleted_at`) にしなかった理由: (1) 除籍行が `GET /participants` に残り、
  編成パネルの「除籍済み」に一度も発言していない下書きが並ぶ。(2) `CreateParticipant` の
  `sort_order` は除籍行も数えた max+1 なので、適用後の編成が 0 起点にならず作成時と一致しない。
  (3) 論理削除の根拠 (設計書 §3: 過去発言の話者解決と round_robin の巡回起点) は発言 0 件では
  成立しないので、行を消して壊れるものが無い。`ParticipantRepository.DeleteRoster` は
  この 1 箇所のためだけの物理削除で、コメントにその条件を書いた。

### 拒否の設計

- 発言が 1 件以上ある会話は 409。400 ではない理由は、body は正しく、同じ body が少し前の
  同じ chat では通る = chat の状態だけが拒む状況だから (ターンの重なりと同じ扱い)。
- 単独 assistant の chat は 400、未知の `presetId` は 404、`presetId` と `preset` の同時指定は 400。
  いずれも作成時と同じ `presetFromBody` を通すので、インポートしたプリセットの検証も作成時と同一。
- 適用が途中で失敗しても chat は消さない (作成時は消す)。利用者が既に持っている chat であり、
  発言 0 件のままなので同じルートで再適用すれば中途半端な編成は消えてやり直せる。

### タイトル

chat の title が空のときだけプリセットの title を使う (作成時と同じ規則)。サイドバーの「＋」が
作る chat は title が空なので、この経路で名前が付く。

### 検証

- `go test ./...` / `go vet ./...` / `pnpm check:client` / `pnpm build:client` すべて通過。
- 追加テスト `TestMultiAgentPresetAppliedAfterCreation` (internal/httpapi/multiagent_test.go):
  「presetId で作った chat」と「空の chat に手入力の参加者を 1 人足してから同じ preset を適用した chat」を
  title / turnRule / scenePrompt と編成の全フィールド (displayName / rolePrompt / baseUrl / modelName /
  sortOrder / deletedAt) で突き合わせて一致を確認 (AC #2)。手入力の参加者が消えることで置換も確認 (AC #4)。
  インポート形式 (inline preset) の適用、title を持つ chat への適用、409 / 404 / 400 の各拒否、
  拒否後に chat が変化しないことも同テスト内。
- 未検証: UI 実機での操作 (Wails デスクトップアプリを起動しての確認)。AC #1 と AC #3 の UI 側は
  型検査とビルドが通ることまでしか確認していないので、チェックを立てていない。

### 追随させた文書

- docs/multi-agent-chat-design.md: §2 プリセットの定義、§5 ルート表と「プリセットの適用」、
  §6 フロントエンド、§8.1 に今回の判断を追記。
- docs/current-spec.md / current-spec.ja.md, presets/multi-agent/README.md,
  frontend/src/components/WorkspaceSidebar.tsx のコメント (「作成時のみ」の記述を訂正)。

### 触っていない既存の乱れ

`internal/search/model.go` は gofmt 未整形 (本タスクの変更対象外なのでそのまま)。
<!-- SECTION:NOTES:END -->
