---
id: TASK-62
title: '多人数会話: インスペクタの内容を、接続先とモデルも含めたプリセット互換の JSON として書き出す'
status: In Review
assignee: []
created_date: '2026-09-27 00:57'
updated_date: '2026-09-27 10:45'
labels: []
dependencies: []
references:
  - docs/multi-agent-presets.md
  - docs/multi-agent-chat-design.md
  - internal/preset/preset.go
  - frontend/src/components/ParticipantPanel.tsx
type: feature
ordinal: 62000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

オーナーの要望 (2026-09-27、TASK-37 の実窓確認のとき): 会話中の編成・設定・状態を、別のチャットに引き継いで新しく始めたい。インスペクタからプリセット互換の JSON ファイルとして書き出し、それを読み込めば完全に再現できるようにする。

今のプリセット形式 (docs/multi-agent-presets.md) は、接続先 (baseUrl) とモデル (modelName) を「使う時点で選ぶもの」としてあえて持たない。今回は「サーバー・モデルも含めて完全に再現」が要望なので、この方針を変える。

## やること (案。着手時に確定する)

- インスペクタに「プリセットとして書き出す」を置く。発言の有無によらず使える
- 書き出す内容: title、turnRule、scenePrompt、stateSheet (その時点の値)、diceTarget、編成順の participants (displayName、rolePrompt、receivesProjectMaterial、facilitator、stateSheet、baseUrl、modelName)。除籍済みの参加者と transcript は含めない
- プリセット形式に participants[].baseUrl / modelName を任意の欄として足し、読み込み (新規作成時・発言 0 件の既存チャットへの適用) で反映する。同梱プリセットは今までどおり持たない
- 保存は markdown エクスポートと同じく、Wails の SaveTextFile (WebView の外では Blob)
- docs/multi-agent-presets.md と設計書 §5・§6・§8.1 (接続先をプリセットに書かない判断の改訂) を更新する

## 着手時に決めること

- 接続先・モデルを含むファイルは、そのマシンの LAN 構成に依存する。接続できない接続先を含むファイルを読み込んだときにどう見せるか (適用は通し、ターン前の 502 で分かるので足りるか、適用時に確認するか)
- 書き出しの JSON に id / group / description を持たせるか (読み込みでは使われない値)
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 インスペクタから、編成・ターン進行ルール・進行役・場面設定・状態シート (共通と参加者ごと)・既定の目標値・参加者ごとの接続先とモデルを含む JSON をファイルに書き出せる
- [x] #2 書き出した JSON を新規作成時または発言 0 件のチャットに読み込むと、書き出し元と同じ編成・設定・状態・接続先・モデルになる (テストで確認)
- [x] #3 接続先・モデルを持たない既存のプリセット JSON は今までどおり適用できる
- [x] #4 docs/multi-agent-presets.md と設計書の該当節が新しい欄と、接続先をプリセットに書く判断の改訂を反映している
- [x] #5 go test / フロントの型検査が通る
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. internal/preset: Participant に baseUrl / modelName (任意、omitempty) を足し、Validate で trim する。同梱プリセットが持っていたら mustLoadBundled で拒む。id / group / description / facilitator に omitempty を付ける
2. internal/preset に FromChat(chat, roster) を足す。chat の title (空なら「無題のチャット」)・turnRule・scenePrompt・stateSheet・commands と、編成順の参加者 (進行役の印は編成に居るときだけ) から組み立て、読み込みと同じ Validate を通す。通らなければ ErrInvalid
3. httpapi: GET /api/chats/{chatId}/export/preset を足す。Validate が通らない (2 人未満・facilitator_alternating で進行役が編成に居ない) ときは 409。createPresetParticipants で baseUrl / modelName を渡す
4. テスト: preset の FromChat 単体、HTTP で「書き出し → 新規作成に適用」「書き出し → 発言 0 件のチャットに適用」の往復、既存形式の JSON が今までどおり通ること、409 の 2 例
5. frontend: api.exportChatPreset、インスペクタ見出しの横にアイコンボタン (無効の理由つき)、SaveTextFile で <title>.json を保存。savefile に JSON 用のファイル名関数、Blob の MIME を引数化。i18n ja/en
6. docs: multi-agent-presets.md の形式表と接続先の段落、設計書 §5 (ルート表と適用の段落)・§6 (インスペクタとプリセット)・§8.1 (接続先をプリセットに書く判断の改訂) を更新
着手時の判断 (2026-09-27、オーナー): 不通の接続先は適用時に確かめず、ターン前の 502 と接続確認で分かるものとする。書き出しに id / group / description は入れない。読み込めない状態では書き出しを止めて理由を出す。ボタンはインスペクタ見出しの横
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- 書き出しはサーバー側 (GET /api/chats/{chatId}/export/preset、組み立ては preset.FromChat) にした。フロントで組み立てる案より、読み込みと同じ Validate を通してから返せるので「書き出したファイルは必ず読み込める」をコードで保証でき、往復を Go のテストで確かめられるため
- タスク文の diceTarget は TASK-65 で commands に置き換わっていたので、書き出しは commands で出す
- 進行役の印は turnRule が facilitator_alternating / weighted のときだけ付ける。chat は規則を変えても facilitatorId を残すため、そのまま写すと round_robin のプリセットに印が付き、読み込みで 400 になる
- id / group / description / facilitator に omitempty を付けた。同梱分は id・group・description を必ず持つ (起動時検証と TestBundledPresets) ので、一覧 API の応答は実質変わらない
- 同梱プリセットが baseUrl / modelName を持つと mustLoadBundled で落とす (起動しない)
- ボタンはインスペクタ見出しの直後。1180px 以下の重ねて出す形では右上を閉じるボタンが占めるため、右端には寄せない
検証
- go test ./... 全件通過、go vet 通過、pnpm check:client 通過、pnpm build:client 通過
- AC #2: TestMultiAgentPresetExportRoundTrip。除籍者・発言・接続先なしの参加者・進行役・共通/参加者の状態・roll の目標値を含むチャットを書き出し、新規作成時と発言 0 件の既存チャット (手入力の参加者あり) の両方に適用して、編成・設定・状態・接続先・モデルが一致すること、書き出し直しても同じ JSON になることを確認
- AC #3: 既存の TestMultiAgentPresets (インライン JSON・同梱の接続先なし) に加えて TestParseEndpoints
- 409 の 2 例 (2 人未満、facilitator_alternating で進行役除籍) と、round_robin に変えた後に進行役の印が出ないことは TestMultiAgentPresetExportRefusals
- ブラウザ確認 (_sandbox/task-38/uiserve + ヘッドレス Chrome、Blob 経路): 1400px と 900px でボタンの位置と重なりを見た。trpg-table から作り GM に接続先を入れたチャットで押すと「TRPG の卓 (GM とプレイヤー 2 名).json」が保存され、中身に GM の baseUrl / modelName・facilitator・状態・commands が入り、そのまま再読み込みで 201。参加者 1 人のチャットでは aria-disabled と理由が出る
未確認 (オーナーの実窓で見てほしい点)
- Wails の SaveTextFile 経路 (ネイティブの保存ダイアログ) での保存。Blob 経路でのみ確認した
- ヘッダの markdown 書き出しと同じダウンロードの図形を使っている。名前 (ツールチップ・aria-label) で区別しているが、見分けにくければ図形を変える
<!-- SECTION:NOTES:END -->
