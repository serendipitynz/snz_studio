package service

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"snzstudio/internal/config"
	"snzstudio/internal/repository"
)

// switchableEndpoint is the state of a switchableEmbeddingServer.
type switchableEndpoint struct {
	// down drops every connection unanswered, which the client sees the way it
	// sees an endpoint it cannot reach.
	down atomic.Bool
	// loading answers 503, as llama-server does while it loads its model.
	loading  atomic.Bool
	requests atomic.Int64
}

// switchableEmbeddingServer serves /embeddings like fakeEmbeddingServer unless the
// returned state says otherwise, and counts every request it receives.
func switchableEmbeddingServer(t *testing.T) (*httptest.Server, *switchableEndpoint) {
	t.Helper()
	state := &switchableEndpoint{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.requests.Add(1)
		if state.down.Load() {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		if state.loading.Load() {
			http.Error(w, `{"error":{"message":"Loading model"}}`, http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		data := make([]map[string]any, len(body.Input))
		for i := range body.Input {
			data[i] = map[string]any{"embedding": []float64{1, 0, 0}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv, state
}

// logBuffer collects log output; probes log from their own goroutine.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLog(t *testing.T) *logBuffer {
	t.Helper()
	buf := &logBuffer{}
	previous := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(previous) })
	return buf
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestUnreachableClientReconnectsWithoutASettingsSave(t *testing.T) {
	srv, endpoint := switchableEmbeddingServer(t)
	client := enabledEmbeddingClient(srv.URL, "m")
	client.retryInterval = 20 * time.Millisecond
	var reconnects atomic.Int64
	client.SetOnReconnect(func() { reconnects.Add(1) })

	endpoint.down.Store(true)
	if got := client.CreateEmbedding("ok"); got != nil {
		t.Fatalf("unreachable endpoint returned %v, want nil", got)
	}
	if client.IsEnabled() {
		t.Fatal("an unreachable endpoint left the client enabled")
	}

	endpoint.down.Store(false)
	waitFor(t, "the client to re-enable itself", client.IsEnabled)
	if got := client.CreateEmbedding("ok"); got == nil {
		t.Fatal("the reconnected client did not embed")
	}
	if n := reconnects.Load(); n != 1 {
		t.Fatalf("onReconnect ran %d times, want 1", n)
	}
}

func TestUnreachableClientDoesNotRetryPerRequestOrLogEachProbe(t *testing.T) {
	logs := captureLog(t)
	srv, endpoint := switchableEmbeddingServer(t)
	client := enabledEmbeddingClient(srv.URL, "m")
	client.retryInterval = 100 * time.Millisecond

	endpoint.down.Store(true)
	client.CreateEmbedding("ok")
	afterDisable := endpoint.requests.Load()
	for i := 0; i < 1000; i++ {
		if got := client.CreateEmbedding("ok"); got != nil {
			t.Fatalf("disabled client returned %v, want nil", got)
		}
	}
	if n := endpoint.requests.Load() - afterDisable; n != 0 {
		t.Fatalf("1000 requests while disabled reached the endpoint %d times, want 0", n)
	}

	time.Sleep(450 * time.Millisecond)
	if probes := endpoint.requests.Load() - afterDisable; probes < 1 || probes > 6 {
		t.Fatalf("%d probes in 450ms at a 100ms interval, want about 4", probes)
	}

	endpoint.down.Store(false)
	// The reconnect is logged just after the client re-enables.
	waitFor(t, "the reconnect to be logged", func() bool { return strings.Contains(logs.String(), "embedding again") })
	output := logs.String()
	if n := strings.Count(output, "\n"); n != 2 {
		t.Fatalf("logged %d lines across the outage, want 2 (disabled, re-enabled):\n%s", n, output)
	}
	if !strings.Contains(output, "Embedding retrieval disabled") || !strings.Contains(output, "embedding again") {
		t.Fatalf("log does not record both the disable and the reconnect:\n%s", output)
	}
}

func TestReconfiguringCancelsTheReconnectProbe(t *testing.T) {
	srv, endpoint := switchableEmbeddingServer(t)
	cfg := testConfig(config.Settings{Editable: config.Editable{EmbeddingMode: "internal"}, EmbeddingTimeoutMs: 5000})
	cfg.SetInternalEmbedding(srv.URL, "m")
	client := NewEmbeddingClient(cfg)
	client.retryInterval = 20 * time.Millisecond
	var reconnects atomic.Int64
	client.SetOnReconnect(func() { reconnects.Add(1) })

	endpoint.down.Store(true)
	client.CreateEmbedding("ok")
	// The sidecar-lost path: the overlay goes, and with it the model.
	cfg.ClearInternalEmbedding()
	client.RefreshConfiguration()
	endpoint.down.Store(false)

	time.Sleep(150 * time.Millisecond)
	if client.IsEnabled() {
		t.Fatal("a probe from before the refresh re-enabled a client with no model")
	}
	if n := reconnects.Load(); n != 0 {
		t.Fatalf("onReconnect ran %d times after the refresh, want 0", n)
	}
}

func TestReconnectEmbedsWhatWasSavedWhileUnreachable(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	memories := repository.NewMemoryRepository(d)
	srv, endpoint := switchableEmbeddingServer(t)
	client := enabledEmbeddingClient(srv.URL, "m")
	client.retryInterval = 20 * time.Millisecond
	sync := NewEmbeddingSyncService(documents, memories, client)
	client.SetOnReconnect(sync.RequestSyncMissing)
	t.Cleanup(sync.WaitIdle)

	endpoint.down.Store(true)
	client.CreateEmbedding("ok")
	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	doc, err := documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID: project.ID, Type: "text", Title: "港町", ContentText: "港町の本文。",
	})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	memory, err := memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID: project.ID, Kind: "semantic", Title: "舞台", Content: "浮遊大陸。",
	})
	if err != nil {
		t.Fatalf("CreateMemory: %v", err)
	}
	// What the save handlers do; both skip while the client is disabled.
	if err := sync.SyncDocument(doc.ID); err != nil {
		t.Fatalf("SyncDocument: %v", err)
	}
	if err := sync.SyncMemories([]string{memory.ID}); err != nil {
		t.Fatalf("SyncMemories: %v", err)
	}
	if _, got := countChunkEmbeddings(t, documents, doc.ID); got != 0 {
		t.Fatalf("%d chunks embedded while the endpoint was down, want 0", got)
	}

	endpoint.down.Store(false)
	waitFor(t, "the document and memory to be embedded", func() bool {
		total, got := countChunkEmbeddings(t, documents, doc.ID)
		left, err := memories.ListMissingEmbedding("m")
		return err == nil && total > 0 && got == total && len(left) == 0
	})
}

// An endpoint that restarts answers 503 while it loads its model. Reconnecting on
// that answer would start the gap fill against an endpoint that rejects every
// input, and nothing would retry it once it failed.
func TestReconnectWaitsForTheEndpointToEmbedAgain(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	memories := repository.NewMemoryRepository(d)
	srv, endpoint := switchableEmbeddingServer(t)
	client := enabledEmbeddingClient(srv.URL, "m")
	client.retryInterval = 20 * time.Millisecond
	sync := NewEmbeddingSyncService(documents, memories, client)
	client.SetOnReconnect(sync.RequestSyncMissing)
	t.Cleanup(sync.WaitIdle)

	endpoint.down.Store(true)
	client.CreateEmbedding("ok")
	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID: project.ID, Kind: "semantic", Title: "舞台", Content: "浮遊大陸。",
	}); err != nil {
		t.Fatalf("CreateMemory: %v", err)
	}

	endpoint.loading.Store(true)
	endpoint.down.Store(false)
	before := endpoint.requests.Load()
	waitFor(t, "probes against the loading endpoint", func() bool { return endpoint.requests.Load()-before >= 3 })
	if client.IsEnabled() {
		t.Fatal("a 503 from a loading endpoint re-enabled the client")
	}

	endpoint.loading.Store(false)
	waitFor(t, "the memory saved during the outage to be embedded", func() bool {
		left, err := memories.ListMissingEmbedding("m")
		return err == nil && len(left) == 0
	})
}
