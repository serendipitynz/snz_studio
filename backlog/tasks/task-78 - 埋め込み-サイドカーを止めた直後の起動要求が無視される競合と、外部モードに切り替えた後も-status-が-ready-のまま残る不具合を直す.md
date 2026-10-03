---
id: TASK-78
title: '埋め込み: サイドカーを止めた直後の起動要求が無視される競合と、外部モードに切り替えた後も status が ready のまま残る不具合を直す'
status: In Review
assignee: []
created_date: '2026-09-28 20:23'
updated_date: '2026-10-03 09:46'
labels: []
dependencies: []
references:
  - internal/embed/manager.go
  - internal/httpapi/server.go
priority: low
type: bug
ordinal: 78000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

出典: 2026-09-22 のセキュリティ・コード品質レビュー (`main` @ `66488d8`。報告はリポジトリ外の `_sandbox/review-2026-09-22/review-report.md`) の F12 (Low、確信度 Medium)。2026-09-29 に `756d32b` で未修正を確認した。

- `Manager.Shutdown` は cancel して sidecar を Stop するだけで、`started` を下ろさない。`started = false` になるのは `superviseSidecar` が終わったとき (`markStopped`)。その前に `EnsureInternalReady` が来ると起動済みとみなして何もしないため、内蔵モードなのにサイドカーが動かない。成立するのは外部から内蔵への素早い切り替えのときで、窓は短い。
- 意図して止めたときは、`superviseSidecar` が status を更新せずに return する (`ctx.Err() != nil` の分岐)。このため外部モードに切り替えた後も `GET /api/embedding/status` が `ready` を返し続ける。これは切り替えのたびに起きる。

## 方針

- `Shutdown` で status を `disabled` にし、`started` の解除を同期的に行う。または世代番号を持たせて、古い run を無視する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Shutdown の直後に EnsureInternalReady を呼ぶと、サイドカーが改めて起動する。偽のバイナリを使うテストがある
- [x] #2 外部モードに切り替えた後、GET /api/embedding/status が ready を返さない。テストがある
- [x] #3 内蔵モードの通常の起動と終了 (TASK-41 で確かめた終了経路を含む) が変わらない
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Manager に世代番号 (gen) を持たせる。EnsureInternalReady は gen を進めて run に渡し、run 側の状態更新 (setState / setProgress / markReady / fail / markStopped) は自分の gen が現行のときだけ反映する。
2. Shutdown は同じロック内で gen を進め、started=false・sidecar=nil・status=disabled にしてから cancel と sidecar の Stop を行う。これで直後の EnsureInternalReady が新しい run を起動し、旧 run の後始末が新しい run の状態を上書きしない。
3. 旧 run が Shutdown と競合して Start を終えた場合は、markReady が古い gen を検出して sidecar を止め、ready コールバックを呼ばない。ready/lost コールバックと Shutdown は専用 mutex で直列化し、Shutdown 後に古い run がオーバーレイを再設定する窓を閉じる。
4. テスト: テストバイナリを偽 llama-server として再実行する (既存の TestSidecarDiesWithItsParent と同じ手法)。Shutdown 直後の再起動、Shutdown 後の status、旧 run が後から状態を書き戻さないことを確かめる。修正前のコードで失敗することも確認する。
5. go vet / go test、実 llama-server の統合テストを回す。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 方針の選択

- Description の 2 案のうち世代番号を採った。Shutdown で started を下ろすだけだと、旧 run の後始末 (`markStopped` / `fail` / `setState`) が新しい run の状態を上書きする。具体的には旧 run の `markStopped` が新 run 稼働中に started=false にし、次の EnsureInternalReady でサイドカーが二重に起動する。世代番号なら旧 run の書き込みをまとめて捨てられる。
- Shutdown は gen を進めたうえで started=false・sidecar=nil・status=disabled (ModelID/Dim は保持) を同じロック内で設定し、その後に cancel と Stop を行う。
- 同じ仕組みで、修正前からあった 2 つの競合も塞いだ。どちらも Description には書かれていない。
  - 旧 run が Shutdown と競合して Start を終えた場合、修正前はそのサイドカーを ready として公開し、ready コールバックで外部モードにオーバーレイを再設定し、プロセスも止めずに残していた。今は `becomeReady` が古い gen を検出し、サイドカーを Stop してコールバックを呼ばない。
  - ready/lost コールバックと Shutdown を `callbackMu` で直列化した。これで「Shutdown → ClearInternalEmbedding」の後に、古い run のコールバックがオーバーレイを戻す窓がなくなる。コールバック (`onEmbeddingReady` / `onEmbeddingLost`) は Manager を呼び返さず軽い処理だけなので、デッドロックや Shutdown の待ちは生じない。

## AC の根拠

- #1: `TestEnsureInternalReadyRightAfterShutdownRestartsSidecar`。テストバイナリを偽 llama-server として再実行し、本番と同じ起動経路 (unix は /bin/sh の見張り経由)・health・probe・kill を通す。Shutdown 直後の EnsureInternalReady で ready コールバックがもう一度呼ばれ、旧サイドカーは応答しなくなる。さらに 1 秒後の EnsureInternalReady でサイドカーが二重に起動しないことも確かめる。修正前の manager.go では 30 秒待っても再起動せず失敗した。markStopped の gen ガードだけを外すと、二重起動の検査で失敗する (ミューテーションで確認済み)。
- #2: `TestShutdownLeavesStatusDisabled`。Shutdown 直後から 1 秒間、Status() が disabled のまま・BaseURL が空であることを確かめる (旧 run が後から書き戻さないことも含む)。修正前は ready で失敗した。`GET /api/embedding/status` は `Manager.Status()` をそのまま返し、設定保存ハンドラの外部モード分岐は `Shutdown()` を呼ぶ。このためテストは Manager 単位にとどめた。httpapi から ready の Manager を作るにはモデル spec を差し替えるための公開 API が要り、テストのためだけに足すことになるので避けた。
- #3: 実 llama-server + ruri GGUF の `TestManagerIntegrationRealSidecar` が通り、終了時 (defer Shutdown) の後に llama-server が残っていない (pgrep で 0 件)。TASK-41 の SIGKILL 経路の自動テスト `TestSidecarDiesWithItsParent` も通過した。sidecar.go / sidecar_unix.go は変えていない。Shutdown の停止処理 (cancel + プロセスグループの kill) も同じまま。実アプリでの Cmd+Q と wails dev の再ビルドは、今回は再実施していない。

## 検証

`go vet ./...` / `go test ./...` 通過。`go test -race -count=3 ./internal/embed/` 通過。
<!-- SECTION:NOTES:END -->
