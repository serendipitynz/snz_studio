package main

import (
	"context"
	"errors"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"snzstudio/internal/bootstrap"
	"snzstudio/internal/updater"
)

// appVersion is wails.json's info.productVersion, stamped in by scripts/wails.mjs
// through -ldflags -X. It is empty in a build that bypassed the launcher, and such a
// build reports every update check as failed rather than guessing its version.
var appVersion string

// Results of CheckForUpdate and InstallUpdate. The UI branches on these and on
// updater's Reason* codes, never on error text.
const (
	updateUpToDate   = "upToDate"
	updateAvailable  = "available"
	updateFailed     = "failed"
	updateRestarting = "restarting"
	// updateManual: the installed copy cannot be replaced in place (Reason says
	// why); the UI sends the user to ReleaseURL instead.
	updateManual = "manual"

	reasonDevBuild = "devBuild"

	updateProgressEvent = "update:progress"
	updateCheckTimeout  = 20 * time.Second
)

type UpdateCheck struct {
	Status         string `json:"status"`
	CurrentVersion string `json:"currentVersion"`
	Version        string `json:"version,omitempty"`
	// ReleaseURL is the new version's release page when one is available, which
	// carries the notes latest.json does not; otherwise the list of releases.
	ReleaseURL string `json:"releaseUrl"`
}

type UpdateResult struct {
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	ReleaseURL string `json:"releaseUrl"`
}

type UpdateProgress struct {
	Downloaded int64 `json:"downloaded"`
	Total      int64 `json:"total"` // 0 when unknown
}

// updateState is the App's part of updating: the release the last check found, a
// guard against a second install starting, and the relaunch shutdown runs.
type updateState struct {
	mu         sync.Mutex
	found      *updater.Release
	installing bool
	relaunch   func() error
}

// GetVersion is bound to the frontend: the running version, "" when unstamped.
func (a *App) GetVersion() string {
	return appVersion
}

// GetAutoCheckUpdates is bound to the frontend: whether to check for a newer version
// at startup.
func (a *App) GetAutoCheckUpdates() bool {
	return a.cfg != nil && a.cfg.AutoCheckUpdates()
}

// SetAutoCheckUpdates is bound to the frontend and saves the choice to app-config.json.
func (a *App) SetAutoCheckUpdates(enabled bool) error {
	if a.cfg == nil {
		return errors.New("the configuration is not loaded")
	}
	if err := a.cfg.SetAutoCheckUpdates(enabled); err != nil {
		log.Printf("update: save the startup check choice: %v", err)
		return err
	}
	return nil
}

// CheckForUpdate is bound to the frontend. Every failure (offline, rate-limited,
// no release yet, unstamped build) comes back as status "failed" rather than an
// error, so that the automatic check at startup can drop it without a word.
func (a *App) CheckForUpdate() UpdateCheck {
	u, err := updater.New(appVersion)
	if err != nil {
		log.Printf("update: %v", err)
		return UpdateCheck{Status: updateFailed, CurrentVersion: appVersion}
	}
	result := UpdateCheck{Status: updateFailed, CurrentVersion: appVersion, ReleaseURL: u.ReleasesPage()}
	ctx, cancel := context.WithTimeout(a.ctx, updateCheckTimeout)
	defer cancel()
	release, err := u.Check(ctx)
	a.update.mu.Lock()
	a.update.found = release
	a.update.mu.Unlock()
	switch {
	case err != nil:
		log.Printf("update: check: %v", err)
	case release == nil:
		result.Status = updateUpToDate
	default:
		result.Status = updateAvailable
		result.Version = release.Version
		result.ReleaseURL = u.ReleasePage(release.Tag)
	}
	return result
}

// InstallUpdate is bound to the frontend and runs only after the user approved
// version, which must be the one the last CheckForUpdate reported. It downloads and
// verifies the update file (reporting progress as update:progress events), puts it
// in place, and quits; shutdown then starts the new version. Status "restarting"
// means the app is about to quit. Any failure leaves the installed app as it was
// and returns "failed" — the reason is logged, not returned, because the user's
// next step is the same whichever step failed.
func (a *App) InstallUpdate(version string) UpdateResult {
	u, err := updater.New(appVersion)
	if err != nil {
		log.Printf("update: %v", err)
		return UpdateResult{Status: updateFailed}
	}
	failed := UpdateResult{Status: updateFailed, ReleaseURL: u.ReleasesPage()}

	a.update.mu.Lock()
	release := a.update.found
	busy := a.update.installing
	if release == nil || release.Version != version || busy {
		a.update.mu.Unlock()
		log.Printf("update: install %q refused (found %v, busy %v)", version, release != nil, busy)
		return failed
	}
	a.update.installing = true
	a.update.mu.Unlock()
	defer func() {
		a.update.mu.Lock()
		a.update.installing = false
		a.update.mu.Unlock()
	}()
	failed.ReleaseURL = u.ReleasePage(release.Tag)

	if bootstrap.IsDev {
		return UpdateResult{Status: updateManual, Reason: reasonDevBuild, ReleaseURL: failed.ReleaseURL}
	}
	target, err := updater.LocateTarget()
	var blocked *updater.BlockedError
	if errors.As(err, &blocked) {
		log.Printf("update: %v", err)
		return UpdateResult{Status: updateManual, Reason: blocked.Reason, ReleaseURL: failed.ReleaseURL}
	}
	if err != nil {
		log.Printf("update: locate the installed app: %v", err)
		return failed
	}

	dir, err := os.MkdirTemp("", "snz-studio-update-")
	if err != nil {
		log.Printf("update: %v", err)
		return failed
	}
	file, err := u.Download(a.ctx, release, dir, func(done, total int64) {
		wailsruntime.EventsEmit(a.ctx, updateProgressEvent, UpdateProgress{Downloaded: done, Total: total})
	})
	if err == nil {
		err = target.Prepare(file)
	}
	if err != nil {
		log.Printf("update: %v", err)
		os.RemoveAll(dir)
		return failed
	}

	// The bundled llama-server runs from inside the installed app, so it is stopped
	// before its files are replaced rather than left for shutdown to stop.
	a.stopEmbedding()
	if err := target.Replace(); err != nil {
		log.Printf("update: %v", err)
		os.RemoveAll(dir)
		a.restartEmbedding()
		return failed
	}

	// On Windows the installer still has to run from dir after this process exits.
	if runtime.GOOS == "darwin" {
		os.RemoveAll(dir)
	}

	a.update.mu.Lock()
	a.update.relaunch = target.Relaunch
	a.update.mu.Unlock()
	// The quit runs after this binding has returned, so the UI receives the result
	// and can say the app is restarting before the window closes.
	go func() {
		time.Sleep(300 * time.Millisecond)
		wailsruntime.Quit(a.ctx)
	}()
	return UpdateResult{Status: updateRestarting, ReleaseURL: failed.ReleaseURL}
}

// relaunchAfterUpdate is shutdown's last step: it starts the new version (macOS) or
// the installer (Windows) once the database is closed and the sidecar stopped.
func (a *App) relaunchAfterUpdate() {
	a.update.mu.Lock()
	relaunch := a.update.relaunch
	a.update.mu.Unlock()
	if relaunch == nil {
		return
	}
	if err := relaunch(); err != nil {
		log.Printf("update: relaunch: %v", err)
	}
}
