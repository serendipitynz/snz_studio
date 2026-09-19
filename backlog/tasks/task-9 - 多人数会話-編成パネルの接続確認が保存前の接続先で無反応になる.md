---
id: TASK-9
title: '多人数会話: 編成パネルの接続確認が保存前の接続先で無反応になる'
status: Done
assignee: []
created_date: '2026-09-19 01:46'
updated_date: '2026-09-19 21:54'
labels: []
milestone: m-0
dependencies: []
references:
  - docs/multi-agent-chat-design.md
  - frontend/src/components/ParticipantPanel.tsx
ordinal: 9000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
編成パネル (設計書 §6) の「接続確認」が、参加者をまだ保存していない接続先に対して何も表示しない。
ボタンを押しても「確認中…」に変わらず、接続済みバッジも失敗バッジも出ず、成功してもモデル選択の
プルダウンが出ない。押しても画面が一切変化しないので、操作した側からは無反応に見える。

原因は確認結果のキーの食い違い。ボタンは編集中の下書きの接続先を送り、結果もその文字列をキーに
保存する (`ParticipantPanel.tsx` の `probeEndpoint` と `onProbe(baseUrl)`)。一方で表示側は保存済みの
`participant.baseUrl` をキーに読み出す (`probe={probes[participant.baseUrl.trim()]}`)。接続先を
入力した直後はこの 2 つが一致しないので、結果は保存されているのに永久に見つからない。

プリセット (TASK-5) から作った参加者は接続先が空で始まるため、必ずこの状態を踏む。
保存済みの接続先を再確認するときだけ一致するので、これまで気づかれなかった。

回避策: 接続先を入力してから先に「保存」を押し、そのあとで「接続確認」を押す。

修正の方向: 表示側の参照キーを、下書きの接続先に合わせる。編集中の値は ParticipantEditor の
ローカル state にあるので、確認結果の参照をそちら側へ移すか、確認結果を参加者 ID で持つ。
どちらにするかは、同じ接続先を複数の参加者で共有したときに確認結果を使い回したいかで決まる
(現状はキーが接続先の文字列なので使い回される)。

なお同じ実機確認で、接続先の URL に `/v1` を付け忘れると
`LLM model list request failed with 404` が返ることを確認している。サーバー側は正しく即座に
エラーを返しており、上記の不具合でそれが画面に出ていなかっただけ。
本タスクの修正でこのエラーは表示されるようになる。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 保存していない接続先に対して接続確認を押すと、確認中・接続済み・失敗のいずれかが画面に出る
- [x] #2 保存前の接続先で接続確認が成功したら、モデル選択のプルダウンにモデルが並ぶ
- [x] #3 接続確認が失敗したときは、サーバーが返したエラー文言が画面に出る (base URL の /v1 付け忘れなら、LM Studio が返す Unexpected endpoint or method. の文言)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Go: sortedStrings が空スライスで nil を返し JSON が {"models": null} になるのを直す (空でも非 nil)。フロントの probe.models.length クラッシュの直接原因。
2. Go: LLM/Embedding の ListModels で、モデル一覧として読めない 200 応答 (data フィールド無し、error フィールド有り) を失敗にする。LM Studio は誤ったパスに 200 + {"error":...} を返すため、現状は「接続OK・0件」に化ける。non-2xx でもサーバーの error 文言を拾って返す。data が空配列で存在する場合は正常 (モデル 0 件のサーバー) として扱う。
3. frontend: ParticipantEditor に probes マップを渡し、編集中の下書き baseUrl をキーに引く。probes のキーは接続先文字列のまま (同じ接続先を共有する参加者間で確認結果が使い回される現状の挙動を維持)。
4. frontend: API 応答を state に入れる境界で models を [] に正規化する。
5. Go テスト: 空 data / 200+error / non-2xx+error の 3 ケース。go test と frontend の lint/build。
6. AC #3 の文言を実機の挙動に合わせて修正 (404 → サーバーが返したエラー文言)。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 原因は 3 つ (タスク本文の診断は 1 つ目のみ)

実機 (`http://192.168.0.219:1234`) に対して確認した結果、再現手順 1 と 2 は別々の不具合だった。

1. **確認結果のキーの食い違い** (本文の診断どおり)。`probes` は下書きの接続先をキーに書き込むのに、
   読み出しは保存済みの `participant.baseUrl` を使っていた。`/v1` 付きの再現手順で「保存を押すと
   接続OKが出る」のはこれ。
2. **`sortedStrings` が空スライスで nil を返す**。`append([]string(nil), in...)` は入力が空だと nil を
   返すため、モデル 0 件のとき handler が `{"models": null}` を返し、フロントの `probe.models.length` が
   TypeError で落ちて ParticipantEditor ごとアンマウントしていた。再現手順 1 の「押しても画面が一切
   変化しない」の正体はこのクラッシュ (コンソールログの `null is not an object`)。
3. **モデル一覧として読めない 200 応答を成功扱いしていた**。LM Studio は `/v1` の無い base URL に
   404 ではなく **200 + `{"error":"Unexpected endpoint or method. (GET /models)"}`** を返す
   (実測)。`respOK` を通り `data` フィールドが無いまま 0 件として成功し、上の 2 と合わさって
   クラッシュしていた。タスク本文と旧 AC #3 の「404 が返る」という前提は実機と食い違っていたため、
   AC #3 を「サーバーが返したエラー文言が出る」に書き換えた (着手時にユーザー承認済み)。

## 選択した方針

- **probes のキーは接続先文字列のまま**にし、読み出し側を下書きに合わせた (着手時にユーザーが選択)。
  参加者 ID をキーにする案は採らない。ローカル LLM サーバー 1 台に複数の役割を割り当てるのが
  本機能の主な使い方で、同じ接続先を指す参加者の間で確認結果が使い回される現状の挙動に価値があるため。
  `ParticipantEditor` には解決済みの probe ではなく `probes` マップを渡し、editor 側が自分の
  下書き `baseUrl` で引く。
- **`sortedStrings` 自体を直した** (呼び出し箇所ごとではなく)。この関数の戻り値は 4 箇所すべてが
  JSON 応答に載るため、nil と空配列の違いがそのまま `null` と `[]` の違いになる。
- **`data` フィールドの有無で判定**する (`Data` をポインタにした)。モデルを 1 つも持たない
  OpenAI 互換サーバーは `{"data":[]}` を返すので、これは正常な 0 件として通す必要がある。
- **フロントの null 正規化は API クライアント 1 箇所**に置いた。`listConfigurationModels` は
  ParticipantPanel と SettingsModal の計 4 箇所から呼ばれ、いずれも結果を直接 `.map()` する。
- **非 2xx でもサーバーの error 文言を拾う**ようにした (従来はステータス番号のみ)。AC #3 が
  求めているのはサーバーの文言なので、200 の経路だけ直すと応答形の違いで出たり出なかったりする。

## 検証

- `go test ./...` 全パス。`parseModelList` に 6 ケースの単体テストを追加
  (正常系 / `data:[]` が nil を返さないこと / 200+error / 200 で data 無し / 404+OpenAI 形 error /
  502 で本文が JSON でない)。
- `go vet ./...` clean、`npm run check:client` (tsc) clean。
- **実機での確認**: 使い捨てテストから `LLMClient.ListModels` を実際の LM Studio に対して実行し、
  `http://192.168.0.219:1234/` → `LLM model list request failed: Unexpected endpoint or method. (GET /models)`、
  `http://192.168.0.219:1234/v1` → モデル 8 件の配列、を確認した。
- サーバーの文言が画面まで届く経路はコード上で追い切った:
  `parseModelList` の error → `fail()` → `{"error": ...}` (500) → `request()` が `Error(data.error)` を
  throw → `probeEndpoint` の catch → `{state:"failed", error: message}` → `<Badge tone="warm">`。
  失敗バッジ自体は既存の未変更コードで、キーの食い違いで到達不能だっただけ。

## 未検証 (AC を未チェックにしている理由)

AC #1〜#3 はいずれも「画面に出る」という描画の主張で、確定には実機の GUI 操作
(接続確認ボタンの押下) が要る。このリポジトリにフロントのテスト基盤が無く、導入は本修正の
範囲を超えるため、3 件とも未チェックのまま In Review に上げる。再現手順をもう一度なぞって
もらった上で、マージ後にチェックする。

## 本修正の範囲外 (申し送り)

`internal/search/model.go` が gofmt 未整形のまま (本タスクの変更前から)。今回は触っていない。
<!-- SECTION:NOTES:END -->
