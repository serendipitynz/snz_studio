---
id: TASK-38
title: 画像ドキュメントの追加フォームを作り、マルチモーダルモデルで derived_text の下書きを生成する
status: Done
assignee: []
created_date: '2026-09-22 20:47'
updated_date: '2026-09-23 09:02'
labels: []
dependencies: []
references:
  - internal/service/llmclient.go
  - internal/httpapi/handlers.go
  - internal/httpapi/server.go
  - internal/service/context.go
  - internal/service/turnengine.go
  - internal/httpapi/multiagent.go
  - frontend/src/pages/ProjectDetailPage.tsx
  - .env.example
  - internal/config/config.go
  - frontend/src/components/SettingsModal.tsx
type: feature
ordinal: 38000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

画像ドキュメントは、画像ファイルそのものが LLM に渡らない。`LLMClient` が送るメッセージの `content` は文字列だけ
(`internal/service/llmclient.go` の `chatMessage`) で、`image_url` パーツを送る経路が無いため、マルチモーダルな
モデルに切り替えても画像は読まれない。AI に届くのはタイトル・ノート・タグ・`derived_text` のテキストだけ
(プロンプト本文は `ContentText → DerivedText → Note` の順で最初に空でないもの、`internal/service/context.go`)。
検索 (FTS5 と埋め込み) の対象も同じ項目である。

さらに、UI から `derived_text` を入力する経路自体が無い。現状のアップロードは素のファイルピッカーで、
選んだファイルごとに `type` / `title` / `file` だけを送る (`frontend/src/pages/ProjectDetailPage.tsx`)。
サーバは `note` / `tags` / `derivedText` を受け取れる (`internal/httpapi/handlers.go`) が、UI から送られていない。
そのため画像を追加しても AI にはファイル名しか見えず、検索にもチャットの文脈にもほとんど効かない。

## 方針

- **画像の追加フォームを新設する**。1 枚の画像について title・note・tags・derived_text を入力して作成する。
  既存の一括アップロード (複数ファイルをまとめて投入する経路) は変えずに残す。**Why**: 一括投入で 1 枚ずつ
  フォームを出すと、markdown / text と混ぜた投入が重くなる。一括で入れた画像に後から説明を付けるのは TASK-39。
- **同名タイトルの扱いは一括経路に揃える**。title の初期値はファイル名。作成時に同じ title の既存ドキュメントが
  あれば、確認ダイアログ (`ConfirmDialog`) を出し、承諾で既存を削除して作り直す。拒否なら作成せずフォームを保つ。
  **Why**: このアプリでは title がドキュメントを見分ける手がかりで、一括経路も同名を「上書き」として扱っている。
  経路によって同名を許したり許さなかったりすると、後で一括投入したときに上書き対象が曖昧になる。
- フォームに「説明文を生成」操作を置く。画像をマルチモーダルモデルに一度だけ送り、生成した説明文を
  `derived_text` 欄に下書きとして入れる。
  「下書き」とは、人間が確認・編集してから保存する編集可能な文で、作成操作までは永続化しないものを指す。
  自動保存はしない (誤読した説明が検索とチャット文脈に黙って入るのを防ぐため)。
- **画像の渡し方**: 生成時点ではドキュメントはまだ作られておらずサーバに `file_path` が無い。生成用の
  エンドポイントを新設し、画像本体を multipart で送る。サーバは画像を保存せず、data URL にして
  `image_url` パーツでモデルに渡し、説明文だけを返す。
- **生成処理の形**: 「画像のバイト列を受けて説明文を返す」サービス関数を 1 つ置く。形式は呼び出し側から受け取らず、
  関数の中で判定する (下の「受け付ける形式」)。multipart を受けるハンドラはそれを呼ぶだけにする。
  **Why**: (1) 関数は data URL (`data:image/png;base64,...`) を組み立てるので形式が要る。判定を関数の中に置けば、
  data URL に書く型と符号化するバイト列の出所が同じになり、呼び出し側の申告を信じる構造に戻らない。
  (2) TASK-39 の id 指定の生成や、将来の一括生成のように経路が増えても、判定が自動で掛かり書き忘れる余地が無い。
  判定結果を data URL に使うので、これは入力の検証というより入力の解釈であり、関数の内側に置くのが自然である。
  - 対象外の形式・上限超過は sentinel error (例: `ErrUnsupportedImageFormat` / `ErrImageTooLarge`) で返し、
    ハンドラが `errors.Is` で 400 系に写す (多人数会話の `turnengine.go` と `multiagent.go` の前例)。
    `fail()` は渡された error を無条件で 500 にするので、写像は各ハンドラに書く。
- チャットのたびに画像を送る方式は採らない。**Why**: 生成を登録時の 1 回に限れば、検索 (FTS5・埋め込み) にも効き、
  非対応モデルへの配慮やトークン消費の増加がチャット経路に波及しない。多人数会話ではターンごとに毎回送ることになるため、
  特に割に合わない。既存のチャット・多人数会話の `LLMClient` 呼び出しは文字列 `content` のまま変えない。
- **設定は既存の review / embedding 系に倣う**: `.env` をブートストラップ既定値とし、実運用値は設定画面
  (`SettingsModal` + `config.UpdateEditable`) で編集・永続化する。接続先 URL は未設定なら `LLM_BASE_URL` に
  フォールバックする (review 系の前例)。モデル名は**フォールバックせず、空なら機能を無効化**する (embedding 系の前例)。
  **Why**: 既定の `LLM_MODEL` はマルチモーダルとは限らず、フォールバックすると画像を見ていない説明文が
  黙って返り得る。モデル候補の取得は既存の `listConfigurationModels` を流用する。設定項目名は実装時に決め、
  既存の命名 (`REVIEW_*` / `EMBEDDING_*`) に揃える。
  候補一覧 (`/v1/models`) はマルチモーダル可否を返さないので、非対応モデルも選べてしまう。これは受け入れ、
  下の「失敗時」の人間確認で担保する。
- **タイムアウトは専用の env 値を足す**。既存の timeout 系と同じく env 専用 (設定画面には出さない) とし、
  既定値は `LLM_TIMEOUT_MS` の既定 (60 秒) より長くする (値は実機確認で決めて記録する)。**Why**: ローカルの
  マルチモーダルモデルは画像入力の前処理が重く、チャット用の 60 秒では足りないことがある。チャット側の
  タイムアウトを延ばすと、テキストだけの応答が詰まったときの失敗検知まで遅れる。
- **受け付ける形式**: 画像ドキュメントとしては現行どおり `image/*` を全部受け付けるが、生成に送る形式は
  マルチモーダルモデルが一般に受け付けるもの (png / jpeg / webp など) に限る。対象外の形式 (svg / heic など) では
  生成操作を無効化し理由を表示する (変換はしない)。フロントの無効化は案内で、拒否は生成サービス関数が担う。
  関数は multipart のパートヘッダや `documents.mime_type` の申告ではなく、先頭バイト (`http.DetectContentType`) で
  形式を判定し、**許可リスト**に無ければ拒否する。**Why**: `http.DetectContentType` は HEIC / AVIF を認識できず
  `application/octet-stream` を、SVG には `text/xml` か `text/plain` を返す。拒否リストでは認識できない形式が通ってしまうが、
  許可リストなら未知の形式は拒否側に倒れる。
- **サイズ上限は生成経路にだけ設ける** (値と縮小の要否は実装時に決めて記録する)。保存経路には上限が無いので、
  「保存はできるが生成はできない」サイズが生じるが、これは意図した非対称とする。**Why**: 保存はローカル
  ディスクに置くだけで制約が無い。生成はモデルへのリクエストサイズと画像入力の処理量に上限がある。
  上限を超える画像では生成操作を無効化し理由を表示する (説明は手で書ける)。
  - 上限の判定も生成サービス関数に置き、形式の判定と同じ場所にまとめる。そのうえでハンドラ側にも、全体を読み込む前の
    安い事前チェック (multipart は Content-Length、TASK-39 の id 経路は `os.Stat`) を置く。**Why**: 全部読み込んでから
    弾くと上限の意味が薄れる。data URL は base64 で約 1.33 倍に膨らむので、メモリの面でも読み込む前に弾く価値がある。
- 生成プロンプトは、検索に効く記述 (写っている物・文字・図の構造・固有名) を日本語で簡潔に書かせる。
  写っていないことの推測は書かせない。
- **失敗時**: 接続失敗・タイムアウト・エンドポイントのエラー応答はエラーとして表示し、フォームの入力内容は失わない。
  非対応モデルは検出しきれない (OpenAI 互換のローカルエンドポイントは `image_url` パーツを黙って捨てて 200 を
  返すことがある)。検出できた場合はエラーにし、検出できない場合は人間の確認を必須にすること (自動保存しない) で担保する。

## スコープ外

- 既存画像ドキュメントの note・tags・derived_text の編集と、そこからの生成 (TASK-39)。
- チャット時・多人数会話のターンで画像そのものをモデルに渡すこと。
- OCR 専用処理 (AGENTS.md の No OCR。モデルが画像内の文字を説明文に含めるのは妨げない)。
- 画像形式の変換、既存画像への一括生成、タイトル・ノート・タグの自動生成。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 画像の追加フォームから title・note・tags・derived_text を入力して画像ドキュメントを作成できる。同じ title の既存ドキュメントがあれば確認ダイアログを経て上書きされ、拒否すれば作成されずフォームが保たれる。既存の一括アップロードは従来どおり動く
- [x] #2 追加フォームで「説明文を生成」を押すと、画像が生成用エンドポイントに multipart で送られ、マルチモーダルモデルの説明文が derived_text 欄に下書きとして入る。生成時点ではサーバに画像もドキュメントも保存されない
- [x] #3 生成結果は人間が編集して作成操作をするまで永続化されない
- [x] #4 画像説明用の設定が .env 既定値 + 設定画面で編集でき、接続先 URL は LLM_BASE_URL にフォールバックし、モデル名が空なら生成操作が無効化され理由が表示される。タイムアウトは専用の env 値で、既定値と根拠が記録されている
- [x] #5 生成対象外の画像形式 (svg / heic など) と上限超過のサイズでは生成操作が無効化され理由が表示される。生成サービス関数は先頭バイトで形式を判定し、許可リスト (png / jpeg / webp など) に無い形式と上限超過を sentinel error で拒否し、ハンドラはそれを 400 系で返す。この判定にテストがある。サイズ上限・縮小の扱いと、保存経路との非対称の理由が記録されている
- [x] #6 接続失敗・タイムアウト・エラー応答でエラーが表示され、フォームの入力内容が失われない
- [x] #7 説明文の生成は画像のバイト列だけを受けるサービス関数にまとまっており (形式は引数で受けず関数内で判定する)、multipart ハンドラはサイズの事前チェック・関数の呼び出し・エラーの写像だけを行う
- [x] #8 既存のチャット・多人数会話の LLM 呼び出しは文字列 content のまま変わらない
- [x] #9 マルチモーダルモデルで実機確認し、追加フォームで生成・保存した derived_text で画像ドキュメントが検索・チャットの参照にヒットすることを確認している (#1〜#3 の保存経路が前提)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. config: Editable に imageDescriptionBaseUrl / imageDescriptionModel、Settings に ImageDescriptionTimeoutMs (env IMAGE_DESCRIPTION_BASE_URL / _MODEL / _TIMEOUT_MS)。URL は空なら利用時に LLMBaseURL へフォールバック、モデルは空なら無効
2. service/imagedescription.go: DescribeImage(ctx, []byte) — 無効/上限/形式 (http.DetectContentType + 許可リスト png/jpeg/webp) を sentinel error で返し、data URL の image_url パーツで専用の multimodal リクエストを送る。既存 chatMessage は触らない
3. httpapi: GET /api/image-description (有効可否・上限・許可形式) と POST /api/image-description (multipart, Content-Length 事前チェック, errors.Is で 4xx 写像)
4. frontend: ImageDocumentDialog を新設 (title/note/tags/derived_text + 説明文を生成 + 同名確認)、ドキュメント欄に画像追加ボタン、SettingsModal に画像説明モデル欄
5. テスト (service の判定・ハンドラの写像・config)、.env.example / README 更新、LM Studio の vision モデルで実機確認
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装
- 設定: `IMAGE_DESCRIPTION_BASE_URL` / `IMAGE_DESCRIPTION_MODEL` (設定画面で編集・app-config.json に永続化) と env 専用の `IMAGE_DESCRIPTION_TIMEOUT_MS`。URL は保存時に LLM の値で埋めず空のまま持ち、利用時に `LLMBaseURL` へフォールバックする (review 系は保存時に埋めるが、それだと後で LLM エンドポイントを変えても追従しないため)。モデルはフォールバックしない。
- 生成: `service.ImageDescriptionService.DescribeImage(ctx, []byte)`。無効 / 上限超過 / 形式 (`http.DetectContentType` + 許可リスト png・jpeg・webp) を sentinel error で返す。リクエストは専用の `visionMessage` 型で組み、既存の `chatMessage` (文字列 content) には触れていない (#8)。
- API: `GET /api/image-description` (有効可否・上限・許可形式) と `POST /api/image-description` (multipart)。ハンドラは Content-Length の事前チェック + MaxBytesReader、関数呼び出し、`errors.Is` での写像 (無効 409 / 形式 415 / 上限 413 / エンドポイント失敗 502) だけ。
- UI: `ImageDocumentDialog` (ドキュメント欄の画像アイコンから開く)。同名タイトルは一括経路と同じ確認ダイアログ → 削除 → 作成。既存の一括アップロードのコードは変更していない。
- multipart のテキスト欄はブラウザが改行を CRLF にするため、作成ハンドラで note / derivedText を LF に揃えた (実機で CRLF 保存を確認して追加)。

## 着手時に決めたこと
- **サイズ上限 10 MiB (生成経路のみ)**。保存経路は無制限のままで、非対称は意図どおり (理由はコメントと .env.example)。
- **縮小は UI で行う**。実測で LM Studio + google/gemma-4-12b-qat は 1024x1024・2048x512 (約 1MP) は通るが 1280x1280・1536x1536・1600x900・2048x2048 を `{"error":"terminated"}` (400) で即拒否し、自前で縮小しなかった。スマホ写真はそのままでは生成できないので、送信前に canvas で 1,048,576 px 以下へ縮小する (PNG は PNG、他は JPEG q0.9)。保存するのは元ファイル。Go 側で縮小しないのは WebP デコーダとリサンプラが標準ライブラリに無く依存追加になるため。**TASK-39 の id 指定経路はサーバで画像を読むので、この縮小を通らない**。TASK-39 では UI が保存済み画像を取得して同じ縮小を経由して POST する形にするか、サーバ側縮小を検討する必要がある。
- **タイムアウト既定 180000 ms**。実測 (LM Studio, gemma-4-12b-qat, reasoning on): 800x500 PNG 64 s / 81 s、1024x1024 写真 94 s、UI 経由 900x600 PNG 82 s。大半は reasoning トークン (1024² 写真で 655 中 614)。遅い側の約 2 倍。

## 検証
- `go test ./...` / `go vet ./...` / `pnpm check:client` すべて通過。
- 追加テスト: 形式判定 (svg / heic / gif / 空 / テキストを拒否、png / jpeg / webp は data URL の型が判定結果と一致)、上限超過、無効、URL フォールバック、エンドポイント失敗 (非対応モデル・LM Studio の文字列エラー・200 のエラー本文・非 JSON・空応答)、タイムアウト、ハンドラの写像 (409 / 415 / 413 事前チェック / 400 / 502)、生成後に uploads が空、CRLF→LF、config の env 既定と永続化。
- 実機 (ローカル LM Studio、ビルド済み SPA と API を同一オリジンで配信する検証用サーバ + ブラウザペイン):
  - モデル未設定で「説明文を生成」が無効になり理由表示 → 設定画面でモデル入力 (候補は LLM エンドポイントから 7 件) → 保存で app-config.json に永続化。
  - SVG で無効化 + 理由表示。4032x3024 JPEG は 1182x886 (49 KB) に縮小して送信され生成成功。
  - 生成直後は uploads 0 件・ドキュメント 0 件。下書きを編集して追加すると note・tags・編集後の derived_text・元画像が保存される。
  - 同名タイトル: 確認ダイアログでキャンセル → フォーム保持・既存無傷、OK → 上書き (旧ファイル削除)。
  - 接続失敗 (connection refused) と非対応モデル (gpt-oss-20b: "does not support image inputs") でエラー表示、入力保持。
  - フォームで生成・手直し・保存した年表画像に対し、チャットで「ミレナが着任したのは何年？」→ 回答「1402年」、参照に画像ドキュメント (#9)。生成結果には「ミレナが」を「ミレナに」とする誤読が 1 か所あり、人間確認を必須にした方針の妥当性も確認できた。

## 未確認 (人間の確認が要るもの)
- Wails の WebView (WKWebView) 上での操作。検証は Chromium 系のブラウザペインで行った。特に canvas 縮小 (`image.decode()` / `toBlob`) と WebP のデコードは WKWebView で未確認。
- 埋め込み検索でのヒット。検証用サーバは埋め込み無効 (FTS のみ) で動かした。
- タイムアウトの UI 表示はユニットテストのみ (実機で 180 s 超を再現していない)。上限超過の UI 無効化も、縮小後に 10 MiB を超える画像を作れず UI では未再現 (判定はサービス・ハンドラのテストで確認)。

## レビュー対応 (PR #37)
- R1 [P2] 縮小時に WebP を JPEG へ再エンコードしていたため、透過 WebP の透明部分が黒で合成され、黒い文字や線画がモデルに届かない。JPEG 元画像だけ JPEG、PNG・WebP は PNG で出すよう修正。ブラウザペインで 1600x900 の透過 WebP (黒文字) を確認: 修正前は送信画像の全 1,048,320 px が不透明な黒、修正後は PNG で背景 alpha 0・文字の不透明な黒 18,564 px。フロントにテストランナーが無いため自動テストは追加していない (依存追加になる)。

- 実機 (Wails アプリ, gemma-4-e4b-it-qat) で透過 WebP (黒文字) が「黒一色の画像」と説明された。上の修正で PNG は alpha を保っていたが、モデル側 (LM Studio) が alpha を捨てるため、透明画素の下地の黒が見えていた。PNG・WebP は縮小の要否にかかわらず白で塗った canvas に描き直して PNG で送るよう再修正 (小さい透過 PNG もそのまま送ると同じ問題になるため)。白地に白い線画の透過画像は判読できなくなるが、アプリのプレビューや一般の画像ビューアで人が見る背景が白系なのでそちらに合わせた。ブラウザペインで確認: 送信画像は PNG・背景 (255,255,255,255)、gemma-4-e4b-it-qat が文字列「霧鐘の塔 見取り図」「東門 → 螺旋階段 → 鐘楼」を読み取った (色は「黒背景に白文字」と誤記。送信画素は白地に黒文字であることを確認済み)。

## 実機確認 (ユーザー, Wails アプリ / WKWebView, 2026-09-23)
- スマホ写真 (IMG_8037.jpg) と 4032x3024 の JPEG・WebP で説明文を生成できた。1MP 超は LM Studio が拒否するので、WKWebView 上でも canvas 縮小が効いていることになる。WebP の追加・表示も確認済み。
- 埋め込み検索は未確認のまま。開発 DB で新規ドキュメント (この PR 以前に追加した slides.md を含む) に埋め込みが 1 件も作られていない。原因は同梱サイドカー (llama-server, ubatch 512) が 512 トークンを超える入力を 500 で拒否し、EmbeddingClient が 1 回の失敗で自身を無効化して以後の同期を黙って飛ばすこと。TASK-38 の変更とは独立した既存の不具合なので、別タスクで扱う。
<!-- SECTION:NOTES:END -->
