package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/repository"
)

// rejectMarker makes the fake endpoint answer 500 for any request carrying it,
// standing in for llama-server rejecting an input longer than its batch.
const rejectMarker = "REJECT-ME"

// fakeEmbeddingServer serves /embeddings with a fixed 3-dim vector per input, and
// counts the inputs it embedded.
func fakeEmbeddingServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var embedded atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data := []map[string]any{}
		for _, in := range body.Input {
			if strings.Contains(in, rejectMarker) {
				http.Error(w, `{"error":{"message":"input is too large to process"}}`, http.StatusInternalServerError)
				return
			}
			data = append(data, map[string]any{"embedding": []float64{1, 0, 0}})
		}
		embedded.Add(int64(len(body.Input)))
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv, &embedded
}

func enabledEmbeddingClient(baseURL, model string) *EmbeddingClient {
	settings := config.Settings{EmbeddingTimeoutMs: 5000}
	settings.EmbeddingBaseURL = baseURL
	settings.EmbeddingModel = model
	return NewEmbeddingClient(testConfig(settings))
}

func countChunkEmbeddings(t *testing.T, documents *repository.DocumentRepository, documentID string) (total, embedded int) {
	t.Helper()
	chunks, err := documents.ListChunksForEmbedding(documentID)
	if err != nil {
		t.Fatalf("ListChunksForEmbedding: %v", err)
	}
	missing, err := documents.ListChunksMissingEmbedding("m")
	if err != nil {
		t.Fatalf("ListChunksMissingEmbedding: %v", err)
	}
	unembedded := 0
	for _, m := range missing {
		if m.DocumentID == documentID {
			unembedded++
		}
	}
	return len(chunks), len(chunks) - unembedded
}

func TestCreateEmbeddingsRequestFailureKeepsClientEnabled(t *testing.T) {
	srv, _ := fakeEmbeddingServer(t)
	client := enabledEmbeddingClient(srv.URL, "m")

	if got := client.CreateEmbeddings([]string{"ok", rejectMarker}); got != nil {
		t.Fatalf("rejected request returned %v, want nil", got)
	}
	if !client.IsEnabled() {
		t.Fatal("an error response disabled the client")
	}
	if got := client.CreateEmbedding("ok"); got == nil {
		t.Fatal("the request after a rejected one was not embedded")
	}
}

func TestCreateEmbeddingsUnreachableEndpointDisablesClient(t *testing.T) {
	srv, _ := fakeEmbeddingServer(t)
	url := srv.URL
	srv.Close()
	client := enabledEmbeddingClient(url, "m")

	if got := client.CreateEmbedding("ok"); got != nil {
		t.Fatalf("unreachable endpoint returned %v, want nil", got)
	}
	if client.IsEnabled() {
		t.Fatal("an unreachable endpoint left the client enabled")
	}
}

func TestSyncDocumentSkipsOnlyTheRejectedChunk(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	memories := repository.NewMemoryRepository(d)
	srv, _ := fakeEmbeddingServer(t)
	client := enabledEmbeddingClient(srv.URL, "m")
	sync := NewEmbeddingSyncService(documents, memories, client)

	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	// 1200 runes then the marker: the first 1000-rune window is clean, the
	// overlapping second window carries the marker and is rejected.
	long, err := documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID:   project.ID,
		Type:        "text",
		Title:       "長い文書",
		ContentText: strings.Repeat("あ", 1200) + rejectMarker,
	})
	if err != nil {
		t.Fatalf("CreateDocument long: %v", err)
	}
	if err := sync.SyncDocument(long.ID); err != nil {
		t.Fatalf("SyncDocument long: %v", err)
	}
	if total, embedded := countChunkEmbeddings(t, documents, long.ID); total != 2 || embedded != 1 {
		t.Fatalf("long document: %d of %d chunks embedded, want 1 of 2", embedded, total)
	}

	later, err := documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID:   project.ID,
		Type:        "text",
		Title:       "後から追加した文書",
		ContentText: "後から追加した短い本文。",
	})
	if err != nil {
		t.Fatalf("CreateDocument later: %v", err)
	}
	if err := sync.SyncDocument(later.ID); err != nil {
		t.Fatalf("SyncDocument later: %v", err)
	}
	if total, embedded := countChunkEmbeddings(t, documents, later.ID); total != 1 || embedded != 1 {
		t.Fatalf("document added after a rejection: %d of %d chunks embedded, want 1 of 1", embedded, total)
	}
}

func TestSyncMissingEmbedsOnlyTheGaps(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	memories := repository.NewMemoryRepository(d)
	srv, embeddedInputs := fakeEmbeddingServer(t)
	sync := NewEmbeddingSyncService(documents, memories, enabledEmbeddingClient(srv.URL, "m"))

	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	create := func(title string) string {
		doc, err := documents.CreateDocument(repository.CreateDocumentInput{
			ProjectID: project.ID, Type: "text", Title: title, ContentText: title + "の本文。",
		})
		if err != nil {
			t.Fatalf("CreateDocument %s: %v", title, err)
		}
		return doc.ID
	}
	embedded := create("埋め込み済み")
	otherModel := create("別モデルで埋め込み済み")
	missing := create("未埋め込み")
	seed := func(documentID, model string) {
		chunks, err := documents.ListChunksForEmbedding(documentID)
		if err != nil {
			t.Fatalf("ListChunksForEmbedding: %v", err)
		}
		if err := documents.UpsertChunkEmbeddings(toChunkEmbeddings(chunks, [][]float64{{0, 1, 0}}, model)); err != nil {
			t.Fatalf("UpsertChunkEmbeddings: %v", err)
		}
	}
	seed(embedded, "m")
	seed(otherModel, "old-model")
	if _, err := memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID: project.ID, Kind: "semantic", Title: "舞台", Content: "浮遊大陸。",
	}); err != nil {
		t.Fatalf("CreateMemory: %v", err)
	}

	if err := sync.SyncMissing(); err != nil {
		t.Fatalf("SyncMissing: %v", err)
	}
	for _, id := range []string{embedded, otherModel, missing} {
		if total, got := countChunkEmbeddings(t, documents, id); got != total {
			t.Fatalf("document %s: %d of %d chunks embedded after SyncMissing", id, got, total)
		}
	}
	if left, err := memories.ListMissingEmbedding("m"); err != nil || len(left) != 0 {
		t.Fatalf("memories still missing an embedding: %v (err %v)", left, err)
	}
	// Two document chunks and one memory were missing; the already-embedded chunk
	// must not be sent again.
	if n := embeddedInputs.Load(); n != 3 {
		t.Fatalf("endpoint embedded %d inputs, want 3", n)
	}
}

func seedSyncCorpus(t *testing.T, projects *repository.ProjectRepository, documents *repository.DocumentRepository, memories *repository.MemoryRepository) {
	t.Helper()
	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	for _, title := range []string{"港町", "灯台"} {
		if _, err := documents.CreateDocument(repository.CreateDocumentInput{
			ProjectID: project.ID, Type: "text", Title: title, ContentText: title + "の本文。",
		}); err != nil {
			t.Fatalf("CreateDocument %s: %v", title, err)
		}
	}
	if _, err := memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID: project.ID, Kind: "semantic", Title: "舞台", Content: "浮遊大陸。",
	}); err != nil {
		t.Fatalf("CreateMemory: %v", err)
	}
}

func TestCorpusPassesRunOneAtATimeAndCollapseWhileRunning(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	memories := repository.NewMemoryRepository(d)
	seedSyncCorpus(t, projects, documents, memories)

	var inFlight, maxInFlight, embedded atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	var first atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			m := maxInFlight.Load()
			if n <= m || maxInFlight.CompareAndSwap(m, n) {
				break
			}
		}
		if first.CompareAndSwap(false, true) {
			close(started)
			<-release
		}
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		data := make([]map[string]any, len(body.Input))
		for i := range body.Input {
			data[i] = map[string]any{"embedding": []float64{1, 0, 0}}
		}
		embedded.Add(int64(len(body.Input)))
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	sync := NewEmbeddingSyncService(documents, memories, enabledEmbeddingClient(srv.URL, "m"))

	sync.RequestRebuild()
	<-started
	// Startup, the sidecar's ready callback and two settings saves all ask while
	// the first pass is still on its first request.
	sync.RequestSyncMissing()
	sync.RequestRebuild()
	sync.RequestSyncMissing()
	sync.RequestRebuild()
	close(release)
	sync.WaitIdle()

	if got := maxInFlight.Load(); got != 1 {
		t.Fatalf("%d embedding requests ran at once, want 1", got)
	}
	// The running rebuild plus one collapsed rebuild, over 2 chunks and 1 memory.
	if got := embedded.Load(); got != 6 {
		t.Fatalf("endpoint embedded %d inputs, want 6 (two full passes)", got)
	}
}

func TestRebuildDiscardsVectorsWhenTheModelSwitchesMidPass(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	memories := repository.NewMemoryRepository(d)
	seedSyncCorpus(t, projects, documents, memories)

	var cfg *config.Config
	var switched atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		// The sidecar comes back with another model while the first batch is
		// being computed by the old one.
		if switched.CompareAndSwap(false, true) {
			cfg.SetInternalEmbedding("http://"+r.Host, "new-model")
		}
		data := make([]map[string]any, len(body.Input))
		for i := range body.Input {
			data[i] = map[string]any{"embedding": []float64{1, 0, 0}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	settings := config.Settings{EmbeddingTimeoutMs: 5000}
	settings.EmbeddingMode = "internal"
	cfg = testConfig(settings)
	cfg.SetInternalEmbedding(srv.URL, "old-model")
	client := NewEmbeddingClient(cfg)
	sync := NewEmbeddingSyncService(documents, memories, client)

	if err := sync.RebuildAll(); err != nil {
		t.Fatalf("RebuildAll: %v", err)
	}
	for _, model := range []string{"old-model", "new-model"} {
		chunks, err := documents.ListChunksMissingEmbedding(model)
		if err != nil {
			t.Fatalf("ListChunksMissingEmbedding: %v", err)
		}
		mems, err := memories.ListMissingEmbedding(model)
		if err != nil {
			t.Fatalf("ListMissingEmbedding: %v", err)
		}
		if len(chunks) != 2 || len(mems) != 1 {
			t.Fatalf("model %s: %d chunks and %d memories missing, want 2 and 1 (nothing stored)", model, len(chunks), len(mems))
		}
	}

	// The pass the switch schedules stores everything under the new model.
	if err := sync.RebuildAll(); err != nil {
		t.Fatalf("RebuildAll after the switch: %v", err)
	}
	if chunks, err := documents.ListChunksMissingEmbedding("new-model"); err != nil || len(chunks) != 0 {
		t.Fatalf("chunks missing a new-model embedding: %d (err %v)", len(chunks), err)
	}
}
