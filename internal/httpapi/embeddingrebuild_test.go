package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/db"
	"snzstudio/internal/repository"
)

// newExternalEmbeddingServer builds a Server in external embedding mode whose
// endpoint counts the inputs it embeds, with one document and one memory stored.
func newExternalEmbeddingServer(t *testing.T) (*Server, *atomic.Int64) {
	t.Helper()
	var embedded atomic.Int64
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		data := make([]map[string]any, len(body.Input))
		for i := range body.Input {
			data[i] = map[string]any{"embedding": []float64{0.5, 0.5}}
		}
		embedded.Add(int64(len(body.Input)))
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
	}))
	t.Cleanup(endpoint.Close)

	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "app.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	cfg := config.New(config.Settings{
		Editable: config.Editable{
			LLMBaseURL:        "http://127.0.0.1:1/v1",
			LLMModel:          "test-model",
			LLMResponseFormat: "standard",
			ReviewBaseURL:     "http://127.0.0.1:1/v1",
			ReviewModel:       "test-model",
			EmbeddingMode:     "external",
			EmbeddingBaseURL:  endpoint.URL + "/v1",
			EmbeddingModel:    "m",
		},
		LLMTimeoutMs:       500,
		EmbeddingTimeoutMs: 500,
	}, filepath.Join(dir, "app-config.json"))
	srv := NewServer(d, cfg, filepath.Join(dir, "uploads"), nil)
	t.Cleanup(srv.embeddingSync.WaitIdle)

	project, err := srv.projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID: project.ID, Type: "text", Title: "港町", ContentText: "港町の本文。",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID: project.ID, Kind: "semantic", Title: "舞台", Content: "浮遊大陸。",
	}); err != nil {
		t.Fatal(err)
	}
	return srv, &embedded
}

func configurationBody(editable config.Editable) map[string]any {
	return map[string]any{
		"llmBaseUrl":        editable.LLMBaseURL,
		"llmModel":          editable.LLMModel,
		"llmResponseFormat": editable.LLMResponseFormat,
		"reviewBaseUrl":     editable.ReviewBaseURL,
		"reviewModel":       editable.ReviewModel,
		"embeddingMode":     editable.EmbeddingMode,
		"embeddingBaseUrl":  editable.EmbeddingBaseURL,
		"embeddingModel":    editable.EmbeddingModel,
	}
}

func TestSavingConfigurationRebuildsEmbeddingsOnlyWhenTheSourceChanges(t *testing.T) {
	srv, embedded := newExternalEmbeddingServer(t)
	h := srv.Handler()

	srv.embeddingSync.RequestRebuild()
	srv.embeddingSync.WaitIdle()
	if got := embedded.Load(); got != 2 {
		t.Fatalf("initial rebuild embedded %d inputs, want 2", got)
	}

	embedded.Store(0)
	saved := srv.cfg.GetEditable()
	saved.LLMModel = "another-chat-model"
	wantStatus(t, doJSON(t, h, "PUT", "/api/configuration", configurationBody(saved)), http.StatusOK)
	srv.embeddingSync.WaitIdle()
	if got := embedded.Load(); got != 0 {
		t.Fatalf("a save that left the embedding fields alone embedded %d inputs, want 0", got)
	}

	saved.EmbeddingModel = "m2"
	wantStatus(t, doJSON(t, h, "PUT", "/api/configuration", configurationBody(saved)), http.StatusOK)
	srv.embeddingSync.WaitIdle()
	if got := embedded.Load(); got != 2 {
		t.Fatalf("changing the embedding model embedded %d inputs, want 2 (a full rebuild)", got)
	}
}

func TestRebuildEmbeddingsEndpoint(t *testing.T) {
	srv, embedded := newExternalEmbeddingServer(t)
	h := srv.Handler()

	wantStatus(t, doJSON(t, h, "POST", "/api/embedding/rebuild", nil), http.StatusAccepted)
	srv.embeddingSync.WaitIdle()
	if got := embedded.Load(); got != 2 {
		t.Fatalf("rebuild embedded %d inputs, want 2", got)
	}

	// Nothing to rebuild with: the default harness has embeddings disabled.
	wantStatus(t, doJSON(t, newTestServer(t).Handler(), "POST", "/api/embedding/rebuild", nil), http.StatusConflict)
}

func TestEmbeddingSourceChanged(t *testing.T) {
	external := config.Editable{EmbeddingMode: "external", EmbeddingBaseURL: "http://a/v1", EmbeddingModel: "m"}
	internal := config.Editable{EmbeddingMode: "internal", EmbeddingBaseURL: "http://a/v1", EmbeddingModel: "m"}
	cases := []struct {
		name          string
		before, after config.Editable
		want          bool
	}{
		{"external unchanged", external, external, false},
		{"external model", external, config.Editable{EmbeddingMode: "external", EmbeddingBaseURL: "http://a/v1", EmbeddingModel: "m2"}, true},
		{"external endpoint", external, config.Editable{EmbeddingMode: "external", EmbeddingBaseURL: "http://b/v1", EmbeddingModel: "m"}, true},
		{"mode", external, internal, true},
		{"internal ignores the external fields", internal, config.Editable{EmbeddingMode: "internal", EmbeddingBaseURL: "http://b/v1", EmbeddingModel: "m2"}, false},
	}
	for _, c := range cases {
		if got := embeddingSourceChanged(c.before, c.after); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// Moving to another endpoint that serves the same model name leaves the old
// endpoint's vectors looking current to SyncMissing, so a rebuild that failed
// must still run on a later save once the endpoint answers.
func TestFailedSourceChangeRebuildRunsOnTheNextSave(t *testing.T) {
	srv, _ := newExternalEmbeddingServer(t)
	h := srv.Handler()
	srv.embeddingSync.RequestRebuild()
	srv.embeddingSync.WaitIdle()

	var failing atomic.Bool
	failing.Store(true)
	var embedded atomic.Int64
	moved := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		if failing.Load() {
			http.Error(w, `{"error":{"message":"loading model"}}`, http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		data := make([]map[string]any, len(body.Input))
		for i := range body.Input {
			data[i] = map[string]any{"embedding": []float64{0.1, 0.9}}
		}
		embedded.Add(int64(len(body.Input)))
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
	}))
	t.Cleanup(moved.Close)

	saved := srv.cfg.GetEditable()
	saved.EmbeddingBaseURL = moved.URL + "/v1"
	wantStatus(t, doJSON(t, h, "PUT", "/api/configuration", configurationBody(saved)), http.StatusOK)
	srv.embeddingSync.WaitIdle()
	if got := embedded.Load(); got != 0 {
		t.Fatalf("the failing endpoint embedded %d inputs", got)
	}

	failing.Store(false)
	wantStatus(t, doJSON(t, h, "PUT", "/api/configuration", configurationBody(saved)), http.StatusOK)
	srv.embeddingSync.WaitIdle()
	if got := embedded.Load(); got != 2 {
		t.Fatalf("the save after recovery embedded %d inputs, want 2 (the owed rebuild)", got)
	}

	// Once the rebuild has gone through, an unchanged save is a gap fill again.
	embedded.Store(0)
	wantStatus(t, doJSON(t, h, "PUT", "/api/configuration", configurationBody(saved)), http.StatusOK)
	srv.embeddingSync.WaitIdle()
	if got := embedded.Load(); got != 0 {
		t.Fatalf("an unchanged save after a completed rebuild embedded %d inputs, want 0", got)
	}
}

func rebuildState(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := doJSON(t, h, "GET", "/api/embedding/rebuild", nil)
	wantStatus(t, rec, http.StatusOK)
	var out struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode rebuild state: %v (body=%s)", err, rec.Body.String())
	}
	return out.State
}

// The settings screen holds its rebuild button on this state, so a rebuild that
// a settings save started has to read as running just like one the button did.
func TestRebuildStateCoversASaveThatStartsARebuild(t *testing.T) {
	srv, _ := newExternalEmbeddingServer(t)
	h := srv.Handler()
	if got := rebuildState(t, h); got != "idle" {
		t.Fatalf("state before any rebuild = %q, want idle", got)
	}

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var released atomic.Bool
	releaseOnce := func() {
		if released.CompareAndSwap(false, true) {
			close(release)
		}
	}
	moved := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		data := make([]map[string]any, len(body.Input))
		for i := range body.Input {
			data[i] = map[string]any{"embedding": []float64{0.1, 0.9}}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
	}))
	t.Cleanup(moved.Close)
	// Runs before moved.Close, which would otherwise wait on a held request.
	t.Cleanup(releaseOnce)

	saved := srv.cfg.GetEditable()
	saved.EmbeddingBaseURL = moved.URL + "/v1"
	wantStatus(t, doJSON(t, h, "PUT", "/api/configuration", configurationBody(saved)), http.StatusOK)
	<-started
	if got := rebuildState(t, h); got != "running" {
		t.Fatalf("state while the save's rebuild runs = %q, want running", got)
	}
	releaseOnce()
	srv.embeddingSync.WaitIdle()
	if got := rebuildState(t, h); got != "done" {
		t.Fatalf("state after the save's rebuild = %q, want done", got)
	}

	if got := rebuildState(t, newTestServer(t).Handler()); got != "idle" {
		t.Fatalf("state with embeddings disabled = %q, want idle", got)
	}
}
