package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"snzstudio/internal/bootstrap"
	"snzstudio/internal/config"
	"snzstudio/internal/db"
	"snzstudio/internal/embed"
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
	ctx      context.Context
	server   *http.Server
	db       *sql.DB
	apiBase  string
	apiToken string
	embedMgr *embed.Manager
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
		a.fatalStartup("Could not locate the data directory", err, "")
		return
	}
	if err := os.MkdirAll(paths.DataDir, 0o755); err != nil {
		a.fatalStartup("Could not create the data directory", err, paths.DataDir)
		return
	}
	// One-time, opt-in migration of an existing (old Node-backend) data dir into
	// this one. Driven by SNZ_MIGRATE_FROM; a no-op when unset or when this dir is
	// already populated. Must run before db.Open so the copied database — not a
	// freshly created empty one — is what gets opened and indexed.
	if err := bootstrap.MaybeSeedDataDir(paths); err != nil {
		a.fatalStartup("Could not migrate the existing data directory", err, paths.DataDir)
		return
	}
	if err := os.MkdirAll(paths.UploadDir, 0o755); err != nil {
		a.fatalStartup("Could not create the uploads directory", err, paths.DataDir)
		return
	}
	if err := os.MkdirAll(paths.ModelsDir, 0o755); err != nil {
		a.fatalStartup("Could not create the models directory", err, paths.DataDir)
		return
	}

	database, err := db.Open(paths.SQLitePath)
	if err != nil {
		a.fatalStartup("Could not open the database", err, paths.DataDir)
		return
	}
	a.db = database

	cfg := config.Load(paths.AppConfigPath)
	a.embedMgr = embed.NewManager(paths.ModelsDir)
	srv := httpapi.NewServer(database, cfg, paths.UploadDir, a.embedMgr)

	// Per-launch token that gates all /api and /files access (see httpapi.withAuth).
	// Generated fresh each startup so it never persists or leaks across runs; the
	// SPA fetches it via the GetApiToken binding and attaches it to every request.
	token, err := randomToken()
	if err != nil {
		a.fatalStartup("Could not generate the API security token", err, paths.DataDir)
		return
	}
	a.apiToken = token
	srv.SetAuthToken(token)

	ln, err := net.Listen("tcp", bootstrap.ListenAddr())
	if err != nil {
		a.fatalStartup(fmt.Sprintf("Could not bind the local API server to %s", bootstrap.ListenAddr()), err, paths.DataDir)
		return
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

	// In internal embedding mode, bring up the bundled sidecar (download +
	// llama-server) in the background. Non-blocking: the app serves immediately and
	// retrieval runs FTS-only until the sidecar reports ready, at which point the
	// server's ready callback overlays the endpoint and rebuilds embeddings once.
	if cfg.Get().EmbeddingMode == "internal" {
		a.embedMgr.EnsureInternalReady(ctx)
	}

	go func() {
		if err := a.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("api: serve: %v", err)
		}
	}()
}

// shutdown gracefully stops the local API server and closes the database when the
// app closes.
func (a *App) shutdown(_ context.Context) {
	if a.embedMgr != nil {
		a.embedMgr.Shutdown()
	}
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
// every API/file request with it. When this binding is unreachable — i.e. the
// SPA is loaded outside the Wails WebView, where window.go is absent — the SPA
// falls back to same-origin relative URLs (there is no Vite proxy); the loopback
// server then rejects unauthenticated calls, which is the intended outcome for
// that unsupported host.
func (a *App) GetApiBase() string {
	return a.apiBase
}

// GetApiToken is bound to the frontend. It returns the per-launch random token
// the SPA must attach to every API/file request — the X-SNZ-Studio-Token header
// for /api, or the `t` query param for <img>-loaded /files. Regenerated each
// startup, so it never persists to disk or leaks across runs.
func (a *App) GetApiToken() string {
	return a.apiToken
}

// randomToken returns a 256-bit cryptographically random token, hex-encoded.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// fatalStartup surfaces an unrecoverable startup error to the user through a
// native error dialog — naming the failure and the data directory so they can
// inspect or relocate it — before exiting, instead of aborting the process
// silently as log.Fatalf did. dataDir may be empty if resolution itself failed.
// A retry-without-restart flow is a follow-up (tracked in HANDOFF.md); this only
// removes the silent-crash behaviour. Callers must return after invoking it.
func (a *App) fatalStartup(title string, cause error, dataDir string) {
	log.Printf("startup fatal: %s: %v", title, cause)
	message := fmt.Sprintf("%v", cause)
	if dataDir != "" {
		message += "\n\nData directory:\n" + dataDir
	}
	if a.ctx != nil {
		_, _ = wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
			Type:    wailsruntime.ErrorDialog,
			Title:   "SNZ Studio — " + title,
			Message: message,
		})
	}
	os.Exit(1)
}
