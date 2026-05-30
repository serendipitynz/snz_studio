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

	"snzstudio/internal/bootstrap"
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
// API, binds the local API server to a port (fixed 127.0.0.1:8787 in dev,
// ephemeral in prod), and serves it on a goroutine so wails.Run keeps driving
// the UI event loop.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	paths, err := bootstrap.ResolveDataPaths()
	if err != nil {
		log.Fatalf("api: resolve data paths: %v", err)
	}
	if err := os.MkdirAll(paths.DataDir, 0o755); err != nil {
		log.Fatalf("api: create data dir %s: %v", paths.DataDir, err)
	}
	// One-time, opt-in migration of an existing (old Node-backend) data dir into
	// this one. Driven by SNZ_MIGRATE_FROM; a no-op when unset or when this dir is
	// already populated. Must run before db.Open so the copied database — not a
	// freshly created empty one — is what gets opened and indexed.
	if err := bootstrap.MaybeSeedDataDir(paths); err != nil {
		log.Fatalf("api: migrate data dir: %v", err)
	}
	if err := os.MkdirAll(paths.UploadDir, 0o755); err != nil {
		log.Fatalf("api: create upload dir %s: %v", paths.UploadDir, err)
	}

	database, err := db.Open(paths.SQLitePath)
	if err != nil {
		log.Fatalf("api: open database %s: %v", paths.SQLitePath, err)
	}
	a.db = database

	cfg := config.Load(paths.AppConfigPath)
	srv := httpapi.NewServer(database, cfg, paths.UploadDir)

	ln, err := net.Listen("tcp", bootstrap.ListenAddr())
	if err != nil {
		log.Fatalf("api: listen on %s: %v", bootstrap.ListenAddr(), err)
	}

	// Both dev and prod use the absolute loopback origin so the SPA can reach
	// this server directly. In dev the WebView loads the SPA from the Wails dev
	// asset server (origin wails.localhost), whose external asset handler only
	// proxies GET to Vite and returns 405 for non-GET (POST/PATCH/DELETE). A
	// relative URL would therefore never reach :8787 for mutations, so the SPA
	// must call the absolute origin and bypass the Wails dev server entirely;
	// the cross-origin call is handled by httpapi's withCORS (Origin reflection).
	// In prod the SPA is served from embedded assets under the wails:// origin
	// and likewise needs the absolute loopback origin.
	a.apiBase = fmt.Sprintf("http://%s", ln.Addr().String())
	log.Printf("api: listening on http://%s (apiBase=%q, dev=%v)", ln.Addr(), a.apiBase, bootstrap.IsDev)
	log.Printf("api: data dir %s", paths.DataDir)

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

// GetApiBase is bound to the frontend. It always returns the absolute loopback
// origin (in both dev and prod). The SPA awaits it once at startup and prefixes
// every API/file request with it. When this binding is unreachable — e.g. the
// Vite dev server opened directly in a browser, where window.go is absent — the
// SPA falls back to relative URLs and the Vite proxy handles /api and /files.
func (a *App) GetApiBase() string {
	return a.apiBase
}
