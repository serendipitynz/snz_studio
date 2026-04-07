import Database from "better-sqlite3";
import { inferDocumentCategory, isDocumentCategory } from "../lib/documentCategory.js";
import { chunkDocumentText } from "../services/documentChunker.js";
import { DocumentRecord } from "../lib/types.js";
import { createId, nowIso, parseTags, safeJsonParse } from "../lib/utils.js";
import { buildSearchText } from "../lib/searchText.js";

function mapDocument(row: Record<string, unknown>): DocumentRecord {
  return {
    id: String(row.id),
    projectId: String(row.project_id),
    type: row.type as DocumentRecord["type"],
    category: isDocumentCategory(String(row.category ?? "misc")) ? (row.category as DocumentRecord["category"]) : "misc",
    title: String(row.title),
    note: String(row.note),
    tags: safeJsonParse<string[]>(String(row.tags_json), []),
    derivedText: String(row.derived_text),
    contentText: String(row.content_text),
    filePath: row.file_path ? String(row.file_path) : null,
    mimeType: row.mime_type ? String(row.mime_type) : null,
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at)
  };
}

export class DocumentRepository {
  constructor(private readonly db: Database.Database) {}

  listChunksForEmbedding(documentId?: string) {
    const query = documentId
      ? `
          SELECT
            c.id AS chunk_id,
            c.document_id,
            c.project_id,
            c.chunk_index,
            c.content,
            d.title,
            d.note,
            d.tags_json,
            d.derived_text
          FROM document_chunks c
          JOIN documents d ON d.id = c.document_id
          WHERE c.document_id = ?
          ORDER BY c.chunk_index ASC
        `
      : `
          SELECT
            c.id AS chunk_id,
            c.document_id,
            c.project_id,
            c.chunk_index,
            c.content,
            d.title,
            d.note,
            d.tags_json,
            d.derived_text
          FROM document_chunks c
          JOIN documents d ON d.id = c.document_id
          ORDER BY d.created_at ASC, c.chunk_index ASC
        `;

    const rows = (documentId ? this.db.prepare(query).all(documentId) : this.db.prepare(query).all()) as Record<string, unknown>[];

    return rows.map((row) => ({
      chunkId: String(row.chunk_id),
      documentId: String(row.document_id),
      projectId: String(row.project_id),
      chunkIndex: Number(row.chunk_index),
      content: String(row.content ?? ""),
      title: String(row.title ?? ""),
      note: String(row.note ?? ""),
      tags: safeJsonParse<string[]>(String(row.tags_json ?? "[]"), []),
      derivedText: String(row.derived_text ?? "")
    }));
  }

  upsertChunkEmbeddings(
    rows: Array<{ chunkId: string; documentId: string; projectId: string; embedding: number[]; model: string }>
  ) {
    if (!rows.length) {
      return;
    }

    const updatedAt = nowIso();
    const tx = this.db.transaction(() => {
      const insert = this.db.prepare(
        `
          INSERT INTO document_chunk_embeddings (chunk_id, document_id, project_id, embedding_json, model, updated_at)
          VALUES (?, ?, ?, ?, ?, ?)
          ON CONFLICT(chunk_id) DO UPDATE SET
            document_id = excluded.document_id,
            project_id = excluded.project_id,
            embedding_json = excluded.embedding_json,
            model = excluded.model,
            updated_at = excluded.updated_at
        `
      );

      for (const row of rows) {
        insert.run(row.chunkId, row.documentId, row.projectId, JSON.stringify(row.embedding), row.model, updatedAt);
      }
    });

    tx();
  }

  rebuildSearchIndex() {
    const rows = this.db
      .prepare(
        `
          SELECT
            d.id AS document_id,
            d.project_id,
            d.title,
            d.note,
            d.tags_json,
            d.derived_text,
            c.id AS chunk_id,
            c.content
          FROM documents d
          JOIN document_chunks c ON c.document_id = d.id
          ORDER BY d.created_at ASC, c.chunk_index ASC
        `
      )
      .all() as Record<string, unknown>[];

    const tx = this.db.transaction(() => {
      this.db.prepare("DELETE FROM document_chunks_fts").run();

      const insert = this.db.prepare(
        `
          INSERT INTO document_chunks_fts (
            project_id, document_id, chunk_id, title, note, tags, derived_text, content
          ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        `
      );

      for (const row of rows) {
        const tags = safeJsonParse<string[]>(String(row.tags_json ?? "[]"), []);
        insert.run(
          String(row.project_id),
          String(row.document_id),
          String(row.chunk_id),
          buildSearchText(String(row.title ?? "")),
          buildSearchText(String(row.note ?? "")),
          buildSearchText(tags.join(" ")),
          buildSearchText(String(row.derived_text ?? "")),
          buildSearchText(String(row.content ?? ""))
        );
      }
    });

    tx();
  }

  backfillInferredCategories() {
    const rows = this.db
      .prepare(
        `
          SELECT id, title, note, derived_text, content_text, file_path, updated_at, created_at, category
          FROM documents
          WHERE category = 'misc' AND updated_at = created_at
        `
      )
      .all() as Record<string, unknown>[];

    if (!rows.length) {
      return;
    }

    const update = this.db.prepare(
      `
        UPDATE documents
        SET category = ?, updated_at = ?
        WHERE id = ?
      `
    );

    const tx = this.db.transaction(() => {
      for (const row of rows) {
        const inferred = inferDocumentCategory({
          fileName: row.file_path ? String(row.file_path) : String(row.title ?? ""),
          title: String(row.title ?? ""),
          note: String(row.note ?? ""),
          contentText: String(row.content_text ?? ""),
          derivedText: String(row.derived_text ?? "")
        });

        if (inferred === "misc") {
          continue;
        }

        update.run(inferred, nowIso(), String(row.id));
      }
    });

    tx();
  }

  listByProject(projectId: string) {
    const rows = this.db
      .prepare("SELECT * FROM documents WHERE project_id = ? ORDER BY created_at DESC")
      .all(projectId) as Record<string, unknown>[];
    return rows.map(mapDocument);
  }

  getDocument(documentId: string) {
    const row = this.db
      .prepare("SELECT * FROM documents WHERE id = ?")
      .get(documentId) as Record<string, unknown> | undefined;
    return row ? mapDocument(row) : null;
  }

  createDocument(input: {
    projectId: string;
    type: DocumentRecord["type"];
    category?: DocumentRecord["category"];
    title: string;
    note?: string;
    tags?: string[] | string;
    derivedText?: string;
    contentText?: string;
    filePath?: string | null;
    mimeType?: string | null;
  }) {
    const createdAt = nowIso();
    const inferredCategory =
      input.category ??
      inferDocumentCategory({
        fileName: input.filePath,
        title: input.title,
        note: input.note,
        contentText: input.contentText,
        derivedText: input.derivedText
      });
    const document: DocumentRecord = {
      id: createId("doc"),
      projectId: input.projectId,
      type: input.type,
      category: inferredCategory,
      title: input.title.trim(),
      note: input.note?.trim() ?? "",
      tags: Array.isArray(input.tags) ? input.tags : parseTags(input.tags),
      derivedText: input.derivedText?.trim() ?? "",
      contentText: input.contentText?.trim() ?? "",
      filePath: input.filePath ?? null,
      mimeType: input.mimeType ?? null,
      createdAt,
      updatedAt: createdAt
    };

    const tx = this.db.transaction(() => {
      this.db
        .prepare(
          `
            INSERT INTO documents (
              id, project_id, type, category, title, note, tags_json, derived_text, content_text, file_path, mime_type, created_at, updated_at
            )
            VALUES (
              @id, @projectId, @type, @category, @title, @note, @tagsJson, @derivedText, @contentText, @filePath, @mimeType, @createdAt, @updatedAt
            )
          `
        )
        .run({
          ...document,
          tagsJson: JSON.stringify(document.tags)
        });

      this.deleteChunks(document.id);

      const joinedSearchBody = [document.contentText, document.derivedText, document.note, document.tags.join(", ")]
        .filter(Boolean)
        .join("\n\n");
      const chunks = chunkDocumentText(joinedSearchBody);

      if (!chunks.length) {
        const chunkId = createId("chunk");
        this.db
          .prepare(
            `
              INSERT INTO document_chunks (id, document_id, project_id, chunk_index, content, created_at)
              VALUES (?, ?, ?, ?, ?, ?)
            `
          )
          .run(chunkId, document.id, document.projectId, 0, "", createdAt);
        this.db
          .prepare(
            `
              INSERT INTO document_chunks_fts (
                project_id, document_id, chunk_id, title, note, tags, derived_text, content
              ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            `
          )
          .run(
            document.projectId,
            document.id,
            chunkId,
            buildSearchText(document.title),
            buildSearchText(document.note),
            buildSearchText(document.tags.join(" ")),
            buildSearchText(document.derivedText),
            ""
          );
      } else {
        chunks.forEach((chunk, index) => {
          const chunkId = createId("chunk");
          this.db
            .prepare(
              `
                INSERT INTO document_chunks (id, document_id, project_id, chunk_index, content, created_at)
                VALUES (?, ?, ?, ?, ?, ?)
              `
            )
            .run(chunkId, document.id, document.projectId, index, chunk, createdAt);
          this.db
            .prepare(
              `
                INSERT INTO document_chunks_fts (
                  project_id, document_id, chunk_id, title, note, tags, derived_text, content
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
              `
            )
            .run(
              document.projectId,
              document.id,
              chunkId,
              buildSearchText(document.title),
              buildSearchText(document.note),
              buildSearchText(document.tags.join(" ")),
              buildSearchText(document.derivedText),
              buildSearchText(chunk)
            );
        });
      }
    });

    tx();
    return document;
  }

  updateDocumentCategory(documentId: string, category: DocumentRecord["category"]) {
    const existing = this.getDocument(documentId);
    if (!existing) {
      return null;
    }

    const updatedAt = nowIso();
    this.db
      .prepare(
        `
          UPDATE documents
          SET category = ?, updated_at = ?
          WHERE id = ?
        `
      )
      .run(category, updatedAt, documentId);

    return this.getDocument(documentId);
  }

  deleteDocument(documentId: string) {
    const document = this.getDocument(documentId);
    if (!document) {
      return null;
    }

    const tx = this.db.transaction(() => {
      this.deleteChunks(documentId);
      this.db.prepare("DELETE FROM documents WHERE id = ?").run(documentId);
    });

    tx();
    return document;
  }

  private deleteChunks(documentId: string) {
    this.db.prepare("DELETE FROM document_chunks WHERE document_id = ?").run(documentId);
    this.db.prepare("DELETE FROM document_chunks_fts WHERE document_id = ?").run(documentId);
  }
}
