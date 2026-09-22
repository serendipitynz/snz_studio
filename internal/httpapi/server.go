package httpapi

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"snzstudio/internal/config"
	"snzstudio/internal/embed"
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

	projects     *repository.ProjectRepository
	documents    *repository.DocumentRepository
	memories     *repository.MemoryRepository
	chats        *repository.ChatRepository
	participants *repository.ParticipantRepository

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
	turnEngine    *service.TurnEngine

	// embedManager owns the bundled embedding sidecar (download + llama-server). It
	// is nil in tests that don't exercise internal embeddings; all uses are
	// nil-guarded. Its ready/lost callbacks drive config's internal overlay.
	embedManager *embed.Manager

	// uploadDir is where image uploads are stored and /files is served from. It
	// is an infrastructure path resolved by the process bootstrap, not part of
	// the editable config snapshot.
	uploadDir string

	// token is the per-launch bearer required on every /api and /files request.
	// It is installed by the process bootstrap (SetAuthToken) and is empty in
	// tests, which disables the check — see withAuth.
	token string
}

// SetAuthToken installs the per-launch token that gates all API and file access.
// The bootstrap (app.go) generates a fresh random token each launch and passes
// it here; the SPA fetches the same token over a Wails binding and attaches it
// to every request. An empty token (the default, used by tests) disables the
// check so handler tests need no token plumbing.
func (s *Server) SetAuthToken(token string) { s.token = token }

// NewServer wires the repository and service graph over a shared DB handle, a
// config snapshot source, and the upload directory. The dependency order is
// repos -> emb/llm -> retrieval -> embeddingSync -> context -> summary/memory
// -> chat/review/organizer.
func NewServer(db *sql.DB, cfg *config.Config, uploadDir string, embedManager *embed.Manager) *Server {
	projects := repository.NewProjectRepository(db)
	documents := repository.NewDocumentRepository(db)
	memories := repository.NewMemoryRepository(db)
	chats := repository.NewChatRepository(db)
	participants := repository.NewParticipantRepository(db)

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
	turnEngine := service.NewTurnEngine(chats, participants, llm, cfg, contextService)

	srv := &Server{
		cfg:           cfg,
		projects:      projects,
		documents:     documents,
		memories:      memories,
		chats:         chats,
		participants:  participants,
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
		turnEngine:    turnEngine,
		embedManager:  embedManager,
		uploadDir:     uploadDir,
	}

	// Wire the sidecar lifecycle to config's internal-embedding overlay: when the
	// sidecar comes up, point embeddings at it (and rebuild once); when it is lost,
	// clear the overlay so retrieval degrades to FTS-only.
	if embedManager != nil {
		embedManager.SetCallbacks(srv.onEmbeddingReady, srv.onEmbeddingLost)
	}
	return srv
}

// onEmbeddingReady is the sidecar manager's ready callback. It overlays the internal
// sidecar endpoint onto config, refreshes the embedding client, and rebuilds
// embeddings exactly once per model (guarded so it does not re-embed on every
// launch/restart of an already-embedded corpus).
func (s *Server) onEmbeddingReady(baseURL, modelID string) {
	s.cfg.SetInternalEmbedding(baseURL, modelID)
	s.embedding.RefreshConfiguration()

	hasDoc, derr := s.documents.HasEmbeddingsForModel(modelID)
	hasMem, merr := s.memories.HasEmbeddingsForModel(modelID)
	if (derr == nil && hasDoc) || (merr == nil && hasMem) {
		return // corpus already embedded for this model — skip the rebuild
	}
	go func() {
		if err := s.embeddingSync.RebuildAll(); err != nil {
			log.Printf("Embedding rebuild (internal sidecar ready) skipped: %v", err)
		}
	}()
}

// onEmbeddingLost clears the internal overlay (sidecar crashed or gave up), leaving
// retrieval on FTS-only until the sidecar recovers.
func (s *Server) onEmbeddingLost() {
	s.cfg.ClearInternalEmbedding()
	s.embedding.RefreshConfiguration()
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
	mux.HandleFunc("GET /api/embedding/status", s.handleGetEmbeddingStatus)

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
	mux.HandleFunc("PATCH /api/memories/{memoryId}/shared", s.handleSetMemorySharedWithAll)
	mux.HandleFunc("DELETE /api/memories/{memoryId}", s.handleDeleteMemory)

	// Documents
	mux.HandleFunc("DELETE /api/documents/{documentId}", s.handleDeleteDocument)
	mux.HandleFunc("PATCH /api/documents/{documentId}/category", s.handleUpdateDocumentCategory)
	mux.HandleFunc("PATCH /api/documents/{documentId}/shared", s.handleUpdateDocumentSharedWithAll)

	// Chats
	mux.HandleFunc("GET /api/chats/{chatId}", s.handleGetChat)
	mux.HandleFunc("PATCH /api/chats/{chatId}", s.handleUpdateChat)
	mux.HandleFunc("PATCH /api/chats/{chatId}/temporary", s.handleSetChatTemporary)
	mux.HandleFunc("DELETE /api/chats/{chatId}", s.handleDeleteChat)
	mux.HandleFunc("POST /api/chats/{chatId}/messages", s.handleSendMessage)
	mux.HandleFunc("POST /api/chats/{chatId}/messages/stream", s.handleSendMessageStream)
	mux.HandleFunc("GET /api/chats/{chatId}/export/markdown", s.handleExportChatMarkdown)

	// Multi-agent chats (docs/multi-agent-chat-design.md §5). Participant updates
	// and removals hang off a flat participant id rather than nesting under the
	// chat, matching how memories and documents are already routed.
	mux.HandleFunc("GET /api/chats/{chatId}/participants", s.handleListParticipants)
	mux.HandleFunc("POST /api/chats/{chatId}/participants", s.handleCreateParticipant)
	mux.HandleFunc("PATCH /api/participants/{participantId}", s.handleUpdateParticipant)
	mux.HandleFunc("DELETE /api/participants/{participantId}", s.handleRemoveParticipant)
	mux.HandleFunc("POST /api/chats/{chatId}/turns/stream", s.handleRunTurnStream)
	mux.HandleFunc("GET /api/multi-agent-presets", s.handleListMultiAgentPresets)
	mux.HandleFunc("POST /api/chats/{chatId}/preset", s.handleApplyMultiAgentPreset)
	mux.HandleFunc("GET /api/messages/{messageId}/memory-draft", s.handleGetMessageMemoryDraft)
	mux.HandleFunc("POST /api/messages/{messageId}/memory", s.handleSaveMessageMemory)
	mux.HandleFunc("POST /api/chats/{chatId}/conclusion-draft", s.handleDraftConclusion)

	// Messages / review
	mux.HandleFunc("POST /api/messages/{messageId}/review", s.handleReviewMessage)
	mux.HandleFunc("POST /api/messages/{messageId}/review/stream", s.handleReviewMessageStream)

	// Static uploaded files (express.static(config.uploadDir)).
	mux.Handle("GET /files/{name}", s.fileHandler())

	// CORS wraps auth so that preflight (OPTIONS) is answered before the token
	// check — browsers never send custom headers on preflight, so requiring the
	// token there would break every cross-origin mutation.
	return withCORS(s.withAuth(mux))
}

// withAuth enforces the per-launch token on every request once one is set.
// Listening on 127.0.0.1 is not itself a trust boundary — any local browser tab
// can reach loopback and CORS reflects whatever Origin asks — so this token, not
// the address, is what actually gates access. /api callers send it in the
// X-SNZ-Studio-Token header (added by the SPA's fetch layer); /files is loaded
// via <img>, which cannot set headers, so its token rides in a `t` query param.
// An empty token (tests) skips the check. OPTIONS never reaches here: withCORS
// short-circuits preflight.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" {
			next.ServeHTTP(w, r)
			return
		}
		var provided string
		if strings.HasPrefix(r.URL.Path, "/files/") {
			provided = r.URL.Query().Get("t")
		} else {
			provided = r.Header.Get("X-SNZ-Studio-Token")
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withCORS reflects the request Origin and answers preflight requests. Origin
// reflection alone is not an access control (it grants whatever Origin asks);
// access is gated by withAuth's per-launch token. Reflection is still needed so
// the WebView/Vite origin — whose exact value is platform-dependent, which a
// single hard-coded origin could not cover — can read responses.
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
			// X-SNZ-Studio-Token (required by withAuth) must be allowed here or the
			// browser blocks every cross-origin request at preflight: the custom
			// token header makes even GETs non-simple, so the WebView's calls to the
			// absolute loopback origin all preflight. Content-Type covers JSON bodies.
			h.Set("Access-Control-Allow-Headers", "Content-Type, X-SNZ-Studio-Token")
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

// bodyStringPtr returns a pointer to the coerced string only when the key is
// present, which is what separates "set this field to an empty string" from
// "leave this field alone" in the PATCH routes. A JSON null counts as absent:
// nothing sends one to mean "blank this".
func bodyStringPtr(m map[string]any, key string) *string {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	s := stringifyJSONValue(v)
	return &s
}

// bodyIntPtr reads an optional integer field, returning (nil, true) when absent
// and (nil, false) when present but not an exact integer — the caller answers
// 400 for the latter rather than silently leaving the field unchanged. A
// fractional value is rejected rather than truncated: 1.9 truncated to 1 would
// quietly put a participant somewhere other than where the caller asked.
func bodyIntPtr(m map[string]any, key string) (*int, bool) {
	v, present := m[key]
	if !present || v == nil {
		return nil, true
	}
	f, isNumber := v.(float64)
	if !isNumber || f != math.Trunc(f) || f < math.MinInt32 || f > math.MaxInt32 {
		return nil, false
	}
	n := int(f)
	return &n, true
}

// bodyBool returns (value, true) only when the field is present and a JSON
// boolean, mirroring `typeof req.body?.x === "boolean"`.
func bodyBool(m map[string]any, key string) (bool, bool) {
	v, ok := m[key].(bool)
	return v, ok
}

// bodyBoolPtr is bodyBool as the optional field of a PATCH: a pointer when the
// field is present and a JSON boolean, nil otherwise. A value of another type is
// read as absent rather than refused, which is bodyString's reading of a
// mistyped field as well.
func bodyBoolPtr(m map[string]any, key string) *bool {
	v, ok := bodyBool(m, key)
	if !ok {
		return nil
	}
	return &v
}
