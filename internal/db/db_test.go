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
		"document_chunk_embeddings", "memory_embeddings", "participants",
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

// openUnmigratedTemp opens a database without applying migrations, so a test can
// build a pre-migration state and migrate it afterwards. It repeats Open's DSN
// deliberately: Open always migrates, and the point here is not to.
func openUnmigratedTemp(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "old.sqlite")
	database, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { database.Close() })
	return database
}

// applyThrough applies the migrations up to and including lastID, reproducing
// the schema of a database created by an earlier version of the app.
func applyThrough(t *testing.T, d *sql.DB, lastID string) {
	t.Helper()
	if _, err := d.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL
		);
	`); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	for _, m := range migrations {
		if err := runMigration(d, m); err != nil {
			t.Fatalf("migration %s: %v", m.id, err)
		}
		if m.id == lastID {
			return
		}
	}
	t.Fatalf("migration %q not found", lastID)
}

// TestMultiAgentMigrationOnExistingDB is the upgrade path: a database from
// before the multi-agent work, holding a chat and its messages, must take
// migration 010 and come out with the existing rows untouched and defaulted to
// the single-assistant shape.
func TestMultiAgentMigrationOnExistingDB(t *testing.T) {
	d := openUnmigratedTemp(t)
	applyThrough(t, d, "009_message_model_name")

	mustExec(t, d, `INSERT INTO projects (id, title, created_at, updated_at)
		VALUES ('p1', 'proj', '2026-01-01', '2026-01-01')`)
	mustExec(t, d, `INSERT INTO chats (id, project_id, title, is_temporary, created_at, updated_at)
		VALUES ('c1', 'p1', 'old chat', 0, '2026-01-01', '2026-01-01')`)
	mustExec(t, d, `INSERT INTO messages (id, chat_id, role, content, created_at)
		VALUES ('m1', 'c1', 'user', 'hello', '2026-01-01')`)

	if err := ApplyMigrations(d); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}

	var (
		title       string
		kind        string
		turnRule    string
		scenePrompt string
	)
	if err := d.QueryRow("SELECT title, kind, turn_rule, scene_prompt FROM chats WHERE id = 'c1'").
		Scan(&title, &kind, &turnRule, &scenePrompt); err != nil {
		t.Fatalf("read migrated chat: %v", err)
	}
	if title != "old chat" || kind != "assistant" || turnRule != "round_robin" || scenePrompt != "" {
		t.Errorf("migrated chat = (%q, %q, %q, %q), want the row unchanged and defaulted to assistant",
			title, kind, turnRule, scenePrompt)
	}

	var (
		content       string
		participantID sql.NullString
	)
	if err := d.QueryRow("SELECT content, participant_id FROM messages WHERE id = 'm1'").
		Scan(&content, &participantID); err != nil {
		t.Fatalf("read migrated message: %v", err)
	}
	if content != "hello" || participantID.Valid {
		t.Errorf("migrated message = (%q, %v), want the row unchanged with a NULL participant_id", content, participantID)
	}

	// participants must cascade with its chat, like the other chat-owned tables.
	mustExec(t, d, `INSERT INTO participants (id, chat_id, display_name, role_prompt, base_url, model_name, sort_order, created_at)
		VALUES ('pt1', 'c1', 'A', 'r', 'http://localhost:1234/v1', 'm', 0, '2026-01-01')`)
	mustExec(t, d, "DELETE FROM chats WHERE id = 'c1'")
	var remaining int
	if err := d.QueryRow("SELECT COUNT(*) FROM participants").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Errorf("participants not cascaded on chat delete: %d rows left", remaining)
	}
}

// TestReceivesProjectMaterialRename covers TASK-32 AC #2: migration 012 renames
// the column rather than replacing it, so a database that already ran 011 keeps
// both the participants that receive the project material and the ones that were
// deliberately cut off from it.
func TestReceivesProjectMaterialRename(t *testing.T) {
	d := openUnmigratedTemp(t)
	applyThrough(t, d, "011_participant_receives_background")

	mustExec(t, d, `INSERT INTO projects (id, title, created_at, updated_at)
		VALUES ('p1', 'proj', '2026-01-01', '2026-01-01')`)
	mustExec(t, d, `INSERT INTO chats (id, project_id, title, is_temporary, created_at, updated_at)
		VALUES ('c1', 'p1', 'trpg', 0, '2026-01-01', '2026-01-01')`)
	mustExec(t, d, `INSERT INTO participants (id, chat_id, display_name, role_prompt, base_url, model_name, sort_order, receives_background, created_at)
		VALUES ('gm', 'c1', 'GM', 'r', '', '', 0, 1, '2026-01-01')`)
	mustExec(t, d, `INSERT INTO participants (id, chat_id, display_name, role_prompt, base_url, model_name, sort_order, receives_background, created_at)
		VALUES ('pc', 'c1', '戦士', 'r', '', '', 1, 0, '2026-01-01')`)

	if err := ApplyMigrations(d); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}

	rows, err := d.Query("SELECT id, receives_project_material FROM participants ORDER BY sort_order")
	if err != nil {
		t.Fatalf("read renamed column: %v", err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var (
			id       string
			receives int
		)
		if err := rows.Scan(&id, &receives); err != nil {
			t.Fatal(err)
		}
		got[id] = receives
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got["gm"] != 1 || got["pc"] != 0 {
		t.Errorf("values after the rename = %v, want gm=1 and pc=0", got)
	}
}

// TestSharedProjectMaterialBackfill covers migration 013's two defaults on a
// database that already has rows: every document stays out of the common project
// material, and among memories only the ones saved from a multi-agent utterance
// go into it (design §4.4, TASK-31).
func TestSharedProjectMaterialBackfill(t *testing.T) {
	d := openUnmigratedTemp(t)
	applyThrough(t, d, "012_rename_receives_background")

	mustExec(t, d, `INSERT INTO projects (id, title, created_at, updated_at)
		VALUES ('p1', 'proj', '2026-01-01', '2026-01-01')`)
	mustExec(t, d, `INSERT INTO documents (id, project_id, type, title, created_at, updated_at)
		VALUES ('d1', 'p1', 'text', 'ルール', '2026-01-01', '2026-01-01')`)
	for _, m := range []struct{ id, source string }{
		{"m_spoken", "multi_agent"},
		{"m_manual", "manual"},
		{"m_chat", "chat"},
		{"m_organized", "organized"},
	} {
		mustExec(t, d, `INSERT INTO memories (id, project_id, kind, title, content, source, locked, created_at, updated_at)
			VALUES (?, 'p1', 'semantic', 't', 'c', ?, 0, '2026-01-01', '2026-01-01')`, m.id, m.source)
	}

	if err := ApplyMigrations(d); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}

	var documentShared int
	if err := d.QueryRow("SELECT shared_with_all FROM documents WHERE id = 'd1'").Scan(&documentShared); err != nil {
		t.Fatalf("read document flag: %v", err)
	}
	if documentShared != 0 {
		t.Errorf("an existing document must stay out of the common project material, got %d", documentShared)
	}

	rows, err := d.Query("SELECT id, shared_with_all FROM memories")
	if err != nil {
		t.Fatalf("read memory flags: %v", err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var (
			id     string
			shared int
		)
		if err := rows.Scan(&id, &shared); err != nil {
			t.Fatal(err)
		}
		got[id] = shared
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"m_spoken": 1, "m_manual": 0, "m_chat": 0, "m_organized": 0}
	for id, wantShared := range want {
		if got[id] != wantShared {
			t.Errorf("memory %s shared_with_all = %d, want %d", id, got[id], wantShared)
		}
	}
}
