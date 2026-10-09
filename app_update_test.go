package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"snzstudio/internal/updater"
	"snzstudio/internal/updatesig"
)

// The startup check and a manual one can overlap. Whichever finishes last must not
// take away an update a successful check found and the dialog is offering.
func TestFailedCheckKeepsTheFoundUpdate(t *testing.T) {
	platform := updater.Platform()
	if platform == "" {
		t.Skip("no update files are published for this platform")
	}
	var answer atomic.Value
	answer.Store("available")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := answer.Load().(string)
		if mode == "down" {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/releases":
			tags := []map[string]any{{"tag_name": "v0.1.0"}}
			if mode == "available" {
				tags = append(tags, map[string]any{"tag_name": "v0.1.1"})
			}
			_ = json.NewEncoder(w).Encode(tags)
		case "/releases/download/v0.1.1/latest.json":
			_ = json.NewEncoder(w).Encode(updatesig.Manifest{
				Version:   "0.1.1",
				Platforms: map[string]updatesig.Asset{platform: {URL: "http://unused.invalid/f", Signature: "AAAA"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("SNZ_UPDATE_BASE_URL", srv.URL)
	previous := appVersion
	appVersion = "0.1.0"
	t.Cleanup(func() { appVersion = previous })

	a := &App{ctx: context.Background()}
	found := func() *updater.Release {
		a.update.mu.Lock()
		defer a.update.mu.Unlock()
		return a.update.found
	}

	if got := a.CheckForUpdate(); got.Status != updateAvailable || got.Version != "0.1.1" {
		t.Fatalf("first check = %+v, want 0.1.1 available", got)
	}
	answer.Store("down")
	if got := a.CheckForUpdate(); got.Status != updateFailed {
		t.Fatalf("check against a failing source = %+v, want failed", got)
	}
	if r := found(); r == nil || r.Version != "0.1.1" {
		t.Fatalf("after a failed check the found update is %+v, want 0.1.1 kept", r)
	}

	// A check that succeeds and finds nothing newer does replace it.
	answer.Store("upToDate")
	if got := a.CheckForUpdate(); got.Status != updateUpToDate {
		t.Fatalf("check = %+v, want upToDate", got)
	}
	if r := found(); r != nil {
		t.Fatalf("after an up-to-date check the found update is %+v, want none", r)
	}
}
