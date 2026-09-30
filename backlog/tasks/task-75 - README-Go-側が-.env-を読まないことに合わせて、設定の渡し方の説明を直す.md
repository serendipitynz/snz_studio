---
id: TASK-75
title: 'README: Go 側が .env を読まないことに合わせて、設定の渡し方の説明を直す'
status: In Review
assignee: []
created_date: '2026-09-28 20:22'
updated_date: '2026-09-30 03:17'
labels: []
dependencies: []
references:
  - README.md
  - README.ja.md
  - .env.example
  - internal/config/config.go
priority: low
type: docs
ordinal: 75000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F9 (Low)。2026-09-29 に `756d32b` で未修正を確認した。

- README.md (137 行付近) / README.ja.md (130 行付近) は「`.env` で `LLM_BASE_URL` などの既定値を上書きできる」、README.md (226 行付近) / README.ja.md (209 行付近) は「`.env` の主な設定」と書いている。
- Go 側に `.env` を読む処理は無い (`config.go` のコメント「loadEnvFile is the OS env in Go」)。Vite の `envDir` が読むのはフロントの `VITE_` 変数だけ。`.env.example` に並ぶ値は、シェルの環境変数として渡さない限り反映されない。
- 影響: UI から設定できない唯一の資格情報である `LLM_API_KEY` を `.env` に書いた利用者は、キーが送られず認証エラーになり、原因を切り分けにくい。

## 方針 (着手時に確定する)

- `pnpm dev` で `.env` の値が反映されないことを実機で確かめる。
- README と `.env.example` を「環境変数として渡す」に改めるか、dev でだけ `.env` を読むようにするかを決める。後者で依存が増えるなら、先にユーザーに確認する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 pnpm dev で .env の値が反映されるかを実機で確かめた結果が記録されている
- [x] #2 README.md / README.ja.md / .env.example の説明が実装と一致している (または実装を説明に合わせている)
- [x] #3 LLM_API_KEY の渡し方が README.md / README.ja.md に具体的に書かれている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. pnpm dev で .env が反映されないことを実機で確かめる (済: 2026-09-30)
2. .env を読む処理は足さず、README.md / README.ja.md / .env.example を「環境変数として渡す」に改める (2026-09-30 ユーザー判断)
3. LLM_API_KEY の渡し方として、pnpm dev と配布版それぞれの具体的な起動例を書く
4. TASK-70 と同じ PR にする (同じ README 節を直すため)
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実機確認 (AC#1、2026-09-30、macOS)

- スクラッチの worktree に、目印の値 (`DATA_DIR=<scratch>/dotenv-datadir`、`LLM_MODEL=dotenv-marker-model`) を書いた `.env` を置き、環境変数を渡さずに `pnpm dev` を起動した。データディレクトリは `./data` のまま (起動ログの `api: data dir`)、目印のディレクトリは作られず、Go プロセスの環境 (`ps eww`) に `DATA_DIR` / `LLM_MODEL` は無かった。`.env` は反映されない。
- 対照として `set -a; . ./.env; set +a; pnpm dev` で起動すると、データディレクトリが目印のパスに切り替わり、Go プロセスの環境にも 2 つが入った。
- リポジトリ直下の既存の `.env` は、中身を読まず触らずに確認した。

## 決めたこと (2026-09-30、ユーザー判断)

- `.env` を読む処理は足さず、説明を実装に合わせた (AC#2)。dev と配布版で挙動が分かれず、依存も増えないため。
- README.md / README.ja.md の開発手順と「LLM 接続」を「環境変数として渡す」に改め、`.env.example` の冒頭に「アプリはこのファイルを読まない」と読み込み方を書いた。docs/current-spec(.ja).md の「`.env` は初期値」も直した。
- `LLM_API_KEY` の渡し方 (AC#3) として、`pnpm dev` と配布版 (ターミナルから実行ファイルを起動) の例、Windows の PowerShell での設定、コマンドに直接書くとシェル履歴に残ることを README に書いた。`set -a; . ./.env.example; set +a` で `.env.example` がそのまま読み込めることを確かめた。
- TASK-70 (キーが送られる範囲) と同じ README の節を直すので、同じ PR にした。
<!-- SECTION:NOTES:END -->
