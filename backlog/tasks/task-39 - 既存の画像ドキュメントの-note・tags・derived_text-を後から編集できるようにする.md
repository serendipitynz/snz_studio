---
id: TASK-39
title: 既存の画像ドキュメントの note・tags・derived_text を後から編集できるようにする
status: To Do
assignee: []
created_date: '2026-09-22 22:13'
updated_date: '2026-09-22 22:37'
labels: []
dependencies:
  - TASK-38
references:
  - internal/httpapi/server.go
  - internal/httpapi/handlers.go
  - internal/repository/document.go
  - internal/service/embeddingsync.go
  - internal/doccategory/doccategory.go
  - frontend/src/pages/ProjectDetailPage.tsx
  - internal/db/schema.go
type: feature
ordinal: 39000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-38 は画像ドキュメントの**追加時**に `derived_text` の下書きを生成する。一方、既に登録済みの画像ドキュメント
(一括アップロードで入れたものを含む) の `derived_text` を後から埋める経路は無い。

- ドキュメント API は作成・削除・`category`・`sharedWithAll` の 4 本だけで、本文系フィールドの更新 API が無い
  (`internal/httpapi/server.go`)。
- repository にも本文系を更新する関数が無い (`UpdateDocumentCategory` / `UpdateDocumentSharedWithAll` のみ)。
- 詳細パネルの `derived_text` は読み取り表示だけ (`frontend/src/pages/ProjectDetailPage.tsx`)。

`CreateDocument` はチャンク・FTS 行を丸ごと作る構造なので、更新でも同じ再構築が要る。これはマルチモーダル生成とは
独立したドキュメント編集基盤の話なので、TASK-38 から切り出した。

## 方針

- 画像ドキュメントの note・tags・derived_text を更新する API と repository 関数を追加する。更新時はチャンクと
  FTS 行を作り直し、埋め込みを再同期する (`CreateDocument` と同じ組み立てを共有し、作成と更新で索引の中身がずれないようにする)。
- **対象は画像ドキュメントだけ**。repository の更新関数は type を問わない形でよいが、API は type が image 以外なら
  400 で拒否する。**Why**: markdown / text の本文編集はスコープ外で、note・tags だけ編集できる中途半端な状態を
  API として公開しない。後で本文編集を足すときは API の制限を外すだけで済む。
- 詳細パネルに編集操作を置く。本文の保存後は、返ってきたドキュメントで `selectedDocument` を置き換え、
  表示とカテゴリ選択のドラフトを更新後の値に追随させる。**Why**: カテゴリのドラフトは `selectedDocument` の変化で
  初期化されるので、置き換えを忘れると再推論前の `misc` がドラフトに残り、隣の「カテゴリを保存」で `misc` に戻せてしまう。
- TASK-38 の「説明文を生成」を編集画面からも使えるようにする。既存ドキュメントは画像がサーバにあるので、
  生成はドキュメント id を指定して行う (画像本体を再送しない)。id を受けるハンドラを追加し、保存済みファイルを読んで
  TASK-38 の生成サービス関数を呼ぶ (生成処理は作り直さない)。生成結果は下書きで、保存操作まで永続化しない。
  - **形式の判定は TASK-38 の生成サービス関数が保存済みファイルのバイト列に対して行う**。ハンドラは
    `documents.mime_type` (アップロード時のクライアント申告をそのまま保存した値) を判定に使わず、形式を関数に渡さない。
    ハンドラが担うのは、読み込む前のサイズの事前チェック (`os.Stat`) と、関数が返す sentinel error の 400 系への写像である。
    フロントの生成ボタンの無効化は `mimeType` を見てよいが、あくまで表示上の案内で、拒否はサーバが担う。
  - 公開パス (`/files/<name>`) からディスク上のパスへの変換は、いま `unlinkFile` に直書きされているだけなので、
    共有ヘルパーに切り出して両方から使う。ファイルが見つからない場合はエラーとして返し、画面に表示する。
- **カテゴリの再推論**: 更新時、現在の category が `misc` のときだけ、更新後の内容で `doccategory.Infer` をかけ直す。
  `misc` 以外 (推論済み・人間が選んだもの) は変えない。
  **Why**: 画像は説明が空のまま登録されると `misc` になりやすく、説明を足したときこそ推論が効く。しかし
  `BackfillInferredCategories` は `category = 'misc' AND updated_at = created_at` の行しか対象にしないので、
  一度更新すると自動バックフィルから外れ、ここで推論しなければ `misc` のまま残る。人間が明示的に `misc` を選んでいた場合も
  上書きされ得るが、`misc` は「分類できない」の受け皿で明示的に選ぶ場面は少なく、カテゴリは詳細モーダルからいつでも戻せる。
  - 効き方は限定的である。`doccategory.Infer` はファイル名とタイトルも見るので、説明が空でも `misc` 以外に
    推論される画像があり、それらは後から説明を足しても再推論されない。本来の条件は「作成時に説明が空だった画像」だが、
    その情報は現在の値から復元できないため、`misc` を条件にする。
- **埋め込み再同期の失敗時**: 作成時と同じ best-effort (ログを出して続行し、リクエストは成功にする) とする。
  チャンクを作り直すと chunk_id が変わり、旧チャンクの埋め込みは `ON DELETE CASCADE` で消えるため、同期に失敗したり
  埋め込みが無効だったりすると、そのドキュメントは再ビルドまでベクトル無し (FTS だけで検索される) になる。これは許容する。
  **Why**: 作成時と同じく、保存済みの更新を失敗扱いにするとクライアントが再試行して状態が分かりにくくなる。
  旧ベクトルを残すと、更新後の本文と食い違うベクトルで検索されるほうが害が大きい。
- タイトル・type・画像ファイルの差し替えは扱わない (差し替えは現行どおり同名の上書き = 削除して再作成)。

## スコープ外

- markdown / text ドキュメントの本文編集。
- 既存画像への一括生成。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 画像ドキュメントの note・tags・derived_text を詳細パネルから編集して保存できる。image 以外のドキュメントに対する更新 API 呼び出しは 400 で拒否される
- [ ] #2 保存後、更新した内容でチャンク・FTS 行が作り直され、埋め込みが再同期される (更新した derived_text で検索にヒットする)。再同期に失敗しても保存は成功扱いになり、そのドキュメントは再ビルドまで FTS だけで検索される
- [ ] #3 作成と更新で索引の組み立てが共有されており、同じ内容なら同じ索引になる
- [ ] #4 更新時、category が misc のときだけ更新後の内容で再推論され、misc 以外の category は変わらない。再推論の結果は保存直後の詳細パネルの表示とカテゴリ選択のドラフトに反映される
- [ ] #5 編集画面から、ドキュメント id 指定で TASK-38 の生成サービス関数を使って説明文を生成でき、生成結果は保存まで永続化されない
- [ ] #6 id 経路でも、保存済みファイルが対象外の形式・上限超過なら 4xx で拒否される (documents.mime_type は判定に使わない)。ファイルが見つからない場合はエラーが表示される
- [ ] #7 公開パスからディスク上のパスへの変換が共有ヘルパーにまとまり、unlinkFile と id 指定の生成の両方から使われている
- [ ] #8 repository の更新処理 (索引の作り直し・カテゴリの再推論を含む) にテストがある
<!-- AC:END -->
