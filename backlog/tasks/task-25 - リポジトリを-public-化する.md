---
id: TASK-25
title: リポジトリを public 化する
status: In Review
assignee: []
created_date: '2026-09-20 02:31'
updated_date: '2026-09-20 11:26'
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
- [x] #1 LICENSE がリポジトリ直下にあり、README からライセンスが分かる
- [x] #2 THIRD_PARTY_NOTICES に TinySegmenter (Taku Kudo・修正 BSD) の著作権表示とライセンス全文があり、internal/search/model.go と segmenter.go が由来であることが読み取れる
- [x] #3 同じファイルに、配布物へ同梱する llama.cpp (MIT) と ruri-v3-30m (Apache-2.0) の表記がある
- [x] #4 docs/HANDOFF.md が削除され、参照していた 6 箇所 (app.go / main.go / handlers.go / server.go / handlers_test.go / local-generation-design.md) に宙に浮いた参照が残っていない
- [x] #5 .github/workflows/build.yml のトリガ方針のコメントが public リポジトリの実態と一致している
- [ ] #6 GitHub 上でリポジトリが public になっている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. LICENSE (MIT, Copyright (c) 2026 Takuya Otani) を直下に置き、README に「ライセンス」節を追加して LICENSE と THIRD_PARTY_NOTICES へ誘導する
2. THIRD_PARTY_NOTICES.md を作成: (a) TinySegmenter 0.2 (c) 2008 Taku Kudo・修正 BSD 全文 (chasen.org の LICENCE.txt 原文) と、internal/search/model.go (モデル同梱) / segmenter.go (アルゴリズム移植) が由来である旨、(b) 配布物同梱の llama.cpp (MIT, b9437), (c) ruri-v3-30m (Apache-2.0, cl-nagoya/ruri-v3-30m @24899e5) の NOTICE
3. docs/HANDOFF.md を削除し、参照 6 箇所を 1 件ずつ処理:
   - app.go:233 追跡先の括弧を落とす (follow-up である事実は残す)
   - main.go:8-10 「Phase 1 scaffold / 後続 Phase で結線」は移行期の記述で現状と不一致。2 文とも削除
   - handlers.go:1042 「(HANDOFF §4)」を落とす (理由は文中に既出)
   - server.go:79 依存順は文中に列挙済みなので出典だけ落とす
   - handlers_test.go:24 「HANDOFF §5 の」を落とす
   - local-generation-design.md:146 の括弧注記と :156 の「全体の経緯」リンク行を削除
4. .github/workflows/build.yml の trigger コメントを public の実態に合わせる (標準ランナーは public では無料。手動のみに保つ理由を別の根拠で書き直す)
5. go build/vet/test + gofmt、pnpm check:client で検証
6. AC#6 (GitHub 上の public 化) は PR マージ後に確認を取ってから実施

7. (着手後の追加依頼) README を英語デフォルトにし、既存の日本語版を README.ja.md へ退避する。AGENTS.md / AGENTS.ja.md・docs/current-spec.md / .ja.md と同じ *.md=英語 / *.ja.md=日本語 の対で揃える
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装 (2026-09-20)

### ライセンス (AC#1〜#3)
- `LICENSE` は MIT (Copyright (c) 2026 Takuya Otani)。持ち主判断のため着手時に確認して決定。
  同梱物 (修正 BSD / MIT / Apache-2.0) のいずれとも衝突しない。
- `THIRD_PARTY_NOTICES.md` は 3 節構成。
  - TinySegmenter: ライセンス全文は npm ラッパーではなく上流 <http://chasen.org/~taku/software/TinySegmenter/LICENCE.txt>
    から取得した原文 (Copyright (c) 2008, Taku Kudo)。`node_modules/tiny-segmenter/LICENSE` が
    別人 (绝云) の MIT であることも明記し、後から同じ取り違えが起きないようにした。
    `model.go` = モデル表の同梱、`segmenter.go` = アルゴリズム移植、と由来を区別して書いてある。
  - llama.cpp: b9437 タグの LICENSE から取得した MIT 全文 (Copyright (c) 2023-2026 The ggml authors)。
  - ruri-v3-30m: 著者名は HF モデルカードの BibTeX から確認 (Hayato Tsukagoshi / Ryohei Sasano)。
    `cl-nagoya` = 名古屋大学と推測できるが所属はカードに明記が無いため書いていない。
    上流 revision 24899e5 のファイル一覧を確認し NOTICE ファイルが無いことを確かめた上で、
    「伝播すべき NOTICE 無し」と明記。Apache-2.0 §4(b) に当たるので「GGUF 変換 + q8_0 量子化
    という改変を加えている (再学習はしていない)」旨を書き、全文を添付した。

### HANDOFF 削除 (AC#4)
`docs/HANDOFF.md` を削除。参照 6 箇所は「参照が指す内容が既に手元にあるか」で 1 件ずつ判断した。
- `handlers.go` / `server.go` / `handlers_test.go`: 節番号が指していた内容 (unlink が要る理由・
  依存順の列挙・ハーネスの性質) はいずれもコメント本文に既に書かれている。出典表記だけ落とした。
- `app.go`: 「retry-without-restart は follow-up」という事実は残し、追跡先の括弧だけ落とした。
  代替の追跡先 (backlog task) は作っていない。必要なら別途起票。
- `main.go`: 「Phase 1 (scaffold) のみ、internal/ は後続 Phase で結線」は移行期の記述で、
  現状 (app.go が全層を結線済み) と食い違っていたので 2 文とも削除した。残りの段落だけで
  main.go の役割は過不足なく説明できている。
- `local-generation-design.md`: 検証規律の括弧注記は直前に `go build/vet/test` と `gofmt` が
  具体的に書かれていて情報が重複するため削除。「全体の経緯」リンク行も削除。

なお削除しても **git 履歴には HANDOFF.md が残り、public 化で履歴ごと公開される**。
task の調査どおり秘密情報は含まないので実害は無いと判断し、履歴書き換えはしていない。

### workflow コメント (AC#5)
public では標準ランナーが無料なので「private だから課金」という根拠が成立しない。
手動限定を保つ理由を「フルマトリクスで数十分かかり、自動で消費されない未署名成果物しか
出ない」に差し替えた。あわせて、task の留意点である「将来 secrets を入れるときに
fork から届くトリガ (pull_request_target 等) を足さない」をコメントとして workflow 自身に
書き込んだ — 注意すべき場所と注意書きが離れていると守られないため。

### README の言語分割 (着手後の追加依頼)
`git mv README.md README.ja.md` の上で英語版 README.md を新規作成。既存の
AGENTS.md / AGENTS.ja.md・docs/current-spec.md / .ja.md と同じ `*.md`=英語 / `*.ja.md`=日本語
の対に揃えた。相互リンクを両方の冒頭に 1 行ずつ置き、`docs/multi-agent-chat-design.md` の
README 参照 2 箇所を、同ファイルが current-spec に使っているのと同じ
`[README](../README.md)（[ja](../README.ja.md)）` 形式へ更新した。

### 検証
- `go build ./...` / `go vet ./...` / `go test -race ./...` すべてグリーン (全 15 パッケージ ok)
- `pnpm check:client` (tsc --noEmit) グリーン
- `gofmt -l` は `internal/search/model.go` のみ報告するが、これは本 task 以前からの
  生成ファイル (gen_model.mjs 出力・DO NOT EDIT) で、今回触っていないため据え置いた
- `grep -rn HANDOFF` で backlog/ 以外に残存参照が無いことを確認 (backlog の task-7・task-25 は
  当時の記録なので触っていない)

### 未達
AC#6 (GitHub 上の public 化) のみ未了。不可逆な外向き操作のため、PR マージ後に確認を取ってから実施する。
<!-- SECTION:NOTES:END -->
