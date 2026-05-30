package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/db"
)

// newTestServer builds a Server backed by a fresh temp SQLite DB and a config
// whose LLM/embedding endpoints point at a closed loopback port (so connection
// checks fail fast and chat falls back) with embeddings disabled (empty model).
// This is the endpoint-less harness from HANDOFF §5: every route except the LLM
// happy paths can be exercised without a live model server.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "app.sqlite"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	settings := config.Settings{
		Editable: config.Editable{
			LLMBaseURL:        "http://127.0.0.1:1/v1",
			LLMModel:          "test-model",
			LLMResponseFormat: "standard",
			ReviewBaseURL:     "http://127.0.0.1:1/v1",
			ReviewModel:       "test-model",
			EmbeddingBaseURL:  "http://127.0.0.1:1/v1",
			EmbeddingModel:    "",
		},
		LLMTimeoutMs:       500,
		EmbeddingTimeoutMs: 500,
	}
	cfg := config.New(settings, filepath.Join(dir, "app-config.json"))
	return NewServer(d, cfg, filepath.Join(dir, "uploads"))
}

func doJSON(t *testing.T, h http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeJSONMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	out := map[string]json.RawMessage{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v (status=%d body=%s)", err, rec.Code, rec.Body.String())
	}
	return out
}

func unmarshalField(t *testing.T, m map[string]json.RawMessage, key string, dst any) {
	t.Helper()
	raw, ok := m[key]
	if !ok {
		t.Fatalf("response missing key %q (keys=%v)", key, mapKeys(m))
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("unmarshal field %q: %v", key, err)
	}
}

func mapKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d (body=%s)", rec.Code, want, rec.Body.String())
	}
}

func wantError(t *testing.T, rec *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	wantStatus(t, rec, status)
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v (body=%s)", err, rec.Body.String())
	}
	if body.Error != message {
		t.Fatalf("error = %q, want %q", body.Error, message)
	}
}

// createProject is a helper that creates a project and returns its id.
func createProject(t *testing.T, h http.Handler, title string) string {
	t.Helper()
	rec := doJSON(t, h, "POST", "/api/projects", map[string]any{"title": title})
	wantStatus(t, rec, http.StatusCreated)
	m := decodeJSONMap(t, rec)
	var project struct {
		ID string `json:"id"`
	}
	unmarshalField(t, m, "project", &project)
	if project.ID == "" {
		t.Fatal("created project has empty id")
	}
	return project.ID
}

func createChat(t *testing.T, h http.Handler, projectID string) string {
	t.Helper()
	rec := doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats", map[string]any{"title": "Chat"})
	wantStatus(t, rec, http.StatusCreated)
	m := decodeJSONMap(t, rec)
	var chat struct {
		ID string `json:"id"`
	}
	unmarshalField(t, m, "chat", &chat)
	return chat.ID
}

func TestHealth(t *testing.T) {
	h := newTestServer(t).Handler()
	rec := doJSON(t, h, "GET", "/api/health", nil)
	wantStatus(t, rec, http.StatusOK)
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.OK {
		t.Fatalf("health body=%s err=%v", rec.Body.String(), err)
	}
}

func TestGetConfiguration(t *testing.T) {
	h := newTestServer(t).Handler()
	rec := doJSON(t, h, "GET", "/api/configuration", nil)
	wantStatus(t, rec, http.StatusOK)

	m := decodeJSONMap(t, rec)
	var cfg struct {
		LLMModel           string `json:"llmModel"`
		LLMResponseFormat  string `json:"llmResponseFormat"`
		LLMConnected       bool   `json:"llmConnected"`
		ReviewConnected    bool   `json:"reviewConnected"`
		EmbeddingConnected bool   `json:"embeddingConnected"`
	}
	unmarshalField(t, m, "configuration", &cfg)
	if cfg.LLMModel != "test-model" || cfg.LLMResponseFormat != "standard" {
		t.Fatalf("unexpected configuration: %+v", cfg)
	}
	if cfg.LLMConnected || cfg.ReviewConnected || cfg.EmbeddingConnected {
		t.Fatalf("dead endpoints should report not connected: %+v", cfg)
	}
}

func TestPutConfiguration(t *testing.T) {
	srv := newTestServer(t)
	h := srv.Handler()

	// Missing endpoint -> 400.
	rec := doJSON(t, h, "PUT", "/api/configuration", map[string]any{"llmBaseUrl": "  "})
	wantError(t, rec, http.StatusBadRequest, "LLM endpoint is required")

	// Valid update: review/embedding base+model default from llm values; persisted.
	rec = doJSON(t, h, "PUT", "/api/configuration", map[string]any{
		"llmBaseUrl":        "http://127.0.0.1:1/v1",
		"llmModel":          "my-model",
		"llmResponseFormat": "llm_jp_thinking",
	})
	wantStatus(t, rec, http.StatusOK)
	m := decodeJSONMap(t, rec)
	var cfg struct {
		LLMModel          string `json:"llmModel"`
		LLMResponseFormat string `json:"llmResponseFormat"`
		ReviewBaseURL     string `json:"reviewBaseUrl"`
		ReviewModel       string `json:"reviewModel"`
	}
	unmarshalField(t, m, "configuration", &cfg)
	if cfg.LLMModel != "my-model" || cfg.LLMResponseFormat != "llm_jp_thinking" {
		t.Fatalf("update not applied: %+v", cfg)
	}
	if cfg.ReviewModel != "my-model" || cfg.ReviewBaseURL != "http://127.0.0.1:1/v1" {
		t.Fatalf("review fields should default from llm: %+v", cfg)
	}

	// The snapshot is reflected by GetEditable.
	if got := srv.cfg.GetEditable().LLMModel; got != "my-model" {
		t.Fatalf("config snapshot llmModel = %q, want my-model", got)
	}
}

func TestConfigurationModels(t *testing.T) {
	h := newTestServer(t).Handler()

	rec := doJSON(t, h, "POST", "/api/configuration/models", map[string]any{"kind": "llm", "baseUrl": ""})
	wantError(t, rec, http.StatusBadRequest, "baseUrl is required")

	rec = doJSON(t, h, "POST", "/api/configuration/models", map[string]any{"kind": "bogus", "baseUrl": "http://127.0.0.1:1/v1"})
	wantError(t, rec, http.StatusBadRequest, "invalid configuration kind")

	// A reachable-looking but dead endpoint surfaces as a 500 (matches the Node
	// next(error) path), not an empty list.
	rec = doJSON(t, h, "POST", "/api/configuration/models", map[string]any{"kind": "llm", "baseUrl": "http://127.0.0.1:1/v1"})
	wantStatus(t, rec, http.StatusInternalServerError)
}

func TestProjectsCRUD(t *testing.T) {
	h := newTestServer(t).Handler()

	// Missing title.
	wantError(t, doJSON(t, h, "POST", "/api/projects", map[string]any{"title": "   "}),
		http.StatusBadRequest, "title is required")

	// Create.
	rec := doJSON(t, h, "POST", "/api/projects", map[string]any{
		"title":        "My Project",
		"description":  "desc",
		"systemPrompt": "sys",
	})
	wantStatus(t, rec, http.StatusCreated)
	m := decodeJSONMap(t, rec)
	var project struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		SystemPrompt string `json:"systemPrompt"`
		ChatCount    int    `json:"chatCount"`
	}
	unmarshalField(t, m, "project", &project)
	if project.Title != "My Project" || project.Description != "desc" || project.SystemPrompt != "sys" {
		t.Fatalf("unexpected project: %+v", project)
	}

	// List.
	rec = doJSON(t, h, "GET", "/api/projects", nil)
	wantStatus(t, rec, http.StatusOK)
	var listBody struct {
		Projects []json.RawMessage `json:"projects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Projects) != 1 {
		t.Fatalf("project list len = %d, want 1", len(listBody.Projects))
	}

	// Detail returns empty (non-null) child collections.
	rec = doJSON(t, h, "GET", "/api/projects/"+project.ID, nil)
	wantStatus(t, rec, http.StatusOK)
	detail := decodeJSONMap(t, rec)
	for _, key := range []string{"documents", "memories", "chats"} {
		var arr []json.RawMessage
		unmarshalField(t, detail, key, &arr)
		if arr == nil {
			t.Fatalf("detail %q should be an empty array, not null", key)
		}
	}

	// Detail 404.
	wantError(t, doJSON(t, h, "GET", "/api/projects/proj_missing", nil),
		http.StatusNotFound, "project not found")

	// Update title.
	rec = doJSON(t, h, "PATCH", "/api/projects/"+project.ID, map[string]any{"title": "Renamed"})
	wantStatus(t, rec, http.StatusOK)
	rec = doJSON(t, h, "PATCH", "/api/projects/proj_missing", map[string]any{"title": "x"})
	wantError(t, rec, http.StatusNotFound, "project not found")

	// Update system prompt.
	rec = doJSON(t, h, "PATCH", "/api/projects/"+project.ID+"/system-prompt", map[string]any{"systemPrompt": "new"})
	wantStatus(t, rec, http.StatusOK)

	// Delete + delete again.
	rec = doJSON(t, h, "DELETE", "/api/projects/"+project.ID, nil)
	wantStatus(t, rec, http.StatusOK)
	wantError(t, doJSON(t, h, "DELETE", "/api/projects/"+project.ID, nil),
		http.StatusNotFound, "project not found")
}

func TestProjectsReorder(t *testing.T) {
	h := newTestServer(t).Handler()

	wantError(t, doJSON(t, h, "POST", "/api/projects/reorder", map[string]any{"projectIds": []string{}}),
		http.StatusBadRequest, "projectIds are required")

	a := createProject(t, h, "A")
	b := createProject(t, h, "B")

	// Mismatched ids -> 400.
	wantError(t, doJSON(t, h, "POST", "/api/projects/reorder", map[string]any{"projectIds": []string{a, "proj_nope"}}),
		http.StatusBadRequest, "projectIds did not match existing projects")

	// Valid full set, reversed.
	rec := doJSON(t, h, "POST", "/api/projects/reorder", map[string]any{"projectIds": []string{b, a}})
	wantStatus(t, rec, http.StatusOK)
	m := decodeJSONMap(t, rec)
	var projects []struct {
		ID string `json:"id"`
	}
	unmarshalField(t, m, "projects", &projects)
	if len(projects) != 2 || projects[0].ID != b || projects[1].ID != a {
		t.Fatalf("reorder result = %+v, want [%s %s]", projects, b, a)
	}
}

func TestMemoriesCRUD(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Mem Project")
	base := "/api/projects/" + projectID + "/memories"

	wantError(t, doJSON(t, h, "POST", base, map[string]any{"content": "  "}),
		http.StatusBadRequest, "content is required")
	wantError(t, doJSON(t, h, "POST", base, map[string]any{"content": "x", "kind": "bogus"}),
		http.StatusBadRequest, "invalid memory kind")

	// Heading marker is stripped and the title is the first sentence only (the
	// trailing "## ..." sentence is dropped at the sentence split).
	rec := doJSON(t, h, "POST", base, map[string]any{"content": "## The hero is named Rin. She is brave."})
	wantStatus(t, rec, http.StatusCreated)
	m := decodeJSONMap(t, rec)
	var memory struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Kind   string `json:"kind"`
		Source string `json:"source"`
		Locked bool   `json:"locked"`
	}
	unmarshalField(t, m, "memory", &memory)
	if memory.Title != "The hero is named Rin" {
		t.Fatalf("memory title = %q, want %q", memory.Title, "The hero is named Rin")
	}
	if memory.Kind != "semantic" || memory.Source != "manual" || !memory.Locked {
		t.Fatalf("memory defaults wrong: %+v", memory)
	}

	// Lock toggle: non-bool -> 400, valid -> 200, missing -> 404.
	wantError(t, doJSON(t, h, "PATCH", "/api/memories/"+memory.ID+"/lock", map[string]any{"locked": "yes"}),
		http.StatusBadRequest, "locked must be a boolean")
	rec = doJSON(t, h, "PATCH", "/api/memories/"+memory.ID+"/lock", map[string]any{"locked": false})
	wantStatus(t, rec, http.StatusOK)
	wantError(t, doJSON(t, h, "PATCH", "/api/memories/mem_missing/lock", map[string]any{"locked": true}),
		http.StatusNotFound, "memory not found")

	// Delete + delete again.
	rec = doJSON(t, h, "DELETE", "/api/memories/"+memory.ID, nil)
	wantStatus(t, rec, http.StatusOK)
	wantError(t, doJSON(t, h, "DELETE", "/api/memories/"+memory.ID, nil),
		http.StatusNotFound, "memory not found")
}

func TestMemoryCreateLockedDefaultAndExplicitFalse(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Lock Project")
	base := "/api/projects/" + projectID + "/memories"

	// No locked field -> defaults to true.
	rec := doJSON(t, h, "POST", base, map[string]any{"content": "fact one"})
	m := decodeJSONMap(t, rec)
	var def struct {
		Locked bool `json:"locked"`
	}
	unmarshalField(t, m, "memory", &def)
	if !def.Locked {
		t.Fatal("locked should default to true when omitted")
	}

	// Explicit false -> false.
	rec = doJSON(t, h, "POST", base, map[string]any{"content": "fact two", "locked": false})
	m = decodeJSONMap(t, rec)
	var explicit struct {
		Locked bool `json:"locked"`
	}
	unmarshalField(t, m, "memory", &explicit)
	if explicit.Locked {
		t.Fatal("locked should be false when explicitly set false")
	}
}

func TestMemoryOrganizeFallback(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Org Project")

	// analyze with a dead LLM falls back to a heuristic plan (no error -> 200).
	rec := doJSON(t, h, "POST", "/api/projects/"+projectID+"/memories/organize/analyze", nil)
	wantStatus(t, rec, http.StatusOK)
	m := decodeJSONMap(t, rec)
	if _, ok := m["plan"]; !ok {
		t.Fatalf("analyze response missing plan: %s", rec.Body.String())
	}

	// analyze for a missing project -> 404.
	wantError(t, doJSON(t, h, "POST", "/api/projects/proj_missing/memories/organize/analyze", nil),
		http.StatusNotFound, "project not found")

	// apply without a plan -> 400.
	wantError(t, doJSON(t, h, "POST", "/api/projects/"+projectID+"/memories/organize/apply", map[string]any{}),
		http.StatusBadRequest, "plan is required")

	// apply with an empty plan -> 200 with the (unchanged) memory list.
	rec = doJSON(t, h, "POST", "/api/projects/"+projectID+"/memories/organize/apply",
		map[string]any{"plan": map[string]any{"summary": "noop", "changes": []any{}}})
	wantStatus(t, rec, http.StatusOK)
	m = decodeJSONMap(t, rec)
	var memories []json.RawMessage
	unmarshalField(t, m, "memories", &memories)
	if memories == nil {
		t.Fatal("apply should return a (possibly empty) memories array, not null")
	}
}

func TestDocuments(t *testing.T) {
	srv := newTestServer(t)
	h := srv.Handler()
	projectID := createProject(t, h, "Doc Project")
	base := "/api/projects/" + projectID + "/documents"

	// Invalid type.
	rec := doMultipart(t, h, base, map[string]string{"type": "spreadsheet"}, "", "", nil, "")
	wantError(t, rec, http.StatusBadRequest, "invalid document type")

	// Image without a file.
	rec = doMultipart(t, h, base, map[string]string{"type": "image"}, "", "", nil, "")
	wantError(t, rec, http.StatusBadRequest, "image file is required")

	// Text document from pasted content; title derived from the first line.
	rec = doMultipart(t, h, base, map[string]string{
		"type":     "text",
		"content":  "Hello world\nSecond line",
		"category": "world",
	}, "", "", nil, "")
	wantStatus(t, rec, http.StatusCreated)
	m := decodeJSONMap(t, rec)
	var textDoc struct {
		ID          string  `json:"id"`
		Type        string  `json:"type"`
		Title       string  `json:"title"`
		Category    string  `json:"category"`
		ContentText string  `json:"contentText"`
		FilePath    *string `json:"filePath"`
	}
	unmarshalField(t, m, "document", &textDoc)
	if textDoc.Type != "text" || textDoc.Title != "Hello world" || textDoc.Category != "world" {
		t.Fatalf("unexpected text document: %+v", textDoc)
	}
	if textDoc.ContentText != "Hello world\nSecond line" || textDoc.FilePath != nil {
		t.Fatalf("text document content/file wrong: %+v", textDoc)
	}

	// Image document with an uploaded file.
	pngData := []byte("\x89PNG\r\n\x1a\nfakepngbytes")
	rec = doMultipart(t, h, base, map[string]string{"type": "image"}, "file", "pic.png", pngData, "image/png")
	wantStatus(t, rec, http.StatusCreated)
	m = decodeJSONMap(t, rec)
	var imageDoc struct {
		ID       string  `json:"id"`
		FilePath *string `json:"filePath"`
		MimeType *string `json:"mimeType"`
	}
	unmarshalField(t, m, "document", &imageDoc)
	if imageDoc.FilePath == nil || !strings.HasPrefix(*imageDoc.FilePath, "/files/") || !strings.HasSuffix(*imageDoc.FilePath, ".png") {
		t.Fatalf("image filePath wrong: %+v", imageDoc.FilePath)
	}
	if imageDoc.MimeType == nil || *imageDoc.MimeType != "image/png" {
		t.Fatalf("image mimeType wrong: %+v", imageDoc.MimeType)
	}

	// The uploaded file is served by /files and round-trips its bytes.
	name := strings.TrimPrefix(*imageDoc.FilePath, "/files/")
	rec = doJSON(t, h, "GET", "/files/"+name, nil)
	wantStatus(t, rec, http.StatusOK)
	if !bytes.Equal(rec.Body.Bytes(), pngData) {
		t.Fatalf("served file bytes mismatch")
	}
	// Missing file -> 404.
	wantStatus(t, doJSON(t, h, "GET", "/files/does-not-exist.png", nil), http.StatusNotFound)

	// Category update: invalid -> 400, valid -> 200, missing -> 404.
	wantError(t, doJSON(t, h, "PATCH", "/api/documents/"+textDoc.ID+"/category", map[string]any{"category": "nope"}),
		http.StatusBadRequest, "invalid document category")
	rec = doJSON(t, h, "PATCH", "/api/documents/"+textDoc.ID+"/category", map[string]any{"category": "character"})
	wantStatus(t, rec, http.StatusOK)
	wantError(t, doJSON(t, h, "PATCH", "/api/documents/doc_missing/category", map[string]any{"category": "world"}),
		http.StatusNotFound, "document not found")

	// Delete the image document; its on-disk file must be removed.
	absFile := filepath.Join(srv.uploadDir, name)
	if _, err := os.Stat(absFile); err != nil {
		t.Fatalf("uploaded file should exist before delete: %v", err)
	}
	rec = doJSON(t, h, "DELETE", "/api/documents/"+imageDoc.ID, nil)
	wantStatus(t, rec, http.StatusOK)
	if _, err := os.Stat(absFile); !os.IsNotExist(err) {
		t.Fatalf("uploaded file should be unlinked after delete, stat err=%v", err)
	}
	wantError(t, doJSON(t, h, "DELETE", "/api/documents/doc_missing", nil),
		http.StatusNotFound, "document not found")
}

func TestChatsCRUD(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Chat Project")

	// Create chat under a missing project -> 404.
	wantError(t, doJSON(t, h, "POST", "/api/projects/proj_missing/chats", map[string]any{"title": "x"}),
		http.StatusNotFound, "project not found")

	chatID := createChat(t, h, projectID)

	// Detail.
	rec := doJSON(t, h, "GET", "/api/chats/"+chatID, nil)
	wantStatus(t, rec, http.StatusOK)
	detail := decodeJSONMap(t, rec)
	var messages []json.RawMessage
	unmarshalField(t, detail, "messages", &messages)
	if messages == nil {
		t.Fatal("messages should be an empty array, not null")
	}
	for _, key := range []string{"project", "chat"} {
		if _, ok := detail[key]; !ok {
			t.Fatalf("chat detail missing %q", key)
		}
	}

	wantError(t, doJSON(t, h, "GET", "/api/chats/chat_missing", nil),
		http.StatusNotFound, "chat not found")

	// Title + temporary toggles.
	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+chatID, map[string]any{"title": "Renamed"}), http.StatusOK)
	wantError(t, doJSON(t, h, "PATCH", "/api/chats/"+chatID+"/temporary", map[string]any{"isTemporary": "yes"}),
		http.StatusBadRequest, "isTemporary must be a boolean")
	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+chatID+"/temporary", map[string]any{"isTemporary": true}), http.StatusOK)
	wantError(t, doJSON(t, h, "PATCH", "/api/chats/chat_missing/temporary", map[string]any{"isTemporary": true}),
		http.StatusNotFound, "chat not found")

	// Delete + delete again.
	wantStatus(t, doJSON(t, h, "DELETE", "/api/chats/"+chatID, nil), http.StatusOK)
	wantError(t, doJSON(t, h, "DELETE", "/api/chats/"+chatID, nil),
		http.StatusNotFound, "chat not found")
}

// TestSendMessageFallback exercises the non-stream send path with a dead LLM:
// the chat service persists a reference/excerpt fallback (no error), so the route
// returns 201 with the user+assistant message pair.
func TestSendMessageFallback(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Send Project")
	chatID := createChat(t, h, projectID)

	wantError(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages", map[string]any{"content": "  "}),
		http.StatusBadRequest, "content is required")

	rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages", map[string]any{"content": "Hello there"})
	wantStatus(t, rec, http.StatusCreated)
	m := decodeJSONMap(t, rec)
	var assistant struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	unmarshalField(t, m, "message", &assistant)
	if assistant.Role != "assistant" {
		t.Fatalf("returned message role = %q, want assistant", assistant.Role)
	}
	var messages []struct {
		Role string `json:"role"`
	}
	unmarshalField(t, m, "messages", &messages)
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("unexpected message list: %+v", messages)
	}
}

// TestSendMessageStream verifies the SSE wiring of the streaming chat route end
// to end (delta frames followed by a done frame carrying chat/messages/summary),
// using the dead-LLM fallback as the delta source.
func TestSendMessageStream(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Stream Project")
	chatID := createChat(t, h, projectID)

	wantError(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages/stream", map[string]any{"content": ""}),
		http.StatusBadRequest, "content is required")

	rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages/stream", map[string]any{"content": "stream please"})
	wantStatus(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}

	events := parseSSE(t, rec.Body.String())
	if _, ok := events["delta"]; !ok {
		t.Fatalf("expected at least one delta event, got events=%v", mapEventNames(events))
	}
	done, ok := events["done"]
	if !ok {
		t.Fatalf("expected a done event, got events=%v", mapEventNames(events))
	}
	var donePayload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(done), &donePayload); err != nil {
		t.Fatalf("decode done payload: %v", err)
	}
	for _, key := range []string{"chat", "messages", "summary"} {
		if _, ok := donePayload[key]; !ok {
			t.Fatalf("done payload missing %q (payload=%s)", key, done)
		}
	}
}

// TestReviewStreamError verifies that a failing review (dead LLM, no fallback)
// surfaces as an SSE error event after the stream has already started, mirroring
// the Node flushHeaders-then-error behaviour.
func TestReviewStreamError(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Review Project")
	chatID := createChat(t, h, projectID)

	// Produce an assistant message to review.
	rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages", map[string]any{"content": "review me"})
	wantStatus(t, rec, http.StatusCreated)
	m := decodeJSONMap(t, rec)
	var assistant struct {
		ID string `json:"id"`
	}
	unmarshalField(t, m, "message", &assistant)

	// Non-stream review with a dead LLM -> 500.
	wantStatus(t, doJSON(t, h, "POST", "/api/messages/"+assistant.ID+"/review", nil), http.StatusInternalServerError)

	// Stream review with a dead LLM -> 200 stream that ends with an error event.
	rec = doJSON(t, h, "POST", "/api/messages/"+assistant.ID+"/review/stream", nil)
	wantStatus(t, rec, http.StatusOK)
	events := parseSSE(t, rec.Body.String())
	if _, ok := events["error"]; !ok {
		t.Fatalf("expected an error event, got events=%v", mapEventNames(events))
	}
}

func TestCORS(t *testing.T) {
	h := newTestServer(t).Handler()

	// Preflight is answered without hitting a route.
	req := httptest.NewRequest(http.MethodOptions, "/api/projects", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("allow-origin = %q, want reflected origin", got)
	}

	// A normal request reflects the origin too.
	req = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("allow-origin = %q, want reflected origin", got)
	}
}

// --- multipart + SSE helpers -------------------------------------------------

func doMultipart(t *testing.T, h http.Handler, target string, fields map[string]string, fileField, fileName string, fileData []byte, fileContentType string) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("write field %q: %v", k, err)
		}
	}
	if fileField != "" {
		mh := make(textproto.MIMEHeader)
		mh.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fileField, fileName))
		if fileContentType != "" {
			mh.Set("Content-Type", fileContentType)
		}
		part, err := mw.CreatePart(mh)
		if err != nil {
			t.Fatalf("create file part: %v", err)
		}
		if _, err := part.Write(fileData); err != nil {
			t.Fatalf("write file part: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest("POST", target, body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// parseSSE collects the last data payload for each event name from a raw SSE body
// (frames are "event: <name>\ndata: <json>\n\n").
func parseSSE(t *testing.T, raw string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, frame := range strings.Split(raw, "\n\n") {
		frame = strings.TrimSpace(frame)
		if frame == "" {
			continue
		}
		var event, data string
		for _, line := range strings.Split(frame, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
		if event != "" {
			out[event] = data
		}
	}
	return out
}

func mapEventNames(events map[string]string) []string {
	names := make([]string, 0, len(events))
	for k := range events {
		names = append(names, k)
	}
	return names
}
