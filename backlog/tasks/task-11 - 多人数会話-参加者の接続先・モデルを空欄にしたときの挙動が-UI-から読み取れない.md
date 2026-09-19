---
id: TASK-11
title: '多人数会話: 参加者の接続先・モデルを空欄にしたときの挙動が UI から読み取れない'
status: To Do
assignee: []
created_date: '2026-09-19 21:55'
labels: []
milestone: m-1
dependencies: []
references:
  - docs/multi-agent-chat-design.md
  - frontend/src/components/ParticipantPanel.tsx
  - frontend/src/i18n/index.tsx
  - internal/service/llmclient.go
ordinal: 11000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
編成パネル (設計書 §6) の参加者は、接続先とモデルを空欄にするとワークスペース設定の値に
フォールバックする。この挙動が画面から読み取れない。

## 1. 説明が読めない側にだけ置かれている

接続先の入力欄のプレースホルダーには `http://127.0.0.1:1234/v1（空欄ならワークスペースの接続先）`
(`i18n/index.tsx` の `participants.baseUrlPlaceholder`) と書かれているが、編成パネルの幅では
`http://127.0.0.1:1234/v1（空欄な…` までしか表示されず、肝心の説明が切れて読めない (実機で確認)。

一方モデルの入力欄のプレースホルダーは `モデル ID` (`participants.modelPlaceholder`) だけで、
モデルも同じくワークスペース設定にフォールバックすることがどこにも書かれていない。
つまり、読めない側にだけ説明があり、読める側には無い。

そもそもプレースホルダーは入力を始めると消えるため、入力済みの参加者を後から見直すときには
どちらにせよ出てこない。入力欄の下の補足行など、幅で切れず入力後も残る場所へ移すのが素直。

## 2. 接続先とモデルが独立にフォールバックする

`resolveTarget` ([llmclient.go](../internal/service/llmclient.go)) はフィールドごとに独立して
ワークスペース設定へ落ちる:

    baseURL, modelName = defaultBaseURL, defaultModel
    if target.Model != ""   { modelName = target.Model }
    if target.BaseURL != "" { baseURL = target.BaseURL }

このため「接続先だけ入力し、モデルは空欄」の参加者は、〈その参加者の接続先〉+
〈ワークスペースのモデル〉という組み合わせで実行される。そのモデルが当の接続先に存在する
保証はない。プリセット (TASK-5) から作った参加者は接続先もモデルも空で始まるので、
接続先だけ埋めた時点でこの状態を通過する。

この組み合わせで実際にターンを実行したときに何が起きるか (実行前にモデル名の検証があるか、
あるとして何が表示されるか) は未確認。まずそこを確認し、利用者が状況を理解できない形で
失敗するようなら対処する。1 の表示改善だけで足りるのか、フォールバックの単位自体を
見直すべきなのかは、その確認結果で決まる。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 参加者の接続先とモデルについて、空欄ならワークスペース設定が使われることが、幅で切れず、入力後も消えない形で画面から読み取れる
- [ ] #2 「接続先だけ入力・モデルは空欄」の参加者でターンを実行したときの挙動が実機で確認され、確認結果がタスクに記録されている
- [ ] #3 その確認で利用者が理解できない形の失敗が起きる場合、対処されている (対処不要と判断した場合はその根拠が記録されている)
<!-- AC:END -->
