# 多人数会話エンジン — snz_studio 組み込み設計

ステータス: **確定 / 実装済み**（2026-09-09 起票、2026-09-19 確定） / 背景: [multi-agent-chat-notes.md](./multi-agent-chat-notes.md)

> 目的: 役割プロンプトとターン進行ルールを差し替え可能な多人数会話エンジンを snz_studio に組み込み、
> LAN 上の複数 LM Studio エンドポイントを使った AI 同士の会話（ディベート・即興劇、将来 TRPG）を
> 既存の project / chat 体験の中で実現する。
> 「確定 / 実装済み」とは、本文の記述が実装と一致し、以後は実装の変更に追随して改訂する段階を指す。
> §7 の Phase A〜C は完了しており、本文はそのまま現在の仕様として読める。実装の入口は
> [turnengine.go](../internal/service/turnengine.go) / [preset](../internal/preset) /
> [ParticipantPanel.tsx](../frontend/src/components/ParticipantPanel.tsx) /
> [MultiAgentChatPage.tsx](../frontend/src/pages/MultiAgentChatPage.tsx)。
> 製品文書側の記載は [AGENTS.md](../AGENTS.md) の Core product shape・[README](../README.md)・
> [current-spec](./current-spec.md)（[ja](./current-spec.ja.md)）にある。

---

## 0. ゴールとスコープ

- 1 つの chat の中で、**接続先の異なる複数のモデルが役割を持って発言し合う**。発言はストリーミング表示され、既存の messages として永続化される。
- ディベート・即興劇などの用途は**個別機能にしない**。エンジンは 1 つで、用途差はプリセット（§6、= データ）で表現する。
- **非ゴール（今回やらない）**: TRPG の状態管理（キャラクターシート・ダイス・構造化出力による判定）、進行役モデルによる発言者指名、複数マシンの並列生成、retrieval（documents / memories）との統合。いずれも将来拡張（§7・§8）。

AGENTS.md の Constraints（local-only / single-user / no heavy real-time architecture / no over-engineering）は維持する。LAN 上の LM Studio は「外部インフラ」に当たらない（既存の外部 LLM エンドポイント方式と同じ扱い）。

## 1. 現状の再確認（再利用できる資産）

| 既存資産 | 本設計での使い方 |
| --- | --- |
| `CompletionTarget` / `resolveTarget()` [llmclient.go](../internal/service/llmclient.go) | 呼び出しごとに参加者の接続先 base URL とモデル名を上書き。**LLMClient は無改修** |
| `CreateChatCompletionStream()` + SSE 経路 [llmclient.go](../internal/service/llmclient.go) / [handlers.go](../internal/httpapi/handlers.go) | ターンの発言をストリーミング配信。既存 `/messages/stream` のハンドラパターンを踏襲 |
| `ListModels` / `EnsureModelLoaded` / `CheckConnection` | 参加者編成時の接続確認と、ターン実行前のモデルロード確認 |
| `messages` テーブル（`model_name` 列あり） | 発言ログの保存先。列追加のみで流用（§3） |
| `internal/db` の migration 機構（9 本の実績） | 10 本目としてスキーマ変更を追加 |
| `summary` サービス | 履歴圧縮への流用候補だったが**流用しない**（§8「実装で解消した判断」） |

## 2. 用語とドメインモデル

- **参加者**とは、多人数会話に属する「表示名 + 役割プロンプト + 接続先 base URL + モデル名」の設定の組を指す。
- **役割プロンプト**は、参加者ごとにモデルへ system メッセージとして渡す、人格・立場・口調・行動規則を指示する文字列。
- **多人数会話**とは、参加者を 2 つ以上持ち、発言生成をターンエンジンが担う chat を指す。既存の単独 assistant の chat と同じ `chats` テーブルに保存し、種別列 `kind` で区別する。
- **ターン**は、ターン進行ルールで選ばれた 1 人の参加者が、モデル呼び出し 1 回分の発言を生成し `messages` に保存されること。
- **ターン進行ルール**（`turn_rule`）は、次に発言する参加者を決める規則。初期実装は 2 種:
  - `round_robin`: 参加者の登録順（`sort_order`）を循環。直近の参加者発言から次を導くため、サーバー側に進行状態を持たない。
  - `manual`: リクエストで参加者を指名。
- **ターンエンジン**（`TurnEngine`）とは、ターン進行ルールに従い参加者のモデル呼び出しと発言保存を繰り返す Go サービスを指す。
- **場面設定**（`scene_prompt`）とは、多人数会話の全参加者のシステムプロンプトに共通して前置される chat 単位の文字列（論題・シーン・世界観など）を指す。
- **プリセット**とは、参加者一式・ターン進行ルール・場面設定の雛形をまとめた、多人数会話に適用できるデータを指す。適用できるのは新規作成時と、発言がまだ 1 件も無い多人数会話に対してである（§5）。

## 3. スキーマ（migration 10）

```sql
ALTER TABLE chats ADD COLUMN kind TEXT NOT NULL DEFAULT 'assistant';       -- 'assistant' | 'multi_agent'
ALTER TABLE chats ADD COLUMN turn_rule TEXT NOT NULL DEFAULT 'round_robin'; -- multi_agent のみ意味を持つ
ALTER TABLE chats ADD COLUMN scene_prompt TEXT NOT NULL DEFAULT '';

CREATE TABLE participants (
  id           TEXT PRIMARY KEY,
  chat_id      TEXT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
  display_name TEXT NOT NULL,
  role_prompt  TEXT NOT NULL,
  base_url     TEXT NOT NULL,
  model_name   TEXT NOT NULL,
  sort_order   INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL,
  deleted_at   TEXT                    -- NULL = 編成に在籍。非 NULL = 除籍済み（過去の発言の帰属だけ残す）
);

ALTER TABLE messages ADD COLUMN participant_id TEXT; -- NULL = 従来の user / assistant 発言
```

- 参加者の発言は `role = 'assistant'` + `participant_id` で保存する。表示名・モデル名は participants を引く（`model_name` 列には従来どおり実際に使ったモデルも記録する）。
- 人間の介入発言（論題の追加投入・野次など）は従来どおり `role = 'user'`・`participant_id IS NULL`。
- 既存 chat は `kind = 'assistant'` のまま一切影響を受けない。
- **参加者の除籍は論理削除**（`deleted_at` を立てる）とし、行は消さない。`messages.participant_id` には
  意図的に外部キーを張らない（既存 `messages` への `ALTER TABLE` を最小に保つため）が、行が消えないので
  過去の発言の表示名は常に解決できる。
  **Why**: 物理削除にすると、(1) 過去の発言の `participant_id` が解決不能になり観戦ビューが表示名を失う、
  (2) `round_robin` の「直近の参加者発言 → 次」が起点を失って次の発言者が決まらない。
  いずれもデータが壊れるのではなく既存の履歴の読み方が壊れるので、除籍は在籍フラグで表す。
- したがって参加者の集合は 2 種類ある。**編成**（`deleted_at IS NULL`、`round_robin` の巡回対象・編成パネルの表示対象）と、
  **帰属解決用の全行**（表示名・モデル名の解決対象）。repository 層は両者を別メソッドで返す。
- `participant_id` は同一 chat の participants しか指さない。ただし ID の解決範囲はルートの形で決まるので、
  検証の意味があるのは chat がルートに現れる操作だけである:
  - `POST /api/chats/{chatId}/turns/stream` の `manual` 指名: `chatId` と `participants.chat_id` の
    一致をサービス層で検証し、他 chat の参加者を指名したら 404。ここは比較対象が 2 つあるので本当の検証になる。
  - `PATCH` / `DELETE /api/participants/{participantId}`: 対象の chat はその参加者自身の `chat_id` で決まり、
    比較相手が無い。よって「同一 chat か」を問う余地はなく、存在しなければ 404 とするだけでよい。
    **Why**: これは既存の API 規約に合わせた結果である。既存ルートも、作成は親配下
    （`POST /api/projects/{projectId}/memories`）、更新・削除は自身のフラットな ID
    （`DELETE /api/memories/{memoryId}` / `PATCH /api/documents/{documentId}/category`）で統一されている。
    単一ユーザー・認証なし（AGENTS.md Constraints）なので、ここに越えるべき権限境界も無い。

## 4. ターンエンジン（`internal/service/turnengine.go`）

### 4.1 1 リクエスト = 1 ターン

サーバー側に常駐の進行ジョブを持たない。1 回の API 呼び出しが 1 ターンを実行して SSE で流し、
連続進行（自動進行）はフロントエンドが次ターンの呼び出しを繰り返すことで実現する。
**Why**: 常駐ジョブとその状態管理を持たずに済み、AGENTS.md の「No heavy real-time architecture」に収まる。
自動進行の停止 = フロントが次のターンを呼ばないこと。

**ターンは HTTP リクエストの寿命に縛られない**（既存 SSE 経路の実際の挙動）。
`CreateChatCompletionStream` は呼び出し元の `context` ではなく `context.Background()` +
チャンク到着ごとに延びるスライディング期限で動き（[llmclient.go](../internal/service/llmclient.go)）、
SSE の書き込み失敗も無視される（[handlers.go](../internal/httpapi/handlers.go) の `_ = sse.Event(...)`）。
つまり進行中のターンは、クライアントが切断してもモデル生成を完走し `messages` に保存される。
本設計はこれを**仕様として受け入れる**:

- 停止できる粒度はターン境界のみ。「自動進行の停止」は進行中のターンを中断しない。UI もそう表示する。
- 切断・リロード後の復帰は、`messages` を読み直すこと（完走したターンはそこに入っている）。
  取りこぼした delta を再送する仕組みは持たない。
- 生成の中断が必要になったら、`TurnEngine` と `LLMClient` に呼び出し元 `context` を通す変更が前提になる
  （既存の単独 assistant チャットにも影響する変更なので、本設計の範囲外・§7 将来）。

### 4.2 ターンの処理手順

0. **その chat のターン実行権を取る**（chat 単位の in-process な排他。取れなければ 409 を返して終了）。
   **Why**: 発言者の決定は「`participant_id` を持つ直近メッセージ」を読んで行い、その結果が `messages` に入るのは
   ターン完了時（手順 5）。よって重なった 2 リクエストは両方とも同じ直近メッセージを読み、同じ参加者を選んで
   二重に発言させる。自動進行はフロント側のループなので（§4.1）、「1 ターン進める」の連打や
   自動進行と手動指名の競合で現実に重なる。単一ユーザー・単一プロセスなので `sync.Mutex` の
   `TryLock` 相当で足り、DB 側の予約列は要らない。
1. ターン進行ルールで発言者を決定（`round_robin`: `participant_id` を持つ直近メッセージ → 編成（`deleted_at IS NULL`）の `sort_order` 順の次。直近発言の参加者が除籍済みで巡回位置が定まらないときは編成の先頭から。`manual`: リクエストの `participantId` 必須）。
2. `EnsureModelLoaded` / `CheckConnection` で接続先を確認。確認もエラー文も `resolveTarget` で解決した後の
   接続先・モデル（= 手順 4 が実際に使う組み合わせ）で行う。**Why**: 参加者は接続先とモデルを空欄にすると
   ワークスペース設定をフィールドごとに独立して継承するので、参加者の生の値で確認すると、継承した側が
   空文字のまま渡り（`EnsureModelLoaded` は空のモデル名を常に拒否する）、エラー文も継承した側が空白になって
   何を試して失敗したのか読めない。失敗はターンをエラーで返す（他の参加者へのフォールバックはしない）。
3. プロンプトを組み立てる（§4.3）。
4. `CompletionTarget{BaseURL, Model}` を渡して `CreateChatCompletionStream` を実行、delta を SSE 転送。
5. 完了した発言を `messages` に保存（`participant_id`・`model_name`・既存の生成メトリクス列）。

### 4.3 プロンプト組み立て

OpenAI 互換 API には「多者会話」のロールが無いため、発言者本人の視点へ写像する（先行事例で一般的な手法）:

- `system` = 場面設定 + その参加者の役割プロンプト + 役割リマインド。
  役割リマインドは毎ターン付与する固定文（「あなたは<表示名>としてのみ発言する」「直前の発言に同意だけで終わらない」等）。
  **Why**: 小型モデルは履歴が伸びると役割を忘れ、同意で収束する（検討記録 §2）。
- 履歴の写像: 自分の過去発言 → `assistant`、他参加者と人間の発言 → `user`（内容を「<表示名>: <本文>」に整形）。
- **プロンプトの末尾は、その参加者が答えるべき直前の発言にする**。`LLMClient` が常に末尾へ置く user メッセージ（§1）に、
  写像した履歴の最後の発言をそのまま入れる。「次はあなたの番です」の進行文で終わらせるのは、答えるべき発言が無いとき
  （最初のターン、および `manual` で直前と同じ参加者を指名したとき）だけにする。
  **Why**: 推論してから答えるモデルは、末尾の進行文を「会話への指示」と読んで発言してよいかを推論し続け、
  内容を返さないことがある。内容が空のターンは保存できないので失敗になる。同梱の 20 の扉 + gemma-4-e4b で計測すると
  （2026-09-19）、進行文を末尾に置いた場合は 4 回中 2 回が空、残りも 1 語の回答に 184〜454 トークンを費やした。
  質問を末尾に置くと 4 回中 4 回が 6 トークンで正しく答えた。
- 履歴は直近 30 発言に制限する（定数 `turnHistoryLimit`）。要約による圧縮は行わない（理由は §8「実装で解消した判断」）。
- 役割リマインドの内容（Phase C で確定）: 「自分としてのみ発言し、他の参加者の発言や動作を代筆しない・1 発言に複数人分を入れない」
  「発言の先頭に名前や記号を付けない」「直前の発言のどこに反応しているかが分かるように述べ、同意だけで終わらない」
  「発言の長さは場面設定の指定に従う」。それぞれ小型モデルで実際に見えた崩れ方（履歴の「<表示名>: 本文」形の模倣、
  複数人分の生成、同調、長さ指定の忘却）への対処で、プリセット側には書かない。

### 4.4 既存 chat フローとの分離

既存の `service/chat.go`（retrieval・memory・summary が絡む単独 assistant 用の文脈組み立て）には手を入れず、
ターンエンジンは独立したサービスにする。共有するのは LLMClient と repository 層だけ。
`kind = 'multi_agent'` の chat に対する既存の応答生成ルート（`POST /messages` / `/messages/stream`）は、
生成を行わずユーザー発言の保存のみ行う（人間の介入発言用）。

## 5. HTTP API

| ルート | 内容 |
| --- | --- |
| `POST /api/projects/{projectId}/chats` | 既存を拡張: `kind: "multi_agent"` を受け付ける。任意で `presetId`（同梱プリセットの id）または `preset`（プリセットの JSON オブジェクトそのもの）を 1 つだけ受け付け、そのプリセットを適用する。両方指定・`kind` が `assistant` のときの指定は 400、未知の `presetId` は 404 |
| `GET /api/multi-agent-presets` | 同梱プリセットの一覧（`id` / `title` / `description` / `group` / `turnRule` / `scenePrompt` / `participants[]`） |
| `POST /api/chats/{chatId}/preset` | 既存の多人数会話にプリセットを適用する。body は作成時と同じ `presetId` または `preset` を 1 つだけ。単独 assistant の chat は 400、未知の `presetId` は 404、`messages` が 1 件以上ある chat は 409。応答は適用後の `{ chat, participants }` |
| `PATCH /api/chats/{chatId}` | 既存を拡張: `turnRule` / `scenePrompt` の更新を受け付ける |
| `GET /api/chats/{chatId}/participants` | 参加者一覧 |
| `POST /api/chats/{chatId}/participants` | 参加者追加 |
| `PATCH /api/participants/{participantId}` | 参加者更新（表示名・役割プロンプト・接続先・モデル・順序） |
| `DELETE /api/participants/{participantId}` | 参加者の除籍（論理削除。過去の発言の帰属は残る、§3） |
| `POST /api/chats/{chatId}/turns/stream` | 1 ターン実行（SSE）。body: `{ "participantId"?: string }`（`manual` 時必須）。当該 chat のターンが実行中なら 409 |

参加者の更新・削除を chat 配下に入れ子にせずフラットな ID にしているのは既存ルートの形に合わせたもので、
同一 chat 検証の要否もそこから決まる（§3 末尾）。
接続先ごとのモデル列挙は既存 `POST /api/configuration/models` を流用する。

**プリセットの適用**とは、chat の `turnRule` / `scenePrompt` をプリセットのものにし、`participants[]` を登録順（= round_robin の巡回順）に
参加者として作ることを指す。chat の title が空ならプリセットの `title` を使う。参加者の接続先・モデルは空で作られ、
ターン実行時はワークスペース既定のエンドポイントに落ちる（参加者ごとに変えるのは編成パネル）。
適用後の chat はプリセットとの結びつきを持たない（以後の編集はすべて編成パネル）。新規作成時に参加者の作成が途中で失敗したら chat ごと削除して返す
（repository 層にトランザクションが無く、編成が欠けた多人数会話を残さないため）。

既存 chat への適用（`POST /api/chats/{chatId}/preset`）は**発言が 1 件も無いあいだだけ**受け付ける。
サイドバーの「＋」は空の名簿で多人数会話を作るので、プリセットは作成後に選べる必要がある一方、
発言のある会話で名簿と場面を丸ごと差し替えると、transcript が既に居ない話者を指す。
リクエスト自体は正しく、chat の状態だけが受け付けを拒む状況なので、409 を返す（ターンの重なりと同じ扱い）。
既存の参加者は**追記ではなく置換**し、しかも論理削除ではなく行ごと消す。
**Why**: (1) 適用後の chat は「そのプリセットで作った chat」と同じでなければならず、追記では編成が一致しない。
(2) 論理削除で行を残すと、除籍済み参加者として画面に出続け、`CreateParticipant` の `sort_order` も残った行の次から始まるため、
作成時（0 起点・除籍行なし）と一致しない。(3) そもそも論理削除の理由（§3: 過去発言の話者解決と `round_robin` の巡回起点）は
発言が 0 件なら成立しないので、消して壊れるものが無い。
既存 chat への適用が途中で失敗したときは chat を消さない（利用者が作った既存の chat であり、作成時と違って捨ててよいものではない）。
発言は 0 件のままなので、同じルートでもう一度適用すれば中途半端な編成は消えてやり直せる。

## 6. フロントエンド

- **編成パネル**: 参加者の CRUD、接続先 base URL 入力 + モデル選択（configuration/models 流用）+ 接続確認表示、ターン進行ルールと場面設定の編集。
  接続先・モデルを空欄にするとワークスペース設定を継承することは、各ラベル横の `(?)` のツールチップで示す。ホバーとフォーカスの
  両方で開き（ポインタ無しでも到達できる）、同じ文を `aria-label` にも持たせる。**Why**: プレースホルダーには置けない — パネル幅で
  文が切れるうえ、入力済みの参加者を見直すときには消えている。入力欄の下の常設の補足行も採らない — モデル側だけで 4 行を占有し、
  一度読めば以後は流すだけの説明に恒久的な場所を使うことになる（実機で確認）。吹き出しは**上向き**に開き、幅はテキストなりではなく
  行幅に固定する。下向きだと説明している当の入力欄を覆って入力内容が見えなくなり、テキスト幅だと長い訳文が `overflow: auto` の
  パネルからはみ出して切れる（いずれも実機で確認）。
- **観戦ビュー**: ChatPage のストリーミング表示を踏襲し、発言者の表示名・モデル名を発言に付す。進行コントロールは「1 ターン進める」「自動進行の開始 / 停止」（= フロントのループ、§4.1）「（manual 時）次の発言者の指名」。
- **markdown エクスポート**: 観戦ビューのヘッダから `GET /api/chats/{chatId}/export/markdown` を呼び、見出し（chat タイトル・
  プロジェクト名・出力日時）、場面設定、ターン進行ルール、編成、話者名つきの発言を 1 枚の markdown として保存する。
  生成は `internal/service/export.go`。ルートは chat 単位で単独アシスタントの chat にも効き、その場合は場面設定・
  ターン進行ルール・編成の 3 節が落ちる。話者名は `ListAll` の全行で解決するので除籍済みの参加者の発言も帰属が残る（§3）。
  保存は Wails の `SaveTextFile` 束縛（ネイティブの保存ダイアログ）。WebView の外（`window.go` が無い環境）では
  Blob ダウンロードに落ちる。Wails v2.16 の macOS 実装はダウンロードのデリゲートを実装していないため、
  `<a download>` だけでは WebView 内で無言で失敗する。
- **プリセット**: 新規作成フォームで `kind = multi_agent` を選ぶとプリセット選択が出る。同じ選択 UI（`components/PresetPicker.tsx`）を
  編成パネルの先頭にも置き、**発言が 1 件も無いあいだだけ**「このプリセットを適用」を出す（発言が 1 件でもあれば選択 UI ごと出さない）。
  編成に参加者が居るときは置き換える旨を in-app の確認ダイアログで確かめてから適用する。同梱プリセットは群（`discussion` 議論・検討 /
  `drama` 演技・雑談 / `hosted` 聞き手つきの対話 / `pair` 1 対 1）ごとに `optgroup` で並び、「プリセットの JSON を読み込む」で
  ローカルの JSON ファイルを選ぶと、その内容を `preset` としてインラインで送って適用できる（読み込んだファイルは一覧には残らない）。
  保存形式は同梱分が `internal/preset/bundled/*.json`（`go:embed`）、追加分が `presets/multi-agent/*.json`（形式の説明は同ディレクトリの README）。決めた理由は §8「実装で解消した判断」。

## 7. 段階分け

| Phase | 内容 | 完了条件 |
| --- | --- | --- |
| A | migration 10 + repository + ターンエンジン + API（§3〜§5） | curl だけで多人数会話を作成し、ターンを進めて SSE で発言が流れる |
| B | 編成パネル + 観戦ビュー（§6） | UI から編成〜自動進行まで操作できる |
| C | プリセット同梱（+ JSON ファイルからの適用）+ 役割リマインドのチューニング + 履歴窓の拡大 | プリセット選択で即開始できる。長い会話で役割が崩れない |
| 将来 | TRPG 対応（chat 単位の JSON 状態の保持と注入・コードによるダイス・構造化出力での判定宣言）、進行役モデルによる発言者指名（`turn_rule` の追加値）、retrieval 統合、生成中断のための呼び出し元 `context` の伝播（§4.1） | — |

Phase A〜C は完了している（A: migration 10 + ターンエンジン + API、B: 編成パネル + 観戦ビュー、C: 同梱プリセット 7 件
+ `presets/multi-agent/` の 17 件 + 役割リマインドと履歴窓のチューニング）。「将来」に挙げた項目は未着手で、
着手時に判断する論点は §8.2 にある。

## 8. 実装で解消した判断と、将来拡張で判断する事項

「実装で解消した判断」とは、起票時に未決として挙げ、実装の中で決着して本文（§3〜§6）へ反映した項目の記録を指す。
「将来拡張で判断する事項」とは、現行実装の完成を妨げず、§7「将来」の拡張に着手するときに判断すればよい論点を指す。
起票時の未決はすべてどちらかに振り分けてあり、現行実装の側に残っている未決は無い。

### 8.1 実装で解消した判断

- **履歴圧縮への summary サービス流用 → 見送り**（2026-09-19）: 直近 30 発言の切り詰めのみとし、要約は入れない。
  **Why**: (1) 毎ターン要約の LLM 呼び出しを足すとローカル小型モデルでは 1 ターンの遅延が倍になる。
  (2) `chat_summaries` は単独 assistant フローの要約で、流用すると §4.4 の分離が崩れる。
  (3) 検討記録 §2 と実機で見えた崩れ方は「事実の忘却」より「役割の忘却・同調」で、これは毎ターンの役割リマインド（§4.3）が受け持つ。
  進行役を置くプリセットでは 1 巡ごとに進行役が要約するので、履歴が切れても文脈が残る（プリセット側の対策）。
  要約が必要になったら多人数会話専用の要約列を chat に持たせる形で足す（`chat_summaries` は流用しない）。
- **プリセットの保存形式 → 同梱 JSON + JSON ファイルからのインライン適用**（2026-09-19）:
  同梱分は `internal/preset/bundled/*.json` を `go:embed`、ファイルからの適用は §5 の `preset`。
  **Why**: エンジン（コード）とプリセット（データ）を分ける §0 の方針に沿い、Go を触らずにプリセットを足せる。単一バイナリも保つ。
  データディレクトリの走査による永続的な追加は、読み込み失敗時の扱いと UI が増えるので入れない。必要になれば `presets/multi-agent/` の
  形式のまま `<dataDir>/multi-agent-presets/` を一覧に足す拡張で対応できる。
- **既存 chat へのプリセット適用の可否と既存参加者の扱い → 発言 0 件に限って許可し、編成は置換**（2026-09-20）:
  ルートは `POST /api/chats/{chatId}/preset`、既存の参加者は行ごと削除してから作り直す。理由は §5 の該当段落。
  **Why**: サイドバーの「＋」で空の名簿の多人数会話を作れるようになった（TASK-13）ため、作成フォーム以外にプリセットを選ぶ場所が要る。
  発言のある会話まで許すと transcript が既に居ない話者を指すので、そこは 409 で拒む。
- **AGENTS.md「Core product shape」への追記文面 → 反映済み**（2026-09-19）: chat を「単独 assistant」と「多人数会話」の
  2 種別として書き、参加者・ターン進行ルール・場面設定・プリセットを多人数会話の構成要素として並べた。
  **Why**: 多人数会話は projects / documents / chats / memories と並ぶ第 5 の製品要素ではなく、chat の種別である（§2）。
  並列の箇条書きに足すと、project 配下に別系統のデータがあるように読める。
  同じ区分（chat の種別として書く）を [AGENTS.ja.md](../AGENTS.ja.md)・[README](../README.md)・
  [current-spec](./current-spec.md)（[ja](./current-spec.ja.md)）にも通した。

### 8.2 将来拡張で判断する事項

いずれも現行実装では不要で、§7「将来」の該当拡張に着手するときに判断する。

- 参加者ごとの API キー列の要否（LM Studio では不要。他の OpenAI 互換サービスを見据えるなら追加）。
- 多人数会話への review サービス適用の要否。
- 人間が参加者の一枠として発言するモード（TRPG のプレイヤー参加）の UI 設計。
