---
id: TASK-7
title: '開発環境: Go ツールチェーンのバージョン差で wails dev が失敗する'
status: In Review
assignee: []
created_date: '2026-09-18 21:33'
updated_date: '2026-09-18 21:55'
labels: []
milestone: m-0
dependencies: []
references:
  - 'https://github.com/serendipitynz/snz_studio/pull/6#issuecomment-5736422678'
ordinal: 7000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
アクティブな Go ツールチェーンが go.mod の go ディレクティブより新しいと、wails dev / wails build が `internal error: package "math" without types was imported from "snzstudio/internal/vector"` で失敗する。Wails v2.12.0 が埋め込んでいる x/tools が新しい Go のエクスポートデータ形式を読めないことによる (2026-09-19 に go1.27.1 + go.mod の go 1.26.3 で発生)。

go.mod に toolchain ディレクティブが無いため、Go は "1.26.3 以上ならどれでもよい" と解釈して手元の最新 (1.27.1) を選ぶ。一方 CI (.github/workflows) は setup-go の go-version-file: go.mod で 1.26.3 を入れるので、現状この失敗はローカル開発でだけ起きる。回避策は `GOTOOLCHAIN=go1.26.3 wails dev`。

対処の候補は 2 つ:
(a) go.mod に toolchain ディレクティブを入れて使用バージョンを固定する。手元の Go を上げても壊れなくなるが、Wails を上げるまで Go も上げられなくなる。
(b) go1.27 のエクスポートデータを読める Wails v2 に上げる。Go の更新に追随できるが、Wails 側の変更点の確認が要る。

どちらを採るか (または両方) は着手時に判断する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 wails dev が、開発者の手元の Go ツールチェーンのバージョンに関わらず起動する (環境変数を毎回手で付けなくてよい)
- [ ] #2 CI の build ワークフローが引き続き通る
- [x] #3 採った対処と、採らなかった候補を落とした理由が記録されている
- [x] #4 README の開発手順が、必要な Go / Wails のバージョン要件と整合している
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 原因の切り分け結果を前提にする: エクスポートデータを読む x/tools は go.mod の依存グラフではなく wails CLI バイナリ側に埋まっている (go.mod だけ v2.16.0 にしても CLI が v2.12.0 なら再現する、を実機確認済み)。したがって Wails 昇格は go.mod と CLI pin の両方を同時に動かす。
2. go.mod: wails v2.12.0 -> v2.16.0、および toolchain go1.27.1 を追加。go ディレクティブ (1.26.3) は言語最低要件として据え置き、toolchain をビルド実行版の固定に使う。なぜこの pin が要るか / なぜ Wails 昇格だけでは不十分かを go.mod のコメントに残す (AC#3)。
3. .github/workflows/build.yml: go install ...cmd/wails@v2.12.0 -> @v2.16.0。setup-go は go-version-file: go.mod のままでよい (toolchain 行を読むか、読まなくても toolchain ディレクティブが auto ダウンロードを起こすかのどちらかで 1.27.1 になる)。
4. README「必要なツール」: Go の要件と Wails CLI のインストール版を更新し、toolchain が go.mod で固定されている旨と CLI 版を合わせる必要がある旨を明記 (AC#4)。
5. docs/HANDOFF.md: 現況記述 (go.mod 行・インストール済み CLI 版・ツールチェーン行) を更新。v2.12.0 のソース行番号を根拠として引いている箇所は「その版で確認した」という史実なので触らない。
6. 検証: go build/vet/test、フロントの lint/typecheck、フル wails build (CLI v2.16.0)、および 4 マイナー分の昇格に対する挙動確認としてビルド済みアプリを実起動して API サーバーとバインドの動作を見る。CI は workflow_dispatch 限定 (課金分) のため実走できない前提で、その旨を Notes に残す。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
(a) toolchain 固定と (b) Wails 昇格の両方を採った。

## 着手時判断の根拠 — タスク本文の見立てに対する訂正

本文は「go.mod に toolchain を入れる」か「Wails を上げる」かの二択として書かれていたが、
切り分けの結果、**エクスポートデータを読む x/tools は go.mod の依存グラフではなく
wails CLI バイナリ側に埋め込まれている**ことが分かった。実測:

| 構成 (いずれも手元 Go = go1.27.1) | `wails build -s` |
| --- | --- |
| go.mod wails v2.12.0 + CLI v2.12.0 (現状) | 失敗 (`internal error: package "math" without types`) |
| `GOTOOLCHAIN=go1.26.3` を付与 | 成功 |
| go.mod wails v2.16.0 (x/tools は v0.47.0 に解決) + CLI v2.12.0 | **失敗したまま** |
| go.mod wails v2.16.0 + CLI v2.16.0 | 成功 |

3 行目が要点で、(b) は go.mod だけでは成立せず、CI の `go install ...@<ver>` と README と
開発者の手元 CLI を同時に動かす変更になる。この対の制約を go.mod のコメント・build.yml の
コメント・README の注記に明記した。

## 落とした選択肢

- **(a) のみ**: AC#1 は満たすが、Wails を上げるまで Go も上げられない状態が残る。
  今回ユーザー判断で「両方」を選択したため採らなかった。
- **(b) のみ**: 今日の go1.27 は直るが toolchain が固定されないため、次の Go リリースで
  Wails が追いつくまで同じ失敗が再発しうる。AC#1 の「バージョンに関わらず」を
  durable には満たさないので単独では不可と判断した。
- **CI で `go-version:` に 1.27.1 を直書き**: setup-go は go.mod の `go` 行しか読まず
  (`actions/setup-go` v5 `src/installer.ts` の `parseGoVersionFile` が `/^go (\d+(\.\d+)*)/m`
  のみ) `toolchain` 行を無視するため、一見こちらが確実に見える。だが pin が go.mod と
  ワークフローの 2 箇所に分かれて drift する。`toolchain` ディレクティブがあれば setup-go が
  floor 版 (1.26.3) を入れても go コマンド側が 1.27.1 を自動取得するので、pin は go.mod 1 箇所に
  残した。代償は CI 実行時のツールチェーン 1 回分のダウンロードで、workflow_dispatch 限定 ·
  macOS universal のフルビルドに対しては無視できる。
- **`go` ディレクティブごと 1.27.1 に上げる**: CI のダウンロードは省けるが、`go` 行は
  言語の下限、`toolchain` は実行版という役割分担を崩す。また `go` 行だけを上げても
  将来の go1.28 では再発する (下限を満たす最新が選ばれるため) ので、どのみち `toolchain` が要る。

## 検証

- 再現: go1.27.1 + CLI v2.12.0 で `wails build -s` が本文どおりのエラーで失敗することを確認。
- `toolchain` ディレクティブが実際に参照されること: 一時的に `toolchain go1.99.0` にすると
  `go: downloading go1.99.0 (darwin/arm64)` → `toolchain not available` となり、
  go コマンドが手元の版ではなく go.mod の指定版を取りに行くことを実証した (AC#1 の
  「環境変数を毎回手で付けなくてよい」の機構的裏付け)。
- `wails dev` 実起動: バインド生成 → コンパイル → ウィンドウ起動 →
  `api: listening on http://127.0.0.1:8787` まで到達。認証なし `GET /api/projects` は
  設計どおり 401 (起動ごとの bearer token) を返し、ルーティングと認証ミドルウェアが
  生きていることを確認。
- `wails dev` は起動のたびに `go mod tidy` を走らせるが、`toolchain` 行と理由コメントが
  それを通過して残ることを確認した (tidy に消されるコメントでは AC#3 の記録が
  次の起動で失われるため)。
- `go build ./...` / `go vet ./...` / `go test ./...` 緑。Wails 昇格に引きずられて
  `golang.org/x/text` が v0.36.0 → v0.39.0 に上がるため、NFKC に依存する
  `internal/search` の golden テストが通ることを特に確認した。
- `pnpm run check:client` (tsc --noEmit) 緑。v2.16.0 で再生成したバインドは
  `GetApiBase` / `GetApiToken` の 2 関数で変化なし (バインドは .gitignore 済みで差分に出ない)。
- `wails build -clean` (フロント込みフル) 成功。CLI と go.mod のバージョン不一致警告も解消。
- HANDOFF.md が根拠として引いている Wails のハード制約 2 件が v2.16.0 でも有効なことを
  ソース比較で確認した (`pkg/commands/build/base.go` と
  `pkg/assetserver/assethandler_external.go` がいずれも v2.12.0 から無変更)。
  よって当該箇所の v2.12.0 という版表記は「その版で確認した」史実として残置した。
- v2.12.0 → v2.16.0 の v2 側の変更で本プロジェクトに関係しうるのは
  originvalidator の wildcard オリジン照合の厳格化 (GHSA-47hv-j4px-h3c9) だが、
  main.go は `options.App` に AssetServer.Assets / OnStartup / OnShutdown / Bind しか
  渡しておらずオリジンパターンを設定していないため影響しない。

## 未検証 — 判断を仰ぐ箇所

- **AC#2 (CI が通る) は未チェックのまま残した**。build.yml は private repo の課金分を
  避けるため `workflow_dispatch` 限定で、こちらから実走させていない。ローカルでは
  CI と同じ手順 (`go install ...cmd/wails@v2.16.0` → フル `wails build`) を再現して緑、
  setup-go のバージョンファイル解析もソースで確認済みだが、いずれも実走の証拠ではない。
  マージ前後に手動 dispatch するかどうかはユーザー判断。
- `wails dev` のログに出る `Embedding retrieval disabled: Embedding request failed with 500` は
  dev の `data/app-config.json` が LAN 上の `192.168.0.219:7997` を向いていて到達できないため。
  今回の変更とは無関係。
- 副作用として手元の `~/go/bin/wails` を v2.12.0 → v2.16.0 に入れ替えた
  ((b) は CLI 側の x/tools を要求するため。README にも同じ手順を記載)。
<!-- SECTION:NOTES:END -->
