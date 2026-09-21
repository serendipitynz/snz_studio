package db

import (
	"database/sql"
	"fmt"
	"time"
)

// migration mirrors the Migration type in schema.ts: either a raw multi-statement
// SQL string, or an up function for conditional logic.
type migration struct {
	id  string
	sql string
	up  func(tx *sql.Tx) error
}

// hasColumn reports whether table has the given column (PRAGMA table_info).
func hasColumn(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, rows.Err()
		}
	}
	return false, rows.Err()
}

// migrations is a faithful port of the migration list in backend/src/db/schema.ts.
var migrations = []migration{
	{
		id: "001_initial_schema",
		sql: `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				id TEXT PRIMARY KEY,
				applied_at TEXT NOT NULL
			);

			CREATE TABLE projects (
				id TEXT PRIMARY KEY,
				title TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				system_prompt TEXT NOT NULL DEFAULT '',
				sort_order INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);

			CREATE TABLE documents (
				id TEXT PRIMARY KEY,
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				type TEXT NOT NULL CHECK(type IN ('markdown', 'text', 'image')),
				title TEXT NOT NULL,
				note TEXT NOT NULL DEFAULT '',
				tags_json TEXT NOT NULL DEFAULT '[]',
				derived_text TEXT NOT NULL DEFAULT '',
				content_text TEXT NOT NULL DEFAULT '',
				file_path TEXT,
				mime_type TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);

			CREATE INDEX idx_documents_project_id ON documents(project_id);

			CREATE TABLE document_chunks (
				id TEXT PRIMARY KEY,
				document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				chunk_index INTEGER NOT NULL,
				content TEXT NOT NULL,
				created_at TEXT NOT NULL
			);

			CREATE INDEX idx_document_chunks_project ON document_chunks(project_id, document_id);

			CREATE VIRTUAL TABLE document_chunks_fts USING fts5(
				project_id UNINDEXED,
				document_id UNINDEXED,
				chunk_id UNINDEXED,
				title,
				note,
				tags,
				derived_text,
				content
			);

			CREATE TABLE chats (
				id TEXT PRIMARY KEY,
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				title TEXT NOT NULL,
				is_temporary INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);

			CREATE INDEX idx_chats_project_id ON chats(project_id);

			CREATE TABLE messages (
				id TEXT PRIMARY KEY,
				chat_id TEXT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
				role TEXT NOT NULL CHECK(role IN ('system', 'user', 'assistant')),
				content TEXT NOT NULL,
				created_at TEXT NOT NULL
			);

			CREATE INDEX idx_messages_chat_id ON messages(chat_id, created_at);

			CREATE TABLE chat_summaries (
				chat_id TEXT PRIMARY KEY REFERENCES chats(id) ON DELETE CASCADE,
				summary TEXT NOT NULL DEFAULT '',
				updated_at TEXT NOT NULL
			);

			CREATE TABLE memories (
				id TEXT PRIMARY KEY,
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				kind TEXT NOT NULL CHECK(kind IN ('semantic', 'procedural', 'episodic')),
				title TEXT NOT NULL,
				content TEXT NOT NULL,
				source_chat_id TEXT REFERENCES chats(id) ON DELETE SET NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);

			CREATE INDEX idx_memories_project_id ON memories(project_id);

			CREATE VIRTUAL TABLE memories_fts USING fts5(
				project_id UNINDEXED,
				memory_id UNINDEXED,
				kind,
				title,
				content
			);

			CREATE TABLE assistant_message_references (
				id TEXT PRIMARY KEY,
				assistant_message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
				source_type TEXT NOT NULL CHECK(source_type IN ('project', 'summary', 'document', 'memory', 'chat')),
				source_id TEXT NOT NULL,
				label TEXT NOT NULL,
				excerpt TEXT NOT NULL DEFAULT '',
				score REAL NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL
			);

			CREATE INDEX idx_assistant_refs_message ON assistant_message_references(assistant_message_id);
		`,
	},
	{
		id: "002_embedding_schema",
		sql: `
			CREATE TABLE IF NOT EXISTS document_chunk_embeddings (
				chunk_id TEXT PRIMARY KEY REFERENCES document_chunks(id) ON DELETE CASCADE,
				document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				embedding_json TEXT NOT NULL,
				model TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);

			CREATE INDEX IF NOT EXISTS idx_document_chunk_embeddings_project
				ON document_chunk_embeddings(project_id, document_id);

			CREATE TABLE IF NOT EXISTS memory_embeddings (
				memory_id TEXT PRIMARY KEY REFERENCES memories(id) ON DELETE CASCADE,
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				kind TEXT NOT NULL,
				embedding_json TEXT NOT NULL,
				model TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);

			CREATE INDEX IF NOT EXISTS idx_memory_embeddings_project
				ON memory_embeddings(project_id, kind);
		`,
	},
	{
		id: "003_message_metrics",
		sql: `
			ALTER TABLE messages ADD COLUMN response_ms INTEGER;
			ALTER TABLE messages ADD COLUMN output_tokens INTEGER;
			ALTER TABLE messages ADD COLUMN tokens_per_second REAL;
		`,
	},
	{
		id: "004_document_categories",
		sql: `
			ALTER TABLE documents ADD COLUMN category TEXT NOT NULL DEFAULT 'misc';
		`,
	},
	{
		id: "005_memory_metadata",
		sql: `
			ALTER TABLE memories ADD COLUMN source TEXT NOT NULL DEFAULT 'manual';
			ALTER TABLE memories ADD COLUMN locked INTEGER NOT NULL DEFAULT 0;
			UPDATE memories
			SET source = CASE WHEN source_chat_id IS NOT NULL THEN 'chat' ELSE 'manual' END,
			    locked = CASE WHEN source_chat_id IS NULL THEN 1 ELSE 0 END
			WHERE source = 'manual' AND locked = 0;
		`,
	},
	{
		id: "006_project_sort_order",
		up: func(tx *sql.Tx) error {
			ok, err := hasColumn(tx, "projects", "sort_order")
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
			_, err = tx.Exec(`
				ALTER TABLE projects ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;
				UPDATE projects
				SET sort_order = (
					SELECT COUNT(*)
					FROM projects p2
					WHERE p2.created_at < projects.created_at
					   OR (p2.created_at = projects.created_at AND p2.id < projects.id)
				);
			`)
			return err
		},
	},
	{
		id: "007_chat_reference_source",
		sql: `
			CREATE TABLE assistant_message_references_new (
				id TEXT PRIMARY KEY,
				assistant_message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
				source_type TEXT NOT NULL CHECK(source_type IN ('project', 'summary', 'document', 'memory', 'chat')),
				source_id TEXT NOT NULL,
				label TEXT NOT NULL,
				excerpt TEXT NOT NULL DEFAULT '',
				score REAL NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL
			);

			INSERT INTO assistant_message_references_new (
				id, assistant_message_id, source_type, source_id, label, excerpt, score, created_at
			)
			SELECT
				id, assistant_message_id, source_type, source_id, label, excerpt, score, created_at
			FROM assistant_message_references;

			DROP TABLE assistant_message_references;
			ALTER TABLE assistant_message_references_new RENAME TO assistant_message_references;
			CREATE INDEX idx_assistant_refs_message ON assistant_message_references(assistant_message_id);
		`,
	},
	{
		id: "008_temporary_chats",
		up: func(tx *sql.Tx) error {
			ok, err := hasColumn(tx, "chats", "is_temporary")
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
			_, err = tx.Exec("ALTER TABLE chats ADD COLUMN is_temporary INTEGER NOT NULL DEFAULT 0;")
			return err
		},
	},
	{
		id: "009_message_model_name",
		sql: `
			ALTER TABLE messages ADD COLUMN model_name TEXT;
		`,
	},
	{
		id: "010_multi_agent_chat",
		sql: `
			ALTER TABLE chats ADD COLUMN kind TEXT NOT NULL DEFAULT 'assistant';
			ALTER TABLE chats ADD COLUMN turn_rule TEXT NOT NULL DEFAULT 'round_robin';
			ALTER TABLE chats ADD COLUMN scene_prompt TEXT NOT NULL DEFAULT '';

			CREATE TABLE participants (
				id TEXT PRIMARY KEY,
				chat_id TEXT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
				display_name TEXT NOT NULL,
				role_prompt TEXT NOT NULL,
				base_url TEXT NOT NULL,
				model_name TEXT NOT NULL,
				sort_order INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL,
				deleted_at TEXT
			);

			CREATE INDEX idx_participants_chat_id ON participants(chat_id, sort_order);

			ALTER TABLE messages ADD COLUMN participant_id TEXT;
		`,
	},
	{
		id: "011_participant_receives_background",
		sql: `
			ALTER TABLE participants ADD COLUMN receives_background INTEGER NOT NULL DEFAULT 1;
		`,
	},
}

// ApplyMigrations applies all pending migrations in order, recording each in
// schema_migrations. Mirrors applyMigrations in schema.ts (idempotent).
func ApplyMigrations(database *sql.DB) error {
	if _, err := database.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL
		);
	`); err != nil {
		return err
	}

	applied := map[string]bool{}
	rows, err := database.Query("SELECT id FROM schema_migrations")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		applied[id] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, m := range migrations {
		if applied[m.id] {
			continue
		}
		if err := runMigration(database, m); err != nil {
			return fmt.Errorf("migration %s: %w", m.id, err)
		}
	}
	return nil
}

func runMigration(database *sql.DB, m migration) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if m.up != nil {
		if err := m.up(tx); err != nil {
			return err
		}
	} else if m.sql != "" {
		if _, err := tx.Exec(m.sql); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (id, applied_at) VALUES (?, ?)",
		m.id, time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return err
	}

	return tx.Commit()
}
