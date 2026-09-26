---
id: TASK-54
title: 'チャット画面: 見出し帯・サイドバー・編成パネルの余白と細部を直す'
status: In Review
assignee: []
created_date: '2026-09-25 22:12'
updated_date: '2026-09-26 05:48'
labels:
  - design
dependencies:
  - TASK-47
ordinal: 54000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
TASK-47 (#47) の後のオーナーの実窓の確認 (2026-09-26) で出た、チャット画面と複数エージェントチャット画面の細部の指摘。TASK-47 は共通仕様の本適用の範囲に留め、画面の見た目の直しはこのタスクに分けた。入力欄の組み直しは TASK-55、編成パネルの区分けは TASK-56 が受け持つ。

指摘 (オーナーの注記つきの画面から):
- 見出し帯
  - 複数エージェントチャットの題名の前にも、チャットの種類を表す図形を置く。オーナーの指定は user-group。Lucide の users か users-round か、どちらに当たるかは着手時に確かめる。単独チャットは messages-square を置いている。左サイドバーの 👥 の絵文字も同じ図形に揃えるかは、着手時に確かめる。
  - 題名と印 (多人数会話・プロジェクト名) の行が、見出し帯の天地の中央にない。今は上 18px、下 35px。オーナーは「バランス的に悩ましい部分もある」としている。
  - 右のアイコンボタンは「マージン取り過ぎ。4px とかでよい」。ボタン同士の間隔 (今は 12px) を指すと読んだ。見出し帯の上の余白 (18px) を指す可能性もあるので、着手時に確かめる。
- 左サイドバー: 区画の見出し (「プロジェクト」「チャット」、今は 11px の大文字間隔) が小さすぎないか。
- 編成パネル (右サイドバー)
  - 余白を取りすぎている。特に参加者のカードの中が窮屈 (区画 18px + カード 16px + 参加者カード 16px の入れ子)。
  - プリセットの区画
    - 説明文 (「発言が 1 件も無いあいだだけ選べます…」) を、選択欄の「プリセット」ラベルの横の (?) の補助表示に移してよい。
    - 「プリセットの JSON を読み込む」のボタンが2行に折り返す。折り返さないようにし、ファイルを選ぶ操作なので語を「プリセット JSON を読み込む…」にする。
    - 補助の説明文 (「参加者は接続先なしで作られ…」) の文字が大きすぎる。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 複数エージェントチャットの見出し帯に、チャットの種類を表す図形がある
- [x] #2 見出し帯の題名の行と右のボタンの余白が、オーナーの確認した値になっている
- [ ] #3 左サイドバーの区画の見出しの大きさが、オーナーの確認した値になっている
- [x] #4 編成パネルの余白を詰め、参加者のカードの入力欄の幅が今より広い
- [x] #5 プリセットの説明が補助表示に入り、読み込みのボタンが折り返さず「…」で終わる語を持ち、補助の説明文が補助文の大きさになる
- [ ] #6 変えた箇所の比を4配色で測り、測定環境 (snz-design doc-5 §5.3) とともに Implementation Notes に記録している。実窓 (WKWebView) の目視はオーナーの確認を記録する
- [x] #7 `pnpm check:client`・`pnpm build:client` が通る
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. icons.tsx に Lucide main の user-group / rotate-cw-fading-clock を写す (raw SVG から、path 無改変)。
2. 見出し帯: PaneHeader を align-items:center + padding 4px 20px (+min-height で全画面の帯高を揃える) に。多人数会話の題名の前に user-group (18px)。両チャットの ⏱️ 絵文字を図形に置換。右ボタン列の gap 12→8px。
3. サイドバー: SidebarSectionLabel 11→12px。renderChatTitle の 👥/⏱️ を図形に置換 (VisuallyHidden で語を添える)。プロジェクト行に folder。
4. 編成パネル: InspectorPane padding 18→12px、パネル内 Card と参加者カードを 12px に (パネル局所、全体の Card は変えない)。
5. プリセット区画: applyNote を「プリセット」ラベル横の (?) Hint へ移動 (HintedField を共有化)。読み込みボタンを nowrap + 語を「プリセット JSON を読み込む…」に。preset.hint を MetaText (12px) に。
6. 検証: pnpm check:client / build:client、Playwright (Chromium+WebKit、/api route 固定) で幾何実測と4配色の比、doc-5 §5.3 形式で記録。実窓目視はオーナー確認待ちとして残す。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 着手時判断 (2026-09-26、ユーザーの指示と私の決定)
- 右のアイコンボタンの「マージン取り過ぎ」は見出し帯の上の余白 (18px) を指す (ユーザー確認)。4px にした。バランスでボタン同士の間隔も 12px → 8px に詰めた (実窓で要確認)。
- 図形はオーナー指定の Lucide user-group (1.48.0、ユーザーが URL で特定)。users / users-round ではない。
- 左サイドバーの 👥 も同じ図形に揃えた (ユーザー指示)。⏱️ は Lucide rotate-cw-fading-clock に置き換え (ユーザー提案)。プロジェクト行には folder を付けた: プロジェクト詳細の見出し帯が既に folder を出しており、「画面種の図形を行にも映す」形で一貫するため。
- 区画の見出しは 11px → 12px にした (値の指定は無かったため私の選択。実窓で要確認)。

## 変えたもの
- 見出し帯 (`PaneHeader`): padding 18px 20px → 4px 20px、align-items flex-start → center (題名の行が天地中央に)、min-height = control (2.4rem) + 8px + 枠 1px で全画面の帯高を揃えた (ダッシュボードの題名だけの帯も同じ高さ)。両チャット画面の右ボタン列の gap を 8px に。
- 複数エージェントチャットの見出し帯に user-group (18px) を追加。両画面の題名の ⏱️ 絵文字を rotate-cw-fading-clock (18px) + VisuallyHidden「一時チャット」に置換。
- サイドバー: `SidebarSectionLabel` 11px → 12px。チャット行の 👥/⏱️ を user-group / rotate-cw-fading-clock + VisuallyHidden (多人数会話 / 一時チャット) に、プロジェクト行に folder を追加 (`renderChatTitle` を Row + 図形に組み替え)。プロジェクト詳細のチャット一覧の ⏱️ も同じ図形に揃えた (絵文字の残りを無くすため。タスクの列挙外だが同種の直し)。
- 編成パネル: `InspectorPane` padding 18px → 12px (チャットのインスペクタも同じ部品なので同時に詰まる)、パネル内の全カードをパネル局所の `PanelCard` (padding 12px) に。共有の `Card` (16px) は他画面のまま。
- プリセット区画: 説明文 (preset.applyNote) を選択欄の「プリセット」ラベル横の (?) 補助表示に移した (`PresetPicker` に labelHint を追加、`HintedField`/`HintRow` を `Hint.tsx` へ移して共有)。読み込みボタンを white-space: nowrap にし、語を「プリセット JSON を読み込む…」(en: Load preset JSON…) に。補助の説明文 (preset.hint) を Subtle → MetaText (12px) に。
- `icons.tsx` に Lucide 1.48.0 の user-group / rotate-cw-fading-clock を追加 (raw SVG から path 無改変。main と 1.48.0 で同一であることを diff で確認)。

## 確かめたこと
測定環境 (snz-design doc-5 §5.3): アプリ snz_studio、main `1a07c85` 起点の作業ツリー (この PR の実装)。共通仕様 snz-design tokens 0.1.1 (写しは変更なし)。OS macOS 26.6.0 (Darwin 25.6.0)。描画エンジン Playwright 1.62.1 の Chromium 151.0.7922.34 / WebKit 26.5 (headless、WKWebView ではない)。倍率 1、1440×800 (1280 では従来どおり2つ目のバッジが折り返すのを確認。帯は折り返し分だけ伸びる)。入力は Playwright の合成事象。配色は標準 Light/Dark・Solarized Light/Dark の4配色。表示言語 ja。測定日 2026-09-26。画面は Vite 開発サーバー、/api/* は route の固定 JSON (リポジトリには足していない)。
- 幾何 (Chromium / WebKit 一致): 帯の padding 上下 4px、帯高 47.4px、題名の中心と帯の中心の差 -0.5px (天地中央)、右ボタンの間隔 8px×3。区画見出し 12px。編成パネル padding 12px、パネル内カード全て 12px、参加者カードの入力欄 262px (変更前は片側 50px の入れ子で 234px → +28px)。読み込みボタンは1行 (内容高 26px)、語尾「…」。補助説明文 12px。一時チャットの見出し帯は図形2つ + 隠し語「一時チャット」。プロジェクト詳細のチャット一覧の一時行も図形 + 隠し語で1行 (24px)。
- 説明文の移動: 「発言が 1 件も無いあいだ…」は role=tooltip の補助表示の中だけに存在し (visibility hidden)、(?) の押下で visible になる (aria-expanded=true)。段落としては消えた。
- 4配色の比 (最小値。文字は 4.5:1、図形は 3:1 が基準):
  - 区画見出し (muted / サイドバー面): 6.55 / 6.72 / 5.73 / 4.86 (標準L / 標準D / SolL / SolD)
  - サイドバー行の図形 (ink / サイドバー面): 14.42 / 11.44 / 12.05 / 10.61
  - 見出し帯の図形 (ink / 帯のグラデ両端の悪い方): 14.42 / 11.44 / 12.05 / 10.61
  - プリセットの補助説明 12px (muted / カード面): 6.05 / 7.28 / 5.39 / 5.26
- `pnpm check:client`・`pnpm build:client` 通過 (Go は変えていない)。

## 残したこと
- AC#3: 区画見出し 12px は私の選択。オーナーの実窓の確認待ち。
- AC#6: 実窓 (WKWebView) の目視はオーナーの確認待ち。見てほしい箇所: 見出し帯の高さと題名の天地中央、ボタン間隔 8px のバランス、区画見出し 12px、user-group / 時計 / folder の図形の見え方、編成パネルの詰まり具合、プリセットの (?) と読み込みボタン。

## レビュー第1ラウンドの [P3] への対応 (2026-09-26)
- 共有化した HintedField が participants 名前空間の i18n キー (participants.hintAbout) を使っていた件: キーを hint.about にリネームし、両ロケールと参照2箇所 (Hint.tsx / ParticipantPanel.tsx) を更新。check/build 通過。

## 実窓確認の指摘への対応 (2026-09-26、オーナー)
- AC#6 の実窓 (WKWebView) の目視: 見出し帯の高さ・題名の天地中央・ボタン間隔 8px・3種の図形 (user-group / rotate-cw-fading-clock / folder)・編成パネルの詰まり・プリセットの (?) と読み込みボタンは問題なしとオーナーが確認。
- AC#3 の区画見出し: 12px (私の選択、オーナー指定ではない) は「まだ小さすぎ。右パネルと比べて不自然」→ 共有トークン font.sizeSmall (14px) に上げた。右パネルの見出し階層 (SectionTitle 18px / SubsectionTitle 15px) の下に収まる値。色は変えていないので比は既記録のとおり (muted / サイドバー面 4.86〜6.72)。check/build 通過。実窓の再確認待ち。

## 実窓確認の指摘への対応 その2 (2026-09-26、オーナー)
- 区画見出し 14px は「多少マシ」だが、見出しとしてマークアップされていない点と大きさの指摘 → SidebarSectionLabel を div のミニラベル (大文字トラッキング) から h2 の見出しに変え、16px / weight 500 (オーナー指定) にした。letter-spacing は見出しには不釣り合いなので外した。色は muted のまま (比は既記録 4.86〜6.72 のとおり)。マークアップが div だったのは初期実装の名残で意図は無い。check/build 通過。実窓の再確認待ち。
<!-- SECTION:NOTES:END -->
