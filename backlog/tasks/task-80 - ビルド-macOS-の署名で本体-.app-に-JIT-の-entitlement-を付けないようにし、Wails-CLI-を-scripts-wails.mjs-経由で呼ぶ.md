---
id: TASK-80
title: >-
  ビルド: macOS の署名で本体 .app に JIT の entitlement を付けないようにし、Wails CLI を
  scripts/wails.mjs 経由で呼ぶ
status: To Do
assignee: []
created_date: '2026-09-28 20:23'
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
- [ ] #1 本体 .app の署名に entitlements が付かず、llama-server には従来どおり付く
- [ ] #2 entitlements を外した .app で WebView の画面が動き、公証が通ることを実機で確かめている
- [ ] #3 build-mac-signed.sh が Wails CLI を scripts/wails.mjs 経由で呼ぶ
<!-- AC:END -->
