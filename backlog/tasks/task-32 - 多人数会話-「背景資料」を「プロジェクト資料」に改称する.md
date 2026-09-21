---
id: TASK-32
title: '多人数会話: 「背景資料」を「プロジェクト資料」に改称する'
status: Done
assignee: []
created_date: '2026-09-21 11:19'
updated_date: '2026-09-21 11:39'
labels: []
milestone: m-1
dependencies: []
references:
  - docs/multi-agent-chat-design.md
  - internal/service/turncontext.go
  - internal/service/turnengine.go
  - internal/model/model.go
  - internal/repository/participant.go
  - internal/db/schema.go
  - internal/preset/preset.go
  - frontend/src/i18n/index.tsx
  - presets/multi-agent/README.md
ordinal: 32000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-18 が導入し TASK-20 が参加者ごとの可否を付けた「背景資料」は、ターンの system prompt 先頭に置く、
そのチャットが属するプロジェクトの説明・検索で得たドキュメント抜粋・検索で得たメモリのまとまりを指す
(設計書 §4.4)。TRPG の用途では、この中身はシナリオ・ダンジョンの構造・NPC の真意といった、
会話の主題そのものになる資料である。「背景」はそれを表していない (2026-09-21、ユーザー指摘)。

## 用語

**プロジェクト資料**とは、ターンの system prompt 先頭に置く、そのチャットが属するプロジェクトの説明・
検索で得たドキュメント抜粋・検索で得たメモリのまとまりを指す。指示対象は現行の「背景資料」と同一で、
語だけを替える。

「共有資料」を採らない理由は 2 つある。(1) TASK-20 以降この資料は共有されるとは限らず、GM だけが読む資料も
同じ語で呼ぶことになるので、語が事実と逆を指す。(2) TASK-31 が導入する部分集合 (話者の設定に関わらず渡す分) が
「共通共有資料」になり、共有と共通という近い語が別の集合を指す構図になる。出どころで名指す
「プロジェクト資料」はどちらの問題も起こさない。

## 改称の対応

| 現行 | 改称後 |
| --- | --- |
| 背景資料 | プロジェクト資料 |
| 「背景資料を渡す」(参加者の真偽値) | 「プロジェクト資料を渡す」 |
| prompt の見出し `【背景資料】` | `【プロジェクト資料】` |
| DB 列 `participants.receives_background` | `receives_project_material` (rename migration) |
| Go `model.Participant.ReceivesBackground` | `ReceivesProjectMaterial` |
| JSON・TS `receivesBackground` (参加者・プリセット) | `receivesProjectMaterial` |
| Go `TurnBackground` / `TurnBackgroundAssembler` / `AssembleTurnBackground` | `TurnMaterial` / `TurnMaterialAssembler` / `AssembleTurnMaterial` |
| Go 定数 `turnBackground*` / `turnBackgroundHeader` | `turnMaterial*` / `turnMaterialHeader` |
| i18n キー `participants.receivesBackground` / `participants.receivesBackgroundHint` | `participants.receivesProjectMaterial` / `...Hint` |
| ログ `[turn] background unavailable` | `[turn] project material unavailable` |

Go の記録型に `Project` を付けないのは、`service` の中では出どころが型のコメントで足りるためである。
参加者の真偽値には付ける: `Participant` の上では役割プロンプト・場面設定と並ぶので、出どころが語に要る。

## 方針

1. **今やる理由**: プリセット JSON の `receivesBackground` は公開フォーマット (`presets/multi-agent/README.md`) だが、
   2026-09-21 に入ったばかりで、同梱プリセット 7 件・追加分 17 件のいずれも使っていない。
   互換の負債が実質ゼロの今なら、旧名の受け入れを残さず単純な改称で済む。
2. **旧名は残さない**。`receivesBackground` を読み続ける後方互換はプリセットの parser に入れない。
   上記の理由で、受け入れるべき既存ファイルが存在しないため。
3. **DB は列の rename**。`ALTER TABLE participants RENAME COLUMN receives_background TO receives_project_material`
   を migration 012 として足す。列を作り直して値を移す形は採らない (SQLite 3.25 以降の RENAME COLUMN で足り、
   既存の値がそのまま残る)。
4. **既に完了したタスクの本文は書き換えない** (TASK-18・TASK-20 の Description / Implementation Notes は
   その時点の記録)。未着手の TASK-31 の本文と AC は新しい語彙に直す。
5. 設計書は改称を記録する。§4.4 に「2026-09-21 に『背景資料』から改称した。指示対象は変えていない」を残し、
   語だけ替わったことが後から読めるようにする。

## スコープ外

- 指示対象の変更。組み立ての内容・予算・検索クエリ・参加者ごとの可否の挙動はいずれも現行のまま。
- TASK-31 の実装 (共通プロジェクト資料と、ドキュメントごとの「全参加者に渡す」)。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 プロジェクト資料・「プロジェクト資料を渡す」の語で、設計書 (§0/§3/§4.4/§5/§6)・current-spec 日英・プリセット README が書かれている
- [x] #2 DB 列が receives_project_material に rename され、既存の値が保たれる (migration 012)
- [x] #3 Go・TypeScript の識別子、prompt の見出し、i18n キーと文言 (en/ja)、ログ文言が対応表どおりに改称されている
- [x] #4 プリセット JSON のフィールドが receivesProjectMaterial になり、旧名 receivesBackground を受け入れる経路が残っていない
- [x] #5 指示対象が変わっていないことが確認できる: go test ./... と pnpm check:client / build:client が通り、改称前と同じ挙動のテストが同じ内容で通る
- [x] #6 未着手の TASK-22・TASK-31 の本文が新しい語彙に直っている (完了済みの TASK-18・TASK-20 の本文はその時点の記録として残す)
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## TASK-32 の実装 (2026-09-21)

**改称の範囲**: 「背景資料」→「プロジェクト資料」。指示対象は変えていない。組み立ての内容・予算・検索クエリ・
参加者ごとの可否の挙動は改称前のままで、既存テストは名前だけ替わって同じ内容で通る。

**残したもの (意図的)**:
- migration の id `011_participant_receives_background` は改称しない。id は `schema_migrations` に記録される値なので、
  変えると導入済みのデータベースが 11 を再実行し、既にある列に `ADD COLUMN` を当てて失敗する。
  schema.go にその理由をコメントで残した。migration 012 が列を rename する。
- 完了済みの TASK-18・TASK-20 の本文はその時点の記録として残した。未着手の TASK-22・TASK-31 は新しい語彙に直した。
- `docs/multi-agent-chat-notes.md` の「背景資料」は別の指示対象 (設計書にとっての事前検討の記録) なので触っていない。

**Go の識別子の付け方**: 記録型は `TurnMaterial` / `AssembleTurnMaterial` / `turnMaterial*` で `Project` を付けず、
参加者の真偽値は `ReceivesProjectMaterial` / `receives_project_material` で付ける。理由は、`service` の中では
出どころが型のコメントで足りるのに対し、`Participant` の上では役割プロンプト・場面設定と並ぶので出どころが語に要ること。
設計書 §4.4 に記録した。

**旧名の受け入れは残していない**。プリセット JSON の `receivesBackground` は 2026-09-21 に入ったばかりで、
同梱 7 件・追加分 17 件のいずれも使っていないため、parser に後方互換を入れる必要が無い。

**確認したこと**:
- `go test ./... -count=1` 全パス、`go vet ./...` 無指摘、変更した .go は `gofmt -l` 無指摘。
- `pnpm check:client` と `pnpm build:client` 成功。
- 追加したテスト: `TestReceivesProjectMaterialRename` (011 まで適用した DB に 1 と 0 の参加者を入れてから全 migration を適用し、
  rename 後に値が保たれることを確認)。これが AC #2 の根拠。
- 危険な一括置換を避けた箇所: Wails の `WindowSetBackgroundColour`、`context.Background()`、styled-components の
  CSS `background:` は対象外。識別子単位で置換したうえで、残った `background` を 1 件ずつ確認した。

**未確認**: 実機での表示。i18n のキー名と文言が両方変わっているので、編成パネルのラベルとツールチップが
「プロジェクト資料を渡す」で出ることは実機で見ておきたい。型検査とビルドまでは通っている。

**実機確認 (2026-09-21、マージ前)**: 編成パネルのラベルとツールチップが「プロジェクト資料を渡す」で表示されることをユーザーが目視で確認済み。
<!-- SECTION:NOTES:END -->
