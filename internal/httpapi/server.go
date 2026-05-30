package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"

	"snzstudio/internal/config"
	"snzstudio/internal/repository"
	"snzstudio/internal/service"
)

// maxJSONBody bounds JSON request bodies, mirroring express.json({ limit: "2mb" }).
const maxJSONBody = 2 << 20

// maxMultipartMemory is the in-memory threshold for multipart parsing; parts
// larger than this spill to temporary files. multer kept uploads unbounded, so
// this only governs buffering, not an upload size cap.
const maxMultipartMemory = 32 << 20

// Server owns the HTTP API ported from backend/src/index.ts. It holds the fully
// wired repository/service graph and serves it over the local loopback mux built
// by Handler. Construction order mirrors the Node backend's module bootstrap.
type Server struct {
	cfg *config.Config

	projects  *repository.ProjectRepository
	documents *repository.DocumentRepository
	memories  *repository.MemoryRepository
	chats     *repository.ChatRepository

	llm           *service.LLMClient
	embedding     *service.EmbeddingClient
	embeddingSync *service.EmbeddingSyncService
	retrieval     *service.RetrievalService
	context       *service.ContextService
	summary       *service.SummaryService
	memoryService *service.MemoryService
	memoryOrg     *service.MemoryOrganizerService
	chatService   *service.ChatService
	reviewService *service.ReviewService

	// uploadDir is where image uploads are stored and /files is served from. It
	// is an infrastructure path resolved by the process bootstrap, not part of
	// the editable config snapshot.
	uploadDir string
}

// NewServer wires the repository and service graph over a shared DB handle, a
// config snapshot source, and the upload directory. The dependency order matches
// HANDOFF §3 Phase 6 (repos -> emb/llm -> retrieval -> embeddingSync -> context
// -> summary/memory -> chat/review/organizer).
func NewServer(db *sql.DB, cfg *config.Config, uploadDir string) *Server {
	projects := repository.NewProjectRepository(db)
	documents := repository.NewDocumentRepository(db)
	memories := repository.NewMemoryRepository(db)
	chats := repository.NewChatRepository(db)

	embedding := service.NewEmbeddingClient(cfg)
	llm := service.NewLLMClient(cfg)

	retrieval := service.NewRetrievalService(db, embedding)
	embeddingSync := service.NewEmbeddingSyncService(documents, memories, embedding)
	contextService := service.NewContextService(projects, chats, documents, memories, retrieval)
	summary := service.NewSummaryService(llm)
	memoryService := service.NewMemoryService(memories, llm)
	chatService := service.NewChatService(chats, contextService, llm, summary, memoryService, embeddingSync, cfg)
	reviewService := service.NewReviewService(chats, contextService, llm, cfg)
	memoryOrg := service.NewMemoryOrganizerService(memories, chats, llm, embeddingSync)

	return &Server{
		cfg:           cfg,
		projects:      projects,
		documents:     documents,
		memories:      memories,
		chats:         chats,
		llm:           llm,
		embedding:     embedding,
		embeddingSync: embeddingSync,
		retrieval:     retrieval,
		context:       contextService,
		summary:       summary,
		memoryService: memoryService,
		memoryOrg:     memoryOrg,
		chatService:   chatService,
		reviewService: reviewService,
		uploadDir:     uploadDir,
	}
}

// RunStartupTasks reproduces the module-load side effects of index.ts: backfill
// inferred document categories, rebuild the (Go-tokenized) FTS indexes, and, if
// embeddings are enabled, rebuild embeddings in the background. The FTS work runs
// synchronously so the API never serves a half-rebuilt index; embedding rebuild
// is fire-and-forget (it needs a live endpoint and may legitimately fail).
func (s *Server) RunStartupTasks() {
	if err := s.documents.BackfillInferredCategories(); err != nil {
		log.Printf("startup: backfill document categories: %v", err)
	}
	if err := s.documents.RebuildSearchIndex(); err != nil {
		log.Printf("startup: rebuild document search index: %v", err)
	}
	if err := s.memories.RebuildSearchIndex(); err != nil {
		log.Printf("startup: rebuild memory search index: %v", err)
	}
	if s.embedding.IsEnabled() {
		go func() {
			if err := s.embeddingSync.RebuildAll(); err != nil {
				log.Printf("Embedding rebuild skipped: %v", err)
			}
		}()
	}
}

// Handler builds the loopback API mux. It uses the method+pattern routing of the
// net/http ServeMux (Go 1.22+), so no third-party router is needed. CORS wraps
// the mux to allow the WebView/Vite origin to reach the loopback server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	// Configuration
	mux.HandleFunc("GET /api/configuration", s.handleGetConfiguration)
	mux.HandleFunc("PUT /api/configuration", s.handlePutConfiguration)
	mux.HandleFunc("POST /api/configuration/models", s.handleListConfigurationModels)

	// Projects
	mux.HandleFunc("GET /api/projects", s.handleListProjects)
	mux.HandleFunc("POST /api/projects", s.handleCreateProject)
	mux.HandleFunc("POST /api/projects/reorder", s.handleReorderProjects)
	mux.HandleFunc("GET /api/projects/{projectId}", s.handleGetProject)
	mux.HandleFunc("PATCH /api/projects/{projectId}", s.handleUpdateProjectTitle)
	mux.HandleFunc("DELETE /api/projects/{projectId}", s.handleDeleteProject)
	mux.HandleFunc("PATCH /api/projects/{projectId}/system-prompt", s.handleUpdateProjectSystemPrompt)
	mux.HandleFunc("POST /api/projects/{projectId}/chats", s.handleCreateChat)
	mux.HandleFunc("POST /api/projects/{projectId}/memories", s.handleCreateMemory)
	mux.HandleFunc("POST /api/projects/{projectId}/memories/organize/analyze", s.handleAnalyzeMemoryOrganization)
	mux.HandleFunc("POST /api/projects/{projectId}/memories/organize/apply", s.handleApplyMemoryOrganization)
	mux.HandleFunc("POST /api/projects/{projectId}/documents", s.handleCreateDocument)

	// Memories
	mux.HandleFunc("PATCH /api/memories/{memoryId}/lock", s.handleSetMemoryLock)
	mux.HandleFunc("DELETE /api/memories/{memoryId}", s.handleDeleteMemory)

	// Documents
	mux.HandleFunc("DELETE /api/documents/{documentId}", s.handleDeleteDocument)
	mux.HandleFunc("PATCH /api/documents/{documentId}/category", s.handleUpdateDocumentCategory)

	// Chats
	mux.HandleFunc("GET /api/chats/{chatId}", s.handleGetChat)
	mux.HandleFunc("PATCH /api/chats/{chatId}", s.handleUpdateChatTitle)
	mux.HandleFunc("PATCH /api/chats/{chatId}/temporary", s.handleSetChatTemporary)
	mux.HandleFunc("DELETE /api/chats/{chatId}", s.handleDeleteChat)
	mux.HandleFunc("POST /api/chats/{chatId}/messages", s.handleSendMessage)
	mux.HandleFunc("POST /api/chats/{chatId}/messages/stream", s.handleSendMessageStream)

	// Messages / review
	mux.HandleFunc("POST /api/messages/{messageId}/review", s.handleReviewMessage)
	mux.HandleFunc("POST /api/messages/{messageId}/review/stream", s.handleReviewMessageStream)

	// Static uploaded files (express.static(config.uploadDir)).
	mux.Handle("GET /files/{name}", s.fileHandler())

	return withCORS(mux)
}

// withCORS reflects the request Origin and answers preflight requests. The API
// only ever listens on 127.0.0.1, so reflecting the origin is safe and covers
// both the dev Vite origin and the production WebView origin (whose exact value
// is platform-dependent), which a single hard-coded origin would not.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			h := w.Header()
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// checkConnections runs the three endpoint reachability probes concurrently, as
// the Node handler did with Promise.all. Each probe can block up to the client's
// (capped) timeout, so doing them serially would triple the latency of the
// configuration endpoints when an endpoint is down.
func (s *Server) checkConnections(settings config.Settings) (llmConnected, reviewConnected, embeddingConnected bool) {
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); llmConnected = s.llm.CheckConnection("", "") }()
	go func() {
		defer wg.Done()
		reviewConnected = s.llm.CheckConnection(settings.ReviewBaseURL, settings.ReviewModel)
	}()
	go func() { defer wg.Done(); embeddingConnected = s.embedding.CheckConnection() }()
	wg.Wait()
	return
}

// workspaceConfiguration is the GET/PUT /api/configuration response body. It
// flattens the seven editable fields (via the embedded Editable, whose JSON tags
// already match the frontend) and adds the three live-connection booleans.
type workspaceConfiguration struct {
	config.Editable
	LLMConnected       bool `json:"llmConnected"`
	ReviewConnected    bool `json:"reviewConnected"`
	EmbeddingConnected bool `json:"embeddingConnected"`
}

func workspaceConfig(editable config.Editable, llmConnected, reviewConnected, embeddingConnected bool) workspaceConfiguration {
	return workspaceConfiguration{
		Editable:           editable,
		LLMConnected:       llmConnected,
		ReviewConnected:    reviewConnected,
		EmbeddingConnected: embeddingConnected,
	}
}

// writeJSON encodes v as the JSON response body with the given status. HTML
// escaping is disabled so the wire bytes match JSON.stringify / res.json from the
// Node backend exactly (functionally identical either way once parsed).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// writeError mirrors the Node `res.status(code).json({ error })` shape.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// fail mirrors the Express error middleware: an unexpected error becomes a 500
// with the error message in `error`.
func fail(w http.ResponseWriter, err error) {
	message := "Unexpected server error"
	if err != nil {
		message = err.Error()
	}
	writeError(w, http.StatusInternalServerError, message)
}

// decodeBody parses a JSON request body into a dynamic map, mirroring how the
// Express handlers read `req.body?.field` with per-field type checks. An empty
// body yields an empty map (not an error), matching express.json. A malformed
// body returns 400 and false; callers should stop.
func decodeBody(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	body := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return map[string]any{}, true
		}
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return nil, false
	}
	return body, true
}

// bodyString coerces a JSON field to a string the way `String(req.body?.x ?? "")`
// would for the inputs the frontend actually sends: a string passes through, a
// number/bool is stringified, and anything else (including absent/null) is "".
func bodyString(m map[string]any, key string) string {
	return stringifyJSONValue(m[key])
}

// stringifyJSONValue coerces a decoded JSON value to a string, mirroring JS
// String(): strings pass through, numbers/bools are stringified, and null/objects
// become "".
func stringifyJSONValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}

// bodyBool returns (value, true) only when the field is present and a JSON
// boolean, mirroring `typeof req.body?.x === "boolean"`.
func bodyBool(m map[string]any, key string) (bool, bool) {
	v, ok := m[key].(bool)
	return v, ok
}
