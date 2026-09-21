# 多人数会話のプリセット (追加分)

多人数会話 ([設計書](../../docs/multi-agent-chat-design.md) §6) のプリセットのうち、
アプリに同梱していないものを置く場所です。同梱分 (ディベート・即興劇など 7 件) は
[internal/preset/bundled/](../../internal/preset/bundled/) にあり、アプリの新規作成フォームの
プリセット一覧に最初から出ます。ここにある 17 件と、同じ形式で自作した JSON は、
新規作成フォームと編成パネル（発言が 1 件も無いあいだ）の「プリセットの JSON を読み込む」から
ファイルを選ぶと同じように適用できます。

用語は設計書 §2 のものをそのまま使います (プリセット・場面設定・参加者・表示名・役割プロンプト・ターン進行ルール)。

## 一覧

| ファイル | 場面 | 参加者数 | 群 |
| --- | --- | --- | --- |
| [04-brainstorming.json](04-brainstorming.json) | ブレインストーミング (図書館の来館者倍増) | 4 | 議論・検討 |
| [05-theme-dialogue.json](05-theme-dialogue.json) | テーマ対話 (AI の小説は作品か) | 3 | 議論・検討 |
| [06-presentation-qa.json](06-presentation-qa.json) | プレゼンと質疑 (検索機能の導入報告) | 3 | 議論・検討 |
| [07-fantasy-tavern.json](07-fantasy-tavern.json) | ファンタジー酒場 (廃坑の依頼) | 4 | 演技・雑談 |
| [08-royal-gossip.json](08-royal-gossip.json) | 王室噂話 (王女の婚約相手) | 3 | 演技・雑談 |
| [09-office-kitchenette.json](09-office-kitchenette.json) | 日本企業の給湯室 (勤怠システム刷新) | 3 | 演技・雑談 |
| [10-mock-trial.json](10-mock-trial.json) | 模擬裁判 (万引きの故意) | 3 | 議論・検討 |
| [11-design-review.json](11-design-review.json) | 設計レビュー会議 (通知の非同期化) | 3 | 議論・検討 |
| [12-negotiation.json](12-negotiation.json) | 商談・交渉 (業務ソフトの価格) | 3 | 議論・検討 |
| [15-book-club.json](15-book-club.json) | 読書会 (『こころ』) | 3 | 聞き手つきの対話 |
| [16-sengoku-war-council.json](16-sengoku-war-council.json) | 戦国の軍議 (籠城か出撃か) | 4 | 演技・雑談 |
| [17-cross-era-dialogue.json](17-cross-era-dialogue.json) | 時代を超えた対談 (棟梁とエンジニア) | 3 | 聞き手つきの対話 |
| [19-complaint-call.json](19-complaint-call.json) | クレーム対応の電話 (客と担当) | 2 | 1 対 1 |
| [20-one-on-one.json](20-one-on-one.json) | 1on1 (上司と部下) | 2 | 1 対 1 |
| [21-socratic-dialogue.json](21-socratic-dialogue.json) | 問答 (先生と生徒、質問だけで掘る) | 2 | 1 対 1 |
| [22-relay-novel.json](22-relay-novel.json) | リレー小説 (作家 2 名の綱引き) | 2 | 1 対 1 |
| [23-parent-teacher-meeting.json](23-parent-teacher-meeting.json) | 保護者面談 (担任と保護者) | 2 | 1 対 1 |

番号は同梱分と通しで、同梱されている 01・02・03・13・14・18・24 が抜けています。
参加者数は AI 参加者 (`participants` の要素) だけを数え、人間の介入発言は含めません。

## 形式

1 プリセット 1 ファイルの JSON です。

```json
{
  "id": "one-on-one",
  "title": "1on1 (上司と部下)",
  "description": "月 1 回の 1on1。想定ターン数: 12〜16",
  "group": "pair",
  "turnRule": "round_robin",
  "scenePrompt": "全参加者の system メッセージに前置される場面設定",
  "participants": [
    { "displayName": "上司", "rolePrompt": "この参加者だけに渡す役割プロンプト" },
    { "displayName": "部下", "rolePrompt": "...", "receivesBackground": false }
  ]
}
```

| フィールド | 内容 | 必須 |
| --- | --- | --- |
| `title` | 一覧に出す名前。新規作成時にチャット名を空にしたときは、これがチャット名になる | 必須 |
| `participants` | 登録順 = `round_robin` の巡回順。`displayName` は必須、`rolePrompt` は空でも可。2 名以上 | 必須 |
| `participants[].receivesBackground` | その参加者のターンでプロジェクトのドキュメント・メモリ（設計書 §4.4 の背景資料）を渡すか。省略時は `true` | 任意 |
| `turnRule` | `round_robin` または `manual`。省略時は `round_robin` | 任意 |
| `scenePrompt` | 場面設定。省略時は空 | 任意 |
| `description` | 一覧で title の下に出す説明 | 任意 |
| `id` / `group` | 同梱分の一覧で使う値。ファイルから読み込むときは使われない (`group` は `discussion` / `drama` / `hosted` / `pair`) | 任意 |

接続先 (`baseUrl`) とモデル (`modelName`) はプリセットに書きません。適用直後の参加者は接続先が空で、
ワークスペースの既定エンドポイントで発言します。参加者ごとに変えるときは編成パネルで設定してください。
知らないフィールド (メモなど) があっても無視されます。

`receivesBackground` を `false` にすると、その参加者のターンではプロジェクトの資料を一切渡さず、検索も行いません。
TRPG の GM のように 1 人だけがシナリオを知っている編成に使います。省略した参加者は全員が同じ資料を読みます。

## 適用の仕組み

読み込んだ JSON は `POST /api/projects/{projectId}/chats` (新規作成時) または
`POST /api/chats/{chatId}/preset` (発言の無い既存チャットへの適用) の body に `preset` として送られ、
同梱分 (`presetId`) と同じ検証・適用処理を通ります。既存チャットへ適用すると、そのときの参加者は
プリセットのものに置き換わります。適用後のチャットはプリセットとの結びつきを持たず、
場面設定・参加者・ターン進行ルールはすべて編成パネルで自由に編集できます。
同じプリセットをもう一度使うときは、もう一度ファイルを選びます。

## 自作するときの要点 (同調収束と役割崩れの対策)

小型モデルは履歴が伸びると役割を忘れ、同意で収束します (設計書 §4.3)。同梱分と追加分は次を守って書いてあります。

- 参加者ごとに「立場・目的」「口調」「他の参加者との食い違い」の 3 つを持たせる。食い違いが無い参加者は 2〜3 ターンで同意役になる。
- 参加者だけが知る情報 (隠し事・裁量の上限・持っている材料) は役割プロンプトに置き、場面設定には置かない。場面設定は全参加者に渡る。
- 「いつ・何をきっかけに明かすか」を役割プロンプトに書く (「3 回目に追及されたら認める」など)。無いと、最初のターンで全部話すか最後まで話さないかになる。
- 場面設定には、共有する事実・進め方・1 発言の長さ・終わり方の合図を書く。終わり方の合図を決めておくと、人間の介入発言で会話を閉じられる。
- 進行役 (司会・裁判官・聞き手) を置くときは「自分の意見は言わない」と明記し、先頭に登録する。1 巡ごとに進行役が要約するので、履歴が切り詰められても文脈が残る。
- 参加者は 3 名を基本、4 名を上限にする。それ以上は 1 巡が長く、直近 30 発言の履歴から自分の前回発言が落ちやすい。

「発言の冒頭に自分の名前を付けない」「他の参加者の台詞を書かない」「直前の発言のどこに反応しているかを含める」
「発言の長さは場面設定に従う」は、エンジンが毎ターン付ける役割リマインドに入っているので、プリセットには書きません。
