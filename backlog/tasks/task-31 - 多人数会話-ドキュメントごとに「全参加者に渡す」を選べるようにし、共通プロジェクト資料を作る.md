---
id: TASK-31
title: '多人数会話: ドキュメントごとに「全参加者に渡す」を選べるようにし、共通プロジェクト資料を作る'
status: In Review
assignee: []
created_date: '2026-09-21 11:11'
updated_date: '2026-09-21 20:17'
labels: []
milestone: m-1
dependencies:
  - TASK-20
  - TASK-32
references:
  - docs/multi-agent-chat-design.md
  - internal/doccategory/doccategory.go
  - internal/repository/document.go
  - internal/service/turncontext.go
  - internal/db/schema.go
  - frontend/src/pages/ProjectDetailPage.tsx
ordinal: 31000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-20 で参加者に「プロジェクト資料を渡す」の真偽値を入れた。false の参加者のターンではプロジェクト資料を一切組み立てない
(検索もしない)。TRPG の GM 用途としては成立するが、プレイヤー役の参加者はプロジェクト説明・世界設定・ルールのような
共有前提まで読めなくなる。実際の卓では、ルール・世界観・用語集は全員が知っていて、シナリオだけ GM が持つ。
TASK-20 の「既知の制約」(設計書 §4.4) はこれを参加者側の 2 値では表現できないこととして記録してある。

## 提案 (ユーザー、2026-09-21)

ドキュメント側に属性を持たせ、「プロジェクト資料を渡す」の on/off によらず渡るものと、on のときだけ渡るものに分ける。
カテゴリ (world / character / rule / plot / timeline / index / story / misc) を手掛かりにする案が出ている
(例: rule・world・index・misc は常に共有、それ以外は on のときのみ)。

## 用語

- **共通プロジェクト資料**とは、話者がプロジェクト資料を渡す設定かどうかに関わらず渡す、プロジェクト資料 (設計書 §4.4) の部分集合を指す。
  現状は該当するものが無く、常に空である。
- **「全参加者に渡す」**とは、そのドキュメントを共通プロジェクト資料に含めることを指す
  (`shared_with_all` / `SharedWithAll` / `sharedWithAll`)。

## 方針

1. 参加者 → ドキュメント ID の集合を持つ案 (設計書 §4.4 の「既知の制約」) より先にこれをやる。
   ドキュメント 1 件につき真偽値 1 つで済み、TRPG という当面の用途は「全員 / 渡す参加者だけ」の 2 値で足りる。
   役割別レビュー (参加者ごとに異なる部分集合) は引き続き別タスク。

2. **着手時の判定 — 本タスクの主要論点**: カテゴリから「全参加者に渡す」を *導出する* のか、
   *新規登録時の初期値を提案するだけ* にしてドキュメントごとの明示値を持つのか。

   **推奨は後者 (明示値 + カテゴリからの初期値提案)**。理由:
   - カテゴリは明示指定が無ければ `doccategory.Infer` の推定値である
     (`internal/repository/document.go` の CreateDocument)。推定はファイル名・タイトル・本文の正規表現一致で、
     `misc` は「どれにも一致しなかった」ときの落とし所である (`doccategory.go` の categoryOrder 末尾)。
   - 隠蔽の判断を推定値に結び付けると、plot の文書が world と推定された 1 件で、シナリオがプレイヤーに渡る。
     推定が外れていることは画面に出ないので、渡ってしまったことにも気付けない。
   - 提案にあった「その他 (misc) も常に共有」は特に危ない。misc は分類に失敗した文書の行き先なので、
     「分類できなかったものは全員に見せる」という規則になる。
   - 推奨する形: `documents.shared_with_all` を明示の真偽値として持ち、**既定は false**
     (= TASK-20 の現状維持。渡さない参加者には何も渡らない)。新規登録時の初期値をカテゴリから提案し
     (rule / world / index を true)、ドキュメント一覧・詳細で 1 クリックで変えられるようにする。
     既定を false にするのは fail-closed のため: 既定を true 側にすると、本機能を入れた瞬間に
     TASK-20 が意図的に伏せた資料がプレイヤーに流れ始める。

3. **検索の扱い**: 渡さない参加者のターンでは、共通プロジェクト資料に限定した検索を 1 回行う。TASK-20 で
   「検索そのものを行わない」にした `AssembleTurnMaterial` 先頭の分岐を、「共通のみを対象に検索する」に変える。
   絞り込みは検索の前 (SQL 側) で行う。検索後に落とすと、予算 (設計書 §4.4) を渡らない文書が食う。
   予算を TASK-18 の定数のまま使うか、共通だけのターン用に別に持つかは実装時に決める。

4. **メモリの扱い**: 着手時にスコープに入れるかを決める。memories の kind (semantic / procedural / episodic) は
   内容の種類であって公開範囲ではないので、そのまま流用はできない。会話から作られたメモリ (TASK-19 の
   「発言のメモリ保存」) は全員が見聞きした内容なので既定で共通扱いにできる可能性があるが、GM の独白を
   保存した場合が例外になる。メモリを本タスクの対象外にするなら、渡さない参加者のターンではメモリを
   引き続き一切渡さない (TASK-20 のまま) と設計書に明記する。

5. **既存語の指示対象が変わる**: 「プロジェクト資料を渡す」false の意味が「何も渡さない」から
   「共通プロジェクト資料だけ渡す」に変わる。設計書 §4.4 と編成パネルのヒント文
   (`participants.receivesProjectMaterialHint`、en / ja 両方) を同じ変更で改訂する。

## スコープ外

- 参加者ごとに異なるドキュメントの部分集合 (役割別レビュー用途)。参加者 → ドキュメント ID の集合が要る別タスク。
- カテゴリ自体の増減。既存の 8 種をそのまま使う。
- 画像ドキュメントは本文と同じ扱い (derived_text も共通プロジェクト資料の対象になる)。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 ドキュメントごとに「全参加者に渡す」を画面で切り替えられ、既定は渡さない (TASK-20 の挙動を変えない)
- [x] #2 プロジェクト資料を渡さない参加者のターンでは、「全参加者に渡す」ドキュメントだけが検索・注入・参照保存の対象になる
- [x] #3 プロジェクト資料を渡す参加者のターンでは、TASK-20 以前と同じくすべてのドキュメントが対象になる
- [x] #4 カテゴリと「全参加者に渡す」の関係が方針 2 の判定どおりに実装され、判定の結論と理由が設計書に記録されている
- [x] #5 「プロジェクト資料を渡す」false の新しい意味 (共通プロジェクト資料だけ渡す) に合わせて、設計書 §4.4 と編成パネルのヒント文 (en / ja) が改訂されている
- [x] #6 メモリを対象に含めるかの判断 (方針 4) が結論と理由つきで設計書に記録されている
- [x] #7 メモリごとに「全参加者に渡す」を画面で切り替えられ、新規作成時の既定は source 由来 (multi_agent は true、manual / chat は false)
- [x] #8 プロジェクト資料を渡さない参加者のターンでは、「全参加者に渡す」メモリだけが検索・注入・参照保存の対象になる
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
着手時の判定 (方針 2 / 3 / 4) を先に確定した。

判定 A (方針 2, AC #4): カテゴリからの導出も、登録時の初期値提案も入れない。shared_with_all は明示の真偽値 1 つで、
新規登録時は一律 false。理由: ドキュメント登録画面 (ドラッグ&ドロップ / ファイル選択) にカテゴリの入力欄が無く、
登録時のカテゴリは常に doccategory.Infer の出力である。そこから初期値を作ると、方針 2 が退けた
「推定が公開範囲を決める」形そのものになる。カテゴリを人間が選べるのは詳細モーダルだけで、
そこには同じ画面に「全参加者に渡す」が並ぶので提案の余地が無い。

判定 B (方針 3): 予算は TASK-18 の定数 (turncontext.go) をそのまま使い、共通だけのターン用に別枠を持たない。
プロンプトの形も履歴の長さも変わらず、ウィンドウの支払いが同じなので別の数字を置く根拠が無い。
渡さない参加者にはプロジェクト説明も渡さない (説明にもシナリオが書かれうる。AC #2 の「〜だけが」の読み)。

判定 C (方針 4, ユーザー 2026-09-22): メモリもスコープに入れる。memories.shared_with_all を持たせ、
新規作成時の既定を source から決める: multi_agent (発言由来 = 全参加者が見聞きした内容) は true、
manual / chat (多人数会話の場で発話されていない) は false。migration の既存行も同じ規則で backfill する。
source は記録された事実であって推定ではないので、判定 A が退けた形には当たらない。

1. migration 013: documents / memories に shared_with_all INTEGER NOT NULL DEFAULT 0 を足し、
   memories は source = 'multi_agent' の行を 1 に backfill する。
2. model.DocumentRecord / model.Memory に SharedWithAll を足し、repository の列・scan・Create を通す。
   UpdateDocumentSharedWithAll / UpdateMemorySharedWithAll を足す。
3. retrieval: MaterialScope (ScopeAll / ScopeSharedWithAll) を SearchDocuments / SearchMemories に足し、
   FTS 候補取得と semantic fallback の SQL に AND shared_with_all = 1 を付ける (絞り込みは検索前)。
4. turncontext: 先頭の早期 return を scope の選択に変える。渡さない参加者は ScopeSharedWithAll で
   検索し、プロジェクト説明は入れない。
5. HTTP: PATCH /api/documents/{id}/shared, PATCH /api/memories/{id}/shared。
6. 画面: ドキュメント一覧・詳細モーダル・メモリ一覧に 1 クリックのトグル。i18n を en / ja 両方。
7. 設計書 §4.4 を改訂 (共通プロジェクト資料の定義、false の新しい意味、判定 A / B / C の記録)、
   participants.receivesProjectMaterialHint を en / ja とも改訂。
8. go test ./... と frontend の型チェック / lint。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 着手時の判定

**判定 A (方針 2, AC #4) — カテゴリからは導出も初期値提案もしない。** `documents.shared_with_all` は明示の真偽値 1 つで、
新規登録時は一律 false。方針 2 が推奨した「カテゴリからの初期値提案」は**採らなかった**。理由: ドキュメント登録画面は
ドラッグ&ドロップとファイル選択だけでカテゴリの入力欄が無く (`ProjectDetailPage.tsx` の `handleUploadFiles` が送るのは
type / title / file のみ)、登録時のカテゴリは常に `doccategory.Infer` の推定値である。そこから初期値を作ると、
方針 2 が退けた「推定が公開範囲を決める」形そのものになる。カテゴリを人間が選べるのはドキュメント詳細モーダルだけで、
そこには「全参加者に渡す」のチェックボックスが同じ画面に並ぶので提案の余地が無い。
`CreateDocumentInput` には `SharedWithAll` を置かず、INSERT でも列を書かない (DB 既定の 0 に任せる)。

**判定 B (方針 3) — 予算は TASK-18 の定数のまま。** prompt の形も履歴の長さも変わらずウィンドウの支払いが同じなので、
共通だけのターン用に別枠を持つ根拠が無い。あわせて、渡さない参加者のターンには**プロジェクト説明も入れない**
(AC #2 の「〜だけが」の読み。説明は人間向けに書くもので、伏せている当のものを名指しすることがあり、
その一部だけを共通と見なす印も無い)。`buildTurnMaterial` に nil の project を渡す形で表現した。

**判定 C (方針 4, AC #6) — メモリもスコープに入れる (ユーザー判断 2026-09-22)。** `memories.shared_with_all` を持たせ、
新規作成時の既定を `source` から決める: `multi_agent` (発言から保存したもの) は true、`manual` / `chat` / `organized` は false。
ユーザーの理由は「このアプリの多人数会話には他の参加者に見えない発言が無いので、発言由来のメモリは既に共有されている」。
当初の提案 (一律 true) に対し、メモリには手動追加・単独チャット抽出・organizer 由来もあり、それらは会話の場で
発話されたことが一度も無い点を指摘して source で分ける形に落とした。`source` は記録された事実であって推定ではないので、
判定 A が退けた形には当たらない。migration 013 は既存行にも同じ規則を当てる。

## 実装

- migration 013 (`internal/db/schema.go`): documents / memories に `shared_with_all INTEGER NOT NULL DEFAULT 0`、
  memories は `source = 'multi_agent'` の行を 1 に backfill。
- 絞り込みは**検索の前**、SQL 側で行う (`MaterialScope` = `ScopeAll` / `ScopeSharedWithAll`、`retrieval.go`)。
  FTS 候補取得と埋め込みの semantic fallback の両方に `AND d.shared_with_all = 1` / `AND m.shared_with_all = 1` を付ける。
  検索後に落とすと、2 件 × 2 chunk + メモリ 3 件しかない予算を渡らない行が食い切り、共通のものが 1 件も残らないターンが出る。
- `AssembleTurnMaterial` の先頭の早期 return を scope の選択に置き換えた (`turncontext.go`)。
- API: `PATCH /api/documents/{id}/shared`、`PATCH /api/memories/{id}/shared` (body `{ sharedWithAll: boolean }`)。
- 画面: ドキュメント一覧・メモリ一覧に 1 クリックのアイコンボタンと真のときだけ出るバッジ、
  ドキュメント詳細モーダルにチェックボックスと 1 行の説明。i18n は en / ja 両方。
- 文書: 設計書 §4.4 (共通プロジェクト資料の定義・判定 A / B / C・「既知の制約」から解消分を削除)、§5 の API 表、
  §6 に操作の置き場所。`participants.receivesProjectMaterialHint` を en / ja とも改訂 (AC #5)。
  波及先として `docs/current-spec.md` / `.ja.md`、`presets/multi-agent/README.md` も改訂した。

## 検証

- `go build ./...` / `go vet ./...` / `go test ./...` すべて通過 (2026-09-22)。
- `pnpm check:client` (tsc --noEmit) と `pnpm build:client` 通過。
- AC #1: `TestAssembleTurnMaterialEmptyWhenNothingShared` — 何も共有していない状態では渡さない参加者の資料が空
  (TASK-20 と同じ)。`handlers_test.go` で、category を `world` と明示して作ったドキュメントでも `sharedWithAll` が false であること、
  PATCH の 400 / 200 / 404 を確認。
- AC #2 / #8: `TestAssembleTurnMaterialNarrowsToCommonMaterial` — 同じ会話に一致する「共通」ドキュメント + メモリと、
  一致するが共通でないドキュメント (灯台守の記録) / メモリ (オルガの過去) / プロジェクト説明 (霧の港町ハーバーン) を並べ、
  prompt と保存参照に前者だけが出ることを確認。
- AC #3: `TestTurnEngineNarrowsMaterialToCommonForSpeaker` — 同じ会話で、渡す参加者は共通でないドキュメントも受け取る。
- AC #7: `TestSharedProjectMaterialBackfill` (migration の既定と backfill)、
  `multiagent_memory_test.go` (発言から保存したメモリが `sharedWithAll` 真)、`handlers_test.go` (手動メモリは偽、PATCH の 400/200/404)。
- TASK-20 の挙動を固定していた `TestAssembleTurnMaterialSkipsSpeaker` / `TestTurnEngineSkipsMaterialForSpeaker` は、
  本タスクが意図的に変える挙動なので上記 2 件に置き換えた。

## 未測定 (レビュー時に人間の目で見てほしい点)

- 実機 (Wails WebView) での見え方。一覧のアイコンボタン 2 つ (共有トグル + 削除) が並んだときの間隔と、
  バッジが 3〜4 個並んだときの折り返しは実機で確認していない。
- 既存 DB を migration 013 に通したときの実データでの挙動 (テストは合成データ)。
<!-- SECTION:NOTES:END -->
