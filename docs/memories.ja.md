# Memory 仕様

## 目的

memory は、複数 chat をまたいで使いたい恒久的な project コンテキストを保存するためのものです。  
document や chat の生ログを置く場所ではありません。

役割の分担は次の通りです。

- documents: 一次資料
- chat summary: 各 chat の流れ
- memories: 再利用したい短い事実やルール

## Memory の kind

### Procedural

assistant がどう振る舞うべきかを表します。

例:

- 文体ルール
- 翻訳ルール
- 出力形式
- 禁止語や避ける表現

### Semantic

project に関する安定した事実です。

例:

- 世界設定上の事実
- 人物設定上の事実
- 用語の固定ルール
- 技術・構造上の固定前提

### Episodic

過去に確定した出来事や決定です。

例:

- すでに本文で起きた重要イベント
- 以前決めた編集方針
- project 上の重要な確定事項

## データモデル

各 memory は次を持ちます。

- `id`
- `project_id`
- `kind`
- `title`
- `content`
- `source_chat_id`
- `source`
- `locked`
- `created_at`
- `updated_at`

### Source

`source` は memory の作成元です。

- `manual`
- `chat`
- `organized`

### Locked

`locked` は、その memory を organizer の rewrite / remove から守るためのフラグです。

ただし、ユーザーの手動操作までは禁止しません。  
つまり `locked` でも次は可能です。

- unlock
- 再 lock
- 手動削除

## 作成経路

### 手動追加

Project 詳細の memory modal から追加できます。

現在の挙動:

- `kind` を選ぶ
- `content` を入力する
- title は content から自動生成する
- 手動追加 memory は既定で `locked = true`
- `source = manual`

### chat からの自動抽出

各 user message のあとで、backend は user 入力から memory を作ることがあります。

重要な制約:

- user message だけを見る
- assistant message は memory 化しない
- 質問文は無視する
- 長すぎる文は無視する
- durable な heuristic に当たる文だけ候補にする

この抽出はかなり保守的です。

例:

- procedural cue: `please use`, `日本語で`, `簡潔に`
- semantic cue: `this project uses`, `このアプリは`, `前提です`
- episodic cue: `we decided`, `前回`, `変更した`

自動抽出された memory は次で作られます。

- `source = chat`
- `locked = false`

## chat での使われ方

memory は毎回すべてが prompt に入るわけではありません。

現在の挙動:

1. `procedural` memory のうち上位 4 件を persistent working instruction として常時入れやすい
2. それ以外は FTS と任意の embedding rerank で relevant memory を検索する
3. 実際に選ばれた memory だけが assistant turn の reference に付く

つまり:

- procedural memory は回答の仕方に強く効く
- semantic / episodic memory は必要時にだけ引かれやすい

## Organizer

memory organizer は手動実行の機能です。  
毎ターン自動では動きません。

分析対象:

- 現在の memories
- recent chat summaries

提案できる変更:

- `create`
- `update`
- `remove`

ルール:

- locked memory は organizer が update/remove しない
- 適用前に UI 上で確認する

## memory に向いているもの

向いている:

- 長く使う指示
- 安定した事実
- 確定した project 上の出来事
- 複数 chat で再利用される短い文

向いていない:

- 長文の設定資料
- 本文そのもの
- 一時的な brainstorming
- 雑談の断片
- 未確定の案

## 運用上の原則

短く再利用したい前提は memory に置く。  
詳細な資料は document に置く。  
会話の進行は chat summary に持たせる。
