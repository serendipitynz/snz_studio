package bootstrap

import (
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// migrationSourceEnv names the environment variable that points at an existing
// (old Node-backend) data directory to seed the data directory from on first
// launch.
//
// Why an explicit env var rather than auto-probing: the Node backend stored its
// data in ./data relative to the process working directory (see
// backend/src/config.ts), so there is no stable absolute "old default" path a
// packaged app could discover — its working directory is unpredictable. An
// explicit source is deterministic, leaves fresh installs untouched (a clean
// install has nothing to migrate, so the user simply does not set the var), and
// is fully testable. In dev the data dir already shares ./data with the Node
// backend, so this is effectively a production-only, first-launch convenience.
const migrationSourceEnv = "SNZ_MIGRATE_FROM"

// MaybeSeedDataDir performs a one-time migration of an existing data directory
// into the destination data dir before the database is opened. It does nothing
// unless SNZ_MIGRATE_FROM is set. The actual copy is idempotent and safe: it is
// a no-op when the destination already holds a database (so it never runs more
// than once and never clobbers live data).
func MaybeSeedDataDir(paths Paths) error {
	src := strings.TrimSpace(os.Getenv(migrationSourceEnv))
	if src == "" {
		return nil
	}
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("resolve %s=%q: %w", migrationSourceEnv, src, err)
	}

	migrated, err := seedDataDir(srcAbs, paths.DataDir)
	if err != nil {
		return err
	}
	if migrated {
		log.Printf("api: seeded data dir %s from %s (%s)", paths.DataDir, srcAbs, migrationSourceEnv)
	} else {
		log.Printf("api: %s set but skipped (destination already has data, or source has none)", migrationSourceEnv)
	}
	return nil
}

// seedDataDir copies an existing data directory's contents (the SQLite database,
// the uploads tree, and app-config.json) into destDataDir, returning whether a
// migration was performed.
//
// It is a deliberate no-op (returns false, nil) when destDataDir already holds
// an app.sqlite — meaning the app was already initialised, so we must not
// overwrite live data — or when srcDir has no app.sqlite to migrate from.
//
// The database is copied with `VACUUM INTO`, which writes a fresh, fully
// checkpointed single file from the source. This captures any rows still living
// in the source's -wal and is the robust alternative to a raw file copy, which
// would silently drop uncheckpointed WAL content. The source is opened read-only
// (mode=ro) and is never modified.
func seedDataDir(srcDir, destDataDir string) (bool, error) {
	srcDB := filepath.Join(srcDir, "app.sqlite")
	if !fileExists(srcDB) {
		return false, nil
	}
	destDB := filepath.Join(destDataDir, "app.sqlite")
	if fileExists(destDB) {
		return false, nil
	}

	if err := os.MkdirAll(destDataDir, 0o755); err != nil {
		return false, fmt.Errorf("create data dir %s: %w", destDataDir, err)
	}
	if err := vacuumInto(srcDB, destDB); err != nil {
		return false, fmt.Errorf("copy database %s -> %s: %w", srcDB, destDB, err)
	}

	// Uploads and the editable config are not WAL-sensitive, so plain file copies
	// are correct. Both are best-effort in the sense that a missing source is fine.
	if err := copyTree(filepath.Join(srcDir, "uploads"), filepath.Join(destDataDir, "uploads")); err != nil {
		return false, fmt.Errorf("copy uploads: %w", err)
	}
	if err := copyFileIfMissing(filepath.Join(srcDir, "app-config.json"), filepath.Join(destDataDir, "app-config.json")); err != nil {
		return false, fmt.Errorf("copy app-config.json: %w", err)
	}
	return true, nil
}

// vacuumInto produces a clean copy of the SQLite database at srcDB into destDB
// (which must not yet exist) using `VACUUM INTO`. The source is opened read-only
// so the migration never mutates it; VACUUM INTO is explicitly supported on
// read-only source databases and emits a checkpointed, single-file copy.
func vacuumInto(srcDB, destDB string) error {
	dsn := "file:" + srcDB + "?mode=ro&_pragma=busy_timeout(5000)"
	src, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := src.Ping(); err != nil {
		return err
	}

	// VACUUM INTO takes a string literal; quote the destination path and escape
	// any single quotes by doubling them. Filesystem paths contain no NUL.
	quoted := "'" + strings.ReplaceAll(destDB, "'", "''") + "'"
	if _, err := src.Exec("VACUUM INTO " + quoted); err != nil {
		return err
	}
	return nil
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// copyTree recursively copies the regular files under srcDir into destDir,
// recreating the directory structure. A missing srcDir is not an error (nothing
// to copy). Symlinks and other non-regular files are skipped.
func copyTree(srcDir, destDir string) error {
	info, err := os.Stat(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", srcDir)
	}

	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

// copyFile copies the regular file src to dest, creating dest's parent directory
// as needed and truncating any existing dest.
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// copyFileIfMissing copies src to dest only when src exists and dest does not.
func copyFileIfMissing(src, dest string) error {
	if !fileExists(src) {
		return nil
	}
	if fileExists(dest) {
		return nil
	}
	return copyFile(src, dest)
}
