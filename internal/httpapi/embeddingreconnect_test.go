package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"snzstudio/internal/repository"
)

// The settings screen's probe finding the endpoint back must not leave the
// client disabled behind a "connected" badge, and what was saved meanwhile must
// get its vectors without a settings save.
func TestReadingConfigurationReconnectsAnUnreachableEmbeddingEndpoint(t *testing.T) {
	var down atomic.Bool
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				_ = conn.Close()
			}
			return
		}
		switch r.URL.Path {
		case "/v1/models":
			writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]string{{"id": "m"}}})
		case "/v1/embeddings":
			var body struct {
				Input []string `json:"input"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			data := make([]map[string]any, len(body.Input))
			for i := range body.Input {
				data[i] = map[string]any{"embedding": []float64{0.5, 0.5}}
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": data})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(endpoint.Close)
	srv := newServerOnEmbeddingEndpoint(t, endpoint.URL+"/v1")
	h := srv.Handler()

	down.Store(true)
	if got := srv.embedding.CreateEmbedding("probe"); got != nil {
		t.Fatalf("unreachable endpoint returned %v, want nil", got)
	}
	projects, err := srv.projects.ListProjects()
	if err != nil || len(projects) == 0 {
		t.Fatalf("ListProjects: %v (%d projects)", err, len(projects))
	}
	doc, err := srv.documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID: projects[0].ID, Type: "text", Title: "灯台", ContentText: "灯台の本文。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.embeddingSync.SyncDocument(doc.ID); err != nil {
		t.Fatal(err)
	}

	readConnected := func() bool {
		t.Helper()
		rec := doJSON(t, h, "GET", "/api/configuration", nil)
		wantStatus(t, rec, http.StatusOK)
		var body struct {
			Configuration struct {
				EmbeddingConnected bool `json:"embeddingConnected"`
			} `json:"configuration"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Configuration.EmbeddingConnected
	}

	if readConnected() || srv.embedding.IsEnabled() {
		t.Fatal("while the endpoint is down the settings screen and the client must both report it unavailable")
	}

	down.Store(false)
	if !readConnected() {
		t.Fatal("the settings screen did not see the endpoint come back")
	}
	if !srv.embedding.IsEnabled() {
		t.Fatal("the settings screen reports the endpoint connected while the client stays disabled")
	}
	srv.embeddingSync.WaitIdle()
	missingChunks, err := srv.documents.ListChunksMissingEmbedding("m")
	if err != nil {
		t.Fatal(err)
	}
	missingMemories, err := srv.memories.ListMissingEmbedding("m")
	if err != nil {
		t.Fatal(err)
	}
	if len(missingChunks) != 0 || len(missingMemories) != 0 {
		t.Fatalf("after reconnecting, %d chunks and %d memories still lack a vector", len(missingChunks), len(missingMemories))
	}
}
