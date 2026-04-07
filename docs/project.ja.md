# Project 仕様

## 目的

Project は SNZ Studio における最上位の作業単位です。  
小説執筆、翻訳補助、技術メモ整理など、ひとつの活動に対する共有コンテキストをまとめます。

各 Project は次を持ちます。

- 共有 document
- 共有 memory
- 複数の chat
- project 単位の system prompt

目指している UX は ChatGPT / Claude の Project に近いものですが、単一ユーザー・ローカル専用に絞っています。

## データモデル

Project は以下を持ちます。

- `id`
- `title`
- `description`
- `system_prompt`
- `created_at`
- `updated_at`

関連レコード:

- `documents`
- `chats`
- `memories`

Project を削除すると、関連する chat、message、summary、document、memory、upload 済みファイルも削除されます。

## Project 単位のコンテキスト

Project 単位のコンテキストは、主に次から構成されます。

1. project title
2. project description
3. project system prompt
4. project memories
5. project documents
6. chat summary
7. recent chat messages

assistant の返答では、project 自体も 1 つの reference source として扱われます。

## Documents

Project 配下には次の document を置けます。

- `markdown`
- `text`
- `image`

各 document には category も付きます。

- `world`
- `character`
- `rule`
- `plot`
- `timeline`
- `index`
- `story`
- `misc`

category は追加時に自動推定され、あとから手動で変更できます。

document retrieval は次を組み合わせます。

- SQLite FTS5 によるキーワード検索
- 任意の embedding による rerank / fallback

大きな創作プロジェクトでは、一次資料は memory ではなく document に置く前提です。

## Chats

chat は必ず 1 つの project に属します。  
1 つの project に複数の chat を持てます。

chat は project の documents / memories を共有しつつ、chat ごとに次を持ちます。

- messages
- summary
- reference history

chat title は空文字で作成できます。  
空のまま作成した場合は、最初の assistant 応答後に自動生成されます。

## Memories

memory は project 配下で共有され、複数 chat をまたいで使われます。  
原資料を置く場所ではなく、短い恒久前提を置く場所です。

kind:

- `procedural`
- `semantic`
- `episodic`

補助 metadata:

- `source`: `manual`, `chat`, `organized`
- `locked`: organizer からの rewrite / remove を防ぐ。ただし手動操作は可能

## Retrieval の挙動

回答時には、主に次の順で context を組み立てます。

1. project settings
2. procedural memories
3. chat summary
4. relevant documents
5. relevant memories
6. recent messages

document retrieval は category-aware です。たとえば:

- 設定確認では `world`, `index`, `timeline` を優先
- 執筆補助では `rule`, `story`, `character` を優先
- プロット相談では `plot`, `timeline` を優先

## UI

Project 詳細画面では次を操作できます。

- project title 編集
- new chat 作成
- document の追加・閲覧
- memory 管理
- system prompt 編集
- chat 一覧表示

右ペインは project asset inspector として機能します。

## 設定との関係

Project はモデル接続設定を持ちません。  
LLM / embedding の endpoint や model は workspace 全体の設定であり、`data/app-config.json` に保存されます。

Project 側は、その project に固有の文書・記憶・指示だけを持ちます。
