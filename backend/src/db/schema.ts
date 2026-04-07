import Database from "better-sqlite3";

const migrations = [
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
        source_type TEXT NOT NULL CHECK(source_type IN ('project', 'summary', 'document', 'memory')),
        source_id TEXT NOT NULL,
        label TEXT NOT NULL,
        excerpt TEXT NOT NULL DEFAULT '',
        score REAL NOT NULL DEFAULT 0,
        created_at TEXT NOT NULL
      );

      CREATE INDEX idx_assistant_refs_message ON assistant_message_references(assistant_message_id);
    `
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
    `
  },
  {
    id: "003_message_metrics",
    sql: `
      ALTER TABLE messages ADD COLUMN response_ms INTEGER;
      ALTER TABLE messages ADD COLUMN output_tokens INTEGER;
      ALTER TABLE messages ADD COLUMN tokens_per_second REAL;
    `
  },
  {
    id: "004_document_categories",
    sql: `
      ALTER TABLE documents ADD COLUMN category TEXT NOT NULL DEFAULT 'misc';
    `
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
    `
  }
];

export function applyMigrations(db: Database.Database) {
  db.exec("PRAGMA foreign_keys = ON;");
  db.exec("PRAGMA journal_mode = WAL;");

  db.exec(`
    CREATE TABLE IF NOT EXISTS schema_migrations (
      id TEXT PRIMARY KEY,
      applied_at TEXT NOT NULL
    );
  `);

  const applied = new Set(
    db
      .prepare("SELECT id FROM schema_migrations")
      .all()
      .map((row: unknown) => String((row as { id: string }).id))
  );

  for (const migration of migrations) {
    if (applied.has(migration.id)) {
      continue;
    }

    const tx = db.transaction(() => {
      db.exec(migration.sql);
      db.prepare("INSERT INTO schema_migrations (id, applied_at) VALUES (?, ?)").run(
        migration.id,
        new Date().toISOString()
      );
    });

    tx();
  }
}
