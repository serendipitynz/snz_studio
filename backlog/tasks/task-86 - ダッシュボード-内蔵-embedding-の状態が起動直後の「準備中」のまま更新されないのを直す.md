---
id: TASK-86
title: 'ダッシュボード: 内蔵 embedding の状態が起動直後の「準備中」のまま更新されないのを直す'
status: Done
assignee: []
created_date: '2026-10-02 00:39'
updated_date: '2026-10-04 02:34'
labels: []
dependencies: []
references:
  - frontend/src/components/ConnectionStatusCard.tsx
  - frontend/src/components/SettingsModal.tsx
priority: low
type: bug
ordinal: 86000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## 背景

TASK-67 の Windows 実機での確認 (2026-10-02) で見つかった。配布版を起動すると、内蔵 embedding はダッシュボードの接続カードで「準備中」のまま変わらない。設定画面を開くと「同梱の埋め込みモデルの準備が整いました」と表示され、サイドカーは ready になっている。

- 接続カード (`ConnectionStatusCard.tsx`) は `GET /api/embedding/status` を、表示したときと設定画面を閉じたときにしか読まない (`load()`)。起動直後はサイドカーがまだ downloading / starting なので、その時点の「準備中」が残り続ける。
- 設定画面 (`SettingsModal.tsx`) は、downloading / starting のあいだ 2 秒ごとに状態を読み直している。
- macOS でも同じことが起きる。`pnpm dev` では、リロードなどで表示し直すことがあり、目立たなかった。

## 方針

- 接続カードも、内蔵 embedding が downloading / starting のあいだは状態を読み直し、ready / error / disabled になったら止める。読み直すのは embedding の状態だけにする。`getConfiguration` はエンドポイントへの接続確認を伴い重いので、繰り返さない。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 配布版を起動した直後にダッシュボードを開いたままにすると、サイドカーが ready になった時点で、内蔵 embedding の表示が「準備中」から「利用可能」に変わる
- [x] #2 サイドカーが error になった場合は「利用できません」に変わる
- [x] #3 状態の読み直しは downloading / starting のあいだだけ行い、接続先の確認 (getConfiguration) は繰り返さない
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. frontend/src/components/embeddingStatusPoll.ts に pollEmbeddingStatus を追加する。間隔ごとに embedding の状態だけを読み直し、downloading / starting のあいだは続け、ready / error / disabled で止める。読み取りの失敗は同じ間隔で再試行する (rebuildPoll と同じ理由)。初回の読み取りは load() が済ませているので、最初の読み直しは 1 間隔後に行う。
2. ConnectionStatusCard: 内蔵モードで状態が downloading / starting のときだけ、effect で pollEmbeddingStatus を動かす。getConfiguration は呼ばない。
3. frontend/test/embeddingStatusPoll.test.ts で、ready / error で止まること・getConfiguration 相当を呼ばないこと・失敗後の再試行・停止を mock timers で確かめる。
4. pnpm check:client / test:client / build:client を通す。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 実装
- frontend/src/components/embeddingStatusPoll.ts に pollEmbeddingStatus を新設した。2 秒ごとに GET /api/embedding/status だけを読み直し、downloading / starting のあいだは続け、ready / error / disabled で止める。最初の読み直しは 1 間隔後に行う。load() が同じ時点の状態をすでに読んでいるため。
- ConnectionStatusCard は、内蔵モードで状態が downloading / starting のときだけ effect で上記を動かす。getConfiguration は呼ばない。
- 読み取りに失敗しても止めずに同じ間隔で再試行する。一度の失敗で止めると、ready になっても「準備中」が残るという、この不具合と同じ状態に戻るため (rebuildPoll と同じ判断)。SettingsModal の既存ポーリングは失敗すると止まるが、範囲外なので触れていない。
- rebuildPoll.ts との共通化は見送った。最初の読み取りを即時に行うか 1 間隔後に行うかが違い、20 行程度の重複を引数で吸収するより別関数のほうが読みやすいと判断した。

## 検証
- pnpm test:client: 9/9 pass。追加した 4 件 (ready で止まる / error で止まる / 失敗後に再試行して ready を反映する / 停止後は読まず、読み取り中の応答も捨てる) を含む。
- pnpm check:client と pnpm build:client: 成功。
- 実ブラウザで確認した: build 済みの frontend/dist を偽 API サーバー (scratchpad、最初の 8 秒は starting、その後 ready または error を返す) から配信し、ダッシュボードを開いたままにした。
  - ready の場合: 「準備中」から 10 秒以内に「利用可能」へ変わった。/api/configuration は 1 回、/api/embedding/status は 5 回読まれ、ready の後 6 秒間は増えなかった (AC #3)。
  - error の場合: 「準備中」から「利用できません」へ変わった。読み取り回数は同じで、error の後は止まった (AC #2)。
- AC #1 は配布版の実機起動を指定しているので、未チェックで残している。上の確認は本番ビルドのフロントエンドに偽の状態遷移を与えたもので、配布版 (Wails WebView と実サイドカー) では確かめていない。

## 補足
- サイドカーは起動に失敗すると error を出したあと、backoff 後に starting に戻り再試行する (internal/embed/manager.go superviseSidecar)。カードは方針どおり error で読み直しを止めるので、その後に回復しても、設定画面を閉じるか表示し直すまで「利用できません」のままになる。設定画面も同じ挙動。

## AC #1 の確認 (マージ後)
- ユーザーが macOS の pnpm dev で、SNZ_LLAMA_SERVER_BIN に起動を 15 秒遅らせるラッパーを指定して確認した。「準備中」から「利用可能」に変わった。
- 配布版の実機では再確認していない。Windows でもこのブランチの pnpm dev はすぐに ready になり、準備中にならなかった。カードの処理は OS に依存しないので、上の確認をもって AC #1 を満たしたとユーザーが判断した。
- error の確認には、/health には応答し embedding の確認リクエストだけを失敗させる偽サイドカーが要る。起動直後に終了するだけの偽物では、waitHealthy がプロセスの終了に気づかず 90 秒「starting」が続く。また error は再試行の合間 (1〜8 秒) だけ出るので、カードが読み逃すことがある。
<!-- SECTION:NOTES:END -->
