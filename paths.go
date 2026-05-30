package main

import (
	"os"
	"path/filepath"
	"strings"
)

// dataPaths holds the resolved filesystem locations the app reads and writes.
type dataPaths struct {
	dataDir       string
	sqlitePath    string
	uploadDir     string
	appConfigPath string
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
	if isDev {
		return filepath.Abs("data")
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "snz-studio"), nil
}

// resolveDataPaths resolves all data locations, honoring the same SQLITE_PATH and
// UPLOAD_DIR overrides as config.ts and defaulting them under the data dir.
func resolveDataPaths() (dataPaths, error) {
	dataDir, err := resolveDataDir()
	if err != nil {
		return dataPaths{}, err
	}

	sqlitePath := filepath.Join(dataDir, "app.sqlite")
	if v := strings.TrimSpace(os.Getenv("SQLITE_PATH")); v != "" {
		if sqlitePath, err = filepath.Abs(v); err != nil {
			return dataPaths{}, err
		}
	}

	uploadDir := filepath.Join(dataDir, "uploads")
	if v := strings.TrimSpace(os.Getenv("UPLOAD_DIR")); v != "" {
		if uploadDir, err = filepath.Abs(v); err != nil {
			return dataPaths{}, err
		}
	}

	return dataPaths{
		dataDir:       dataDir,
		sqlitePath:    sqlitePath,
		uploadDir:     uploadDir,
		appConfigPath: filepath.Join(dataDir, "app-config.json"),
	}, nil
}
