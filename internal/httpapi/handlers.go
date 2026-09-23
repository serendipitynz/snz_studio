package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"

	"snzstudio/internal/config"
	"snzstudio/internal/doccategory"
	"snzstudio/internal/embed"
	"snzstudio/internal/model"
	"snzstudio/internal/repository"
	"snzstudio/internal/service"
	"snzstudio/internal/util"
)

// --- Configuration -----------------------------------------------------------

func (s *Server) handleGetConfiguration(w http.ResponseWriter, _ *http.Request) {
	settings := s.cfg.Get()
	llmConnected, reviewConnected, embeddingConnected := s.checkConnections(settings)
	editable := s.cfg.GetEditable()
	writeJSON(w, http.StatusOK, map[string]any{
		"configuration": workspaceConfig(editable, llmConnected, reviewConnected, embeddingConnected),
	})
}

func (s *Server) handlePutConfiguration(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}

	llmBaseURL := strings.TrimSpace(bodyString(m, "llmBaseUrl"))
	llmModel := strings.TrimSpace(bodyString(m, "llmModel"))
	llmResponseFormat := "standard"
	if bodyString(m, "llmResponseFormat") == "llm_jp_thinking" {
		llmResponseFormat = "llm_jp_thinking"
	}
	reviewBaseURL := strings.TrimSpace(bodyString(m, "reviewBaseUrl"))
	if reviewBaseURL == "" {
		reviewBaseURL = llmBaseURL
	}
	reviewModel := strings.TrimSpace(bodyString(m, "reviewModel"))
	if reviewModel == "" {
		reviewModel = llmModel
	}
	embeddingBaseURL := strings.TrimSpace(bodyString(m, "embeddingBaseUrl"))
	embeddingModel := strings.TrimSpace(bodyString(m, "embeddingModel"))
	embeddingMode := strings.TrimSpace(bodyString(m, "embeddingMode"))
	// Unlike the review fields these stay empty rather than being filled from the
	// LLM values: an empty URL keeps following the LLM endpoint at use time, and an
	// empty model is what disables image description.
	imageDescriptionBaseURL := strings.TrimSpace(bodyString(m, "imageDescriptionBaseUrl"))
	imageDescriptionModel := strings.TrimSpace(bodyString(m, "imageDescriptionModel"))

	if llmBaseURL == "" {
		writeError(w, http.StatusBadRequest, "LLM endpoint is required")
		return
	}

	updated, err := s.cfg.UpdateEditable(config.Editable{
		LLMBaseURL:        llmBaseURL,
		LLMModel:          llmModel,
		LLMResponseFormat: llmResponseFormat,
		ReviewBaseURL:     reviewBaseURL,
		ReviewModel:       reviewModel,
		EmbeddingBaseURL:  embeddingBaseURL,
		EmbeddingModel:    embeddingModel,
		EmbeddingMode:     embeddingMode,

		ImageDescriptionBaseURL: imageDescriptionBaseURL,
		ImageDescriptionModel:   imageDescriptionModel,
	})
	if err != nil {
		fail(w, err)
		return
	}

	// Apply the embedding-mode switch before refreshing the client: internal starts
	// (or keeps) the sidecar asynchronously; external stops it and drops the overlay
	// so the persisted external endpoint is used.
	if s.embedManager != nil {
		if updated.EmbeddingMode == "external" {
			s.embedManager.Shutdown()
			s.cfg.ClearInternalEmbedding()
		} else {
			s.embedManager.EnsureInternalReady(context.Background())
		}
	}

	s.embedding.RefreshConfiguration()

	// Warm the models concurrently, matching the Node Promise.all. Results are
	// advisory (the connection probe below reports reachability), so ignore them.
	// Embedding uses the EFFECTIVE settings (cfg.Get) so internal mode targets the
	// sidecar overlay — empty until the sidecar is ready — not the persisted
	// external model.
	eff := s.cfg.Get()
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); s.llm.EnsureModelLoaded(updated.LLMModel, updated.LLMBaseURL) }()
	go func() { defer wg.Done(); s.llm.EnsureModelLoaded(updated.ReviewModel, updated.ReviewBaseURL) }()
	go func() {
		defer wg.Done()
		s.embedding.EnsureModelLoaded(eff.EmbeddingModel, eff.EmbeddingBaseURL)
	}()
	wg.Wait()

	if s.embedding.IsEnabled() {
		go func() {
			if err := s.embeddingSync.RebuildAll(); err != nil {
				log.Printf("Embedding rebuild skipped: %v", err)
			}
		}()
	}

	llmConnected, reviewConnected, embeddingConnected := s.checkConnections(s.cfg.Get())
	writeJSON(w, http.StatusOK, map[string]any{
		"configuration": workspaceConfig(updated, llmConnected, reviewConnected, embeddingConnected),
	})
}

func (s *Server) handleListConfigurationModels(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	kind := strings.TrimSpace(bodyString(m, "kind"))
	baseURL := strings.TrimSpace(bodyString(m, "baseUrl"))
	if baseURL == "" {
		writeError(w, http.StatusBadRequest, "baseUrl is required")
		return
	}

	switch kind {
	case "llm":
		models, err := s.llm.ListModels(baseURL)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": models})
	case "embedding":
		models, err := s.embedding.ListModels(baseURL)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": models})
	default:
		writeError(w, http.StatusBadRequest, "invalid configuration kind")
	}
}

// handleGetEmbeddingStatus reports the internal embedding sidecar's lifecycle state
// (download progress / ready / error) so the settings UI can show "downloading…"
// and switch on once embeddings are live. Returns disabled when no manager is wired
// (e.g. tests).
func (s *Server) handleGetEmbeddingStatus(w http.ResponseWriter, _ *http.Request) {
	if s.embedManager == nil {
		writeJSON(w, http.StatusOK, embed.Status{State: embed.StateDisabled})
		return
	}
	writeJSON(w, http.StatusOK, s.embedManager.Status())
}

// --- Projects ----------------------------------------------------------------

func (s *Server) handleListProjects(w http.ResponseWriter, _ *http.Request) {
	projects, err := s.projects.ListProjects()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	title := strings.TrimSpace(bodyString(m, "title"))
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	description := ""
	if v, ok := m["description"].(string); ok {
		description = v
	}
	systemPrompt := ""
	if v, ok := m["systemPrompt"].(string); ok {
		systemPrompt = v
	}

	project, err := s.projects.CreateProject(repository.CreateProjectInput{
		Title:        title,
		Description:  description,
		SystemPrompt: systemPrompt,
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"project": project})
}

func (s *Server) handleReorderProjects(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	raw, _ := m["projectIds"].([]any)
	projectIDs := make([]string, 0, len(raw))
	for _, v := range raw {
		projectIDs = append(projectIDs, stringifyJSONValue(v))
	}
	if len(projectIDs) == 0 {
		writeError(w, http.StatusBadRequest, "projectIds are required")
		return
	}

	reordered, err := s.projects.ReorderProjects(projectIDs)
	if errors.Is(err, repository.ErrProjectReorderMismatch) {
		writeError(w, http.StatusBadRequest, "projectIds did not match existing projects")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": reordered})
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.GetProject(r.PathValue("projectId"))
	if err != nil {
		fail(w, err)
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	documents, err := s.documents.ListByProject(project.ID)
	if err != nil {
		fail(w, err)
		return
	}
	memories, err := s.memories.ListByProject(project.ID)
	if err != nil {
		fail(w, err)
		return
	}
	chats, err := s.chats.ListByProject(project.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project":   project,
		"documents": documents,
		"memories":  memories,
		"chats":     chats,
	})
}

func (s *Server) handleUpdateProjectTitle(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	title := strings.TrimSpace(bodyString(m, "title"))
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	project, err := s.projects.UpdateProjectTitle(r.PathValue("projectId"), title)
	if err != nil {
		fail(w, err)
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": project})
}

func (s *Server) handleUpdateProjectSystemPrompt(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	systemPrompt := ""
	if v, ok := m["systemPrompt"].(string); ok {
		systemPrompt = v
	}
	project, err := s.projects.UpdateProjectSystemPrompt(r.PathValue("projectId"), systemPrompt)
	if err != nil {
		fail(w, err)
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": project})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.GetProject(r.PathValue("projectId"))
	if err != nil {
		fail(w, err)
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	// Snapshot the related documents before the cascade delete so their uploaded
	// files can be unlinked afterwards (the DB cascade handles rows only).
	relatedDocuments, err := s.documents.ListByProject(project.ID)
	if err != nil {
		fail(w, err)
		return
	}

	deleted, err := s.projects.DeleteProject(project.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if !deleted {
		writeError(w, http.StatusInternalServerError, "failed to delete project")
		return
	}

	for _, document := range relatedDocuments {
		s.unlinkFile(document.FilePath)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleCreateChat(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.GetProject(r.PathValue("projectId"))
	if err != nil {
		fail(w, err)
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	title := bodyString(m, "title")
	isTemporary, _ := bodyBool(m, "isTemporary")
	kind := bodyString(m, "kind")
	if kind == "" {
		kind = model.ChatKindAssistant
	}
	if kind != model.ChatKindAssistant && kind != model.ChatKindMultiAgent {
		writeError(w, http.StatusBadRequest, "kind must be \"assistant\" or \"multi_agent\"")
		return
	}
	chosen, ok := s.presetFromBody(w, m, kind)
	if !ok {
		return
	}

	input := repository.CreateChatInput{
		ProjectID:   project.ID,
		Title:       title,
		IsTemporary: isTemporary,
		Kind:        kind,
	}
	if chosen != nil {
		input.TurnRule = chosen.TurnRule
		input.ScenePrompt = chosen.ScenePrompt
		if strings.TrimSpace(title) == "" {
			input.Title = chosen.Title
		}
	}
	chat, err := s.chats.CreateChat(input)
	if err != nil {
		fail(w, err)
		return
	}
	if chosen != nil {
		applied, err := s.applyPresetRoster(chat.ID, chosen)
		if err != nil {
			fail(w, err)
			return
		}
		if applied == nil {
			writeError(w, http.StatusNotFound, "chat not found")
			return
		}
		chat = *applied
	}
	writeJSON(w, http.StatusCreated, map[string]any{"chat": chat})
}

// --- Memories ----------------------------------------------------------------

func (s *Server) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.GetProject(r.PathValue("projectId"))
	if err != nil {
		fail(w, err)
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}

	content := strings.TrimSpace(bodyString(m, "content"))
	kind := bodyString(m, "kind")
	if kind == "" {
		kind = "semantic"
	}
	// locked defaults to true unless the body explicitly sends false.
	locked := true
	if b, ok := bodyBool(m, "locked"); ok && !b {
		locked = false
	}

	if content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if kind != "semantic" && kind != "procedural" && kind != "episodic" {
		writeError(w, http.StatusBadRequest, "invalid memory kind")
		return
	}

	memory, err := s.memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID: project.ID,
		Title:     service.GenerateMemoryTitle(content, kind),
		Content:   content,
		Kind:      kind,
		Source:    "manual",
		Locked:    locked,
	})
	if err != nil {
		fail(w, err)
		return
	}

	if err := s.embeddingSync.SyncMemories([]string{memory.ID}); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"memory": memory})
}

func (s *Server) handleSetMemoryLock(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	locked, isBool := bodyBool(m, "locked")
	if !isBool {
		writeError(w, http.StatusBadRequest, "locked must be a boolean")
		return
	}
	memory, err := s.memories.SetMemoryLocked(r.PathValue("memoryId"), locked)
	if err != nil {
		fail(w, err)
		return
	}
	if memory == nil {
		writeError(w, http.StatusNotFound, "memory not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"memory": memory})
}

// handleSetMemorySharedWithAll puts a memory into the common project material or
// takes it out (design §4.4).
func (s *Server) handleSetMemorySharedWithAll(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	sharedWithAll, isBool := bodyBool(m, "sharedWithAll")
	if !isBool {
		writeError(w, http.StatusBadRequest, "sharedWithAll must be a boolean")
		return
	}
	memory, err := s.memories.SetMemorySharedWithAll(r.PathValue("memoryId"), sharedWithAll)
	if err != nil {
		fail(w, err)
		return
	}
	if memory == nil {
		writeError(w, http.StatusNotFound, "memory not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"memory": memory})
}

func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	memory, err := s.memories.DeleteMemory(r.PathValue("memoryId"))
	if err != nil {
		fail(w, err)
		return
	}
	if memory == nil {
		writeError(w, http.StatusNotFound, "memory not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "memory": memory})
}

func (s *Server) handleAnalyzeMemoryOrganization(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.GetProject(r.PathValue("projectId"))
	if err != nil {
		fail(w, err)
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	plan, err := s.memoryOrg.AnalyzeProject(project.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": plan})
}

func (s *Server) handleApplyMemoryOrganization(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.GetProject(r.PathValue("projectId"))
	if err != nil {
		fail(w, err)
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	var body struct {
		Plan *model.MemoryOrganizationPlan `json:"plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Plan == nil {
		writeError(w, http.StatusBadRequest, "plan is required")
		return
	}

	memoriesAfter, err := s.memoryOrg.ApplyProjectPlan(project.ID, *body.Plan)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"memories": memoriesAfter})
}

// --- Documents ---------------------------------------------------------------

func (s *Server) handleCreateDocument(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.GetProject(r.PathValue("projectId"))
	if err != nil {
		fail(w, err)
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	docType := strings.TrimSpace(r.FormValue("type"))
	if docType != "markdown" && docType != "text" && docType != "image" {
		writeError(w, http.StatusBadRequest, "invalid document type")
		return
	}

	file, header, ferr := r.FormFile("file")
	hasFile := ferr == nil
	if hasFile {
		defer file.Close()
	} else if !errors.Is(ferr, http.ErrMissingFile) {
		fail(w, ferr)
		return
	}

	contentText := r.FormValue("content")
	var filePath *string
	var mimeType *string

	switch {
	case docType == "image":
		if !hasFile {
			writeError(w, http.StatusBadRequest, "image file is required")
			return
		}
		name := uuid.NewString() + filepath.Ext(header.Filename)
		if err := s.saveUpload(file, name); err != nil {
			fail(w, err)
			return
		}
		public := toPublicFilePath(name)
		filePath = &public
		mt := header.Header.Get("Content-Type")
		mimeType = &mt
	case hasFile:
		data, err := io.ReadAll(file)
		if err != nil {
			fail(w, err)
			return
		}
		contentText = string(data)
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" && hasFile && header.Filename != "" {
		title = header.Filename
	}
	if title == "" {
		firstLine := contentText
		if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
			firstLine = firstLine[:idx]
		}
		firstLine = strings.TrimSpace(firstLine)
		if firstLine == "" {
			firstLine = "Untitled document"
		}
		title = util.Truncate(firstLine, 80)
	}

	category := strings.TrimSpace(r.FormValue("category"))
	if !doccategory.IsValid(category) {
		category = ""
	}

	document, err := s.documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID:   project.ID,
		Type:        docType,
		Category:    category,
		Title:       title,
		Note:        formText(r, "note"),
		Tags:        util.ParseTags(r.FormValue("tags")),
		DerivedText: formText(r, "derivedText"),
		ContentText: contentText,
		FilePath:    filePath,
		MimeType:    mimeType,
	})
	if err != nil {
		// The upload was written before the row existed; remove it so a failed
		// insert leaves no orphaned file. unlinkFile is a no-op when filePath is
		// nil (non-image documents).
		s.unlinkFile(filePath)
		fail(w, err)
		return
	}

	// Embedding sync is best-effort here, as it is at startup and on sidecar-ready
	// (see RunStartupTasks / onEmbeddingReady): the document is already persisted,
	// so a sync failure must not fail the request — otherwise the client treats a
	// successful create as an error and may retry, duplicating the document. A
	// later rebuild backfills the embedding.
	if err := s.embeddingSync.SyncDocument(document.ID); err != nil {
		log.Printf("create document %s: embedding sync skipped: %v", document.ID, err)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"document": document})
}

// formText reads a multipart text field with LF line endings. A browser encodes
// every newline in a form field as CRLF when it builds multipart/form-data, so a
// note or description typed into a textarea would otherwise be stored with CRLF
// while documents read from files keep LF.
func formText(r *http.Request, key string) string {
	return strings.ReplaceAll(r.FormValue(key), "\r\n", "\n")
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	document, err := s.documents.DeleteDocument(r.PathValue("documentId"))
	if err != nil {
		fail(w, err)
		return
	}
	if document == nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	s.unlinkFile(document.FilePath)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "document": document})
}

func (s *Server) handleUpdateDocumentCategory(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	category := strings.TrimSpace(bodyString(m, "category"))
	if !doccategory.IsValid(category) {
		writeError(w, http.StatusBadRequest, "invalid document category")
		return
	}
	document, err := s.documents.UpdateDocumentCategory(r.PathValue("documentId"), category)
	if err != nil {
		fail(w, err)
		return
	}
	if document == nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": document})
}

// handleUpdateDocumentSharedWithAll puts a document into the common project
// material or takes it out (design §4.4).
func (s *Server) handleUpdateDocumentSharedWithAll(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	sharedWithAll, isBool := bodyBool(m, "sharedWithAll")
	if !isBool {
		writeError(w, http.StatusBadRequest, "sharedWithAll must be a boolean")
		return
	}
	document, err := s.documents.UpdateDocumentSharedWithAll(r.PathValue("documentId"), sharedWithAll)
	if err != nil {
		fail(w, err)
		return
	}
	if document == nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": document})
}

// --- Chats -------------------------------------------------------------------

func (s *Server) handleGetChat(w http.ResponseWriter, r *http.Request) {
	chat, err := s.chats.GetChat(r.PathValue("chatId"))
	if err != nil {
		fail(w, err)
		return
	}
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat not found")
		return
	}
	project, err := s.projects.GetProject(chat.ProjectID)
	if err != nil {
		fail(w, err)
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	summary, err := s.chats.GetSummary(chat.ID)
	if err != nil {
		fail(w, err)
		return
	}
	messages, err := s.chats.GetMessagesWithReferences(chat.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project":  project,
		"chat":     chat,
		"summary":  summary,
		"messages": messages,
	})
}

// handleUpdateChat applies the fields the body actually carries: the title, and
// for a multi-agent chat the turn rule and scene prompt (design §5). Each field
// is keyed on its presence rather than on its value, so a body sent to change
// the scene prompt alone does not blank the title.
func (s *Server) handleUpdateChat(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	chatID := r.PathValue("chatId")
	title := bodyStringPtr(m, "title")
	turnRule := bodyStringPtr(m, "turnRule")
	scenePrompt := bodyStringPtr(m, "scenePrompt")
	facilitatorID := bodyStringPtr(m, "facilitatorId")

	chat, err := s.chats.GetChat(chatID)
	if err != nil {
		fail(w, err)
		return
	}
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat not found")
		return
	}

	if turnRule != nil || scenePrompt != nil || facilitatorID != nil {
		// kind is fixed at creation, so a single-assistant chat can never reach a
		// state where these fields mean anything; accepting them would store
		// settings that nothing reads.
		if chat.Kind != model.ChatKindMultiAgent {
			writeError(w, http.StatusBadRequest, "turnRule, scenePrompt and facilitatorId apply to multi-agent chats only")
			return
		}
		if turnRule != nil && !isKnownTurnRule(*turnRule) {
			writeError(w, http.StatusBadRequest, "turnRule must be \"round_robin\", \"manual\", \"facilitator_alternating\" or \"weighted\"")
			return
		}
		// An empty value clears the choice; anything else has to be on this chat's
		// roster. A removed participant is refused here although the engine
		// tolerates one it finds stored (design §4.2 step 1): tolerating what a
		// removal left behind is not a reason to let the panel write it.
		if facilitatorID != nil && *facilitatorID != "" && !s.isRosterMember(w, chatID, *facilitatorID) {
			return
		}
		chat, err = s.chats.UpdateMultiAgentSettings(chatID, turnRule, scenePrompt, facilitatorID)
		if err != nil {
			fail(w, err)
			return
		}
		if chat == nil {
			writeError(w, http.StatusNotFound, "chat not found")
			return
		}
	}

	if title != nil {
		chat, err = s.chats.UpdateChatTitle(chatID, *title)
		if err != nil {
			fail(w, err)
			return
		}
		if chat == nil {
			writeError(w, http.StatusNotFound, "chat not found")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"chat": chat})
}

func isKnownTurnRule(turnRule string) bool {
	switch turnRule {
	case model.TurnRuleRoundRobin, model.TurnRuleManual, model.TurnRuleFacilitatorAlternating, model.TurnRuleWeighted:
		return true
	}
	return false
}

// isRosterMember answers whether the participant is on the chat's roster, and
// writes the refusal itself when it is not, so the caller only has to return.
func (s *Server) isRosterMember(w http.ResponseWriter, chatID, participantID string) bool {
	participant, err := s.participants.GetParticipant(participantID)
	if err != nil {
		fail(w, err)
		return false
	}
	if participant == nil || participant.ChatID != chatID || participant.DeletedAt != nil {
		writeError(w, http.StatusBadRequest, "facilitatorId must name a participant on this chat's roster")
		return false
	}
	return true
}

func (s *Server) handleSetChatTemporary(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	isTemporary, isBool := bodyBool(m, "isTemporary")
	if !isBool {
		writeError(w, http.StatusBadRequest, "isTemporary must be a boolean")
		return
	}
	chat, err := s.chats.SetTemporary(r.PathValue("chatId"), isTemporary)
	if err != nil {
		fail(w, err)
		return
	}
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chat": chat})
}

func (s *Server) handleDeleteChat(w http.ResponseWriter, r *http.Request) {
	chat, err := s.chats.DeleteChat(r.PathValue("chatId"))
	if err != nil {
		fail(w, err)
		return
	}
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "chat": chat})
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	content := strings.TrimSpace(bodyString(m, "content"))
	if content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	chatID := r.PathValue("chatId")

	chat, err := s.chats.GetChat(chatID)
	if err != nil {
		fail(w, err)
		return
	}
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat not found")
		return
	}

	// In a multi-agent chat this route is the human's intervention, not a turn:
	// the message is stored and nothing is generated, because who speaks next is
	// the turn engine's decision (design §4.4).
	var assistantMessage *model.Message
	if chat.Kind == model.ChatKindMultiAgent {
		stored, storeErr := s.storeHumanMessage(chatID, content)
		if storeErr != nil {
			fail(w, storeErr)
			return
		}
		assistantMessage = stored
	} else {
		assistantMessage, err = s.chatService.SendMessage(chatID, content)
		if err != nil {
			fail(w, err)
			return
		}
		chat, err = s.chats.GetChat(chatID)
		if err != nil {
			fail(w, err)
			return
		}
		if chat == nil {
			writeError(w, http.StatusNotFound, "chat not found")
			return
		}
	}
	summary, err := s.chats.GetSummary(chatID)
	if err != nil {
		fail(w, err)
		return
	}
	messages, err := s.chats.GetMessagesWithReferences(chatID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"message":  assistantMessage,
		"chat":     chat,
		"summary":  summary,
		"messages": messages,
	})
}

func (s *Server) handleSendMessageStream(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	content := strings.TrimSpace(bodyString(m, "content"))
	if content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	chatID := r.PathValue("chatId")

	// The chat is read before the stream opens so that a missing chat is still a
	// 404 rather than an SSE error frame on a 200 response.
	chat, err := s.chats.GetChat(chatID)
	if err != nil {
		fail(w, err)
		return
	}
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat not found")
		return
	}
	isMultiAgent := chat.Kind == model.ChatKindMultiAgent

	sse, err := NewSSEWriter(w)
	if err != nil {
		fail(w, err)
		return
	}

	if isMultiAgent {
		// Same store-only intervention as the non-streaming route (§4.4). The
		// response still ends in a done frame — with no delta before it — so the
		// frontend consumes both chat kinds through one parser.
		if _, storeErr := s.storeHumanMessage(chatID, content); storeErr != nil {
			_ = sse.Event("error", map[string]string{"message": storeErr.Error()})
			return
		}
	} else {
		_, streamErr := s.chatService.SendMessageStream(chatID, content, func(chunk string) {
			_ = sse.Event("delta", map[string]string{"content": chunk})
		})
		if streamErr != nil {
			_ = sse.Event("error", map[string]string{"message": streamErr.Error()})
			return
		}
	}

	chat, err = s.chats.GetChat(chatID)
	if err != nil {
		_ = sse.Event("error", map[string]string{"message": err.Error()})
		return
	}
	if chat == nil {
		_ = sse.Event("error", map[string]string{"message": "chat not found"})
		return
	}
	messages, err := s.chats.GetMessagesWithReferences(chatID)
	if err != nil {
		_ = sse.Event("error", map[string]string{"message": err.Error()})
		return
	}
	summary, err := s.chats.GetSummary(chatID)
	if err != nil {
		_ = sse.Event("error", map[string]string{"message": err.Error()})
		return
	}
	_ = sse.Event("done", map[string]any{
		"chat":     chat,
		"messages": messages,
		"summary":  summary,
	})
}

// --- Messages / review -------------------------------------------------------

func (s *Server) handleReviewMessage(w http.ResponseWriter, r *http.Request) {
	result, err := s.reviewService.ReviewMessage(r.PathValue("messageId"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleReviewMessageStream(w http.ResponseWriter, r *http.Request) {
	sse, err := NewSSEWriter(w)
	if err != nil {
		fail(w, err)
		return
	}

	result, streamErr := s.reviewService.ReviewMessageStream(r.PathValue("messageId"), func(chunk string) {
		_ = sse.Event("delta", map[string]string{"content": chunk})
	})
	if streamErr != nil {
		_ = sse.Event("error", map[string]string{"message": streamErr.Error()})
		return
	}
	_ = sse.Event("done", result)
}

// --- Static files ------------------------------------------------------------

// fileHandler serves uploaded files from uploadDir, mirroring
// express.static(config.uploadDir). Uploaded names are flat (uuid+ext), so the
// single-segment {name} pattern plus an explicit separator guard prevents any
// path traversal out of the upload directory.
func (s *Server) fileHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(s.uploadDir, name))
	})
}

// saveUpload writes a multipart file to uploadDir under name.
func (s *Server) saveUpload(src multipart.File, name string) error {
	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return err
	}
	dst, err := os.Create(filepath.Join(s.uploadDir, name))
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

// toPublicFilePath mirrors fileStorage.toPublicFilePath.
func toPublicFilePath(name string) string {
	return "/files/" + name
}

// unlinkFile removes the on-disk upload backing a public /files path, ignoring a
// missing file. The DB cascade only removes rows, so deletes/cascades that drop a
// document must unlink its file here.
func (s *Server) unlinkFile(filePath *string) {
	if filePath == nil || !strings.HasPrefix(*filePath, "/files/") {
		return
	}
	_ = os.Remove(filepath.Join(s.uploadDir, path.Base(*filePath)))
}
