// Package repository ports backend/src/repositories/*.ts to Go. Each repository
// wraps the shared *sql.DB and mirrors the SQL and behaviour of its TypeScript
// counterpart. Text written to the *_fts tables is pre-tokenized through
// search.BuildSearchText, exactly as the Node backend did, so the FTS index
// stays compatible with search.ToFtsQuery at query time.
//
// Concurrency note: db.Open pins the pool to a single connection to mirror
// better-sqlite3. A *sql.Tx therefore holds the only connection for its
// lifetime, so every statement issued while a transaction is open must go
// through the *sql.Tx (helpers take a dbtx for this), and any read whose results
// feed a transaction is fully drained and closed before Begin.
package repository

import "database/sql"

// dbtx is satisfied by both *sql.DB and *sql.Tx, letting helper queries run
// either standalone or inside a transaction.
type dbtx interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// scanner is satisfied by both *sql.Row and *sql.Rows, so a single scan helper
// can serve QueryRow and Query iteration.
type scanner interface {
	Scan(dest ...any) error
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ptrArg unwraps an optional value for SQL binding: nil pointer -> SQL NULL,
// otherwise the dereferenced value. Mirrors the TS `?? null` binding pattern
// without relying on driver-specific pointer handling.
func ptrArg[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

func strPtr(n sql.NullString) *string {
	if n.Valid {
		v := n.String
		return &v
	}
	return nil
}

func int64Ptr(n sql.NullInt64) *int64 {
	if n.Valid {
		v := n.Int64
		return &v
	}
	return nil
}

func float64Ptr(n sql.NullFloat64) *float64 {
	if n.Valid {
		v := n.Float64
		return &v
	}
	return nil
}

// inPlaceholders builds a "?, ?, ..." list of length n (n >= 1) for IN clauses.
func inPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, 0, n*3-2)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',', ' ')
		}
		b = append(b, '?')
	}
	return string(b)
}
