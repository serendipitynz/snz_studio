import Database from "better-sqlite3";
import { DocumentCategory, RetrievedDocumentChunk, RetrievedDocumentReference, SearchReference } from "../lib/types.js";
import { safeJsonParse, truncate } from "../lib/utils.js";
import { toFtsQuery } from "../lib/searchText.js";
import { cosineSimilarity, normalizeScores } from "../lib/vector.js";
import { EmbeddingClient } from "./embeddingClient.js";
import { config } from "../config.js";

type DocumentCandidateRow = {
  source_id: string;
  label: string;
  category: DocumentCategory;
  full_content: string;
  chunk_content: string;
  chunk_id: string;
  chunk_index: number;
  note: string;
  derived_text: string;
  score: number;
};

type MemoryCandidateRow = {
  source_id: string;
  label: string;
  excerpt: string;
  score: number;
};

function clampScore(value: number) {
  return Math.max(0, Math.min(1, value));
}

function semanticToUnitRange(value: number) {
  return clampScore((value + 1) / 2);
}

type RetrievalIntent = "reference" | "writing" | "plot" | "translation" | "general";

function detectRetrievalIntent(query: string): RetrievalIntent {
  if (/(翻訳|訳して|英訳|和訳|訳文|用語統一)/iu.test(query)) {
    return "translation";
  }

  if (/(プロット|構想|展開案|次どう|章構成|起きること|流れ)/iu.test(query)) {
    return "plot";
  }

  if (/(続き|本文|執筆|書いて|書き直|推敲|場面|シーン|会話文|地の文|文体)/iu.test(query)) {
    return "writing";
  }

  if (/(設定|世界観|ルール|正史|索引|時系列|人物|キャラ|用語|年表|整合|矛盾|確認)/iu.test(query)) {
    return "reference";
  }

  return "general";
}

function getCategoryWeight(category: DocumentCategory, intent: RetrievalIntent) {
  const tables: Record<RetrievalIntent, Record<DocumentCategory, number>> = {
    general: {
      world: 1.05,
      character: 1.05,
      rule: 1.05,
      plot: 1,
      timeline: 1.05,
      index: 1.05,
      story: 1,
      misc: 1
    },
    reference: {
      world: 1.2,
      character: 1.15,
      rule: 1.15,
      plot: 0.9,
      timeline: 1.15,
      index: 1.25,
      story: 0.8,
      misc: 1
    },
    writing: {
      world: 1,
      character: 1.15,
      rule: 1.25,
      plot: 1.05,
      timeline: 1.1,
      index: 0.95,
      story: 1.2,
      misc: 1
    },
    plot: {
      world: 1,
      character: 1.1,
      rule: 1,
      plot: 1.25,
      timeline: 1.15,
      index: 1,
      story: 0.95,
      misc: 1
    },
    translation: {
      world: 1.05,
      character: 1.05,
      rule: 1.25,
      plot: 0.9,
      timeline: 0.95,
      index: 1.1,
      story: 1.2,
      misc: 1
    }
  };

  return tables[intent][category] ?? 1;
}

function debugInfo(message: string) {
  if (config.debugRetrieval) {
    console.info(message);
  }
}

export class RetrievalService {
  constructor(
    private readonly db: Database.Database,
    private readonly embeddings: EmbeddingClient
  ) {}

  async searchDocuments(projectId: string, query: string, limit = 4, chunksPerDocument = 3): Promise<RetrievedDocumentReference[]> {
    const startedAt = Date.now();
    const ftsQuery = toFtsQuery(query);
    const intent = detectRetrievalIntent(query);
    debugInfo(
      `[retrieval] documents start ${JSON.stringify({
        projectId,
        queryChars: query.length,
        hasFtsQuery: Boolean(ftsQuery),
        intent,
        limit,
        chunksPerDocument,
        embeddingEnabled: this.embeddings.isEnabled()
      })}`
    );
    const candidateRows = ftsQuery ? this.fetchDocumentFtsCandidates(projectId, ftsQuery, limit, chunksPerDocument) : [];
    const queryEmbedding = await this.embeddings.createEmbedding(query);

    const ranked = this.rankDocumentCandidates(candidateRows, queryEmbedding, chunksPerDocument, intent);
    if (ranked.length >= limit || !queryEmbedding) {
      debugInfo(
        `[retrieval] documents done ${JSON.stringify({
          projectId,
          durationMs: Date.now() - startedAt,
          candidateRowCount: candidateRows.length,
          rankedCount: ranked.length,
          usedSemanticFallback: false,
          hadQueryEmbedding: Boolean(queryEmbedding)
        })}`
      );
      return ranked.slice(0, limit);
    }

    const seenDocumentIds = new Set(ranked.map((item) => item.sourceId));
    const fallback = this.searchDocumentsBySemantic(projectId, queryEmbedding, limit, chunksPerDocument, seenDocumentIds, intent);
    debugInfo(
      `[retrieval] documents done ${JSON.stringify({
        projectId,
        durationMs: Date.now() - startedAt,
        candidateRowCount: candidateRows.length,
        rankedCount: ranked.length,
        fallbackCount: fallback.length,
        usedSemanticFallback: true,
        hadQueryEmbedding: true
      })}`
    );
    return [...ranked, ...fallback].slice(0, limit);
  }

  searchChunksInDocument(documentId: string, query: string, limit = 5): RetrievedDocumentChunk[] {
    const ftsQuery = toFtsQuery(query);

    if (!ftsQuery) {
      const fallbackRows = this.db
        .prepare(
          `
            SELECT id AS chunk_id, chunk_index, content
            FROM document_chunks
            WHERE document_id = ?
            ORDER BY chunk_index ASC
            LIMIT ?
          `
        )
        .all(documentId, limit) as Record<string, unknown>[];

      return fallbackRows.map((row) => ({
        chunkId: String(row.chunk_id),
        chunkIndex: Number(row.chunk_index),
        content: String(row.content || ""),
        score: 0
      }));
    }

    const rows = this.db
      .prepare(
        `
          SELECT
            c.id AS chunk_id,
            c.chunk_index AS chunk_index,
            c.content AS content,
            bm25(document_chunks_fts, 10.0, 2.0, 1.0, 1.0, 4.0) * -1 AS score
          FROM document_chunks_fts
          JOIN document_chunks c ON c.id = document_chunks_fts.chunk_id
          WHERE document_chunks_fts.document_id = ? AND document_chunks_fts MATCH ?
          ORDER BY score DESC, c.chunk_index ASC
          LIMIT ?
        `
      )
      .all(documentId, ftsQuery, limit) as Record<string, unknown>[];

    return rows.map((row) => ({
      chunkId: String(row.chunk_id),
      chunkIndex: Number(row.chunk_index),
      content: String(row.content || ""),
      score: Number(row.score)
    }));
  }

  async searchMemories(projectId: string, query: string, limit = 4): Promise<SearchReference[]> {
    const startedAt = Date.now();
    const ftsQuery = toFtsQuery(query);
    debugInfo(
      `[retrieval] memories start ${JSON.stringify({
        projectId,
        queryChars: query.length,
        hasFtsQuery: Boolean(ftsQuery),
        limit,
        embeddingEnabled: this.embeddings.isEnabled()
      })}`
    );
    const candidateRows = ftsQuery ? this.fetchMemoryFtsCandidates(projectId, ftsQuery, Math.max(limit * 3, 8)) : [];
    const queryEmbedding = await this.embeddings.createEmbedding(query);

    const ranked = this.rankMemoryCandidates(candidateRows, queryEmbedding);
    if (ranked.length >= limit || !queryEmbedding) {
      debugInfo(
        `[retrieval] memories done ${JSON.stringify({
          projectId,
          durationMs: Date.now() - startedAt,
          candidateRowCount: candidateRows.length,
          rankedCount: ranked.length,
          usedSemanticFallback: false,
          hadQueryEmbedding: Boolean(queryEmbedding)
        })}`
      );
      return ranked.slice(0, limit);
    }

    const seenMemoryIds = new Set(ranked.map((item) => item.sourceId));
    const fallback = this.searchMemoriesBySemantic(projectId, queryEmbedding, limit, seenMemoryIds);
    debugInfo(
      `[retrieval] memories done ${JSON.stringify({
        projectId,
        durationMs: Date.now() - startedAt,
        candidateRowCount: candidateRows.length,
        rankedCount: ranked.length,
        fallbackCount: fallback.length,
        usedSemanticFallback: true,
        hadQueryEmbedding: true
      })}`
    );
    return [...ranked, ...fallback].slice(0, limit);
  }

  private fetchDocumentFtsCandidates(projectId: string, ftsQuery: string, limit: number, chunksPerDocument: number) {
    const candidateLimit = Math.max(limit * chunksPerDocument * 4, 12);
    return this.db
      .prepare(
        `
          SELECT
            d.id AS source_id,
            d.title AS label,
            d.category AS category,
            d.content_text AS full_content,
            c.content AS chunk_content,
            c.id AS chunk_id,
            c.chunk_index AS chunk_index,
            d.note AS note,
            d.derived_text AS derived_text,
            bm25(document_chunks_fts, 10.0, 2.0, 1.0, 1.0, 4.0) * -1 AS score
          FROM document_chunks_fts
          JOIN documents d ON d.id = document_chunks_fts.document_id
          JOIN document_chunks c ON c.id = document_chunks_fts.chunk_id
          WHERE document_chunks_fts.project_id = ? AND document_chunks_fts MATCH ?
          ORDER BY score DESC
          LIMIT ?
        `
      )
      .all(projectId, ftsQuery, candidateLimit) as DocumentCandidateRow[];
  }

  private fetchMemoryFtsCandidates(projectId: string, ftsQuery: string, limit: number) {
    return this.db
      .prepare(
        `
          SELECT
            m.id AS source_id,
            m.title AS label,
            m.content AS excerpt,
            bm25(memories_fts, 2.0, 6.0, 8.0) * -1 AS score
          FROM memories_fts
          JOIN memories m ON m.id = memories_fts.memory_id
          WHERE memories_fts.project_id = ? AND memories_fts MATCH ?
          ORDER BY score DESC
          LIMIT ?
        `
      )
      .all(projectId, ftsQuery, limit) as MemoryCandidateRow[];
  }

  private rankDocumentCandidates(
    rows: DocumentCandidateRow[],
    queryEmbedding: number[] | null,
    chunksPerDocument: number,
    intent: RetrievalIntent
  ) {
    if (!rows.length) {
      return [];
    }

    const ftsScores = normalizeScores(rows.map((row) => Number(row.score)));
    const semanticScores = new Map<string, number>();

    if (queryEmbedding) {
      const chunkIds = rows.map((row) => row.chunk_id);
      const placeholders = chunkIds.map(() => "?").join(", ");
      const embeddingRows = this.db
        .prepare(`SELECT chunk_id, embedding_json FROM document_chunk_embeddings WHERE model = ? AND chunk_id IN (${placeholders})`)
        .all(this.embeddings.getModel(), ...chunkIds) as Array<{ chunk_id: string; embedding_json: string }>;

      for (const row of embeddingRows) {
        const embedding = safeJsonParse<number[]>(row.embedding_json, []);
        semanticScores.set(row.chunk_id, semanticToUnitRange(cosineSimilarity(queryEmbedding, embedding)));
      }
    }

    const groups = new Map<
      string,
      {
        label: string;
        fullContent: string;
        fallbackText: string;
        chunks: RetrievedDocumentChunk[];
        bestScore: number;
      }
    >();

    rows.forEach((row, index) => {
      const hybridScore = queryEmbedding
        ? ftsScores[index] * 0.55 + (semanticScores.get(row.chunk_id) ?? 0) * 0.45
        : ftsScores[index] || Number(row.score);
      const weightedScore = hybridScore * getCategoryWeight(row.category, intent);

      const existing = groups.get(row.source_id);
      const chunk: RetrievedDocumentChunk = {
        chunkId: row.chunk_id,
        chunkIndex: Number(row.chunk_index),
        content: String(row.chunk_content || ""),
        score: weightedScore
      };

      if (!existing) {
        groups.set(row.source_id, {
          label: row.label,
          fullContent: row.full_content,
          fallbackText: String(row.chunk_content || row.derived_text || row.note || ""),
          chunks: [chunk],
          bestScore: weightedScore
        });
        return;
      }

      if (!existing.chunks.some((current) => current.chunkId === chunk.chunkId) && existing.chunks.length < chunksPerDocument) {
        existing.chunks.push(chunk);
      }

      if (weightedScore > existing.bestScore) {
        existing.bestScore = weightedScore;
      }
    });

    const results: RetrievedDocumentReference[] = [];

    for (const [sourceId, group] of [...groups.entries()].sort((left, right) => right[1].bestScore - left[1].bestScore)) {
      const excerpt = group.chunks.length
        ? group.chunks
            .sort((left, right) => right.score - left.score)
            .slice(0, chunksPerDocument)
            .sort((left, right) => left.chunkIndex - right.chunkIndex)
            .map((chunk) => `[chunk ${chunk.chunkIndex + 1}] ${truncate(chunk.content, 150)}`)
            .join("\n")
        : truncate(group.fallbackText, 220);

      results.push({
        sourceType: "document",
        sourceId,
        label: group.label,
        category: rows.find((row) => row.source_id === sourceId)?.category ?? "misc",
        excerpt: truncate(excerpt, 500),
        score: group.bestScore,
        chunks: group.chunks.sort((left, right) => left.chunkIndex - right.chunkIndex).slice(0, chunksPerDocument),
        includeFullDocument: false,
        fullDocumentContent: group.fullContent,
        retrievalMode: "search"
      });
    }

    return results;
  }

  private searchDocumentsBySemantic(
    projectId: string,
    queryEmbedding: number[],
    limit: number,
    chunksPerDocument: number,
    seenDocumentIds: Set<string>,
    intent: RetrievalIntent
  ) {
    const rows = this.db
      .prepare(
        `
          SELECT
            e.document_id,
            e.chunk_id,
            e.embedding_json,
            d.title,
            d.category,
            d.content_text AS full_content,
            d.note,
            d.derived_text,
            c.content AS chunk_content,
            c.chunk_index
          FROM document_chunk_embeddings e
          JOIN documents d ON d.id = e.document_id
          JOIN document_chunks c ON c.id = e.chunk_id
          WHERE e.project_id = ? AND e.model = ?
        `
      )
      .all(projectId, this.embeddings.getModel()) as Array<{
      document_id: string;
      chunk_id: string;
      embedding_json: string;
      title: string;
      category: DocumentCategory;
      full_content: string;
      note: string;
      derived_text: string;
      chunk_content: string;
      chunk_index: number;
    }>;

    const scored = rows
      .map((row) => ({
        ...row,
        score:
          semanticToUnitRange(cosineSimilarity(queryEmbedding, safeJsonParse<number[]>(row.embedding_json, []))) *
          getCategoryWeight(row.category, intent)
      }))
      .filter((row) => row.score > 0.55)
      .sort((left, right) => right.score - left.score);

    const groups = new Map<
      string,
      {
        label: string;
        fullContent: string;
        fallbackText: string;
        chunks: RetrievedDocumentChunk[];
        bestScore: number;
      }
    >();

    for (const row of scored) {
      if (seenDocumentIds.has(row.document_id)) {
        continue;
      }

      const existing = groups.get(row.document_id);
      const chunk: RetrievedDocumentChunk = {
        chunkId: row.chunk_id,
        chunkIndex: Number(row.chunk_index),
        content: String(row.chunk_content || ""),
        score: row.score
      };

      if (!existing) {
        groups.set(row.document_id, {
          label: row.title,
          fullContent: row.full_content,
          fallbackText: String(row.chunk_content || row.derived_text || row.note || ""),
          chunks: [chunk],
          bestScore: row.score
        });
      } else if (!existing.chunks.some((current) => current.chunkId === chunk.chunkId) && existing.chunks.length < chunksPerDocument) {
        existing.chunks.push(chunk);
      }

      if (groups.size >= limit) {
        break;
      }
    }

    return [...groups.entries()].map(([sourceId, group]) => ({
      sourceType: "document" as const,
      sourceId,
      label: group.label,
      category: scored.find((row) => row.document_id === sourceId)?.category ?? "misc",
      excerpt: truncate(
        group.chunks
          .sort((left, right) => left.chunkIndex - right.chunkIndex)
          .map((chunk) => `[chunk ${chunk.chunkIndex + 1}] ${truncate(chunk.content, 150)}`)
          .join("\n") || group.fallbackText,
        500
      ),
      score: group.bestScore,
      chunks: group.chunks.sort((left, right) => left.chunkIndex - right.chunkIndex).slice(0, chunksPerDocument),
      includeFullDocument: false,
      fullDocumentContent: group.fullContent,
      retrievalMode: "search" as const
    }));
  }

  private rankMemoryCandidates(rows: MemoryCandidateRow[], queryEmbedding: number[] | null) {
    if (!rows.length) {
      return [];
    }

    const ftsScores = normalizeScores(rows.map((row) => Number(row.score)));
    const semanticScores = new Map<string, number>();

    if (queryEmbedding) {
      const memoryIds = rows.map((row) => row.source_id);
      const placeholders = memoryIds.map(() => "?").join(", ");
      const embeddingRows = this.db
        .prepare(`SELECT memory_id, embedding_json FROM memory_embeddings WHERE model = ? AND memory_id IN (${placeholders})`)
        .all(this.embeddings.getModel(), ...memoryIds) as Array<{ memory_id: string; embedding_json: string }>;

      for (const row of embeddingRows) {
        const embedding = safeJsonParse<number[]>(row.embedding_json, []);
        semanticScores.set(row.memory_id, semanticToUnitRange(cosineSimilarity(queryEmbedding, embedding)));
      }
    }

    return rows
      .map((row, index) => ({
        sourceType: "memory" as const,
        sourceId: row.source_id,
        label: row.label,
        excerpt: truncate(row.excerpt || "", 220),
        score: queryEmbedding ? ftsScores[index] * 0.5 + (semanticScores.get(row.source_id) ?? 0) * 0.5 : ftsScores[index]
      }))
      .sort((left, right) => right.score - left.score);
  }

  private searchMemoriesBySemantic(projectId: string, queryEmbedding: number[], limit: number, seenMemoryIds: Set<string>) {
    const rows = this.db
      .prepare(
        `
          SELECT
            e.memory_id,
            e.embedding_json,
            m.title,
            m.content
          FROM memory_embeddings e
          JOIN memories m ON m.id = e.memory_id
          WHERE e.project_id = ? AND e.model = ?
        `
      )
      .all(projectId, this.embeddings.getModel()) as Array<{ memory_id: string; embedding_json: string; title: string; content: string }>;

    return rows
      .filter((row) => !seenMemoryIds.has(row.memory_id))
      .map((row) => ({
        sourceType: "memory" as const,
        sourceId: row.memory_id,
        label: row.title,
        excerpt: truncate(row.content, 220),
        score: semanticToUnitRange(cosineSimilarity(queryEmbedding, safeJsonParse<number[]>(row.embedding_json, [])))
      }))
      .filter((row) => row.score > 0.58)
      .sort((left, right) => right.score - left.score)
      .slice(0, limit);
  }
}
