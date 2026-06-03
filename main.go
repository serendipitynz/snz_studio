// Command snzstudio is the Wails v2 desktop shell for SNZ Studio.
//
// The React SPA is built into frontend/dist and embedded here, then served by
// the Wails AssetServer (OS-native WebView). The application's HTTP API and
// static /files are NOT served through the AssetServer bridge — see app.go for
// why — but from a local 127.0.0.1 net/http server started in App.startup.
//
// Phase 1 (scaffold) only stands up the window, the local server, and the
// GetApiBase binding. The DB/repository/service/HTTP-API layers under
// internal/ are wired in later phases (see docs/HANDOFF.md).
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// assets holds the built SPA. The `all:` prefix keeps dotfiles (e.g. the
// committed .gitkeep placeholder) so the embed resolves even before a build.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "SNZ Studio",
		Width:     1280,
		Height:    832,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []any{
			app,
		},
	})
	if err != nil {
		log.Fatalf("wails: %v", err)
	}
}
