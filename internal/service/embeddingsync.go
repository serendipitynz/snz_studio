package service

import (
	"strings"

	"snzstudio/internal/repository"
)

const embeddingBatchSize = 32

// EmbeddingSyncService ports embeddingSyncService.ts: it (re)computes and stores the
// embeddings for document chunks and memories in batches, skipping entirely when
// embeddings are disabled.
type EmbeddingSyncService struct {
	documents  *repository.DocumentRepository
	memories   *repository.MemoryRepository
	embeddings *EmbeddingClient
}

// NewEmbeddingSyncService builds an EmbeddingSyncService.
func NewEmbeddingSyncService(documents *repository.DocumentRepository, memories *repository.MemoryRepository, embeddings *EmbeddingClient) *EmbeddingSyncService {
	return &EmbeddingSyncService{documents: documents, memories: memories, embeddings: embeddings}
}

// buildDocumentChunkEmbeddingText mirrors buildDocumentChunkEmbeddingText.
func buildDocumentChunkEmbeddingText(title, note string, tags []string, derivedText, content string) string {
	parts := []string{}
	if title != "" {
		parts = append(parts, "Document title: "+title)
	}
	if note != "" {
		parts = append(parts, "Note: "+note)
	}
	if len(tags) > 0 {
		parts = append(parts, "Tags: "+strings.Join(tags, ", "))
	}
	if derivedText != "" {
		parts = append(parts, "Derived text: "+derivedText)
	}
	if content != "" {
		parts = append(parts, "Content:\n"+content)
	}
	return strings.Join(parts, "\n")
}

// buildMemoryEmbeddingText mirrors buildMemoryEmbeddingText.
func buildMemoryEmbeddingText(kind, title, content string) string {
	parts := []string{"Memory kind: " + kind}
	if title != "" {
		parts = append(parts, "Title: "+title)
	}
	parts = append(parts, "Content: "+content)
	return strings.Join(parts, "\n")
}

// SyncDocument mirrors syncDocument(documentId).
func (s *EmbeddingSyncService) SyncDocument(documentID string) error {
	if !s.embeddings.IsEnabled() {
		return nil
	}
	chunks, err := s.documents.ListChunksForEmbedding(documentID)
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}

	inputs := make([]string, len(chunks))
	for i, chunk := range chunks {
		inputs[i] = buildDocumentChunkEmbeddingText(chunk.Title, chunk.Note, chunk.Tags, chunk.DerivedText, chunk.Content)
	}
	vectors := s.embedInBatches(inputs)
	if vectors == nil {
		return nil
	}
	return s.documents.UpsertChunkEmbeddings(toChunkEmbeddings(chunks, vectors, s.embeddings.GetModel()))
}

// SyncMemories mirrors syncMemories(memoryIds).
func (s *EmbeddingSyncService) SyncMemories(memoryIDs []string) error {
	if !s.embeddings.IsEnabled() || len(memoryIDs) == 0 {
		return nil
	}
	memories, err := s.memories.ListForEmbedding(memoryIDs)
	if err != nil {
		return err
	}
	if len(memories) == 0 {
		return nil
	}

	inputs := make([]string, len(memories))
	for i, memory := range memories {
		inputs[i] = buildMemoryEmbeddingText(memory.Kind, memory.Title, memory.Content)
	}
	vectors := s.embedInBatches(inputs)
	if vectors == nil {
		return nil
	}
	return s.memories.UpsertMemoryEmbeddings(toMemoryEmbeddings(memories, vectors, s.embeddings.GetModel()))
}

// RebuildAll mirrors rebuildAll: re-embed every chunk and memory.
func (s *EmbeddingSyncService) RebuildAll() error {
	if !s.embeddings.IsEnabled() {
		return nil
	}

	chunks, err := s.documents.ListChunksForEmbedding("")
	if err != nil {
		return err
	}
	if len(chunks) > 0 {
		inputs := make([]string, len(chunks))
		for i, chunk := range chunks {
			inputs[i] = buildDocumentChunkEmbeddingText(chunk.Title, chunk.Note, chunk.Tags, chunk.DerivedText, chunk.Content)
		}
		if vectors := s.embedInBatches(inputs); vectors != nil {
			if err := s.documents.UpsertChunkEmbeddings(toChunkEmbeddings(chunks, vectors, s.embeddings.GetModel())); err != nil {
				return err
			}
		}
	}

	memories, err := s.memories.ListForEmbedding(nil)
	if err != nil {
		return err
	}
	if len(memories) == 0 {
		return nil
	}
	inputs := make([]string, len(memories))
	for i, memory := range memories {
		inputs[i] = buildMemoryEmbeddingText(memory.Kind, memory.Title, memory.Content)
	}
	vectors := s.embedInBatches(inputs)
	if vectors == nil {
		return nil
	}
	return s.memories.UpsertMemoryEmbeddings(toMemoryEmbeddings(memories, vectors, s.embeddings.GetModel()))
}

func toChunkEmbeddings(chunks []repository.ChunkForEmbedding, vectors [][]float64, model string) []repository.ChunkEmbedding {
	out := make([]repository.ChunkEmbedding, len(chunks))
	for i, chunk := range chunks {
		out[i] = repository.ChunkEmbedding{
			ChunkID:    chunk.ChunkID,
			DocumentID: chunk.DocumentID,
			ProjectID:  chunk.ProjectID,
			Embedding:  vectors[i],
			Model:      model,
		}
	}
	return out
}

func toMemoryEmbeddings(memories []repository.MemoryForEmbedding, vectors [][]float64, model string) []repository.MemoryEmbedding {
	out := make([]repository.MemoryEmbedding, len(memories))
	for i, memory := range memories {
		out[i] = repository.MemoryEmbedding{
			MemoryID:  memory.ID,
			ProjectID: memory.ProjectID,
			Kind:      memory.Kind,
			Embedding: vectors[i],
			Model:     model,
		}
	}
	return out
}

// embedInBatches mirrors embedInBatches: embeds inputs in chunks of 32, returning
// nil as soon as any batch is unavailable.
//
// All inputs here are stored CORPUS items (document chunks and memories), so when
// the active model uses an asymmetric prefix scheme (ruri) they get the DOCUMENT
// prefix — queries get the query prefix in retrieval.go instead. The prefix is baked
// into the stored vectors, so changing it requires a RebuildAll.
func (s *EmbeddingSyncService) embedInBatches(inputs []string) [][]float64 {
	docPrefix := s.embeddings.ActivePrefixScheme().Document
	vectors := make([][]float64, 0, len(inputs))
	for start := 0; start < len(inputs); start += embeddingBatchSize {
		end := start + embeddingBatchSize
		if end > len(inputs) {
			end = len(inputs)
		}
		batch := inputs[start:end]
		if docPrefix != "" {
			prefixed := make([]string, len(batch))
			for i, in := range batch {
				prefixed[i] = docPrefix + in
			}
			batch = prefixed
		}
		vecs := s.embeddings.CreateEmbeddings(batch)
		if vecs == nil {
			return nil
		}
		vectors = append(vectors, vecs...)
	}
	return vectors
}
