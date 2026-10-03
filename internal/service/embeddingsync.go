package service

import (
	"log"
	"strings"
	"sync"

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

	// The corpus-wide passes (RebuildAll, SyncMissing) run on one worker at a
	// time: startup, the sidecar's ready callback and a settings save can all ask
	// for one, and two passes over the same rows only duplicate the work. Requests
	// that arrive while a pass runs collapse into one pending pass.
	mu      sync.Mutex
	idle    *sync.Cond
	running bool
	current corpusPass
	pending corpusPass
	// rebuildOwed stays set from a rebuild request until a rebuild completes. A
	// rebuild that fails (the new endpoint is down) leaves the old source's
	// vectors under a model name that may not have changed, and SyncMissing
	// cannot tell those from current ones, so gap fills run as rebuilds until
	// one gets through.
	rebuildOwed bool
	// rebuildCompleted records that a rebuild has gone through since startup, so
	// the settings screen can say so after the fact.
	rebuildCompleted bool
}

// corpusPass is ordered so that a larger value covers a smaller one: a full
// rebuild also fills every gap SyncMissing would.
type corpusPass int

const (
	passNone corpusPass = iota
	passMissing
	passRebuild
)

// RebuildState is where the corpus-wide rebuild stands, for the settings screen.
type RebuildState string

const (
	// RebuildIdle: no rebuild has been asked for since startup.
	RebuildIdle RebuildState = "idle"
	// RebuildRunning: a rebuild is running or waiting for the worker. Asking for
	// another would only queue one more pass over the same corpus.
	RebuildRunning RebuildState = "running"
	// RebuildDone: the last rebuild replaced the corpus.
	RebuildDone RebuildState = "done"
	// RebuildIncomplete: the last rebuild did not get through (see rebuildOwed);
	// the next gap fill runs it again.
	RebuildIncomplete RebuildState = "incomplete"
)

// NewEmbeddingSyncService builds an EmbeddingSyncService.
func NewEmbeddingSyncService(documents *repository.DocumentRepository, memories *repository.MemoryRepository, embeddings *EmbeddingClient) *EmbeddingSyncService {
	s := &EmbeddingSyncService{documents: documents, memories: memories, embeddings: embeddings}
	s.idle = sync.NewCond(&s.mu)
	return s
}

// RequestRebuild schedules a RebuildAll on the worker and returns at once.
func (s *EmbeddingSyncService) RequestRebuild() {
	s.schedule(passRebuild)
}

// RequestSyncMissing schedules a SyncMissing on the worker and returns at once.
func (s *EmbeddingSyncService) RequestSyncMissing() {
	s.schedule(passMissing)
}

// RebuildState reports where the corpus-wide rebuild stands. A gap fill that
// runs as an owed rebuild counts as a rebuild.
func (s *EmbeddingSyncService) RebuildState() RebuildState {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.current == passRebuild || s.pending == passRebuild:
		return RebuildRunning
	case s.rebuildOwed:
		return RebuildIncomplete
	case s.rebuildCompleted:
		return RebuildDone
	default:
		return RebuildIdle
	}
}

// WaitIdle blocks until no pass is running or pending.
func (s *EmbeddingSyncService) WaitIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.running {
		s.idle.Wait()
	}
}

func (s *EmbeddingSyncService) schedule(pass corpusPass) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pass == passRebuild {
		s.rebuildOwed = true
	} else if s.rebuildOwed {
		pass = passRebuild
	}
	if pass > s.pending {
		s.pending = pass
	}
	if !s.running {
		s.running = true
		go s.drain()
	}
}

func (s *EmbeddingSyncService) drain() {
	for {
		s.mu.Lock()
		pass := s.pending
		s.pending = passNone
		s.current = pass
		if pass == passNone {
			s.running = false
			s.idle.Broadcast()
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		switch pass {
		case passRebuild:
			complete, err := s.rebuildAll()
			if err != nil {
				log.Printf("Embedding rebuild skipped: %v", err)
			}
			// A rebuild requested while this one ran is still pending and owes
			// its own completion: it may target a source this pass never saw.
			s.mu.Lock()
			if complete {
				s.rebuildCompleted = true
				if s.pending != passRebuild {
					s.rebuildOwed = false
				}
			}
			s.mu.Unlock()
		case passMissing:
			if err := s.SyncMissing(); err != nil {
				log.Printf("Embedding sync skipped: %v", err)
			}
		}
	}
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
	model := s.embeddings.GetModel()
	chunks, err := s.documents.ListChunksForEmbedding(documentID)
	if err != nil {
		return err
	}
	_, err = s.embedChunks(chunks, model, false)
	return err
}

// SyncMemories mirrors syncMemories(memoryIds).
func (s *EmbeddingSyncService) SyncMemories(memoryIDs []string) error {
	if !s.embeddings.IsEnabled() || len(memoryIDs) == 0 {
		return nil
	}
	model := s.embeddings.GetModel()
	memories, err := s.memories.ListForEmbedding(memoryIDs)
	if err != nil {
		return err
	}
	_, err = s.embedMemories(memories, model, false)
	return err
}

// SyncMissing embeds only the chunks and memories that have no embedding for the
// active model — those left behind by an earlier failure, and everything after a
// model switch. It is the startup catch-up; RebuildAll stays the explicit rebuild.
// Callers outside tests go through RequestSyncMissing so it never overlaps another
// corpus pass.
func (s *EmbeddingSyncService) SyncMissing() error {
	if !s.embeddings.IsEnabled() {
		return nil
	}
	model := s.embeddings.GetModel()

	chunks, err := s.documents.ListChunksMissingEmbedding(model)
	if err != nil {
		return err
	}
	if _, err := s.embedChunks(chunks, model, false); err != nil {
		return err
	}
	memories, err := s.memories.ListMissingEmbedding(model)
	if err != nil {
		return err
	}
	_, err = s.embedMemories(memories, model, false)
	return err
}

// RebuildAll mirrors rebuildAll: re-embed every chunk and memory. Callers outside
// tests go through RequestRebuild so it never overlaps another corpus pass.
func (s *EmbeddingSyncService) RebuildAll() error {
	_, err := s.rebuildAll()
	return err
}

// rebuildAll reports whether the rebuild replaced the corpus: it ran, the model
// held, neither half failed outright, and the endpoint stayed reachable. Inputs
// the endpoint rejects one by one do not count against it — keeping the rebuild
// owed for them would turn every later gap fill into a full rebuild for good.
// They lose their old vectors instead (embedChunks), so gap fills retry them.
func (s *EmbeddingSyncService) rebuildAll() (bool, error) {
	if !s.embeddings.IsEnabled() {
		return false, nil
	}
	model := s.embeddings.GetModel()

	chunks, err := s.documents.ListChunksForEmbedding("")
	if err != nil {
		return false, err
	}
	chunksDone, err := s.embedChunks(chunks, model, true)
	if err != nil {
		return false, err
	}
	memories, err := s.memories.ListForEmbedding(nil)
	if err != nil {
		return false, err
	}
	memoriesDone, err := s.embedMemories(memories, model, true)
	if err != nil {
		return false, err
	}
	return chunksDone && memoriesDone && s.embeddings.IsEnabled(), nil
}

// embedChunks embeds chunks and stores the vectors under model, the model that was
// active when the caller started. The vectors are dropped if the active model has
// moved on by the time they are ready: they may then come from either model, and
// one row per chunk means storing them would overwrite whatever the new model's
// pass stores. The switch that moved the model schedules that pass itself.
// It reports whether the vectors were stored (trivially so for no chunks).
//
// With dropFailed (a rebuild), a chunk left without a vector also loses the one
// it had: a rebuild may follow a source change that kept the model name, and the
// old vector would then look current to SyncMissing for good. Without it the
// chunk is missing, so the next gap fill retries it.
func (s *EmbeddingSyncService) embedChunks(chunks []repository.ChunkForEmbedding, model string, dropFailed bool) (bool, error) {
	if len(chunks) == 0 {
		return true, nil
	}
	inputs := make([]string, len(chunks))
	for i, chunk := range chunks {
		inputs[i] = buildDocumentChunkEmbeddingText(chunk.Title, chunk.Note, chunk.Tags, chunk.DerivedText, chunk.Content)
	}
	vectors := s.embedInBatches(inputs, model)
	if vectors == nil || !s.modelStillActive(model) {
		return false, nil
	}
	if err := s.documents.UpsertChunkEmbeddings(toChunkEmbeddings(chunks, vectors, model)); err != nil {
		return false, err
	}
	if dropFailed {
		failed := []string{}
		for i, chunk := range chunks {
			if vectors[i] == nil {
				failed = append(failed, chunk.ChunkID)
			}
		}
		if err := s.documents.DeleteChunkEmbeddings(failed); err != nil {
			return false, err
		}
	}
	return true, nil
}

// embedMemories is the memory counterpart of embedChunks.
func (s *EmbeddingSyncService) embedMemories(memories []repository.MemoryForEmbedding, model string, dropFailed bool) (bool, error) {
	if len(memories) == 0 {
		return true, nil
	}
	inputs := make([]string, len(memories))
	for i, memory := range memories {
		inputs[i] = buildMemoryEmbeddingText(memory.Kind, memory.Title, memory.Content)
	}
	vectors := s.embedInBatches(inputs, model)
	if vectors == nil || !s.modelStillActive(model) {
		return false, nil
	}
	if err := s.memories.UpsertMemoryEmbeddings(toMemoryEmbeddings(memories, vectors, model)); err != nil {
		return false, err
	}
	if dropFailed {
		failed := []string{}
		for i, memory := range memories {
			if vectors[i] == nil {
				failed = append(failed, memory.ID)
			}
		}
		if err := s.memories.DeleteMemoryEmbeddings(failed); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *EmbeddingSyncService) modelStillActive(model string) bool {
	if current := s.embeddings.GetModel(); current != model {
		log.Printf("Embedding model changed from %q to %q mid-pass; discarded the vectors computed for %q", model, current, model)
		return false
	}
	return true
}

// toChunkEmbeddings pairs chunks with their vectors, leaving out the chunks whose
// vector is nil (the input failed on its own; see embedInBatches).
func toChunkEmbeddings(chunks []repository.ChunkForEmbedding, vectors [][]float64, model string) []repository.ChunkEmbedding {
	out := make([]repository.ChunkEmbedding, 0, len(chunks))
	for i, chunk := range chunks {
		if vectors[i] == nil {
			continue
		}
		out = append(out, repository.ChunkEmbedding{
			ChunkID:    chunk.ChunkID,
			DocumentID: chunk.DocumentID,
			ProjectID:  chunk.ProjectID,
			Embedding:  vectors[i],
			Model:      model,
		})
	}
	return out
}

// toMemoryEmbeddings is the memory counterpart of toChunkEmbeddings.
func toMemoryEmbeddings(memories []repository.MemoryForEmbedding, vectors [][]float64, model string) []repository.MemoryEmbedding {
	out := make([]repository.MemoryEmbedding, 0, len(memories))
	for i, memory := range memories {
		if vectors[i] == nil {
			continue
		}
		out = append(out, repository.MemoryEmbedding{
			MemoryID:  memory.ID,
			ProjectID: memory.ProjectID,
			Kind:      memory.Kind,
			Embedding: vectors[i],
			Model:     model,
		})
	}
	return out
}

// embedInBatches embeds inputs in batches of 32 and returns one vector per input.
// It gives up and returns nil once the active model is no longer model, since the
// remaining batches would be computed by a different model than the one they are
// to be stored under.
// A failed batch is retried one input at a time, so an input the endpoint rejects
// (e.g. one longer than the model accepts) leaves a nil at its own position rather
// than dropping its 31 neighbours. Once the client disables itself the remaining
// inputs stay nil. It returns nil only when no input was embedded.
//
// All inputs here are stored CORPUS items (document chunks and memories), so when
// the active model uses an asymmetric prefix scheme (ruri) they get the DOCUMENT
// prefix — queries get the query prefix in retrieval.go instead. The prefix is baked
// into the stored vectors, so changing it requires a RebuildAll.
func (s *EmbeddingSyncService) embedInBatches(inputs []string, model string) [][]float64 {
	docPrefix := s.embeddings.ActivePrefixScheme().Document
	vectors := make([][]float64, 0, len(inputs))
	embedded := 0
	for start := 0; start < len(inputs); start += embeddingBatchSize {
		if !s.modelStillActive(model) {
			return nil
		}
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
			vecs = make([][]float64, len(batch))
			for i, in := range batch {
				if !s.embeddings.IsEnabled() {
					break
				}
				vecs[i] = s.embeddings.CreateEmbedding(in)
			}
		}
		for _, v := range vecs {
			if v != nil {
				embedded++
			}
		}
		vectors = append(vectors, vecs...)
	}
	if embedded == 0 {
		return nil
	}
	return vectors
}
