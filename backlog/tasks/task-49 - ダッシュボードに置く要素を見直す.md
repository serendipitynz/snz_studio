---
id: TASK-49
title: ダッシュボードに置く要素を見直す
status: In Review
assignee: []
created_date: '2026-09-24 22:15'
updated_date: '2026-09-26 08:45'
labels:
  - design
dependencies: []
ordinal: 49000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
オーナーの指摘 (2026-09-25、PR #42 の実画面の確認): ダッシュボード (ProjectListPage) に何を置くかは要検討。今はプロジェクトの並びと作成フォームの2つの区画だけで、画面の大半が空いている。共通デザインの本適用 (TASK-43〜48) とは別の、画面の構成を決めるタスクとして扱う。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 ダッシュボードに置く要素と、その並びをオーナーと合意している
- [x] #2 合意した構成で画面を実装し、`pnpm check:client`・`pnpm build:client` が通る
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
合意 (2026-09-26、オーナー): 置く要素は 最近のチャット / プロジェクト一覧 (行の情報を増やす) / 接続状態 の3つを、この順で上→下・左→右に並べる。作成フォームは常設の区画をやめ、一覧の見出しのボタン + ダイアログにする。マイルストーンは付けない。

1. バックエンド: Project に lastActivityAt (プロジェクト自身と配下チャットの updated_at の新しい方) を派生列で足す。プロジェクトの updated_at はチャットの発言で動かないため、そのままでは「最終更新」が実態とずれる
2. バックエンド: GET /api/chats/recent?limit=N (既定 8) を足す。全プロジェクト横断でチャットを updated_at 降順に返し、プロジェクト名を添える。repository と handler のテストを足す
3. フロント: api client に型と getRecentChats を足す
4. フロント: CreateProjectDialog (タイトル・システムプロンプト、入力ありで閉じると破棄確認、主操作は右寄せ)
5. フロント: ダッシュボードを 2 列 (最近のチャット | プロジェクト) + 下段に接続状態 (LLM・レビュー・埋め込みのバッジとモデル名、設定を開くボタン) に組み直す。プロジェクトの行にチャット数・最終更新・システムプロンプトの冒頭を添える
6. i18n (en/ja) と docs/current-spec(.ja).md の Dashboard 節を更新
7. go test ./...、pnpm check:client、pnpm build:client
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
合意 (2026-09-26、オーナーへの選択式の確認): 置く要素は「最近のチャット」「プロジェクト一覧 (行の情報を増やす)」「接続状態」の3つ。候補に挙げた「はじめての案内」は採らなかった。並びは上→下・左→右にこの順。作成フォームは常設の区画をやめてボタン + ダイアログ。マイルストーンは付けない。

実装の判断:
- プロジェクトの行の「最終更新」は projects.updated_at ではなく、配下チャットの updated_at との新しい方 (lastActivityAt) を SQL で派生させた。発言で動くのはチャットの updated_at だけなので、プロジェクトの値だけでは使われ方とずれる
- 最近のチャットは GET /api/chats/recent (既定 8 件、limit は 1〜50、範囲外は 400) を新設した。既存の GET /api/projects/{id} をプロジェクトごとに呼んで合わせる案は、プロジェクト数だけリクエストが増えるので採らなかった。一時チャットも含め、サイドバーと同じ印 (多人数・一時) を付ける
- 接続状態は GET /api/configuration を読む。サーバーが読むたびに接続先へ確認しに行くため、確認中は見出し横にスピナーを出す。設定を閉じたら読み直す。レビューのモデルが空のときはサーバーの確認と同じくチャットのモデルを表示する
- 左右の区画の見出しの高さをそろえた (プロジェクト側はボタンで高くなるため、最近のチャット側に min-height を持たせた)
- 仕様書 (docs/current-spec(.ja).md) の Dashboard 節と API 節、README の設定の入口の記述 (古い「Dashboard の Configuration」) を更新

確認:
- go test ./... 全件 pass (TestChatRepositoryListRecent・TestRecentChats を追加、TestProjectRepository に lastActivityAt を追加)、go vet pass
- pnpm check:client・pnpm build:client pass
- 一時的な API ハーネス (シードデータ入り) + Vite + Playwright (Chromium) で描画: 1440px のライト・ダーク、800px の1列、作成ダイアログを撮影。空のダイアログは Escape で確認なしに閉じて焦点が「新しいプロジェクト」に戻る、入力ありの Escape は破棄確認が出て「編集を続ける」で残る、作成で閉じて一覧が 3→4 件になる、タイトル空はダイアログ内に失敗が出る、最近のチャットの行を押すとそのチャットへ移る、行の焦点の枠が行の内側に描かれる、を確認
- 実窓 (Wails) では見ていない。内蔵埋め込みの準備中の表示 (ハーネスでは埋め込みを動かしていないため「未接続」になる) と、実データでの見え方はオーナーの確認に残す
<!-- SECTION:NOTES:END -->
