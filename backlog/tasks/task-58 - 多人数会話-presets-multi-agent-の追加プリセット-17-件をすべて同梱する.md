---
id: TASK-58
title: '多人数会話: presets/multi-agent の追加プリセット 17 件をすべて同梱する'
status: Done
assignee: []
created_date: '2026-09-26 09:22'
updated_date: '2026-09-26 11:09'
labels: []
dependencies: []
references:
  - internal/preset/preset.go
  - internal/preset/preset_test.go
  - presets/multi-agent/README.md
  - docs/multi-agent-chat-design.md
  - docs/current-spec.md
  - docs/current-spec.ja.md
  - README.md
  - README.ja.md
type: chore
ordinal: 58000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
今は同梱 8 件 (`internal/preset/bundled/`) と、ファイルから読み込む追加分 17 件 (`presets/multi-agent/`) に分かれている。追加分もアプリの新規作成フォーム・編成パネルのプリセット一覧に最初から出るよう、全 25 件を同梱にする。JSON ファイルからの読み込み (`preset` のインライン適用) は自作プリセット用に残す。

分けていた理由の確認 (2026-09-26):
- TASK-5 の同梱の選定は「4 群からの代表」で、品質の足切りではない。設計書 §8 の判断も保存形式 (go:embed + インライン適用) を決めたもので、件数を絞る理由は書かれていない。
- 追加分 17 件はすべて `preset.Parse` の検証を通り、`id`・`group` を持ち、同梱分とも互いにも id が重ならない (`-overlay` で差し込んだ一時テストで確認)。`receivesProjectMaterial: false`・`facilitator`・`stateSheet` を使うものは無いので、`preset_test.go` の同梱分への検査にも掛からない。
- 合計 88KB でバイナリへの影響は無視できる。ファイル名の番号は 01〜25 の通しなので、移すだけで一覧の順序が決まる。

`presets/multi-agent/README.md` の置き場所: `docs/` へ移す (オーナーの判断)。ファイル名は `docs/multi-agent-presets.md` を既定とする。中身は形式の説明・適用の仕組み・自作するときの要点を残し、「同梱していないもの」という前置きと 17 件の一覧表は落とす (一覧はアプリのプリセット選択が持ち、表を残すと JSON と二重管理になるため)。`presets/` ディレクトリは空になるので消す。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 presets/multi-agent/*.json の 17 件が internal/preset/bundled/ に git mv で移り、GET /api/multi-agent-presets が 25 件を番号順に返す
- [x] #2 presets/multi-agent/README.md が docs/multi-agent-presets.md に移り、同梱していない前提の記述と 17 件の一覧表が無く、形式・適用の仕組み・自作の要点が残っている。presets/ ディレクトリが無い
- [x] #3 README.md / README.ja.md のディレクトリ説明と同梱件数、docs/current-spec(.ja).md の同梱件数と追加分の置き場所、設計書 §6・Phase の進捗・§8 の presets/multi-agent/ への言及が新しい構成に合っている
- [x] #4 設計書 §8「実装で解消した判断」に、全件を同梱にした判断と理由 (日付つき) が追記されている
- [x] #5 リポジトリ内に presets/multi-agent への参照が残っていない (backlog/ の完了済みタスクの記録は除く)
- [x] #6 go test ./... と go vet ./...、pnpm check:client が通る
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. presets/multi-agent/*.json を internal/preset/bundled/ へ git mv。
2. presets/multi-agent/README.md を docs/multi-agent-presets.md へ git mv し、前置きと一覧表を落として同梱 25 件 + 自作 JSON の読み込みという前提に書き直す。相対リンクを docs/ 基準に直す。
3. README(en/ja)・current-spec(en/ja)・設計書 (§6 / Phase 進捗 / §8) の記述を更新し、§8 に全件同梱の判断を追記。
4. preset_test に同梱件数 25 の検査は足さない (件数を固定すると次の追加で壊れるだけで、読み込みの失敗は mustLoadBundled の panic で捕まる)。
5. go test / go vet / gofmt / pnpm check:client。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## やったこと
- 17 件の JSON を `git mv` で `internal/preset/bundled/` へ移した。中身は変えていない (description にも「同梱していない」旨の記述は無かった)。
- `presets/multi-agent/README.md` を `docs/multi-agent-presets.md` へ `git mv` し、前置き・17 件の一覧表・「番号は同梱分と通し」の注記を落とした。代わりに「同梱に足すときは番号付きのファイル名で置き、他と重ならない id と 4 つのうちの group を付ける。起動時に全件を検証し、id の欠け・重複や検証に通らないファイルがあると起動しない」を書いた。group は起動時に検証されず、知らない値は一覧の「その他」に並ぶ (PresetPicker の GROUP_ORDER) ので、そのとおりに書いている。
- `presets/` は Finder の `.DS_Store` だけが残っていたので、それごと消した。
- 設計書: §6 の保存形式の段落、Phase の進捗 (過去の記録なので「2026-09-26 に 25 件すべてを同梱に移した」を足す形)、§8.1 の保存形式の判断の拡張の注記を直し、§8.1 に「同梱するプリセットの範囲 → 25 件すべて」を保存形式の判断の直後に追記した。

## 決めたこと
- 同梱件数 (25) を固定するテストは足していない。次にプリセットを足すと壊れるだけで、読み込みの失敗は `mustLoadBundled` の panic と既存の `preset_test` が捕まえるため。

## 検証
- AC #1: `-overlay` で一時テストを差し込み、`Bundled()` が 25 件を返し、埋め込みファイル名の順 (01〜25) と一致することを確認した。`GET /api/multi-agent-presets` は `preset.Bundled()` をそのまま返す (`internal/httpapi/multiagent.go:257`)。
- AC #5: `grep -rn "presets/multi-agent"` を backlog/ を除いて実行し、該当なし。
- AC #6: `go vet ./...`・`go test ./...`・`pnpm check:client` が通った。Go のコードは変えていないので gofmt は対象外。
- アプリを起動しての見た目の確認 (25 件の optgroup の並び) はしていない。
<!-- SECTION:NOTES:END -->
