---
id: TASK-5
title: '多人数会話: プリセット同梱と会話品質チューニング'
status: Done
assignee: []
created_date: '2026-09-08 22:28'
updated_date: '2026-09-19 01:39'
labels: []
milestone: m-0
dependencies:
  - TASK-4
references:
  - docs/multi-agent-chat-design.md
ordinal: 5000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/multi-agent-chat-design.md §6〜§7 Phase C。ディベート用・即興劇用のプリセット (参加者一式 + ターン進行ルール + 場面設定の雛形) を同梱し、新規作成時に選択できるようにする。履歴圧縮 (summary サービス流用の要否 = §8 未決) と役割リマインド文のチューニングを行う。保存形式 (ハードコード / 同梱 JSON / ユーザー編集可能) は着手時に決めて設計書へ反映する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 プリセット選択だけでディベート / 即興劇を開始できる (Phase C 完了条件)
- [x] #2 長い会話 (20 ターン以上) で役割崩れ・同調収束が起きにくいことを実機の LM Studio で確認した
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 公開語彙の対応表 (JSON のフィールド名・API ルート・群の値) を先に確定する。
2. internal/preset を新設: MultiAgentPreset 型、bundled/*.json を go:embed で読む Bundled()/Find()、インライン JSON 用の Parse()/Validate()。同梱は 01 debate, 02 improv, 03 group-interview, 13 detective, 14 podcast, 18 mock-job-interview, 24 twenty-questions の 7 件。
3. サンドボックスの Markdown 24 件を変換スクリプトで JSON 化する。同梱 7 件は internal/preset/bundled/、残り 17 件は presets/multi-agent/ に置き、README に形式とインポート手順を書く。
4. HTTP API: GET /api/multi-agent-presets (一覧)、POST /api/projects/{projectId}/chats に presetId (同梱) または preset (インライン JSON) を追加。適用 = turn_rule/scene_prompt を持つ chat 作成 + 参加者を登録順に作成 (接続先・モデルは空 = ワークスペース既定)。title が空ならプリセットの title。途中失敗時は chat を削除して返す。
5. フロント: api/client.ts に型と listMultiAgentPresets、createChat の presetId/preset。ProjectDetailPage の新規作成フォームで kind=multi_agent のときにプリセット選択 (群ごとに optgroup) と JSON ファイル読み込みを出す。i18n en/ja。
6. ターンエンジンのチューニング: 役割リマインドに「直前の発言のどこに反応しているか」「発言の長さは場面設定に従う」「複数人分を書かない」を加え、turnHistoryLimit を 20→30。履歴圧縮 (summary 流用) は見送りと決め、理由を設計書 §8 に書く。
7. 設計書 §5/§6/§8 に保存形式 (同梱 JSON + インライン import)、履歴圧縮の見送り理由、リマインドの内容を反映する。
8. テスト: preset パッケージ (同梱の読み込み・検証)、httpapi (一覧・presetId 適用・インライン適用・不正 JSON の 400・assistant kind への preset 指定 400)、turnengine (リマインド文)。go test ./... / go vet / gofmt / pnpm check:client / pnpm build:client。
9. AC #2 (実機 20 ターン以上) はユーザーの実機確認に委ね、未チェックのまま In Review へ。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 保存形式と範囲 (着手時に決めた事項)
- 保存形式は「同梱 JSON (`internal/preset/bundled/*.json` を `go:embed`) + JSON ファイルからのインライン適用」。
  Go ハードコードはデータとコードを分ける §0 の方針に反し、データディレクトリ走査は読み込み失敗時の UI が増えるので採らなかった。
  設計書 §5 / §6 / §8 に反映済み (ステータスの DRAFT 解除は TASK-6)。
- 同梱は 4 群から 7 件: debate / improv-late-night-diner / group-interview-waiting-room / detective-questioning /
  podcast / mock-job-interview / twenty-questions。残り 17 件は `presets/multi-agent/*.json` に置き、README に形式と
  読み込み手順を書いた。JSON は `_sandbox/multi-agent-presets` の Markdown から変換スクリプトで生成し、
  description から設計書・TASK への言及を落とした以外は本文をそのまま使っている。
- 履歴圧縮 (summary 流用) は見送り。理由は設計書 §8 に書いた (ローカル小型モデルでの遅延倍増・§4.4 の分離・
  崩れ方が事実の忘却ではなく役割の忘却)。代わりに `turnHistoryLimit` を 20 → 30 に上げ、20 の扉 1 ゲーム分と
  4 名編成の数巡が窓に収まるようにした。

## 役割リマインドの変更
サンドボックス README の申し送りを反映し、「他の参加者の動作も代筆しない・1 発言に複数人分を入れない」
「直前の発言のどこに反応しているかが分かるように述べる」「発言の長さは場面設定の指定に従う」を追加した。
`TestTurnEnginePromptMapping` でリマインド文の存在を検証している。

## API と UI
- `GET /api/multi-agent-presets` と、`POST /chats` の `presetId` / `preset`。両方は同じ `preset.Parse` を通るので、
  ファイルから読んだものも同梱と同じ検証を受ける。参加者作成が途中で失敗したら chat を削除して返す。
- 新規作成フォームで多人数会話を選ぶとプリセット選択 (群ごとの optgroup) と「JSON を読み込む」ボタンが出る。
  ファイルは `File.text()` で読んで body にインラインで送るだけで、アップロード経路は増やしていない。

## 検証
- `go test ./...` / `go vet ./...` / `gofmt` / `pnpm check:client` / `pnpm build:client` すべて通過。
- 追加テスト: `internal/preset` (同梱 7 件の読み込み・群と必須項目・Parse の 5 種の拒否)、
  `TestMultiAgentPresets` (一覧、presetId 適用と登録順、title の優先、インライン適用、404 / 400 の 5 種、
  拒否後に chat が残らないこと)。

## 未確認 (AC #2)
LM Studio がこの環境に無いため、20 ターン以上の実機確認はユーザーに委ねる。確認には同梱の「20 の扉」
(出題者が 1 語以外を言い始めるか、回答者が質問回数を数え間違えるかで崩れが見える) が向く。

## 実機確認で見つかった不具合と修正 (2026-09-19)
同梱の「20 の扉」を LAN 上の LM Studio 2 台 (gemma-4-e4b) で実行したところ、出題者のターンが
断続的に失敗した。原因はプロンプトの末尾に置いていた進行キュー
「（進行）次は「X」の番です。X として発言してください。」。推論してから答えるモデルはこれを
会話への指示と読み、「まだ発言すべきでないのでは」と推論し続けて content を空で返す。
内容が空のターンは保存できないので `RunTurn` はエラーになる (これが「回答できない」の正体)。

出題者側のエンドポイントに実プロンプトを投げて計測した (temperature 0.7、履歴は 3 問目まで):
- 進行キューを末尾に置く: 4 回中 2 回が content 空。残り 2 回も 1 語の回答に 184 / 429 トークン。
- 直前の質問を末尾に置く: 4 回中 4 回が 6 トークンで正しく回答。reasoning は 0 トークン。

修正は `buildTurnPrompt` の追加。写像した履歴の最後が user ロール (他参加者か人間の発言) なら、
それを `LLMClient` が必ず末尾に置く user スロットへ移す。進行キューは答えるべき発言が無いとき
(最初のターン、manual で直前と同じ参加者を指名したとき) だけに残す。
リマインド文は変えていない。キューを外した条件では、リマインドを入れたままでも 1 語で答えたため
(上記の計測の C 条件)、原因ではないと切り分けられた。設計書 §4.3 に理由と計測値を書いた。

## AC #2 の実機確認 (2026-09-19)
修正後のターンエンジンで、同梱の「20 の扉」を LAN 上の LM Studio 2 台に対して 30 ターン連続で実行した。
回答者 = 192.168.0.103:1234/v1、出題者 = 192.168.0.66:1234/v1、いずれも google/gemma-4-e4b。
プリセットをそのまま適用し、round_robin で 1 ターンずつ RunTurn を呼んだ (所要 866 秒)。

結果:
- ターン失敗 0 / 30。修正前に出ていた content 空によるターン失敗は再現しなかった。
- 出題者は 15 回の回答すべてを「はい」「いいえ」「どちらとも言えません」のいずれか 1 語で返した
  (2〜10 文字、各 0.4〜1.4 秒)。1 語ルールの逸脱なし。
- 回答者は 1 問目から 15 問目まで番号を連続して正しく数え、1 ターン 1 問を守り、同じ質問の繰り返しも無かった。
- 同調収束 (相手に同意して会話が止まる) は起きなかった。役割の非対称が最後まで保たれた。

したがって「長い会話 (20 ターン以上) で役割崩れ・同調収束が起きにくい」は満たされたと判断する。

観測した性能上の注意 (不具合ではない): 回答者側のターンは 4〜132 秒とばらつきが大きい。
質問を組み立てる側は推論トークンを多く使うためで、出題者側 (1 秒未満) との差が大きい。
自動進行では長い沈黙に見えるので、待ち時間の表示が要るなら別タスクで扱う。

確認の再現方法: リポジトリ外の worktree に `internal/service` のライブテストを置き、
SNZ_LIVE_ENDPOINTS="<回答者>|<出題者>" SNZ_LIVE_MODEL=... で実行した。
リポジトリ本体には残していない (実エンドポイントが要るテストを通常のスイートに入れないため)。
<!-- SECTION:NOTES:END -->
