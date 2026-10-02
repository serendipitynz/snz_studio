---
id: TASK-80
title: >-
  ビルド: macOS の署名で本体 .app に JIT の entitlement を付けないようにし、Wails CLI を
  scripts/wails.mjs 経由で呼ぶ
status: Done
assignee: []
created_date: '2026-09-28 20:23'
updated_date: '2026-10-02 05:45'
labels:
  - security
dependencies: []
references:
  - scripts/build-mac-signed.sh
  - scripts/wails.mjs
priority: low
type: chore
ordinal: 80000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F14 (Low)。2026-09-29 に `756d32b` で未修正を確認した。

- `build-mac-signed.sh` は `ENT_ARGS` (`allow-jit`、`allow-unsigned-executable-memory`) を `llama-server` だけでなく本体 `.app` の署名にも付けている (129 行と 133 行)。Go の本体はこれを必要とせず、Hardened Runtime のコード注入への耐性を不要に弱めている。WebView の JIT は WebKit の別プロセスで処理されるはず (要実機確認)。
- `WAILS="${WAILS:-$HOME/go/bin/wails}"` を直接起動していて、README が必須としている `scripts/wails.mjs` (GOTOOLCHAIN の固定) を通らない。手元の Go が新しいとバインドの生成が失敗する既知の問題 (TASK-7) を再現し得る。
- TASK-67 がこのスクリプトのサイドカー取得の部分を書き換える。どちらを先にしてもよいが、衝突しないように順序を決めてから着手する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 本体 .app の署名に entitlements が付かず、llama-server には従来どおり付く
- [x] #2 entitlements を外した .app で WebView の画面が動き、公証が通ることを実機で確かめている
- [x] #3 build-mac-signed.sh が Wails CLI を scripts/wails.mjs 経由で呼ぶ
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. TASK-67 は Done 済みなので順序の衝突はない。現行の build-mac-signed.sh に対して直す
2. Wails の起動を $WAILS の直接起動から node scripts/wails.mjs build に置き換え、WAILS 変数を削除する
3. 本体 .app の codesign から ENT_ARGS を外し、llama-server だけに entitlements を付ける。理由をコメントに残す
4. bash -n で構文を確認し、署名ビルドを実行して codesign -d --entitlements で .app / llama-server の entitlements を確認する
5. 公証の結果と、起動した .app で WebView の画面が動くことを確かめる (AC #2)
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 変更

- 本体 .app の codesign から `--entitlements` を外し、`allow-jit` / `allow-unsigned-executable-memory` は llama-server だけに付けるようにした。Go の本体は書き込み可能な実行メモリを使わず、WKWebView の JS の JIT は WebKit の WebContent プロセスで動く。そのプロセスは Apple 自身の entitlements (`com.apple.security.cs.allow-jit` ほか) を持っていることを `codesign -d --entitlements -` で確かめた
- Wails の起動を `$HOME/go/bin/wails` の直接起動から `node scripts/wails.mjs build` に変えた。`WAILS` の環境変数による上書きは使い方の欄に載っていなかったので、残さずに削除した (wails.mjs は PATH 上の `wails` を起動する)
- TASK-67 は Done になっていたので、着手順序の調整は要らなかった

## 検証 (2026-10-02、Apple Silicon、macOS 26.6.2)

- `scripts/build-mac-signed.sh` を最後まで実行 (exit 0)。ビルドは wails.mjs 経由で darwin/universal
- AC #1: `codesign -d --entitlements -` で、本体 .app は entitlements なし (flags=0x10000(runtime) で hardened runtime は有効)、`Contents/Resources/llama-server` には 2 つの entitlements が付いていることを確認
- AC #2: notarytool の結果は Accepted (submission id 30b3408a-065a-49b7-b174-4b6792ab96d3)。staple と validate も成功し、`spctl` は「accepted, source=Notarized Developer ID」。公証済み DMG をマウントして .app を起動し、WebKit の Networking プロセスがローカル API のポートに接続していること、ダッシュボード (プロジェクト一覧と最近のチャット) が描画されていることをスクリーンショットで確認した。サイドカーの llama-server も起動していた
- `bash -n` で構文を確認。shellcheck は手元に無いため未実行

## 見ていないこと

- 手元の Go は go.mod の toolchain と同じ go1.27.1 なので、「より新しい Go でバインド生成が壊れる」状況を wails.mjs が防ぐ場面そのものは再現していない
- WebView の画面は起動直後のダッシュボードまでを見た。チャットの送信など JS の重い操作までは触っていない
<!-- SECTION:NOTES:END -->
