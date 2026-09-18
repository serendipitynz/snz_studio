---
id: TASK-7
title: '開発環境: Go ツールチェーンのバージョン差で wails dev が失敗する'
status: To Do
assignee: []
created_date: '2026-09-18 21:33'
labels: []
dependencies: []
references:
  - 'https://github.com/serendipitynz/snz_studio/pull/6#issuecomment-5736422678'
ordinal: 7000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
アクティブな Go ツールチェーンが go.mod の go ディレクティブより新しいと、wails dev / wails build が `internal error: package "math" without types was imported from "snzstudio/internal/vector"` で失敗する。Wails v2.12.0 が埋め込んでいる x/tools が新しい Go のエクスポートデータ形式を読めないことによる (2026-09-19 に go1.27.1 + go.mod の go 1.26.3 で発生)。

go.mod に toolchain ディレクティブが無いため、Go は "1.26.3 以上ならどれでもよい" と解釈して手元の最新 (1.27.1) を選ぶ。一方 CI (.github/workflows) は setup-go の go-version-file: go.mod で 1.26.3 を入れるので、現状この失敗はローカル開発でだけ起きる。回避策は `GOTOOLCHAIN=go1.26.3 wails dev`。

対処の候補は 2 つ:
(a) go.mod に toolchain ディレクティブを入れて使用バージョンを固定する。手元の Go を上げても壊れなくなるが、Wails を上げるまで Go も上げられなくなる。
(b) go1.27 のエクスポートデータを読める Wails v2 に上げる。Go の更新に追随できるが、Wails 側の変更点の確認が要る。

どちらを採るか (または両方) は着手時に判断する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 wails dev が、開発者の手元の Go ツールチェーンのバージョンに関わらず起動する (環境変数を毎回手で付けなくてよい)
- [ ] #2 CI の build ワークフローが引き続き通る
- [ ] #3 採った対処と、採らなかった候補を落とした理由が記録されている
- [ ] #4 README の開発手順が、必要な Go / Wails のバージョン要件と整合している
<!-- AC:END -->
