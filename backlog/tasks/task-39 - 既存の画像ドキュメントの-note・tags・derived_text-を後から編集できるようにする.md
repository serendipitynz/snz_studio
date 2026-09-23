---
id: TASK-39
title: 既存の画像ドキュメントの note・tags・derived_text を後から編集できるようにする
status: Done
assignee: []
created_date: '2026-09-22 22:13'
updated_date: '2026-09-23 10:43'
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
- TASK-38 の「説明文を生成」を編集画面からも使えるようにする。フロントが保存済み画像を `/files/<name>` から取得し、
  追加ダイアログと同じ前処理 (1 メガピクセル超の縮小、PNG / WebP の白背景への合成) を通してから、既存の
  `POST /api/image-description` に送る。生成結果は下書きで、保存操作まで永続化しない。
  **Why (着手時に改訂, 2026-09-23)**: 当初は「ドキュメント id を受けるハンドラがサーバ上のファイルをそのまま生成関数に渡す」
  方針だったが、TASK-38 の実機確認で、LM Studio + Gemma 4 は 1 メガピクセルを超える画像を縮小せず拒否し、透過画像は
  アルファを捨てて黒く描写することが分かった。その回避はフロントの前処理が担っており、サーバで同じことをするには
  WebP のデコードと縮小のために新しい依存 (`golang.org/x/image`) が要る。ループバック内の取得は再送のコストがほぼ無いため、
  前処理を 1 か所に保つフロント経路を採る。これに伴い id 指定の生成ハンドラと、公開パス → ディスクパスの共有ヘルパーの
  切り出しは不要になった。
  - 形式・サイズの判定は既存の生成 API がバイト列に対して行う (TASK-38 と同じ)。フロントの生成ボタンの無効化は
    表示上の案内で、拒否はサーバが担う。保存済みファイルが取得できない場合はエラーとして画面に表示する。
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
- [x] #1 画像ドキュメントの note・tags・derived_text を詳細パネルから編集して保存できる。image 以外のドキュメントに対する更新 API 呼び出しは 400 で拒否される
- [x] #2 保存後、更新した内容でチャンク・FTS 行が作り直され、埋め込みが再同期される (更新した derived_text で検索にヒットする)。再同期に失敗しても保存は成功扱いになり、そのドキュメントは再ビルドまで FTS だけで検索される
- [x] #3 作成と更新で索引の組み立てが共有されており、同じ内容なら同じ索引になる
- [x] #4 更新時、category が misc のときだけ更新後の内容で再推論され、misc 以外の category は変わらない。再推論の結果は保存直後の詳細パネルの表示とカテゴリ選択のドラフトに反映される
- [x] #5 編集画面から、保存済み画像をフロントで取得し、追加ダイアログと同じ前処理 (1 メガピクセル超の縮小・PNG / WebP の白背景合成) を通して既存の生成 API で説明文を生成できる。生成結果は保存まで永続化されない
- [x] #6 保存済み画像が対象外の形式・上限超過なら生成は拒否され理由が表示される (判定は既存の生成 API がバイト列に対して行う)。保存済みファイルが取得できない場合はエラーが表示される
- [x] #7 repository の更新処理 (索引の作り直し・カテゴリの再推論を含む) にテストがある
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. repository: CreateDocument のチャンク・FTS 組み立てを writeDocumentIndex(tx, doc, createdAt) に切り出し、UpdateDocumentContent(id, note/tags/derivedText) から共有する。更新は 1 トランザクションで行 UPDATE → deleteChunks → writeDocumentIndex。category が misc のときだけ更新後の内容で doccategory.Infer をかけ直す
2. httpapi: PATCH /api/documents/{id}/content (JSON: note, tags はカンマ区切り文字列, derivedText。3 つとも文字列必須)。image 以外は 400、無ければ 404。保存後の SyncDocument は作成時と同じ best-effort
3. frontend: prepareForDescription を ImageDocumentDialog から共有モジュールに移し、Blob を受ける形にする。詳細パネルに画像用の編集フォーム (ImageDocumentEditor) を置き、/files から保存済み画像を取得 → 前処理 → 既存 POST /api/image-description で下書き生成。保存後は返ってきた document で state と selectedDocument を置き換える
4. テスト: repository (索引の作り直し・旧語が FTS から消える・作成と同内容なら同じ索引・misc のみ再推論)、httpapi (image 以外 400・404・必須フィールド・保存成功)。go test / vet, frontend typecheck・build
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装
- repository: `CreateDocument` のチャンク・FTS 組み立てを `writeDocumentIndex(tx, doc, createdAt)` に切り出し、新設の `UpdateDocumentContent` と共有した (#3)。更新は 1 トランザクションで行 UPDATE → 旧チャンク・FTS 削除 → 再構築。category が misc のときだけ更新後の内容で `doccategory.Infer` をかけ直す (#4)。
- API: `PATCH /api/documents/{id}/content` (JSON: note / tags (カンマ区切り文字列) / derivedText)。3 つとも文字列必須にした。PATCH で 1 つ欠けたクライアントが黙ってその欄を空にしないため。image 以外は 400、無ければ 404 (#1)。保存後の `SyncDocument` は作成時と同じ best-effort (#2)。
- UI: 詳細パネルの画像ドキュメントに「メモ・タグ・説明文を編集」を置き、`ImageDocumentEditor` を開く。保存後は返ってきた document で state と `selectedDocument` を置き換え、編集状態を閉じる。カテゴリのドラフトは `selectedDocument` の変化で初期化されるので、再推論後の値に追随する (#4)。
- 生成 (#5, #6): エディタが保存済み画像を `/files/<name>` から取得し、追加ダイアログと同じ `prepareForDescription` (1MP 超の縮小・PNG / WebP の白背景合成) を通して既存の `POST /api/image-description` に送る。`prepareForDescription` と生成不可理由の判定 (`generateBlockerReason`) は `components/prepareForDescription.ts` に移し、ダイアログと共有した。

## 着手時に決めたこと
- **生成経路をタスク記載の「id 指定のサーバ側ハンドラ」から「フロントで取得して前処理 → 既存 API」に変更した (ユーザー判断, 2026-09-23)**。TASK-38 の実機確認で LM Studio + Gemma 4 が 1MP 超を拒否し、透過画像を黒く描写することが分かっており、その回避はフロントの前処理にある。サーバで同じことをするには `golang.org/x/image` の追加が要る。これに合わせて説明文の方針と AC #5〜#7 を改訂した (旧 #7 の共有ヘルパー切り出しは不要になり削除、旧 #8 のテストの AC が #7 になった)。

## 検証
- `go test ./...` / `go vet ./...` / `pnpm check:client` / `pnpm build:client` はすべて通過。
- 追加したテスト:
  - repository (#7): 更新後に新しい語で FTS にヒットし、旧 derived_text・note・tag ではヒットしない。旧チャンクの埋め込みは cascade で消える。作成と更新で同じ内容なら `document_chunks` と FTS 行が id 以外一致する (#3)。misc の画像は説明文を足すと world に再推論され、character のまま更新しても変わらない (#4)。
  - httpapi: 必須フィールド欠落・404・text ドキュメントの 400・更新成功 (misc → world がレスポンスに乗る)。偽の `/embeddings` を立てた再同期成功 (更新後の本文が埋め込み入力に含まれ、現行チャンク数と同数のベクトルが保存される)。到達不能な埋め込みエンドポイントでも 200 を返し、ベクトル 0 件になる (#2)。
- 実機 (ビルド済み SPA と API を同一オリジンで配信する検証用サーバ + ブラウザペイン、LM Studio gemma-4-e4b-it-qat、埋め込み無効):
  - 1600x900 の透過 PNG (黒文字) の編集画面で生成すると、送信画像は 1365x768 の PNG で角の画素が (255,255,255,255)。生成結果は「『港の灯台地図』という日本語のタイトルが、白地の背景の中央に書かれている。」。生成直後の API では derived_text が空のまま (#5)。
  - misc の画像に「世界設定」を含む内容を保存すると、表示・カテゴリ選択とも「世界設定」になり、「カテゴリを保存」は無効 (ドラフト = 保存値) (#4)。その後 note / tags / derived_text を書き換えて保存するとカテゴリは世界設定のまま、表示にタグ・メモが出て、チャンクは新しい内容 1 件になった。
  - 保存済みファイルを退避すると「保存済みの画像を読み込めないため説明文を生成できません (HTTP 404)」が出て生成ボタンが無効 (#6)。GIF の画像ドキュメントは形式の理由表示で無効。GIF を image/png と偽って既存 API に直接送ると 415 (判定はバイト列)。
  - text ドキュメントへの更新 API は 400 (`only image documents can be edited`)。

## 未確認 (人間の確認が要るもの)
- Wails の WebView (WKWebView) 上での操作。確認は Chromium 系のブラウザペインで行った。
- 実際の埋め込みモデルでの再同期と、それによる検索ヒット。同梱サイドカーは TASK-41 の不具合で埋め込みが作られない状態のため、偽のエンドポイントのテストだけで確認した。

## レビュー対応 (PR #38)
- R1 [P2] 保存中に詳細ダイアログを閉じて別の画像を開くと、遅れて返った保存応答が元の画像を選択し直し、開いていた画像の編集中の入力を捨てていた (保存は埋め込み同期を待つので遅くなり得る)。保存中はダイアログを閉じられないようにし (追加ダイアログと同じ扱い)、応答で選択を置き換えるのは選択中のドキュメントが同じときだけにした。ブラウザペインで PATCH を 4 秒遅らせて確認: 保存中は Escape と「閉じる」(無効化) のどちらでも閉じず、応答後に保存内容が表示されて編集状態が閉じた。
<!-- SECTION:NOTES:END -->
