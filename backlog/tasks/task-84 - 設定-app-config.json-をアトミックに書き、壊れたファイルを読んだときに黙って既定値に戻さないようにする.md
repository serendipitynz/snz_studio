---
id: TASK-84
title: '設定: app-config.json をアトミックに書き、壊れたファイルを読んだときに黙って既定値に戻さないようにする'
status: In Review
assignee: []
created_date: '2026-09-28 20:23'
updated_date: '2026-10-04 05:39'
labels: []
dependencies: []
references:
  - internal/config/config.go
priority: low
type: bug
ordinal: 84000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F18 (Low)。2026-09-29 に `756d32b` で未修正を確認した。

- `config.go` の保存 (351 行付近) は `os.WriteFile` で直接上書きする。書き込みはロックを解放した後なので、同時の `PUT /api/configuration` で書き込みの順序が入れ替わり得る。
- 読み込み側は、JSON が壊れていると**黙って**環境変数の既定値に戻る。

## 方針

- 一時ファイルに書いて `os.Rename` する。書き込みもロックの中で行う。
- パースに失敗したらログに出し、壊れたファイルを `.bak` に退避する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 app-config.json の書き込みが一時ファイルと rename で行われ、ロックの中で行われる
- [x] #2 同時の PUT /api/configuration の後、最後に受け付けた内容がファイルに残る。テストがある
- [x] #3 壊れた app-config.json を読んだとき、ログに出し、壊れたファイルを退避してから既定値で起動する。テストがある
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. UpdateEditable: 書き込みを c.mu の中へ移し、同じディレクトリの一時ファイルに書いて Sync してから os.Rename する (writeFileAtomic)
2. applyOverrides: NotExist 以外の読み込みエラーはログに出す。JSON パース失敗はログに出し、app-config.json を app-config.json.bak へ rename してから Defaults() のまま続ける
3. テスト: 並行 UpdateEditable 後にファイル内容が GetEditable と一致し一時ファイルが残らないこと / 壊れたファイルで既定値・.bak 退避・ログ出力されること
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装
- `UpdateEditable` は `c.mu` を関数の最後まで保持し、書き込みも同じロックの中で行う。書き込みは同じディレクトリの一時ファイル (`.app-config.json.*.tmp`) に書いて `Sync` してから `os.Rename` する (`writeFileAtomic`)。ロック中のディスク書き込みで `Get` の読み手が待つが、数百バイトのファイルなので待ちはごく短い。ロックを書き込み専用に分ける案は、読み手を待たせない代わりに 2 本のロックの順序を管理する必要があり、この規模では見合わないので採らなかった。
- `applyOverrides` は JSON のパースに失敗したら `app-config.json` を `app-config.json.bak` へ rename し、ログを出して `Defaults()` のまま起動する。退避しないと、次の保存で既定値がユーザーの設定を上書きして元に戻せなくなる。ファイルが存在しない場合 (初回起動) はこれまでどおり何もログに出さない。存在しない以外の読み込みエラー (権限など) はログに出すが退避はしない。
- `.bak` は 1 世代のみで、再び壊れたファイルを読むと前の `.bak` を上書きする。

## 検証
- AC #2: `TestConcurrentUpdateEditableKeepsLastAccepted` (32 並行の `UpdateEditable` 後、ファイル内容が `GetEditable()` と一致し、一時ファイルが残らない)。`PUT /api/configuration` のファイル書き込みは `UpdateEditable` だけを通り、ハンドラはその後にモデルのウォームアップと接続確認を外部へ出すため、テストは config パッケージで行った。旧実装に対して `-count=20` で 15 回失敗、新実装は `-race -count=50` で全回成功。
- AC #3: `TestLoadMovesCorruptFileAsideAndUsesDefaults` (既定値で起動・元の場所にファイルがない・`.bak` が元の内容と一致・ログにパスと退避先が出る) と `TestLoadMissingFileIsSilent`。旧実装では前者が 20/20 回失敗。
- `go vet ./...`、`go test ./...` すべて成功。`staticcheck ./internal/config/` 指摘なし。golangci-lint はリポジトリに設定がなく、`config.go` の新規コードに指摘なし (既存テストの `os.Unsetenv` に errcheck の指摘があるが今回の範囲外)。
- 実アプリでの壊れたファイルからの起動は手動確認していない。
<!-- SECTION:NOTES:END -->
