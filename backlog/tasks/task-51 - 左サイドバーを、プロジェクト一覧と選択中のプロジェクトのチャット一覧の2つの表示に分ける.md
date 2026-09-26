---
id: TASK-51
title: 左サイドバーを、プロジェクト一覧と選択中のプロジェクトのチャット一覧の2つの表示に分ける
status: In Review
assignee: []
created_date: '2026-09-25 07:11'
updated_date: '2026-09-26 02:10'
labels:
  - design
dependencies: []
references:
  - ../snz-design
ordinal: 51000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
オーナーの改善案 (2026-09-25、TASK-44 (#44) の実窓の確認の後)。今の左サイドバーは、プロジェクトの一覧を常に全部並べ、その下に選択中のプロジェクトのチャット一覧を置いている。これを、プロジェクトを選んでいるかどうかで2つの表示に分ける。

プロジェクトを選んでいないとき (初期値: プロジェクト一覧):

```
SNZ STUDIO    🏠
----------------
プロジェクト
[雑談 (8)      ]
[翻訳 (1)      ]
[小説 (0)      ]
:
```

プロジェクトを選んでいるとき: 選んだプロジェクトの行だけを残し、その左に戻るボタン、その下にチャット一覧 (作成の + を含む):

```
SNZ STUDIO    🏠
----------------
プロジェクト
[←][雑談 (8)  ]
チャット     (+)
[👥 ディベート]
[複数AIモデル  ]
[お話し        ]
[挨拶          ]
:
```

「プロジェクトを選んでいるとき」は、プロジェクト詳細の画面と、そのプロジェクトのチャットの画面を指す。

着手時に決めること:
- 戻るボタン (←) の振る舞い。次のどちらにするか。
  - ダッシュボード (`/`) へ移る。
  - 画面は移らず、サイドバーの表示だけをプロジェクト一覧へ戻す。開いているチャットはそのままで、別のプロジェクトを選べる。

TASK-44 で当てた現在地の印 (`aria-current` と、面・枠・帯) と、900px 以下の畳まれた入口 (snz-design doc-9 §6.8) は、両方の表示で保つ。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 プロジェクトを選んでいないとき (ダッシュボード) は、左サイドバーにプロジェクト一覧だけが並ぶ
- [x] #2 プロジェクト詳細と、そのプロジェクトのチャットの画面では、選んだプロジェクトの行1つと戻るボタン、その下にチャット一覧 (作成の + を含む) が並び、他のプロジェクトの行は出ない
- [x] #3 戻るボタンが、着手時にオーナーと合意した振る舞いを持ち、キーボードで届き、語 (読み上げの名札) を持つ
- [x] #4 現在地の印 (aria-current と、面・枠・帯) と、900px 以下の畳まれた入口が、両方の表示で TASK-44 の形のまま働く
- [ ] #5 `pnpm check:client`・`pnpm build:client` が通る。実窓 (WKWebView) の目視はオーナーの確認を記録する
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 戻るボタンの振る舞い (着手時判断): オーナーと合意した「ダッシュボード (/) へ移る」。サイドバーの表示はルート (currentProjectId の有無) だけから決まり、追加の状態を持たない。
2. WorkspaceSidebar の destinations を currentProjectId で分ける: 無いときはプロジェクト一覧だけ (今の形からチャット節を除いたもの)。あるときは、選んだプロジェクトの行1つの左に戻るリンク (IconButton の見た目の Link、to="/"、aria-label「プロジェクト一覧へ戻る」、ArrowLeftIcon) を置き、その下にチャット節 (作成の + を含む) を出す。
3. 現在地の印 (aria-current true/page、面・枠・帯) と畳まれた入口は同じ destinations を両変種が使うので、そのまま保たれる。戻るリンクは畳まれた入口では他のリンクと同じく leaveEntry で入口を閉じる。
4. icons.tsx に Lucide の arrow-left を足し、i18n に sidebar.backToProjects (en/ja) を足す。
5. pnpm check:client・build:client を通し、Vite の開発サーバー + Playwright で AC#1〜#4 (両表示・戻るの語とキーボード到達・900px 以下) を確かめる。実窓の目視はオーナー確認待ちとして記録する。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 変えたもの
- 着手時判断 (AC#3): 戻るボタンの振る舞いは、オーナーと合意して「ダッシュボード (/) へ移る」にした。サイドバーの表示はルート (currentProjectId の有無) だけから決まり、サイドバー固有の状態を持たない。
- `WorkspaceSidebar` のプロジェクト節を2表示に分けた: プロジェクトを選んでいないときは一覧の全行 (今までの形)。選んでいるときは、選んだプロジェクトの行1つの左に戻るリンクを置き、他の行は出さない。チャット節 (作成の + を含む) は今までどおり選択中だけ出る。行の描画は `renderProjectRow` に1つにまとめ、`aria-current` (チャット表示中は true、詳細では page) と面・枠・帯は TASK-44 の形のまま。
- 戻るは操作ではなく行き先なので、ボタンではなくリンク (`IconButton.withComponent(Link)`、to="/") にして、読み上げへリンクとして届ける (doc-9 §6.8)。名札は `aria-label`「プロジェクト一覧へ戻る」、title も同じ語。畳まれた入口の中では他のリンクと同じく `leaveEntry` で入口を閉じてから移る。
- `icons.tsx` に Lucide 1.48.0 の arrow-left を足し、i18n に `sidebar.backToProjects` (en/ja) を足した。
- 両変種 (サイドバー / 900px 以下の畳まれた入口) は同じ `destinations` を描くので、切り分けは両方に同時に効く。

## 確かめたこと
- 検証環境: Vite の開発サーバー + Playwright 1.62.1 (playwright-core)。Chromium 151 と WebKit 26.5 (どちらも headless。**WKWebView ではない**)。`/api/*` は route が固定の JSON を返した (プロジェクト3・チャット3。リポジトリには足していない)。1280×800 と 800×800。WebKit の Tab は Option+Tab で送った。
- 17 項目が両エンジンで全て通った:
  - AC#1: ダッシュボードでプロジェクト3行だけ。戻るリンク・チャット節・作成の + は無い。
  - AC#2: プロジェクト詳細とチャット画面で、選んだプロジェクトの行1つ + 戻るリンク + チャット3行 + 作成の +。他のプロジェクトの語は nav に出ない。
  - AC#3: 戻るリンクが焦点を受け (Tab の巡回でも届く)、名札「プロジェクト一覧へ戻る」を持ち、Enter でダッシュボードへ移って一覧の全行が戻る。
  - AC#4: 詳細で行の `aria-current="page"`、チャットでプロジェクト行 "true" + チャット行 "page"、選択の帯 (inset 3px の影) が描かれる。800px では畳まれた入口になり、開くと同じ2表示 (一覧 / 1行+戻る+チャット) が出て、行き先や戻るを選ぶと閉じてトリガーの aria-expanded が false に戻る。
- `pnpm check:client`・`pnpm build:client` が通った (Go は変えていない)。

## 残したこと
- AC#5 の実窓 (WKWebView) の目視はオーナーの確認待ち。見てほしい箇所: 戻るリンクの見た目 (アイコンボタンの形) と行との並び、2表示の切り替わり、畳まれた入口の中の戻る。

## 実窓確認の指摘への対応 (2026-09-26、オーナーの注釈画像)
- ヘッダー (SNZ STUDIO + ホーム) を広いサイドバーから削除。畳まれた入口の帯のロゴは残す。`sidebar.home` の語も外した。ダッシュボードへは戻るリンクで届く。
- 行の高さをアイコンボタンに合わせた: `SidebarLink` を flex + `min-height: size.control` (2.4rem) にし、選択行・チャット行・戻る/作成ボタンが同じ 38.4px (1行のとき。折り返す題は伸びる)。
- 左右の余白を狭めた: `SidebarPane` の padding 18px → 12px (`SIDEBAR_PANE_PADDING` として共有)。
- 設定の上の仕切り線を端から端まで (`FullBleedDivider`、pane の padding を負の margin で打ち消す)。
- 設定を歯車だけの `IconButton` (名札・title「設定」) にし、横に小さく `snz studio v0.0.0`。バージョンは package.json の `version` (0.0.0 を追加) を Vite の `define` (`__APP_VERSION__`) で焼き込む。
- 検証: Chromium + WebKit で既存 17 項目全て通過。幾何も計測: 戻る/選択行/チャット行の高さ 38.39px で一致、チャット行の左右 inset 13px、仕切り線の inset 1px (枠のみ)、ヘッダー無し、歯車はアイコンのみ、バージョン表記あり。`pnpm check:client`・`build:client` 通過。

## 実窓確認の指摘への対応 その2 (2026-09-26)
- フッターの上下余白を詰めた: pane の flex gap (16px) と下 padding (12px) が効いていたのを、`SettingsFooter` の負の margin で上 8px・下 6px に (指示どおり)。Chromium で実測 上 8px / 下 6px、フッター域 66px → 52.4px。既存 17 項目も通過。check/build 通過。

## レビュー第3ラウンドの [P3] への対応 (2026-09-26)
- 負の margin (上 8px・下 6px) を広いサイドバーだけに限定した (`PaneFooterFit`)。畳まれた入口のパネルは自前の 18px padding のまま (指摘の対象外)。広いサイドバーの実測は変わらず 上 8px / 下 6px。check/build と 17 項目通過。P1/P2 なしの 3 ラウンド目で回数上限のため、この修正の再レビューは掛けていない。
<!-- SECTION:NOTES:END -->
