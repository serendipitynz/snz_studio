import Database from "better-sqlite3";
import { Memory, MemoryKind, MemorySource } from "../lib/types.js";
import { createId, nowIso } from "../lib/utils.js";
import { buildSearchText } from "../lib/searchText.js";

function mapMemory(row: Record<string, unknown>): Memory {
  return {
    id: String(row.id),
    projectId: String(row.project_id),
    kind: row.kind as MemoryKind,
    title: String(row.title),
    content: String(row.content),
    sourceChatId: row.source_chat_id ? String(row.source_chat_id) : null,
    source: (row.source as MemorySource | undefined) ?? "manual",
    locked: Boolean(row.locked),
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at)
  };
}

export class MemoryRepository {
  constructor(private readonly db: Database.Database) {}

  getMemory(memoryId: string) {
    const row = this.db.prepare("SELECT * FROM memories WHERE id = ?").get(memoryId) as Record<string, unknown> | undefined;
    return row ? mapMemory(row) : null;
  }

  listForEmbedding(memoryIds?: string[]) {
    if (memoryIds?.length) {
      const placeholders = memoryIds.map(() => "?").join(", ");
      const rows = this.db
        .prepare(
          `SELECT id, project_id, kind, title, content FROM memories WHERE id IN (${placeholders}) ORDER BY created_at ASC`
        )
        .all(...memoryIds) as Record<string, unknown>[];

      return rows.map((row) => ({
        id: String(row.id),
        projectId: String(row.project_id),
        kind: row.kind as MemoryKind,
        title: String(row.title),
        content: String(row.content)
      }));
    }

    const rows = this.db
      .prepare("SELECT id, project_id, kind, title, content FROM memories ORDER BY created_at ASC")
      .all() as Record<string, unknown>[];

    return rows.map((row) => ({
      id: String(row.id),
      projectId: String(row.project_id),
      kind: row.kind as MemoryKind,
      title: String(row.title),
      content: String(row.content)
    }));
  }

  upsertMemoryEmbeddings(rows: Array<{ memoryId: string; projectId: string; kind: MemoryKind; embedding: number[]; model: string }>) {
    if (!rows.length) {
      return;
    }

    const updatedAt = nowIso();
    const tx = this.db.transaction(() => {
      const insert = this.db.prepare(
        `
          INSERT INTO memory_embeddings (memory_id, project_id, kind, embedding_json, model, updated_at)
          VALUES (?, ?, ?, ?, ?, ?)
          ON CONFLICT(memory_id) DO UPDATE SET
            project_id = excluded.project_id,
            kind = excluded.kind,
            embedding_json = excluded.embedding_json,
            model = excluded.model,
            updated_at = excluded.updated_at
        `
      );

      for (const row of rows) {
        insert.run(row.memoryId, row.projectId, row.kind, JSON.stringify(row.embedding), row.model, updatedAt);
      }
    });

    tx();
  }

  rebuildSearchIndex() {
    const rows = this.db
      .prepare("SELECT project_id, id, kind, title, content FROM memories ORDER BY created_at ASC")
      .all() as Record<string, unknown>[];

    const tx = this.db.transaction(() => {
      this.db.prepare("DELETE FROM memories_fts").run();

      const insert = this.db.prepare(
        "INSERT INTO memories_fts (project_id, memory_id, kind, title, content) VALUES (?, ?, ?, ?, ?)"
      );

      for (const row of rows) {
        insert.run(
          String(row.project_id),
          String(row.id),
          buildSearchText(String(row.kind)),
          buildSearchText(String(row.title)),
          buildSearchText(String(row.content))
        );
      }
    });

    tx();
  }

  listByProject(projectId: string) {
    const rows = this.db
      .prepare("SELECT * FROM memories WHERE project_id = ? ORDER BY locked DESC, updated_at DESC, created_at DESC")
      .all(projectId) as Record<string, unknown>[];
    return rows.map(mapMemory);
  }

  listByProjectAndKind(projectId: string, kind: MemoryKind) {
    const rows = this.db
      .prepare("SELECT * FROM memories WHERE project_id = ? AND kind = ? ORDER BY locked DESC, updated_at DESC")
      .all(projectId, kind) as Record<string, unknown>[];
    return rows.map(mapMemory);
  }

  createMemory(input: {
    projectId: string;
    kind: MemoryKind;
    title: string;
    content: string;
    sourceChatId?: string | null;
    source?: MemorySource;
    locked?: boolean;
  }) {
    const memory: Memory = {
      id: createId("memory"),
      projectId: input.projectId,
      kind: input.kind,
      title: input.title.trim(),
      content: input.content.trim(),
      sourceChatId: input.sourceChatId ?? null,
      source: input.source ?? "manual",
      locked: input.locked ?? false,
      createdAt: nowIso(),
      updatedAt: nowIso()
    };

    const tx = this.db.transaction(() => {
      this.db
        .prepare(
          `
            INSERT INTO memories (id, project_id, kind, title, content, source_chat_id, source, locked, created_at, updated_at)
            VALUES (@id, @projectId, @kind, @title, @content, @sourceChatId, @source, @locked, @createdAt, @updatedAt)
          `
        )
        .run({
          ...memory,
          locked: Number(memory.locked)
        });

      this.db
        .prepare("INSERT INTO memories_fts (project_id, memory_id, kind, title, content) VALUES (?, ?, ?, ?, ?)")
        .run(
          memory.projectId,
          memory.id,
          buildSearchText(memory.kind),
          buildSearchText(memory.title),
          buildSearchText(memory.content)
        );
    });

    tx();
    return memory;
  }

  updateMemory(input: { memoryId: string; kind: MemoryKind; title: string; content: string; locked?: boolean }) {
    const existing = this.db
      .prepare("SELECT * FROM memories WHERE id = ?")
      .get(input.memoryId) as Record<string, unknown> | undefined;

    if (!existing) {
      return null;
    }

    const updatedAt = nowIso();
    const tx = this.db.transaction(() => {
      this.db
        .prepare(
          `
            UPDATE memories
            SET kind = ?, title = ?, content = ?, locked = COALESCE(?, locked), updated_at = ?
            WHERE id = ?
          `
        )
        .run(input.kind, input.title.trim(), input.content.trim(), input.locked == null ? null : Number(input.locked), updatedAt, input.memoryId);

      this.db.prepare("DELETE FROM memories_fts WHERE memory_id = ?").run(input.memoryId);
      this.db
        .prepare("INSERT INTO memories_fts (project_id, memory_id, kind, title, content) VALUES (?, ?, ?, ?, ?)")
        .run(
          String(existing.project_id),
          input.memoryId,
          buildSearchText(input.kind),
          buildSearchText(input.title.trim()),
          buildSearchText(input.content.trim())
        );
    });

    tx();
    const row = this.db.prepare("SELECT * FROM memories WHERE id = ?").get(input.memoryId) as Record<string, unknown>;
    return mapMemory(row);
  }

  setMemoryLocked(memoryId: string, locked: boolean) {
    const existing = this.db
      .prepare("SELECT * FROM memories WHERE id = ?")
      .get(memoryId) as Record<string, unknown> | undefined;

    if (!existing) {
      return null;
    }

    const updatedAt = nowIso();
    this.db
      .prepare(
        `
          UPDATE memories
          SET locked = ?, updated_at = ?
          WHERE id = ?
        `
      )
      .run(Number(locked), updatedAt, memoryId);

    const row = this.db.prepare("SELECT * FROM memories WHERE id = ?").get(memoryId) as Record<string, unknown>;
    return mapMemory(row);
  }

  deleteMemory(memoryId: string) {
    const row = this.db.prepare("SELECT * FROM memories WHERE id = ?").get(memoryId) as Record<string, unknown> | undefined;
    if (!row) {
      return null;
    }

    const tx = this.db.transaction(() => {
      this.db.prepare("DELETE FROM memories_fts WHERE memory_id = ?").run(memoryId);
      this.db.prepare("DELETE FROM memories WHERE id = ?").run(memoryId);
    });

    tx();
    return mapMemory(row);
  }

  hasSimilarMemory(projectId: string, title: string, content: string) {
    const row = this.db
      .prepare(
        `
          SELECT id
          FROM memories
          WHERE project_id = ? AND lower(title) = lower(?) AND lower(content) = lower(?)
          LIMIT 1
        `
      )
      .get(projectId, title.trim(), content.trim()) as { id: string } | undefined;
    return Boolean(row);
  }
}
