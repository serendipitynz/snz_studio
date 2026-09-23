package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/db"
	"snzstudio/internal/repository"
)

type contentDocument struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Note        string   `json:"note"`
	Tags        []string `json:"tags"`
	DerivedText string   `json:"derivedText"`
}

func createImageDocument(t *testing.T, h http.Handler, projectID string) contentDocument {
	t.Helper()
	rec := doMultipart(t, h, "/api/projects/"+projectID+"/documents", map[string]string{"type": "image", "title": "pic.png"},
		"file", "pic.png", []byte("\x89PNG\r\n\x1a\n"), "image/png")
	wantStatus(t, rec, http.StatusCreated)
	var doc contentDocument
	unmarshalField(t, decodeJSONMap(t, rec), "document", &doc)
	return doc
}

func TestUpdateDocumentContent(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "p")
	image := createImageDocument(t, h, projectID)
	target := "/api/documents/" + image.ID + "/content"

	rec := doJSON(t, h, "PATCH", target, map[string]any{"note": "n", "tags": "a, b ,"})
	wantError(t, rec, http.StatusBadRequest, "note, tags and derivedText must be strings")

	rec = doJSON(t, h, "PATCH", "/api/documents/nope/content", map[string]any{"note": "", "tags": "", "derivedText": ""})
	wantError(t, rec, http.StatusNotFound, "document not found")

	rec = doMultipart(t, h, "/api/projects/"+projectID+"/documents", map[string]string{"type": "text", "content": "body"}, "", "", nil, "")
	wantStatus(t, rec, http.StatusCreated)
	var text contentDocument
	unmarshalField(t, decodeJSONMap(t, rec), "document", &text)
	rec = doJSON(t, h, "PATCH", "/api/documents/"+text.ID+"/content", map[string]any{"note": "n", "tags": "", "derivedText": ""})
	wantError(t, rec, http.StatusBadRequest, "only image documents can be edited")

	rec = doJSON(t, h, "PATCH", target, map[string]any{"note": "メモ", "tags": "a, b ,", "derivedText": "世界設定と魔法体系の詳細"})
	wantStatus(t, rec, http.StatusOK)
	var updated contentDocument
	unmarshalField(t, decodeJSONMap(t, rec), "document", &updated)
	if updated.Note != "メモ" || updated.DerivedText != "世界設定と魔法体系の詳細" || len(updated.Tags) != 2 || updated.Tags[1] != "b" {
		t.Fatalf("updated = %+v", updated)
	}
	// The image was created in misc; the response already carries the category
	// inferred from the new description, which is what the UI's draft follows.
	if image.Category != "misc" || updated.Category != "world" {
		t.Fatalf("category %q -> %q, want misc -> world", image.Category, updated.Category)
	}
}

// newEmbeddingTestServer is newTestServer with embeddings enabled against
// embeddingBaseURL. The DB is returned too, for counting stored vectors.
func newEmbeddingTestServer(t *testing.T, embeddingBaseURL string) (*Server, *sql.DB) {
	t.Helper()
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
			EmbeddingBaseURL:  embeddingBaseURL,
			EmbeddingModel:    "test-embedding-model",
		},
		LLMTimeoutMs:       500,
		EmbeddingTimeoutMs: 500,
	}, filepath.Join(dir, "app-config.json"))
	return NewServer(d, cfg, filepath.Join(dir, "uploads"), nil), d
}

func documentEmbeddingCount(t *testing.T, d *sql.DB, documentID string) int {
	t.Helper()
	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM document_chunk_embeddings WHERE document_id = ?", documentID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestUpdateDocumentContentResyncsEmbeddings(t *testing.T) {
	var lastInputs []string
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		lastInputs = body.Input
		data := make([]map[string]any, len(body.Input))
		for i := range body.Input {
			data[i] = map[string]any{"embedding": []float64{0.5, 0.5}}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
	}))
	t.Cleanup(embeddingServer.Close)
	srv, d := newEmbeddingTestServer(t, embeddingServer.URL)
	h := srv.Handler()

	image := createImageDocument(t, h, createProject(t, h, "p"))
	rec := doJSON(t, h, "PATCH", "/api/documents/"+image.ID+"/content", map[string]any{"note": "", "tags": "", "derivedText": "夕暮れの桟橋"})
	wantStatus(t, rec, http.StatusOK)

	if !strings.Contains(strings.Join(lastInputs, "\n"), "夕暮れの桟橋") {
		t.Fatalf("the last embedding request did not carry the updated text: %q", lastInputs)
	}
	chunks, err := srv.documents.ListChunksForEmbedding(image.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := documentEmbeddingCount(t, d, image.ID); got == 0 || got != len(chunks) {
		t.Fatalf("embeddings after update = %d, want one per chunk (%d)", got, len(chunks))
	}
}

// An update is saved even when the embedding endpoint cannot be reached, and
// leaves the document without vectors rather than with ones for its old text.
func TestUpdateDocumentContentSurvivesEmbeddingFailure(t *testing.T) {
	srv, d := newEmbeddingTestServer(t, "http://127.0.0.1:1/v1")
	h := srv.Handler()

	image := createImageDocument(t, h, createProject(t, h, "p"))
	chunks, err := srv.documents.ListChunksForEmbedding(image.ID)
	if err != nil || len(chunks) == 0 {
		t.Fatalf("chunks = %d, %v", len(chunks), err)
	}
	if err := srv.documents.UpsertChunkEmbeddings([]repository.ChunkEmbedding{{
		ChunkID: chunks[0].ChunkID, DocumentID: image.ID, ProjectID: chunks[0].ProjectID, Embedding: []float64{1}, Model: "old",
	}}); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, h, "PATCH", "/api/documents/"+image.ID+"/content", map[string]any{"note": "", "tags": "", "derivedText": "桟橋"})
	wantStatus(t, rec, http.StatusOK)
	var body struct {
		Document contentDocument `json:"document"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Document.DerivedText != "桟橋" {
		t.Fatalf("body = %s, %v", rec.Body.String(), err)
	}
	if got := documentEmbeddingCount(t, d, image.ID); got != 0 {
		t.Errorf("embeddings after a failed sync = %d, want 0", got)
	}
}
