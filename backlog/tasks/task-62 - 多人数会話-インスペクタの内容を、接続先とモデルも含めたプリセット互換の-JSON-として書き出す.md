---
id: TASK-62
title: '多人数会話: インスペクタの内容を、接続先とモデルも含めたプリセット互換の JSON として書き出す'
status: To Do
assignee: []
created_date: '2026-09-27 00:57'
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
- [ ] #1 インスペクタから、編成・ターン進行ルール・進行役・場面設定・状態シート (共通と参加者ごと)・既定の目標値・参加者ごとの接続先とモデルを含む JSON をファイルに書き出せる
- [ ] #2 書き出した JSON を新規作成時または発言 0 件のチャットに読み込むと、書き出し元と同じ編成・設定・状態・接続先・モデルになる (テストで確認)
- [ ] #3 接続先・モデルを持たない既存のプリセット JSON は今までどおり適用できる
- [ ] #4 docs/multi-agent-presets.md と設計書の該当節が新しい欄と、接続先をプリセットに書く判断の改訂を反映している
- [ ] #5 go test / フロントの型検査が通る
<!-- AC:END -->
