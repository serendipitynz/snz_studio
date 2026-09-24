---
id: TASK-42
title: '共通デザイン: snz-design の共通トークンを試験適用する'
status: In Progress
assignee: []
created_date: '2026-09-24 06:30'
updated_date: '2026-09-24 06:52'
labels:
  - design
dependencies: []
references:
  - ../snz-design
ordinal: 42000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
snz-design (5 アプリの共通デザイン) の TASK-13 が追跡する試験適用の、snz_studio 側の実装タスク。
Emotion と既存のテーマ設定 (`snz.theme.family` / `snz.theme.mode`) のまま、共通の 4 配色 (標準 Light / 標準 Dark / Solarized Light / Solarized Dark) を描けるかを確かめる。

試験ブランチ `trial/snz-design-tokens` で行い、PR は開かない (2026-09-24 オーナー判断)。結果を見て、本適用 (snz-design の TASK-20) で何を取り込むかを決める。

参照する snz-design の文書: doc-7 (テーマ切替と既存設定移行の共通仕様) §6.2・§6.4、doc-10 (共通デザイントークン) §7、doc-4 §5.3 (会話の保持条件)。snz-design は兄弟ディレクトリ `../snz-design` にある。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 配色系統に 標準 が加わり、標準 と Solarized の色値が snz-design の共通トークンから来ている
- [x] #2 2 つのキーがどちらも無いときは 標準 + OS追従、片方だけ無いときは現状の既定 (Solarized / Light) で描かれ、保存値を書き換えない
- [x] #3 localStorage へ書けないときも選んだ配色がその起動の間は効く
- [x] #4 設定モーダルと会話 (user と assistant の描き分け・生成中の発言) を 4 配色で確かめ、差異を snz-design へ返している
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
試験ブランチ `trial/snz-design-tokens` (main `29dbdea` から分岐) で実施。snz-design `9584b5b` の `tokens/dist/snz-tokens.ts` を `frontend/src/styles/themes/snz-tokens.ts` へ写した。

## 変えたもの
- `themes/snz.ts` (新規): 共通の 29 役割を `ThemeTokens` (約 55 値) へ写す変換。共通の採用案は不透明な面でグラデーションを持たないので、`*From` / `*To` の組は同じ値にし、`buildTokens` が rgb から作っていた半透明の面は共通の淡い面 (`accentSoft`・`dangerSoft`・`warnSoft`) と区切り線に置き換えた。
- `themes/standard.ts` (新規) と `themes/solarized.ts`: 標準 と Solarized をこの変換から作る。旧 Solarized (teal のアクセント・半透明の面) は git の履歴に残る。
- `ThemeController.tsx`: 2 つのキーがどちらも無いときだけ 標準 + OS追従、片方だけ無いときはそのキーの旧既定 (solarized / light)、収録外の値は 標準 + OS追従 で描き、保存値は書き換えない (snz-design doc-7 §6.2・§5.1)。`localStorage` の読み書きを try/catch し、書けなくてもその起動の間は選択が効く。
- 設定モーダルの配色系統に「標準」を足した (`settings.themeStandard`)。

## 確かめたこと (2026-09-24、macOS 26.6.2・Chromium 152・vite の開発サーバー、API はブラウザ内のモック)
- `pnpm run check:client` 成功。
- 保存値ごとの描画 (OS が明るい状態): 両方無し → 標準 Light / catppuccin だけ → Catppuccin Light / dark だけ → Solarized Dark / solarized + light → Solarized Light / github + auto → GitHub Light / bogus + light・solarized + bogus → 標準 Light。どの場合も保存値は変わらなかった。OS が暗い状態では、両方無し → 標準 Dark、solarized + auto → Solarized Dark、catppuccin だけ → Catppuccin Light のまま。
- `setItem` が例外を投げる状態で配色を選ぶと、画面は選んだ配色になり、例外は画面へ抜けなかった。
- 会話の描き分け (4 配色): assistant の枠 対 面 は 7.52 / 6.63 / 5.41 / 5.88、本文は user 10.61〜13.39・assistant 9.65〜11.53。面どうしの差は小さい (user 対 assistant 1.03〜1.29) ので、区別は assistant のアクセントの枠と user の右寄せが担う。

## 残したこと
- キーボードでの焦点の表示は測れていない (内蔵ブラウザのペインが表示されておらず、キー入力を送れなかった)。`ui.tsx` に焦点の描き方が無いことは既知 (snz-design doc-13 §10)。
- 角丸は `ui.tsx` の大半が数値を直書きしており、テーマの `radius` を読むのは 1 か所だけ。共通の 10px は画面にほぼ反映されない。
- 保存できなかったことを利用者へ伝える表示は無い。
- Wails の WebView (WKWebView) では見ていない。

snz-design へ返した差異は snz-design の doc-14 (Web系4アプリの試験適用の結果) にある。
<!-- SECTION:NOTES:END -->
