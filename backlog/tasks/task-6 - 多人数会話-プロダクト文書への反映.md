---
id: TASK-6
title: '多人数会話: プロダクト文書への反映'
status: Done
assignee: []
created_date: '2026-09-08 22:28'
updated_date: '2026-09-19 09:58'
labels: []
milestone: m-0
dependencies:
  - TASK-5
references:
  - docs/multi-agent-chat-design.md
ordinal: 6000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
多人数会話の実装完了後、AGENTS.md の Core product shape・README・docs/current-spec (ja/en) へ機能を追記し、docs/multi-agent-chat-design.md のステータスを DRAFT から確定へ更新する (§8 未決の解消内容を反映)。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 AGENTS.md / README / current-spec に多人数会話が記載されている
- [x] #2 設計書の未決事項が解消済みとして更新されている
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. 対応表を先に確定する (設計書ステータスの新段階名・§8 の 2 群の名・編成/roster の英語語彙)。確定済み。
2. docs/multi-agent-chat-design.md: 冒頭ステータスを DRAFT / 実装前設計 → 確定 / 実装済み にし、「この文書は確定仕様ではない」の但し書きを実装済み文書としての読み方に差し替える。§8 を「実装で解消した判断」と「将来拡張で判断する事項」の 2 群に組み替え、AGENTS.md 追記文面の項目を解消として畳む (追記先を指す)。
3. AGENTS.md と AGENTS.ja.md: Core product shape / コアとなる製品構造に多人数会話を追記する (chat の種別として書き、参加者・ターン進行ルール・場面設定・プリセットの存在に触れる)。Required UX 側にも 1 行足す。
4. README.md: できること に多人数会話を追加、構成ツリーを実態に合わせる (9→10 migrations, service に turnengine, internal/preset, presets/, frontend の MultiAgentChatPage/ParticipantPanel, ルート数 22→35)、データモデルに participants、API の要点に turns/stream と multi-agent-presets。
5. docs/current-spec.md と docs/current-spec.ja.md: 多人数会話の節を新設 (参加者と編成・ターン進行ルール・場面設定・プリセット・1 リクエスト = 1 ターン・切断してもターンは完走する・人間の介入発言)、テーブル一覧に participants、API 群に participants CRUD / turn stream / preset 一覧、最終更新日を更新。
6. 検証: go vet ./... / go test ./... / gofmt -l / pnpm check:client。文書のみの変更なので回帰は想定しないが、プロジェクトのゲートとして通す。設計書とコードの一致 (ルート・テーブル・定数) は grep で実機の値を確認してから書く。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 対応表を先に確定した (semantic-generation)
争点語は 4 語: 設計書のステータス段階名 (確定 / 実装済み)、§8 を割る 2 群の名 (実装で解消した判断 /
将来拡張で判断する事項)、英語版の「編成」(roster)。用語そのものは設計書 §2 と出荷済み UI の i18n
(en: participant / roster / turn rule / scene / preset、ja: 参加者 / 編成 / ターン進行ルール / 場面設定 /
プリセット) をそのまま流用し、新語は上記 3 つの段階名・群名だけに留めた。いずれも初出定義を本文に書いている。

## AGENTS.md への書き方 (§8 の未決だった論点)
多人数会話を projects / documents / chats / memories と**並べず**、chat の 2 種別 (単独 assistant /
多人数会話) として書いた。並列の箇条書きに 5 つ目として足すと、project 配下に別系統のデータがあるように
読めるため。設計書 §2 の「多人数会話とは ... chat を指す」と同じ切り方で、AGENTS.ja.md / README /
current-spec (ja/en) も同じ区分で通してある。判断とその理由は設計書 §8.1 に記録した。

## 設計書のステータス
DRAFT / 実装前設計 → **確定 / 実装済み** (2026-09-19)。§8 は「実装で解消した判断」(履歴圧縮の見送り・
プリセットの保存形式・AGENTS.md の追記文面) と「将来拡張で判断する事項」(参加者ごとの API キー列・
多人数会話への review 適用・人間が参加者枠で発言するモードの UI) に組み替えた。
後者を残したままステータスを確定にしたのは、3 件がいずれも §7「将来」の拡張に着手するときの論点で、
現行実装の完成を妨げないため。§7 の表の下に Phase A〜C 完了を明記し、§1 / §4.3 / §6 に残っていた
「§8 未決」参照も解消後の表現へ直した。

## 実装との突き合わせ (記載の根拠)
文書に書いた値はすべてコードで確認した: migration は 010_multi_agent_chat までの 10 本、`/api` ルートは 35
(README の 22 は古い値だった)、`turnHistoryLimit = 30` (turnengine.go:36)、重複ターンは
`http.StatusConflict` (multiagent.go:208)、`multi_agent` chat への message ルートは生成せず保存のみ
(handlers.go:858 付近)、同梱プリセット 7 件 / `presets/multi-agent/` 17 件、除籍済み参加者は編成とは別に
一覧 (ParticipantPanel.tsx:241)。README の構成ツリー・レイヤ一覧・データモデル・API の要点もこの実測値に
合わせて更新した。

## 検証
go vet ./... / go test ./... (全パッケージ ok) / pnpm check:client (tsc --noEmit、exit 0)。
文書のみの変更なので回帰は想定していないが、プロジェクトのゲートとして通した。
`gofmt -l` は `internal/search/model.go` を挙げるが、これは本タスク以前から (9c816a1) の未整形で、
本タスクでは触っていないため直していない。
<!-- SECTION:NOTES:END -->
