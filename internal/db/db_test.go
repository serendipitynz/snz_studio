package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	"snzstudio/internal/search"
)

func openTemp(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func mustExec(t *testing.T, d *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := d.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func TestMigrationsCreateExpectedTables(t *testing.T) {
	d := openTemp(t)

	want := []string{
		"schema_migrations", "projects", "documents", "document_chunks",
		"document_chunks_fts", "chats", "messages", "chat_summaries",
		"memories", "memories_fts", "assistant_message_references",
		"document_chunk_embeddings", "memory_embeddings",
	}
	for _, name := range want {
		var got string
		err := d.QueryRow(
			"SELECT name FROM sqlite_master WHERE name = ?", name,
		).Scan(&got)
		if err != nil {
			t.Errorf("expected object %q to exist: %v", name, err)
		}
	}

	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(migrations) {
		t.Errorf("schema_migrations count = %d, want %d", count, len(migrations))
	}
}

func TestApplyMigrationsIdempotent(t *testing.T) {
	d := openTemp(t)
	if err := ApplyMigrations(d); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(migrations) {
		t.Errorf("after re-apply count = %d, want %d", count, len(migrations))
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	d := openTemp(t)
	_, err := d.Exec(`
		INSERT INTO documents (id, project_id, type, title, created_at, updated_at)
		VALUES ('doc1', 'missing-project', 'text', 't', '2026-01-01', '2026-01-01')
	`)
	if err == nil {
		t.Fatal("expected foreign key violation, got nil")
	}
}

// TestFts5Bm25 validates the trickiest driver capability: that modernc's FTS5 is
// compiled in, MATCH works on tokenized Japanese text, and bm25() is callable in
// both the bare and weighted forms the retrieval layer uses.
func TestFts5Bm25(t *testing.T) {
	d := openTemp(t)

	mustExec(t, d, `INSERT INTO projects (id, title, created_at, updated_at)
		VALUES ('p1', 'proj', '2026-01-01', '2026-01-01')`)
	mustExec(t, d, `INSERT INTO documents (id, project_id, type, title, created_at, updated_at)
		VALUES ('d1', 'p1', 'text', '東京案内', '2026-01-01', '2026-01-01')`)
	mustExec(t, d, `INSERT INTO document_chunks (id, document_id, project_id, chunk_index, content, created_at)
		VALUES ('c1', 'd1', 'p1', 0, '東京タワーは美しい建物です', '2026-01-01')`)

	// Pre-tokenize exactly as the app does before FTS insertion.
	ftsContent := search.BuildSearchText("東京タワーは美しい建物です")
	ftsTitle := search.BuildSearchText("東京案内")
	mustExec(t, d, `INSERT INTO document_chunks_fts
		(project_id, document_id, chunk_id, title, note, tags, derived_text, content)
		VALUES ('p1', 'd1', 'c1', ?, '', '', '', ?)`, ftsTitle, ftsContent)

	matchQuery := search.ToFtsQuery("東京")
	if matchQuery == "" {
		t.Fatal("ToFtsQuery returned empty for 東京")
	}

	var chunkID string
	var score float64
	err := d.QueryRow(`
		SELECT chunk_id, bm25(document_chunks_fts) AS score
		FROM document_chunks_fts
		WHERE document_chunks_fts MATCH ?
	`, matchQuery).Scan(&chunkID, &score)
	if err != nil {
		t.Fatalf("bm25 MATCH query: %v", err)
	}
	if chunkID != "c1" {
		t.Errorf("matched chunk = %q, want c1", chunkID)
	}

	// Weighted bm25 form used by retrievalService.ts must also be accepted.
	var weighted float64
	err = d.QueryRow(`
		SELECT bm25(document_chunks_fts, 10.0, 2.0, 1.0, 1.0, 4.0) * -1 AS score
		FROM document_chunks_fts
		WHERE document_chunks_fts MATCH ?
	`, matchQuery).Scan(&weighted)
	if err != nil {
		t.Fatalf("weighted bm25 query: %v", err)
	}

	// A non-matching term must return no rows.
	var dummy string
	err = d.QueryRow(`
		SELECT chunk_id FROM document_chunks_fts WHERE document_chunks_fts MATCH ?
	`, search.ToFtsQuery("大阪")).Scan(&dummy)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows for 大阪, got %v (chunk=%q)", err, dummy)
	}
}
