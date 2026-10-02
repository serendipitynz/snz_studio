---
id: TASK-86
title: 'ダッシュボード: 内蔵 embedding の状態が起動直後の「準備中」のまま更新されないのを直す'
status: To Do
assignee: []
created_date: '2026-10-02 00:39'
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
- [ ] #1 配布版を起動した直後にダッシュボードを開いたままにすると、サイドカーが ready になった時点で、内蔵 embedding の表示が「準備中」から「利用可能」に変わる
- [ ] #2 サイドカーが error になった場合は「利用できません」に変わる
- [ ] #3 状態の読み直しは downloading / starting のあいだだけ行い、接続先の確認 (getConfiguration) は繰り返さない
<!-- AC:END -->
