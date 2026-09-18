module snzstudio

go 1.26.3

// Wails CLI がバインド生成で使う x/tools は CLI バイナリ側に埋め込まれており、
// go.mod の依存グラフからは差し替えられない。そのため CLI より新しい Go で
// ビルドするとエクスポートデータを読めず
// `internal error: package "math" without types` でバインド生成が落ちる。
//
// このディレクティブが効かせられるのは下限だけで、上限ではない: 手元の Go が
// これより新しければ GOTOOLCHAIN=auto はそちらを使う (実測確認済み)。上限を
// 効かせるのは GOTOOLCHAIN の厳密指定だけで、それを行うのが scripts/wails.mjs
// (pnpm dev / build:app と CI のビルドが通るランチャ)。この行はその pin の
// 唯一の出所でもある — ランチャがここを読むので、下限と上限は定義上ずれない。
// 下限としての役割は、素の go コマンド (go test / go build / CI の setup-go が
// 入れる floor 版) をこの版まで引き上げ、CLI が読めない古い Go を弾くこと。
//
// 対の制約: この値は下の wails の require と
// .github/workflows/build.yml の go install (CLI 版) と揃えて上げること。
toolchain go1.27.1

require (
	github.com/google/uuid v1.6.0
	github.com/wailsapp/wails/v2 v2.16.0
	golang.org/x/text v0.39.0
	modernc.org/sqlite v1.49.1
)

require (
	git.sr.ht/~jackmordaunt/go-toast/v2 v2.0.3 // indirect
	github.com/bep/debounce v1.2.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/jchv/go-winloader v0.0.0-20210711035445-715c2860da7e // indirect
	github.com/labstack/echo/v4 v4.13.3 // indirect
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/leaanthony/go-ansi-parser v1.6.1 // indirect
	github.com/leaanthony/gosod v1.0.4 // indirect
	github.com/leaanthony/slicer v1.6.0 // indirect
	github.com/leaanthony/u v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/samber/lo v1.49.1 // indirect
	github.com/tkrajina/go-reflector v0.5.8 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/wailsapp/go-webview2 v1.0.22 // indirect
	github.com/wailsapp/mimetype v1.4.1 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	modernc.org/libc v1.72.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
