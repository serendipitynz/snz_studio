---
id: TASK-89
title: '埋め込み: 全件の再計算の走行中は「作り直す」を押せないようにし、終わったことを設定画面に出す'
status: Done
assignee: []
created_date: '2026-10-03 00:50'
updated_date: '2026-10-03 03:24'
labels: []
dependencies:
  - TASK-73
references:
  - frontend/src/components/SettingsModal.tsx
  - internal/service/embeddingsync.go
  - internal/httpapi/handlers.go
priority: low
type: enhancement
ordinal: 89000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-73 (PR #74) で、設定モーダルに「埋め込みの作り直し」区画 (`POST /api/embedding/rebuild`) を足した。動作確認でユーザーから、バックグラウンドで作り直している間も「作り直す」ボタンを押せることについて指摘があった (2026-10-03)。

押し直しても、データが壊れることはない。`EmbeddingSyncService` のワーカーが走行中の要求を保留の 1 件にまとめるので、再計算が同時に走ることもない。ただし次の 2 点が使い勝手の問題になっている:

- 走行中に押し直すと、走行後に全件の再計算がもう 1 回走る。内容の変わらない再計算なので、大きなコーパスでは数分の CPU / ネットワークを無駄に使う。押せる見た目なので、反応が無いと思って押し直されやすい
- 押した後に出る「バックグラウンドで作り直しています。終わるまでは今のベクトルで検索します。」は、押した結果を表示しているだけで、終わっても変わらない。モーダルを閉じて開き直すと消える。画面がサーバー側の再計算の状態を知らないため

## 方針

- サーバーが、全件の再計算が走行中か保留中かを返す。既存の `GET /api/embedding/status` に項目を足すか、別の口を設けるかは着手時に決める
- 設定モーダルはその間「作り直す」を無効にし、無効の理由 (doc-8 §6.1 の無効の扱い) として作り直している最中であることを述べる。終わるまで状態を取り直し、終わったら完了を述べる
- 設定の保存で始まった全件の再計算 (埋め込みソースの変更、TASK-73 のやり残しの再試行) も同じ表示にする
- 画面の変更なので、snz-design doc-16 / doc-17 を読んでから進め、doc-17 の設定モーダルの行を更新する
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 全件の再計算が走行中か保留中の間、設定モーダルの「作り直す」は無効になり、その理由が読める
- [x] #2 全件の再計算が終わると、設定モーダルの表示が完了に変わる。モーダルを開き直しても、そのときの状態が表示される
- [x] #3 設定の保存で始まった全件の再計算の間も、同じ表示になる
- [x] #4 サーバーが返す再計算の状態にテストがある
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. EmbeddingSyncService が走行中のパスを持ち、RebuildState() で全件の再計算の状態を返す: running (走行中か保留中) / incomplete (やり残し rebuildOwed が残っている) / done (この起動で1回完了した) / idle (まだ走っていない)
2. GET /api/embedding/rebuild を足し {state} を返す (着手時の判断: /api/embedding/status はサイドカーの状態なので混ぜない。2026-10-03 ユーザー確認)
3. 設定モーダル: 開いたとき・作り直しを押した後・設定を保存した後に状態を取り直し、running の間は 2 秒ごとに取り直す。running の間は「作り直す」を無効にし理由を述べる。running から抜けたら完了 / 終わりきらなかったを述べて読み上げる
4. service と httpapi にテストを足す (状態の遷移、保存で始まった作り直し、無効のとき)
5. doc-17 の設定モーダルの行は、マージ後に snz-design の別 PR で更新する (2026-10-03 ユーザー確認)
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 着手時に決めたこと (2026-10-03 ユーザー確認)

- 状態を返す口は `GET /api/embedding/rebuild` を新設した。`GET /api/embedding/status` は内蔵サイドカーの状態 (embed.Status) で、画面は内蔵モードのときだけ読む。作り直しは外部モードでも走るので混ぜなかった
- 状態は running / done / incomplete / idle の4つ。走り終えたことだけでは「完了」と言えない (接続先に届かず終わりきらない作り直しがある) ので、TASK-73 のやり残し (rebuildOwed) が残っていれば incomplete とし、次の保存で再試行されることと、すぐやり直すなら「作り直す」を押すことを述べる
- doc-17 の設定モーダルの行は、マージ後に snz-design の別 PR で更新する (TASK-61 の snz-design #28 と同じ流れ)

## 実装

- `EmbeddingSyncService` に走行中のパス (`current`) と、この起動で作り直しが1回完了したか (`rebuildCompleted`) を持たせ、`RebuildState()` で返す。保留中の作り直しも running に数える (走行中のギャップ埋めの後ろに積まれた作り直しを含む)。やり残しのためにギャップ埋めが作り直しへ格上げされた場合も running
- 設定モーダルは開いたとき・「作り直す」の後・設定の保存の後に状態を読み、running の間は 2 秒ごとに読み直す。running の間「作り直す」は aria-disabled と理由「作り直している最中です。終わると、もう一度押せます。」(doc-8 §5.4)。この開いている間に見ていた作り直しが終わったときだけ、完了 / 終わりきらなかったを読み上げる
- 「作り直す」の POST はサーバーが応答前に作り直しを積むので、応答の時点で画面を running にしてから読み直す (次の読み取りまでの間に押し直せる隙を作らない)
- README / README.ja の作り直しの段落に、区画の表示と `GET /api/embedding/rebuild` を1文足した

## 検証

- AC#4: `TestRebuildStateFollowsTheRebuild` (service) が idle → ギャップ埋めの最中も idle → 作り直しの最中 running → done → ギャップ埋めの後ろに作り直しを積むと running → 接続先が 503 だと incomplete → やり残しの格上げで running → done を確かめる。`TestRebuildStateCoversASaveThatStartsARebuild` (httpapi) が、埋め込みの接続先を変える保存で始まった作り直しの間 GET が running、終われば done、埋め込み無効なら idle を返すことを確かめる。変異を3つ入れて (保留中を数えない / 完了を記録しない / やり残しを見ない) それぞれ落ちることを確認した
- AC#1〜#3: `pnpm build:client` の成果物を、API をモックした node サーバー (作り直しを 6 秒 running にする) で配り、アプリ内ブラウザ (Chromium 系。WKWebView の実窓ではない) で確かめた。押すと running の文と aria-disabled・理由が出て、もう一度押しても POST は1回のまま。終わると完了の文に変わり読み上げられる。閉じて開き直すと完了の文が出る。走行中に閉じて開き直すと running と無効が出る。埋め込みモデルを変えて保存すると running と無効になり、終わりきらなかった結果では incomplete の文が出て読み上げられる
- `go vet ./...`・`go test ./...`・`pnpm check:client`・`pnpm test:client`・`pnpm build:client` 通過。`gofmt -l` は既存の `internal/search/model.go` だけを挙げる (本タスクでは触れていない)

## 確かめていないこと

- 実窓 (WKWebView) での見え方と読み上げ。4配色の比は測っていない (区画の文は既存の `Subtle`、ボタンは既存の無効の形をそのまま使い、新しい色・部品は足していない)

## レビューと実機確認

- PR #75 の Codex レビュー1回目の [P2]: 走行中を見た後に状態の読み取りが1回失敗するとポーリングが止まり、終わっても無効と「作り直しています」が残る。失敗した読み取りも同じ 2 秒間隔で再試行するようにし、ポーリングを `frontend/src/components/rebuildPoll.ts` に切り出して `frontend/test/rebuildPoll.test.ts` で再試行と停止を確かめた (3f889b9)。[P3] の波括弧は切り出しで解消。2回目で APPROVE
- 2026-10-03 ユーザーが mac 実機 (実窓) で動作を確認した
<!-- SECTION:NOTES:END -->
