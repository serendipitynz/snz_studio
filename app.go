package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

// App is the Wails application object. It owns the lifecycle of the local
// loopback HTTP server that serves the SNZ Studio API (/api) and uploaded
// files (/files).
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
	apiBase string
}

// NewApp constructs the App. The HTTP server is created lazily in startup once
// the Wails runtime context is available.
func NewApp() *App {
	return &App{}
}

// startup runs after the WebView is created. It binds the local API server to
// a port (fixed in dev so the Vite proxy can target it, ephemeral in prod) and
// serves it on a goroutine so wails.Run keeps driving the UI event loop.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

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

	a.server = &http.Server{
		Handler:           a.handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := a.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("api: serve: %v", err)
		}
	}()
}

// shutdown gracefully stops the local API server when the app closes.
func (a *App) shutdown(_ context.Context) {
	if a.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.server.Shutdown(ctx); err != nil {
		log.Printf("api: shutdown: %v", err)
	}
}

// GetApiBase is bound to the frontend. The SPA prefixes every API/file request
// with this value (empty in dev, where the Vite proxy handles routing).
func (a *App) GetApiBase() string {
	return a.apiBase
}

// handler builds the local API mux. Phase 1 exposes only a health check; the 22
// routes ported from backend/src/index.ts and the /files static handler are
// mounted here in Phase 6.
func (a *App) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}
