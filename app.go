package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"snzstudio/internal/config"
	"snzstudio/internal/db"
	"snzstudio/internal/httpapi"
)

// App is the Wails application object. It owns the lifecycle of the local
// loopback HTTP server that serves the SNZ Studio API (/api) and uploaded
// files (/files), plus the SQLite handle that backs them.
//
// Why a separate net/http server instead of mounting handlers on the Wails
// AssetServer: chat/review responses stream over SSE, which depends on
// http.Flusher delivering each frame immediately. The AssetServer reaches the
// WebView through a native custom-scheme bridge whose incremental flushing is
// not guaranteed, so the API is served from a real loopback connection where
// Flusher behaves correctly. See internal/httpapi/sse.go and the migration plan.
type App struct {
	ctx     context.Context
	server  *http.Server
	db      *sql.DB
	apiBase string
}

// NewApp constructs the App. The HTTP server is created lazily in startup once
// the Wails runtime context is available.
func NewApp() *App {
	return &App{}
}

// startup runs after the WebView is created. It resolves the data directory,
// opens the SQLite database, wires the repository/service graph behind the HTTP
// API, binds the local API server to a port (fixed in dev so the Vite proxy can
// target it, ephemeral in prod), and serves it on a goroutine so wails.Run keeps
// driving the UI event loop.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	paths, err := resolveDataPaths()
	if err != nil {
		log.Fatalf("api: resolve data paths: %v", err)
	}
	if err := os.MkdirAll(paths.dataDir, 0o755); err != nil {
		log.Fatalf("api: create data dir %s: %v", paths.dataDir, err)
	}
	if err := os.MkdirAll(paths.uploadDir, 0o755); err != nil {
		log.Fatalf("api: create upload dir %s: %v", paths.uploadDir, err)
	}

	database, err := db.Open(paths.sqlitePath)
	if err != nil {
		log.Fatalf("api: open database %s: %v", paths.sqlitePath, err)
	}
	a.db = database

	cfg := config.Load(paths.appConfigPath)
	srv := httpapi.NewServer(database, cfg, paths.uploadDir)

	ln, err := net.Listen("tcp", apiListenAddr())
	if err != nil {
		log.Fatalf("api: listen on %s: %v", apiListenAddr(), err)
	}

	// In dev the SPA is served by the Vite dev server and reaches the API
	// through Vite's proxy, so the frontend uses relative URLs (empty base).
	// In prod the SPA is served from embedded assets under the wails:// origin
	// and must call the loopback server by its absolute origin.
	if isDev {
		a.apiBase = ""
	} else {
		a.apiBase = fmt.Sprintf("http://%s", ln.Addr().String())
	}
	log.Printf("api: listening on http://%s (apiBase=%q, dev=%v)", ln.Addr(), a.apiBase, isDev)
	log.Printf("api: data dir %s", paths.dataDir)

	a.server = &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Rebuild the Go-tokenized FTS indexes before accepting traffic so no request
	// can observe a half-rebuilt index (embedding rebuild, if enabled, continues
	// in the background). Mirrors the Node backend's pre-listen bootstrap.
	srv.RunStartupTasks()

	go func() {
		if err := a.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("api: serve: %v", err)
		}
	}()
}

// shutdown gracefully stops the local API server and closes the database when the
// app closes.
func (a *App) shutdown(_ context.Context) {
	if a.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.server.Shutdown(ctx); err != nil {
			log.Printf("api: shutdown: %v", err)
		}
	}
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			log.Printf("api: close database: %v", err)
		}
	}
}

// GetApiBase is bound to the frontend. The SPA prefixes every API/file request
// with this value (empty in dev, where the Vite proxy handles routing).
func (a *App) GetApiBase() string {
	return a.apiBase
}
