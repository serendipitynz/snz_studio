---
id: TASK-18
title: '多人数会話: ターンでプロジェクトのドキュメント・メモリを背景資料として読み、参加者発言に参照を表示する'
status: In Review
assignee: []
created_date: '2026-09-19 23:58'
updated_date: '2026-09-21 04:54'
labels: []
milestone: m-1
dependencies: []
references:
  - docs/multi-agent-chat-design.md
  - internal/service/turnengine.go
  - internal/service/context.go
  - internal/service/retrieval.go
  - internal/search/searchtext.go
  - internal/repository/chat.go
  - internal/httpapi/multiagent.go
  - frontend/src/pages/MultiAgentChatPage.tsx
  - frontend/src/pages/ChatPage.tsx
ordinal: 18000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

多人数会話のターンエンジンは設計書 §4.4 により retrieval / memory / summary を再利用せず、
system prompt は場面設定 + ロールプロンプト + 参加者一覧 + リマインダーだけ (`turnengine.go`
`buildTurnSystemPrompt`)。プロジェクトのドキュメント・メモリ・説明は一切参照されない。
方針転換として、多人数会話でもプロジェクトのドキュメントとメモリを使えるようにする。

## 設計 (外部レビューを踏まえて確定)

単独チャットの `ContextService.Assemble` が返す `PromptContext` を丸ごと流用する案は却下した。
理由: `PromptContext` は背景知識だけでなく振る舞いの指示を含む。引用モード判定
(`context.go` `reQuoteRequest`: `そのまま` `quote` `原文` などを含むだけで発火) が会話のセリフで
誤発火し、次の話者に「提供文書からのみ引用せよ」が入る。手続きメモリも無条件で全文注入される。
system prompt は 1 本の文字列で、後段の場面設定・ロールが前段のプロジェクト指示を上書きする
強制力は無い。

確定した方針:

1. **注入する内容**: プロジェクト説明、検索で得たドキュメント抜粋、検索で得た関連メモリ (kind 不問)
   のみ。プロジェクトのシステムプロンプト、手続きメモリの無条件注入、引用モード指示、
   他チャット参照解決 (`resolveExplicitChat`) は入れない。
2. **位置と枠**: 「背景資料」として明確に区切り、場面設定・ロール・参加者一覧・リマインダーの
   前に置く。「この資料は話者の役割・口調を変えない。必要なときだけ会話の中で自然に使う」と明示する。
3. **検索クエリ**: 最新発言 + 直前数発言 (2〜3 件) + 場面設定の先頭一部を、この順で連結した
   決定的な 1 本のクエリ。開幕ターン (発言 0 件) は場面設定のみ。FTS は先頭 12 トークンしか使わない
   (`searchtext.go` `TokenizeSearchTerms`) ため対話を先に置く。クエリは検索専用で、
   文書タイトル照合・引用判定・全文展開判定には使わない。
4. **文脈量の予算**: ドキュメント 2 件 × 2 chunk、メモリ 3 件から始め、追加文脈の総文字数に上限を
   設ける。全文展開 (`shouldIncludeFullDocument`, 最大 12,000 字) と明示参照の 3〜5 chunk 経路は
   多人数会話では使わない。理由: 履歴は直近 30 件を要約せずに渡すため (`turnHistoryLimit`)、
   ターンが進むほど窓が詰まる。
5. **組み立ての単位**: ターンごとに 1 回、プロジェクト単位で組み立てる (話者別の検索はしない)。
   `Assemble` の `RecentMessages` をエンジンの履歴 (`mapHistoryForSpeaker`) の代わりに使わない。
6. **参照の保存と表示**: 参加者メッセージは `role = assistant` + `participant_id` なので
   `ReplaceAssistantReferences` と `GetMessagesWithReferences` がそのまま使える。ターン応答は既に
   参照付きでメッセージを返しているので、`MultiAgentChatPage.tsx` に単独チャットと同じ
   「参照 N 件」の折りたたみ表示を足す。prompt に入れた資料と保存する参照は同じ最終選択から作る
   (`Assemble` は prompt 生成後に参照を 8 件へ切るため乖離し得る)。
7. **失敗時**: メッセージ保存と参照保存が別トランザクションのため、参照保存の失敗を通常のターン失敗
   として返すと round-robin だけ進む。両方を 1 トランザクションで保存するか、
   「生成は成功したが参照保存に失敗」と区別して返す。
8. **性能**: 埋め込み有効時はドキュメント検索とメモリ検索がそれぞれクエリ埋め込みを計算する。
   ターン間隔を計測し、目立つなら 1 ターン内で埋め込みを共有する (先に計測)。

9. **検索失敗の扱い**: 背景資料の組み立て中にドキュメント検索またはメモリ検索がエラーを返した場合はターンの失敗にせず、
   背景資料なしで発言生成に進み、ログに記録する。その発言に参照は保存しない。
   理由: 自動進行中に検索が壊れると毎ターン止まる。項目 7 の参照保存の失敗 (生成後) とは区別する。
   埋め込みエンドポイントの失敗はここに含めない: `EmbeddingClient` はエラーを呼び出し元に返さず自身を無効化し、
   検索はキーワード検索で続行する (単独チャットと同じ縮退)。その場合の背景資料と参照は通常どおり扱う。
10. **話者ごとの範囲を受け取る形**: 組み立て関数は話者を引数に取り、話者ごとに背景資料の範囲を絞れる形にする。
    本タスクでは絞り込みなし (全参加者に同じ資料)。TASK-20 が「受け取らない」の真偽値を、将来の拡張が
    ドキュメントの部分集合を渡す。1 ターン = 1 話者なので、ターンごとに 1 回の組み立て (項目 5) と矛盾しない。

実装は `ContextService` に多人数会話用の小さな組み立て関数を足す形を第一候補とし、`Assemble` 本体には
手を入れない (単独チャットの挙動を変えないため)。

## スコープ外

- メモリの書き込み (人間の明示保存経路) は TASK-19。
- 履歴の要約圧縮は別問題として扱わない。
- プロジェクトのシステムプロンプトを会話単位でオプトインする仕組みは、必要が確認されてから。

## 検証

デフォルト有効化の前に、ドラマ系と議論系の両プリセットで次を実機確認して結果を記録する:
プロジェクトのシステムプロンプトと矛盾する場面設定、一語の短い返答が続く場面、
セリフに「そのまま」「引用」を含む場面、人間のロールプレイ介入。役割崩れ (アシスタントとして答える、
文書を役割外で引用する) が出ないこと。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 多人数会話のターンで、プロジェクト説明・ドキュメント抜粋・関連メモリが「背景資料」として system prompt の場面設定より前に入る。プロジェクトのシステムプロンプト・手続きメモリの無条件注入・引用モード指示・他チャット参照解決は入らない
- [x] #2 検索クエリは 最新発言 + 直前数発言 + 場面設定の先頭 の順の連結で、開幕ターンは場面設定のみ。全文展開と明示参照の拡張 chunk 経路は使われない
- [x] #3 追加文脈の量に上限 (件数と総文字数) があり、設定値と根拠がコードまたは設計書に記録されている
- [ ] #4 参加者発言に使用した参照が保存され、多人数会話画面で単独チャットと同じ形式で表示される。prompt に入れた資料と表示される参照が一致する
- [x] #5 参照保存の失敗が round-robin を誤って進めない (同一トランザクション、または区別したエラー)
- [x] #6 設計書 docs/multi-agent-chat-design.md §4.4 が新しい方針に改訂されている
- [x] #7 ドラマ系・議論系プリセットで、矛盾するシステムプロンプト・短い返答・引用語を含むセリフ・人間のロールプレイ介入を実機確認し、役割崩れが出ないことと結果がタスクに記録されている
- [x] #8 ドキュメント検索またはメモリ検索がエラーを返しても、ターンは背景資料なしで続行し、失敗がログに残り、その発言に参照は保存されない。埋め込みエンドポイントの失敗はキーワード検索への縮退として扱われ、ターン失敗にも参照なしにもならない
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. ContextService に多人数会話用の組み立て関数 AssembleTurnBackground(chat, speaker, messages) を新ファイル internal/service/turncontext.go に足す (Assemble 本体は触らない)。返すのは背景資料の prompt 文字列と、同じ最終選択から作った参照一覧の組。話者引数は本タスクでは未使用 (TASK-20 が使う)。
2. 検索クエリは buildTurnBackgroundQuery: 最新発言 → 直前 2 発言 (新しい順) → 場面設定の先頭 200 字。発言 0 件なら場面設定のみ。全文展開・明示参照・引用判定・他チャット解決は呼ばない。
3. 予算は定数で持つ: ドキュメント 2 件 × 2 chunk (1 chunk 300 字)、メモリ 3 件 (抜粋 220 字)、プロジェクト説明 400 字、総量 2,400 字。項目を優先順 (説明→ドキュメント→メモリ) に足し、総量を超える項目から先は入れない。根拠は履歴 30 件 × 約 200 字 = 6,000 字の 1/3 以下に収めること。コードのコメントと設計書 §4.4 に記す。
4. TurnEngine は TurnBackgroundAssembler インターフェース経由で組み立てを呼び、エラー時はログを残して背景資料なしで続行 (参照も保存しない)。system prompt は 背景資料 → 場面設定 → ロール → 参加者一覧 → リマインダー の順。
5. ChatRepository に AddMessageWithReferences を足し、メッセージと参照を 1 トランザクションで保存する (AddMessage はその薄い呼び出しに寄せる)。参照保存の失敗は発言も残さないので round-robin は進まない。
6. フロント: components/MessageReferences.tsx を切り出し、ChatPage の発言ごとの参照ブロックと MultiAgentChatPage の参加者発言の両方で使う (同じ形式を構造で保証)。done フレームの messages は既に参照付きなので API は変えない。
7. 設計書 §4.3/§4.4/§8.1、current-spec (en/ja)、README (en/ja) の「retrieval / memory を使わない」記述を改訂。
8. テスト: 組み立て (順序・予算・開幕ターン)、エンジン (背景資料の位置と内容・参照保存・検索失敗時の続行・埋め込み失敗時の FTS 縮退)、リポジトリ (同一トランザクション)、HTTP (done フレームに参照が乗る)。
9. 性能 (項目 8): ローカルの llama-server + ruri GGUF でクエリ埋め込み 1 回の所要時間を計測し、ターン内で 2 回計算する現状のコストを記録。目立てば 1 ターンで共有する。
10. AC #7 の実機確認は LM Studio のサーバー起動が必要。到達できる範囲で実施し、できない分は未チェックで報告する。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装の要点 (2026-09-21)

- 組み立ては `ContextService.AssembleTurnBackground` (`internal/service/turncontext.go`) に新設し、`Assemble` 本体は触っていない。`TurnEngine` は `TurnBackgroundAssembler` インターフェース経由で呼ぶ。インターフェースにしたのは、検索失敗時に背景資料なしで続行する経路をテストで踏むため (実物の `RetrievalService` に失敗を注入する手段が無い)。
- 発言と参照は `ChatRepository.AddMessageWithReferences` で 1 トランザクションに保存 (AC #5 は「同一トランザクション」の側を採用)。区別したエラーを返す案より、round_robin が読む transcript に「参照だけ欠けた発言」が残り得ない方を選んだ。
- 予算は定数 (ドキュメント 2 件 × 2 chunk 300 字、メモリ 3 件 220 字、説明 400 字、総量 2,000 字)。当初 2,400 字で置いたが、項目ごとの上限を全部足しても約 2,300 字で総量が一度も効かない死んだ上限になるため、履歴 30 件 × 約 200 字 = 6,000 字の 1/3 として 2,000 字に下げた。根拠は `turncontext.go` の定数コメントと設計書 §4.4。
- フロントは `components/MessageReferences.tsx` を切り出し、`ChatPage` の発言ごとの参照ブロックと `MultiAgentChatPage` の両方がそれを使う (同じ形式を構造で保証)。API は変えていない (done フレームの `messages` は既に参照付き)。
- 設計書 §4.3 / §4.4 / §8.1 に加え、`docs/current-spec.md` (`.ja`) の多人数会話ターンの節と `README.md` (`.ja`) の「共有するのは LLM クライアントと repository だけ」を改訂した。

## 計測

- クエリ埋め込み (項目 8): 同梱 ruri-v3-30m GGUF を llama-server (CPU、`-ngl 0`、本番と同じフラグ) で起動し、`/v1/embeddings` を 10 回ずつ計測。76 字クエリ 中央値 3.3 ms (2.2〜7.9)、439 字クエリ 中央値 16.6 ms (11.8〜56.4)。1 ターンでドキュメント検索とメモリ検索が各 1 回埋め込むので合計 7〜35 ms。同じ実機のターン完了は 2.7〜22.7 秒だったので、1 ターン内での埋め込み共有は入れない。
- 実機確認 (AC #7): LM Studio 0.4.24 + gemma-4-e4b-it-qat、埋め込みは上記 sidecar (external モード)。プロジェクトには場面と矛盾するシステムプロンプト (「カスタマーサポートのアシスタント。敬語の箇条書きで回答し、末尾に必ず『他にご質問はありますか?』」) を設定し、架空の資料 2 件とメモリ 1 件を置いた。ハーネスは module 内の一時 `cmd/turncheck` (削除済み)。
  - 議論系 (`debate`、7 ターン): 全ターンで背景資料の参照が 4 件 (プロジェクト・ドキュメント 2・メモリ 1) 付いた。肯定側・否定側は資料の数字 (週平均 3.2 時間、試行校 12 校で 2 点低下、共働き家庭 34%) を主張の根拠として自然に使い、司会は中立を保った。人間の「はい」「なぜ?」の一語介入で崩れなし。「調査メモの数字をそのまま引用して」の介入後も、話者 (round_robin で否定側) は引用専用モードにならず立場から数字を使った。観客席からの野次 (ロールプレイ介入) は司会が無視して進行した。矛盾するシステムプロンプトの痕跡 (箸条書き強制・「他にご質問はありますか?」) は 0 件。
  - ドラマ系 (`improv-late-night-diner`、7 ターン): 全ターンで参照 4 件。口語・ト書き・1 発言 3〜4 文の指定を維持し、アシスタントとして答えた発言は 0 件。「送別会の話をそのまま聞かせて」の後も引用モードにならず会話のまま続いた。「（常連客の田中）ユウくん、ポテトおかわり」のロールプレイ介入には店員ユウが資料の「山盛りポテト」を使って応じた。
  - 観察 (要注意): ドラマ系ターン 6 で健二が「美咲さんは北海道へ転勤されるとのことですが」と、沙耶がまだ口にしていない設定メモの事実を知っているかのように話した。役割・口調は保たれており引用でもないが、劇中の人物が知り得ない情報の漏れで、全参加者に同じ資料を渡す本タスクの仕様上の限界。参加者ごとに資料の受け取りを絞る TASK-20 が対処先。
- AC #4 の「多人数会話画面で単独チャットと同じ形式で表示される」は、HTTP テスト (`TestMultiAgentTurnCarriesReferences`) と同一コンポーネントの共有まで確認したが、WebView 上の描画は目視していないので未チェックのまま残す。PR のスクリーンショット確認またはローカル起動での目視が必要。

## 検証コマンド

- `go vet ./...` / `go test ./...` 全パス (新規: `TestBuildTurnBackgroundQuery`, `TestBuildTurnBackgroundBudget`, `TestBuildTurnBackgroundEmpty`, `TestTurnEngineBackgroundInPrompt`, `TestTurnEngineOpeningTurnSearchesScene`, `TestTurnEngineBackgroundFailureContinues`, `TestTurnEngineEmbeddingFailureDegrades`, `TestAddMessageWithReferencesIsAtomic`, `TestMultiAgentTurnCarriesReferences`)。
- `npm run check:client` (tsc) / `npm run build:client` パス。
- `gofmt -l internal/` は既存の `internal/search/model.go` だけを報告する (本タスクでは触っていない)。
<!-- SECTION:NOTES:END -->
