import { DocumentRepository } from "../repositories/documentRepository.js";
import { MemoryRepository } from "../repositories/memoryRepository.js";
import { EmbeddingClient } from "./embeddingClient.js";

const EMBEDDING_BATCH_SIZE = 32;

function buildDocumentChunkEmbeddingText(input: {
  title: string;
  note: string;
  tags: string[];
  derivedText: string;
  content: string;
}) {
  return [
    input.title ? `Document title: ${input.title}` : "",
    input.note ? `Note: ${input.note}` : "",
    input.tags.length ? `Tags: ${input.tags.join(", ")}` : "",
    input.derivedText ? `Derived text: ${input.derivedText}` : "",
    input.content ? `Content:\n${input.content}` : ""
  ]
    .filter(Boolean)
    .join("\n");
}

function buildMemoryEmbeddingText(input: { kind: string; title: string; content: string }) {
  return [`Memory kind: ${input.kind}`, input.title ? `Title: ${input.title}` : "", `Content: ${input.content}`]
    .filter(Boolean)
    .join("\n");
}

export class EmbeddingSyncService {
  constructor(
    private readonly documents: DocumentRepository,
    private readonly memories: MemoryRepository,
    private readonly embeddings: EmbeddingClient
  ) {}

  async syncDocument(documentId: string) {
    if (!this.embeddings.isEnabled()) {
      return;
    }

    const chunks = this.documents.listChunksForEmbedding(documentId);
    if (!chunks.length) {
      return;
    }

    const vectors = await this.embedInBatches(
      chunks.map((chunk) =>
        buildDocumentChunkEmbeddingText({
          title: chunk.title,
          note: chunk.note,
          tags: chunk.tags,
          derivedText: chunk.derivedText,
          content: chunk.content
        })
      )
    );

    if (!vectors) {
      return;
    }

    this.documents.upsertChunkEmbeddings(
      chunks.map((chunk, index) => ({
        chunkId: chunk.chunkId,
        documentId: chunk.documentId,
        projectId: chunk.projectId,
        embedding: vectors[index],
        model: this.embeddings.getModel()
      }))
    );
  }

  async syncMemories(memoryIds: string[]) {
    if (!this.embeddings.isEnabled() || !memoryIds.length) {
      return;
    }

    const memories = this.memories.listForEmbedding(memoryIds);
    if (!memories.length) {
      return;
    }

    const vectors = await this.embedInBatches(
      memories.map((memory) =>
        buildMemoryEmbeddingText({
          kind: memory.kind,
          title: memory.title,
          content: memory.content
        })
      )
    );

    if (!vectors) {
      return;
    }

    this.memories.upsertMemoryEmbeddings(
      memories.map((memory, index) => ({
        memoryId: memory.id,
        projectId: memory.projectId,
        kind: memory.kind,
        embedding: vectors[index],
        model: this.embeddings.getModel()
      }))
    );
  }

  async rebuildAll() {
    if (!this.embeddings.isEnabled()) {
      return;
    }

    const chunks = this.documents.listChunksForEmbedding();
    if (chunks.length) {
      const vectors = await this.embedInBatches(
        chunks.map((chunk) =>
          buildDocumentChunkEmbeddingText({
            title: chunk.title,
            note: chunk.note,
            tags: chunk.tags,
            derivedText: chunk.derivedText,
            content: chunk.content
          })
        )
      );

      if (vectors) {
        this.documents.upsertChunkEmbeddings(
          chunks.map((chunk, index) => ({
            chunkId: chunk.chunkId,
            documentId: chunk.documentId,
            projectId: chunk.projectId,
            embedding: vectors[index],
            model: this.embeddings.getModel()
          }))
        );
      }
    }

    const memories = this.memories.listForEmbedding();
    if (!memories.length) {
      return;
    }

    const vectors = await this.embedInBatches(
      memories.map((memory) =>
        buildMemoryEmbeddingText({
          kind: memory.kind,
          title: memory.title,
          content: memory.content
        })
      )
    );

    if (!vectors) {
      return;
    }

    this.memories.upsertMemoryEmbeddings(
      memories.map((memory, index) => ({
        memoryId: memory.id,
        projectId: memory.projectId,
        kind: memory.kind,
        embedding: vectors[index],
        model: this.embeddings.getModel()
      }))
    );
  }

  private async embedInBatches(inputs: string[]) {
    const vectors: number[][] = [];

    for (let index = 0; index < inputs.length; index += EMBEDDING_BATCH_SIZE) {
      const batch = inputs.slice(index, index + EMBEDDING_BATCH_SIZE);
      const batchVectors = await this.embeddings.createEmbeddings(batch);
      if (!batchVectors) {
        return null;
      }
      vectors.push(...batchVectors);
    }

    return vectors;
  }
}
