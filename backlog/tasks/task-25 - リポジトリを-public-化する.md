---
id: TASK-25
title: リポジトリを public 化する
status: To Do
assignee: []
created_date: '2026-09-20 02:31'
labels: []
milestone: m-1
dependencies: []
references:
  - internal/search/model.go
  - internal/search/segmenter.go
  - .github/workflows/build.yml
  - docs/HANDOFF.md
  - README.md
type: chore
ordinal: 25000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
このリポジトリを public に切り替える。公開前提で足りないのはライセンス周りと、非公開前提で
書かれた記述の整理。秘密情報の混入は履歴も含めて調査済みで、対応不要だった。

調査で問題なしと確認した範囲 (2026-09-20):

- `.env` は追跡外。履歴上 `.env` 系で追加されたのは `.env.example` のみ。API キー様の
  文字列 (sk-/ghp_/AKIA/Bearer) も追跡ファイルに無し
- 全 181 commit の author が個人メール。社内固有名の混入なし
- `data/` と `sample-docs/` (実データ) は ignore 済み。個人の絶対パスも 0 件
- `.git` は 11MB、最大 blob は `internal/search/testdata/golden.json` (1.2MB)。
  GGUF・サイドカーバイナリは履歴に入っていない
- API は `127.0.0.1` バインドのみ (prod はエフェメラルポート)
- `build/appicon.png` は自作 (生成 AI 不使用) のためライセンス確認不要
- `wails.json` の author email は公開して問題ないものとして据え置く
- ローカルのみの除外設定 (`.git/info/exclude` の `_sandbox`、グローバル ignore の
  `.claude/settings.local.json`) は先行して `.gitignore` へ移済み

やること:

1. LICENSE を置く。public は「読めるが全権利留保」が既定なので、ライセンスが無いと
   fork も再利用も建前上できない
2. THIRD_PARTY_NOTICES を用意する。`internal/search/model.go` と `segmenter.go` は
   TinySegmenter 0.2 (Taku Kudo) のモデル同梱＋アルゴリズム移植で、上流は修正 BSD ＝
   著作権表示の保持が再頒布条件。現状はソースコメントの出典 1 行しかない。
   なお `node_modules/tiny-segmenter/LICENSE` は npm ラッパー作者の MIT で上流とは別物なので、
   根拠として引かないこと。配布物 (.dmg / NSIS) が同梱する llama.cpp (MIT) と
   ruri-v3-30m (Apache-2.0、NOTICE 必要) も同じファイルにまとめる
3. `docs/HANDOFF.md` (76KB の開発ログ) を削除する。ただしコードコメントと設計文書から
   節番号で参照されているので、参照側の後始末が要る:
   `app.go:233` / `main.go:10` / `internal/httpapi/handlers.go:1042` (§4) /
   `internal/httpapi/server.go:79` (§3 Phase 6) / `internal/httpapi/handlers_test.go:24` (§5) /
   `docs/local-generation-design.md:146,156`。参照が指す内容を残すか、参照ごと落とすかは
   1 件ずつ判断する
4. `.github/workflows/build.yml` 冒頭の "this is a private repo, GitHub-hosted minutes are
   billed" というコメントが public では不正確になるので直す

公開後の効果として TASK-8 (build ワークフローの実走確認) のコスト障壁が消える。public
リポジトリでは GitHub-hosted 標準ランナーの分数が課金対象外で、private で 10 倍換算だった
macOS ランナーも同じ。TASK-8 は public 化直後に着手できる。

セキュリティ上の留意点: 現在の workflow は `workflow_dispatch` のみで secrets 未使用の
ため、fork からの窃取経路は無い。将来 Apple / Windows の署名 secrets を入れるときに、
fork の PR から secrets に届くトリガ (`pull_request_target` 等) を足さないこと。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 LICENSE がリポジトリ直下にあり、README からライセンスが分かる
- [ ] #2 THIRD_PARTY_NOTICES に TinySegmenter (Taku Kudo・修正 BSD) の著作権表示とライセンス全文があり、internal/search/model.go と segmenter.go が由来であることが読み取れる
- [ ] #3 同じファイルに、配布物へ同梱する llama.cpp (MIT) と ruri-v3-30m (Apache-2.0) の表記がある
- [ ] #4 docs/HANDOFF.md が削除され、参照していた 6 箇所 (app.go / main.go / handlers.go / server.go / handlers_test.go / local-generation-design.md) に宙に浮いた参照が残っていない
- [ ] #5 .github/workflows/build.yml のトリガ方針のコメントが public リポジトリの実態と一致している
- [ ] #6 GitHub 上でリポジトリが public になっている
<!-- AC:END -->
