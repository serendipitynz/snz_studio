// Package db ports backend/src/db (schema + migrations + connection) to Go,
// using the pure-Go modernc.org/sqlite driver so that cross-platform desktop
// builds need no C SQLite toolchain. FTS5 and bm25() are compiled into modernc.
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open opens (creating if needed) the SQLite database at path, sets the same
// pragmas the Node backend used (foreign_keys ON, WAL journal), pins a single
// connection to mirror better-sqlite3's single-connection model, and applies
// all pending migrations.
func Open(path string) (*sql.DB, error) {
	dsn := "file:" + path +
		"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// better-sqlite3 is single-connection and synchronous; pinning to one
	// connection avoids WAL writer contention and keeps PRAGMA state stable.
	database.SetMaxOpenConns(1)

	if err := database.Ping(); err != nil {
		database.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if err := ApplyMigrations(database); err != nil {
		database.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	return database, nil
}
