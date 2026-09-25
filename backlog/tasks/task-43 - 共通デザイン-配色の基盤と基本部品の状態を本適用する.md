---
id: TASK-43
title: '共通デザイン: 配色の基盤と基本部品の状態を本適用する'
status: In Review
assignee: []
created_date: '2026-09-24 20:11'
updated_date: '2026-09-25 02:12'
labels:
  - design
dependencies: []
references:
  - ../snz-design
ordinal: 43000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
snz-design の TASK-20 (snz_studio への共通デザインの本適用) が追跡する実装タスクの1本目。試験 (TASK-42) の変更を main へ取り込み、配色の写しを tokens-v0.1.0 に揃え、`styles/ui.tsx` の基本部品に焦点・hover・押下・無効の描き方と共通の角丸を当てる。画面ごとの作業は TASK-44〜48 が持つ。

参照する snz-design の文書: doc-16 (適用ガイド) §6・§7.4・§11、doc-8 (基本部品) §5.1・§5.3・§5.4・§6.1〜§6.3、doc-7 §6.2。適用の結果は snz-design の「snz_studioの共通デザイン適用記録」に記録する。snz-design は兄弟ディレクトリ `../snz-design` にある。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 試験の変更 (標準 と Solarized を共通の値で描く、2つのキーの読み替えと両方書き、保存できないときもその起動中は選択が効く、TabFocusesLinks) が入り、写し `frontend/src/styles/themes/snz-tokens.ts` が `vendor.mjs verify` で 0.1.1 と照合される
- [x] #2 ボタン・アイコンのみボタン・入力欄・選択欄・サイドバーの項目・最新へ戻るボタンが、キーボードで焦点を受けたときに外側へ焦点の枠 (2px・offset 1px・焦点の色) を描き、ポインタで押したときは描かない (doc-8 §5.1)
- [x] #3 ボタンの3変種が doc-8 §6.1 の主操作・通常・破壊的操作の面・語・輪郭で描かれ、hover と押下で面を動かし、位置は動かさない
- [x] #4 無効の部品が破線の輪郭と不透明度 0.45 で描かれ、カーソルを not-allowed に変えない (doc-8 §5.4)
- [x] #5 標準 と Solarized の面と操作部品の角丸が共通寸法 (10px / 6px) から来て、追加の5配色系統は各自の値を持つ
- [x] #6 AGENTS.md と AGENTS.ja.md に共通デザインのガイドと適用記録への入口がある (doc-16 §11)
- [x] #7 `pnpm check:client`・`pnpm build:client`・`go test ./...` が通る
- [x] #8 入れ子のパネルの面 (区画) と、その中の行の面、押下の段を、snz-design の tokens 0.1.1 (snz-design TASK-27) から写して当てている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 試験ブランチ trial/snz-design-tokens の5コミット (TASK-42 の台帳を含む) を main 起点の作業ブランチへ cherry-pick する。TASK-42 は試験として Done にする。
2. 写し snz-tokens.ts を `vendor.mjs copy 0.1.0` で置き直し、verify で照合する (doc-15 §8。snz_studio の利用側の作業は無し)。
3. ThemeTokens に基本部品の状態が読む役割を足す: accentHover・onAccent・surfaceHover・focus・radiusSm。標準 と Solarized は themes/snz.ts が共通の値から写し、追加の5配色系統は buildTokens が各自の値から導く (onAccent はアクセントとの比が最大の候補、accentHover は color-mix、focus はアクセント)。
4. ui.tsx: Button の変種を doc-8 §6.1 の名前へ改める (solid→primary、ghost→normal、warm→danger。既定は今の見た目を保つ primary)。hover・押下は面だけを動かす。ボタン・アイコンのみボタン・入力欄・選択欄・サイドバーの項目・RouterLink・最新へ戻るボタンに :focus-visible の外側の枠と、:disabled / aria-disabled の破線と 0.45 を当てる。焦点と無効の寸法は snz-tokens の共通寸法から読む。
5. 角丸: 面 (区画・カード・モーダル・会話・ドロップゾーン・ボタン) は theme.radius、入力欄とサイドバーの項目は theme.radiusSm。丸い部品 (バッジ・アイコンのみボタン・接続の点) は丸のまま。追加の5配色系統は 18px / 12px。
6. AGENTS.md と AGENTS.ja.md に共通デザインの入口の1節を足す。
7. check:client・build:client・go test を通し、vite の開発サーバーで4配色の焦点・hover・無効の描画と比を測る。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
試験ブランチ `trial/snz-design-tokens` の 5 コミット (TASK-42 の台帳を含む) を main `29dbdea` から切ったこのブランチへ cherry-pick し、その上に本適用の基盤を足した。写しは snz-design のタグ `tokens-v0.1.0` から `vendor.mjs copy` で置き直した (試験の写しは手で足した注記があり照合を通らなかった。値は同じ)。

## 変えたもの
- `ThemeTokens` に基本部品の状態が読む 5 値を足した: `accentHover`・`onAccent`・`surfaceHover`・`focus`・`radiusSm`。標準 と Solarized は `themes/snz.ts` が共通の役割から写す。追加の 5 配色系統は `buildTokens` が導く: `onAccent` は背景・本文・白・黒のうちアクセントとの比が最大のもの (これらの配色系統は主ボタンをアクセントで塗る前提で作られていないため)、`accentHover` は `color-mix` でアクセントを本文色へ 14% 寄せたもの、`focus` はアクセント、角丸は 18px / 12px。
- `ui.tsx`: ボタンの変種を doc-8 §6.1 の名前へ改めた (`solid`→`primary`、`ghost`→`normal`、`warm`→`danger`)。既定は今までの無印のボタンと同じ `primary`。1画面に主操作を1つにする選び分けは画面ごとのタスク (TASK-44〜48) が持つ。複数エージェントチャットの自動進行の停止は破壊的操作ではないので `normal` にした。破壊的操作は面を塗らず、語と輪郭だけを危険色にする。
- 焦点の枠 (`:focus-visible` で外側に 2px・offset 1px・焦点の色) を、ボタン・アイコンのみボタン・入力欄 3 種・サイドバーのリンクとボタン・`RouterLink`・最新へ戻るボタンに当てた。無効 (`:disabled` と `aria-disabled="true"`) は破線・不透明度 0.45・既定のカーソル。hover と押下は面だけを動かし、無効の間は動かさない。
- 角丸: 区画・カード・一覧の項目・会話・入力欄の箱・ドロップゾーン・モーダル・ボタン・メニュー (チャットの種類) は `theme.radius`、入力欄・サイドバーの項目・メニューの項目・補助表示は `theme.radiusSm`。丸い部品 (バッジ・アイコンのみボタン・接続の点・最新へ戻るボタン) は丸のまま。
- AGENTS.md と AGENTS.ja.md に「共通デザイン (snz-design)」の節を足した (snz-design doc-16 §11)。

## 確かめたこと
測定環境 (snz-design doc-5 §5.3): macOS 26.6.2、Chromium 152.0.7977.130 (Claude の内蔵ブラウザ)、倍率 1、1280×800、ポインタと物理キーボード (内蔵ブラウザから Tab を送った)、表示言語 ja、2026-09-25。画面は Vite の開発サーバーで出し、`/api/*` は scratch の設定の中間処理が固定の JSON を返した (リポジトリには足していない)。**WKWebView (実窓) では見ていない。** 比は描かれた色 (半透明は下の面と合成) から WCAG 2.x の式で求めた。

- `pnpm check:client`・`pnpm build:client`・`go test ./...`・`go vet .`・`go build ./...` が通った。
- `vendor.mjs verify frontend/src/styles/themes/snz-tokens.ts` → tokens-v0.1.0 の `tokens/dist/snz-tokens.ts` と一致。
- 保存値の読み方 (OS は明るい側): 両方無し → 標準 Light / `mode=dark` だけ → Solarized Dark / `family=catppuccin` だけ → Catppuccin Light / solarized + light → Solarized Light / bogus + light・solarized + bogus → 標準 Light。どれも保存値は変わらなかった。`setItem` が例外を投げる状態で設定モーダルから明暗を Dark に選ぶと 標準 Dark になり、保存値は無いまま、例外は画面へ抜けなかった。
- 焦点: ダッシュボードで Tab を 12 回送り、止まったすべての要素 (リンク・入力欄・複数行入力・ボタン・サイドバーのリンクとボタン) で `:focus-visible` が真、`outline` が 2px solid 焦点の色、offset 1px だった。ボタンをポインタで押した直後は `:focus-visible` が偽で枠は無かった。内蔵ブラウザの画面の記録には焦点の枠が描かれなかったので、描画ではなく算出された値で確かめた。
- 無効: ボタン (主操作)・サイドバーのボタン・入力欄・複数行入力に `disabled` を置くと、輪郭が dashed、不透明度 0.45、カーソルが default だった。
- 角丸: 標準 でボタン 10px・区画 10px・入力欄 6px・サイドバーの項目 6px。Catppuccin でボタン 18px・入力欄 12px。

プロジェクト詳細の画面での比 (標準 Light / 標準 Dark / Solarized Light / Solarized Dark):
| 測定点 | 比 |
|---|---|
| 主操作の語 対 面 | 7.52 / 7.91 / 5.41 / 6.79 |
| 主操作の語 対 面 (hover) | 9.73 / 9.88 / 7.26 / 8.40 |
| 通常のボタンの語 | 14.42 / 11.44 / 12.05 / 10.61 |
| 通常のボタンの輪郭 対 周りの面 | 4.05 / 3.83 / 4.13 / 3.26 |
| 破壊的操作の語 (輪郭も同じ色) | 6.78 / 6.16 / 6.17 / 5.39 |
| アイコンのみボタンの輪郭 対 周りの面 | 4.05 / 3.83 / 4.13 / 3.26 |
| 入力欄の輪郭 対 周りの面 | 3.42 / 4.49 / 3.64 / 3.76 |
| 選択欄の輪郭 対 周りの面 | 3.42 / 4.49 / 3.64 / 3.76 |
| 入力欄の語 | 14.42 / 11.44 / 12.05 / 10.61 |
| 焦点の枠 対 カード / 区画 / サイドバー | 6.17・7.31・7.31 / 7.88・6.73・6.73 / 4.76・5.41・5.41 / 6.79・5.88・5.88 |

Catppuccin Latte では主操作が黒の語 対 teal の面で 5.61、焦点の枠 対 カードで 3.68 (追加の配色系統は snz-design doc-7 §6.3 で測定の対象外。描けていることの確認のみ)。

## 残したこと
- WKWebView の実窓での見え方 (焦点の枠の描画・選択欄の輪郭に著者の指定が効くか) はオーナーの確認を待つ。
- サイドバーの先頭の行とダッシュボードのカードで焦点の枠が切れる・隠れる件は、包む箱の側の問題なので TASK-44・TASK-45 で直す。
- 無効の部品が焦点を受けて理由を語で持つこと (doc-8 §5.4)・処理中のボタンの語と幅・1画面に主操作1つは、呼ぶ側の画面ごとに決まるので TASK-44〜48 が持つ。
- チャットの種類のメニューの項目の焦点の描き方は TASK-44 (doc-9 §6.11) で揃える。

## オーナーの実画面の確認と、その後の方針 (2026-09-25)
- 区画 (`Card`) とプロジェクトの行 (`Item`) が同じ面で見分けられず、行の hover も面が変わらない。原因は2つ。この変換が区画と行を両方 `surface-alt` に写したこと (Solarized では地とも同じ値)、共通の `surface-hover` が `surface` から1段の値で `surface-alt` の上ではほぼ差が出ないこと (Solarized Light で 1.02)。
- 共通の面の段を snz-design TASK-27 で足す (区画の面の役割、hover・押下の相対規則。tokens 0.2.0)。この PR はそれを待って写しを置き直し、区画と行と hover・押下を当て直す (AC#8)。
- 区画のボックスの影を消した。変換が `shadow` を `shadow.modal` に写していたので、ボックスにモーダルの影が付いていた。モーダルの影は残す。ボックス間の隙間を 14px から共通の `space.sm` (0.55rem) に狭めた。
- ダッシュボードに置く要素の見直しは別タスク (TASK-49)。

## tokens 0.1.1 の取り込み (2026-09-25)
- 写しを tokens-v0.1.1 から置き直した (`vendor.mjs verify` で一致)。snz-design TASK-27 で、入れ子のパネルの面 (`surface-nested`) と押下の段 (`surface-pressed`・`accent-pressed`) が足された版。
- `ThemeTokens` に `surfaceNested`・`surfaceItem`・`surfacePressed`・`accentPressed` を足した。標準 と Solarized は、区画 (`Card`) を `surface-nested` に、区画の中の行 (`Item`) を `surface` に写す (snz-design doc-9 §6.3: 行を区画より暗い面に置くと hover が効かない)。`surfaceElevate` は会話の user の面と入力欄の箱にも使っているので、行には専用のキー `surfaceItem` を立てた。追加の5配色系統は、区画を `surfaceCardFaint`、行を `surfaceElevate` のままにして見た目を変えていない。
- 押下: 主操作は `accentPressed`、通常・破壊的・アイコンのみボタン・サイドバーのボタン・最新へ戻るボタンは `surfacePressed`。hover と押下を別の面にした。チャットの種類のメニューの項目の hover を `surfaceCardFaint` から `surfaceHover` に替えた (`surfaceCardFaint` は区画の面と同じ値になったため)。
- 確かめたこと (Chromium 152、macOS 26.6.2、1280×800、固定データの API): ダッシュボードで ボックス / 区画 / 行 の面は Solarized Light #fdf6e3 / #f6efdc / #fdf6e3、Solarized Dark #073642 / #04303c / #073642、標準 Light #ffffff / #f4f6f8 / #ffffff、標準 Dark #232833 / #1d222a / #232833。ボックス 対 区画 と 区画 対 行 はどちらも 1.06 / 1.08 / 1.08 / 1.08 (Sol L / Sol D / 標準 L / 標準 D)、行の輪郭 対 区画 は 1.25 / 1.29 / 1.28 / 1.42、行の語は 12.05 / 10.61 / 14.42 / 11.44、区画の見出しは 11.33 / 11.48 / 13.31 / 12.38。`:active` の規則の面は 標準 Dark で #323847 (`surface-pressed`) と #accdf1 (`accent-pressed`)。`pnpm check:client`・`pnpm build:client`・`go test ./...` が通った。
- 行の hover は、行が押せる部品になる画面のタスク (TASK-45・TASK-46) で当てる。
<!-- SECTION:NOTES:END -->
