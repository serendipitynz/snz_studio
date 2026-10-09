---
id: TASK-92.3
title: 'UI: 起動時の自動確認、設定画面の版表示・手動確認・自動確認の切り替え、更新ダイアログを作る'
status: Done
assignee: []
created_date: '2026-10-07 22:53'
updated_date: '2026-10-09 21:30'
labels: []
dependencies:
  - TASK-92.2
references:
  - frontend/src/components/SettingsModal.tsx
  - frontend/src/components/Dialog.tsx
  - frontend/src/i18n/index.tsx
  - internal/config/config.go
parent_task_id: TASK-92
priority: medium
type: feature
ordinal: 95000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
TASK-92 の UI 側。TASK-92.2 のバインドを使い、自動確認・手動確認・更新ダイアログを作る。

## 作業

- **自動確認**: 起動後に遅らせて 1 回行い、初回の描画や起動時の読み込みと競合させない。失敗しても何も出さない。オフラインで起動するたびにエラーが出るアプリは、確認しないアプリより悪い。
- **設定画面 (`components/SettingsModal.tsx`)**: 実行中の版を表示する (今は macOS の About にしか版が出ず、Windows では更新されたかどうかを確かめられない)。手動確認のボタンを置き、「最新です」「新しい版があります」「確認できませんでした」の 3 通りの結果を出す。自動確認のオン/オフを置き、`app-config.json` (`internal/config`) に保存する。既定値は有効 (TASK-92 で決定)。
- **更新ダイアログ**: 新しい版の番号を見せ、利用者が承認するまでダウンロードしない。承認後は進捗を出す。
  - macOS は入れ替え後に新しい版で起動し直す。
  - Windows はインストーラに渡した時点でアプリが終了する。再起動を待つ状態のまま残らないようにする。
  - 「システムがパスワードや管理者の承認を求めることがある」と書く。仕組みの名前は出さない。Windows は per-user インストール (TASK-92.2) なので通常は承認を求めないが、確かめる前に「求めない」とは書かない。
  - latest.json の notes が空でも読めるようにし、Releases ページへのリンクを置く。
  - 承認後の失敗は「更新は行われませんでした」とだけ伝える。
  - macOS で入れ替えられないとき (TASK-92.2 が理由を返す場合) は、Releases ページを開く案内に切り替える。
- 確認には in-app のダイアログ (`components/Dialog` / `ConfirmDialog`) を使う。`window.confirm` は使わない (Wails の WebView では表示されず、常に false になる)。
- 新しい画面要素は snz-design の adoption guide (doc-16) と snz_studio の adoption record (doc-17) に沿わせる。
- 新しい文言は ja と en の両方の辞書 (`frontend/src/i18n`) に入れる。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 オフラインで起動しても、自動確認はエラーを何も出さない
- [x] #2 設定画面に実行中の版が出て、手動確認が「最新」「新しい版あり」「確認できなかった」の 3 通りを出し分ける
- [x] #3 新しい版の番号を見せて承認を得るまで、何もダウンロードもインストールもしない
- [ ] #4 進捗が見え、macOS は新しい版で起動し直し、Windows はアプリが終了した後に再起動を待つ状態のまま残らない
- [x] #5 システムがパスワードや管理者の承認を求めることがあると伝え、notes が空でもダイアログが読める
- [x] #6 新しい文言が ja と en の両方の辞書にあり、snz-design の doc-16 / doc-17 に沿っている
- [x] #7 pnpm build と pnpm test が通る
- [x] #8 起動後に初回の描画を遅らせずに自動確認が走り、既定で有効で、設定画面でオフにでき、その選択が app-config.json に保存される
- [x] #9 承認後の失敗を「更新は行われなかった」と伝え、エラー文面で分岐していない
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Go: app-config.json に autoCheckUpdates (既定 true、キーが無ければ true) を持たせる。Editable には入れず、接続設定の PUT (モデルの暖機や再構築を起こす) を通さない。Config に専用の getter/setter を足し、保存は既存の原子的な書き込みを共用する。バインド GetAutoUpdateCheck / SetAutoUpdateCheck を app_update.go に足す。
2. Frontend: 確認の流れ (遅らせた自動確認、失敗を握りつぶす) を純粋なモジュールに切り出し、node --test で確かめる。
3. UpdateProvider (main.tsx に置く) が起動 5 秒後に 1 回だけ自動確認し、新しい版があれば更新ダイアログを開く。設定画面からも同じダイアログを開ける。
4. 更新ダイアログ: 提示 → ダウンロード中 (進捗) → 再起動中 / 失敗 (更新は行われませんでした) / 手動 (理由コードごとの案内 + Releases ページを開く)。doc-9 §6.6 の構造 (見出し・本体・操作域、実行に初期焦点を置かない、処理中は閉じない)。
5. 設定画面に「更新」区画: 実行中の版、自動確認のチェックボックス、今すぐ確認と 3 通りの結果。
6. ja/en の辞書に文言を足す。check:client / build:client / test:client / go test / go vet。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- 自動確認の保存先: app-config.json の `autoCheckUpdates` (既定 true。キーが無い既存のファイルも true)。Editable には入れず、バインド `GetAutoCheckUpdates` / `SetAutoCheckUpdates` で読み書きする。接続設定の PUT を通すと、モデルの暖機や埋め込みの再構築が走るため。保存はこのキーだけを書き換える。Editable 全体を書くと、環境変数から来た接続の既定値がファイルに固定され、あとで環境を変えても効かなくなるため。
- 自動確認: `UpdateProvider` (main.tsx) が起動の 5 秒後に 1 回だけ確認する。流れは `components/updateCheck.ts` に切り出し、バインドの reject を含むすべての失敗を黙って捨てる。新しいバージョンがあれば更新ダイアログを開く。設定画面の手動確認も同じダイアログを開く。
- 更新ダイアログ (`components/UpdateDialog.tsx`): 案内 → 進捗 (`update:progress`。ダウンロードが終わったら「確かめて入れ替えています」に切り替える) → 再起動中。承認後の失敗は status だけで分岐し、「更新は行われませんでした。」を操作域に出す (doc-17 §3 の意図的な例外と同じ置き場所)。`manual` は理由コードごとの文と「Releases ページを開く」に切り替える。知らない理由コードには文を出さず、案内だけにする。latest.json の notes は UpdateCheck に含まれず、ダイアログは notes を使わない。代わりに Releases ページへのリンクを置く。リンクは BrowserOpenURL で既定のブラウザに開く (WebView 内で遷移させないため)。処理中は閉じられず、Escape と「後で」は理由を読み上げる。初期の焦点は面に置く (doc-9 §6.6)。
- 文言: ユーザーの指示 (2026-10-10) で、日本語の「版」は「バージョン」にした。既存の `settings.storedChoiceUnknown` の「この版」も直したので、UI の辞書に「版」は残っていない。README.ja.md の「版」は TASK-92.4 で直す (ユーザーと決定)。
- 確かめたこと:
  - AC #7: `pnpm check:client`、`pnpm build:client`、`pnpm test:client` (24 件。うち updateCheck.test.ts の 6 件が新規)、`go test ./...` (config の新しいテスト 4 件を含む)、`go vet ./...` を macOS と GOOS=windows の両方で通した。`pnpm build:app` (本番ビルド) も通った。package.json に `build` / `test` という名前のスクリプトは無いので、上の名前で読み替えた。
  - 画面の確認 (`wails dev` の localhost:34115 を Chromium で開き、DATA_DIR をスクラッチに、SNZ_UPDATE_BASE_URL をローカルの偽の配布元に向けた。WKWebView の実窓ではない):
    - AC #1: 配布元が 503 を返す状態で起動し、8 秒待っても、ダイアログも告知 (role=alert) も 0 件だった。ログには check の失敗が残る。バインドの reject と failed の結果はテストでも確かめた。
    - AC #2: 設定に「実行中のバージョン: v0.1.0」が出て、手動確認で「最新バージョンです。」「新しいバージョン v0.1.1 があります。」(ダイアログが重なって開く)「新しいバージョンを確認できませんでした。…」の 3 通りが出た。
    - AC #3: 自動確認でダイアログが出た時点で、配布元へのリクエストは一覧と latest.json の 2 件だけだった。dev ビルドで「更新する」を押すと devBuild の案内に切り替わり、ダウンロードは起きなかった。
    - AC #5: ダイアログに「システムがパスワードや管理者の承認を求めることがあります。」が出る。notes が無い状態でダイアログを出している。
    - AC #6: 新しいキーは en と ja の両方にあり、ja の表は Record<MessageKey, string> なので欠けると tsc が落ちる。モーダルは見出し域・本体域・操作域 (右寄せ、取り消し → 実行) で、開いたときの焦点は面 (activeElement が role=dialog) にある。Escape は上のダイアログだけを閉じ、焦点は「今すぐ確認」に戻った。新しい配色の組はリンク (accent) 対 モーダルの面 (surface) だけで、snz-tokens の値から WCAG 2.x の式で 7.52 / 6.63 / 5.41 / 5.88 (標準 明 / 標準 暗 / Solarized 明 / Solarized 暗)。ほかは既存の部品 (Checkbox・FailureNotice・Progress・ActionButton・Subtle) をそのまま使った。
    - AC #8: 起動から約 5 秒後にダイアログが出た (初回の描画は待たない)。最初の起動は app-config.json が無い状態で有効。設定でオフにすると app-config.json が `{"autoCheckUpdates": false}` だけになり、読み直して 8 秒待っても配布元へのリクエストは 0 件だった。
- 未確認 (AC #4 と #9 は未チェック): ダウンロードの進捗、承認後の失敗の表示、再起動中の表示は、まだ画面で見ていない。dev ビルドはダウンロードの前に devBuild の案内に分岐する。その分岐を一時的に外す確認と開発サーバの再起動は、このセッションの自動許可の判定で止められた。本番ビルドはネイティブの窓なので、このセッションでは操作できなかった。ユーザーの手元で、本番ビルドを偽の配布元に向けて確かめる (2026-10-10 に決定)。macOS の入れ替えと再起動、Windows の流れそのものは TASK-92.2 の e2e で確かめてある。この UI から通すのは TASK-92 の DoD #2 で行う。
- doc-17 (snz-design の適用記録) の更新は、統合済みリビジョンが要るので、マージの後に snz-design 側で行う。

- レビュー 1 回目 (Codex) の対応: [P2] 起動時の確認と手動確認が重なり、手動確認が新しいバージョンを見つけたあとで起動時の確認が失敗すると、Go 側の見つかった版が消え、ダイアログの「更新する」が必ず失敗した。失敗した確認は前の結果を消さないようにした (成功した確認は、「新しいものは無い」も含めて置き換える)。app_update_test.go で、修正前に落ち、修正後に通ることを確かめた。[P3] 新しい if の本体を波かっこで囲んだ。

- AC #9 (2026-10-10、ユーザーが手元で確認): HEAD を pnpm build:app でビルドした本番ビルドを、署名の通らない更新ファイルを配る偽の配布元に向けて起動し、「更新する」を押した。進捗の帯が伸び、その間はダイアログを閉じられず、そのあと「更新は行われませんでした。」が出た。「ダウンロードしたファイルを確かめて入れ替えています」は見えなかった。署名検証がダウンロードの直後に一瞬で失敗するためで、想定どおり。
- AC #4 は未チェックのまま Done にした (2026-10-10 ユーザーと決定)。進捗が見えることは上の確認で確かめた。macOS で新しいバージョンで起動し直すこと、Windows で再起動待ちの状態のまま残らないことは、TASK-92 の DoD #2 (更新機能を載せたリリースを 2 つ続けて公開した後の確認) で確かめる。本物の署名鍵を使う e2e のやり直しはしない。
- PR: https://github.com/serendipitynz/snz_studio/pull/93
<!-- SECTION:NOTES:END -->
