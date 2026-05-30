// Package bootstrap resolves the process-level configuration the desktop shell
// needs before the HTTP API starts: the filesystem locations for app data, the
// one-time data migration, and the dev/prod environment split. These are
// infrastructure concerns kept out of internal/config (which is scoped to the
// service layer's editable settings) and out of the root main package, so the
// repository root holds only main.go and app.go.
package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
)

// Paths holds the resolved filesystem locations the app reads and writes.
type Paths struct {
	DataDir       string
	SQLitePath    string
	UploadDir     string
	AppConfigPath string
}

// resolveDataDir decides where app data lives. The DATA_DIR override mirrors
// backend/src/config.ts; otherwise the default is ./data in dev (cwd-relative, so
// `wails dev` reuses the existing Node backend's data) and the per-user config
// directory (~/Library/Application Support/snz-studio, %AppData%\snz-studio) in
// production, per the migration plan.
func resolveDataDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("DATA_DIR")); v != "" {
		return filepath.Abs(v)
	}
	if IsDev {
		return filepath.Abs("data")
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "snz-studio"), nil
}

// ResolveDataPaths resolves all data locations, honoring the same SQLITE_PATH and
// UPLOAD_DIR overrides as config.ts and defaulting them under the data dir.
func ResolveDataPaths() (Paths, error) {
	dataDir, err := resolveDataDir()
	if err != nil {
		return Paths{}, err
	}

	sqlitePath := filepath.Join(dataDir, "app.sqlite")
	if v := strings.TrimSpace(os.Getenv("SQLITE_PATH")); v != "" {
		if sqlitePath, err = filepath.Abs(v); err != nil {
			return Paths{}, err
		}
	}

	uploadDir := filepath.Join(dataDir, "uploads")
	if v := strings.TrimSpace(os.Getenv("UPLOAD_DIR")); v != "" {
		if uploadDir, err = filepath.Abs(v); err != nil {
			return Paths{}, err
		}
	}

	return Paths{
		DataDir:       dataDir,
		SQLitePath:    sqlitePath,
		UploadDir:     uploadDir,
		AppConfigPath: filepath.Join(dataDir, "app-config.json"),
	}, nil
}
