package service

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/repository"
)

// failingLLMServer returns a server that always answers 500, so every LLM call
// fails and the services exercise their offline fallback paths.
func failingLLMServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// serviceGraph wires the full set of repositories and services against one DB and a
// (failing) LLM endpoint, with embeddings disabled.
type serviceGraph struct {
	db        *sql.DB
	projects  *repository.ProjectRepository
	documents *repository.DocumentRepository
	memories  *repository.MemoryRepository
	chats     *repository.ChatRepository
	chat      *ChatService
	organizer *MemoryOrganizerService
}

func newServiceGraph(t *testing.T, llmBaseURL string) *serviceGraph {
	t.Helper()
	d := newServiceTestDB(t)
	cfg := testConfig(config.Settings{
		Editable:     config.Editable{LLMBaseURL: llmBaseURL, LLMModel: "test-model"},
		LLMTimeoutMs: 2000,
	})

	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	memories := repository.NewMemoryRepository(d)
	chats := repository.NewChatRepository(d)

	embeddings := NewEmbeddingClient(cfg) // disabled: no embedding model
	llm := NewLLMClient(cfg)
	retrieval := NewRetrievalService(d, embeddings)
	embeddingSync := NewEmbeddingSyncService(documents, memories, embeddings)
	contextSvc := NewContextService(projects, chats, documents, memories, retrieval)
	summary := NewSummaryService(llm)
	memoryService := NewMemoryService(memories, llm)
	chatSvc := NewChatService(chats, contextSvc, llm, summary, memoryService, embeddingSync, cfg)
	organizer := NewMemoryOrganizerService(memories, chats, llm, embeddingSync)

	return &serviceGraph{
		db:        d,
		projects:  projects,
		documents: documents,
		memories:  memories,
		chats:     chats,
		chat:      chatSvc,
		organizer: organizer,
	}
}

func TestSendMessageFallbackPath(t *testing.T) {
	srv := failingLLMServer(t)
	g := newServiceGraph(t, srv.URL)

	project, err := g.projects.CreateProject(repository.CreateProjectInput{Title: "Saga", Description: "A fantasy project"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	chat, err := g.chats.CreateChat(repository.CreateChatInput{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	assistant, err := g.chat.SendMessage(chat.ID, "この物語の設定について教えて")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if assistant.Role != "assistant" {
		t.Fatalf("role = %q, want assistant", assistant.Role)
	}
	if !strings.HasPrefix(assistant.Content, "Local LLM endpoint could not be reached") {
		t.Fatalf("expected fallback response, got %q", assistant.Content)
	}
	// The fallback carries null metrics (no successful generation).
	if assistant.ResponseMs != nil || assistant.ModelName != nil {
		t.Fatalf("fallback metrics should be nil, got responseMs=%v modelName=%v", assistant.ResponseMs, assistant.ModelName)
	}

	// User + assistant turns are persisted in order.
	messages, err := g.chats.ListMessages(chat.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("unexpected message log: %+v", messages)
	}

	// References were attached, including the always-present project reference.
	withRefs, err := g.chats.GetMessagesWithReferences(chat.ID)
	if err != nil {
		t.Fatalf("GetMessagesWithReferences: %v", err)
	}
	var assistantRefs []string
	for _, m := range withRefs {
		if m.Role == "assistant" {
			for _, ref := range m.References {
				assistantRefs = append(assistantRefs, ref.SourceType)
			}
		}
	}
	if !containsString(assistantRefs, "project") {
		t.Fatalf("expected a project reference, got %v", assistantRefs)
	}

	// An empty-title chat gets a fallback title from the first user message.
	updated, err := g.chats.GetChat(chat.ID)
	if err != nil {
		t.Fatalf("GetChat: %v", err)
	}
	if strings.TrimSpace(updated.Title) == "" {
		t.Fatal("expected a fallback chat title to be set")
	}
}

func TestTemporaryChatSkipsMemoryExtraction(t *testing.T) {
	srv := failingLLMServer(t)
	g := newServiceGraph(t, srv.URL)

	project, err := g.projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	chat, err := g.chats.CreateChat(repository.CreateChatInput{ProjectID: project.ID, IsTemporary: true})
	if err != nil {
		t.Fatalf("CreateChat temporary: %v", err)
	}

	// This message carries a durable procedural cue that would normally be stored.
	if _, err := g.chat.SendMessage(chat.ID, "今後は必ず日本語で回答してください。"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	memories, err := g.memories.ListByProject(project.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(memories) != 0 {
		t.Fatalf("temporary chat must not extract memories, got %d", len(memories))
	}
}

// TestChatTurnRefusedWhileGenerating covers TASK-83 AC #1 in the service: while a
// single-assistant turn is generating, a second send to the same chat — on
// either path — is refused with ErrTurnInProgress and stores nothing, and the
// slot is free again once the first turn finishes.
func TestChatTurnRefusedWhileGenerating(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			close(entered)
			<-release
		})
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	g := newServiceGraph(t, srv.URL)

	project, err := g.projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	chat, err := g.chats.CreateChat(repository.CreateChatInput{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := g.chat.SendMessageStream(chat.ID, "最初の質問", func() {}, func(string) {})
		firstDone <- err
	}()
	<-entered // the first turn is waiting on its completion

	if _, err := g.chat.SendMessage(chat.ID, "割り込み"); !errors.Is(err, ErrTurnInProgress) {
		t.Fatalf("SendMessage during a turn: err = %v, want ErrTurnInProgress", err)
	}
	started := false
	if _, err := g.chat.SendMessageStream(chat.ID, "割り込み", func() { started = true }, func(string) {}); !errors.Is(err, ErrTurnInProgress) {
		t.Fatalf("SendMessageStream during a turn: err = %v, want ErrTurnInProgress", err)
	}
	if started {
		t.Fatal("a refused stream must not start")
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first turn: %v", err)
	}
	messages, err := g.chats.ListMessages(chat.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("%d messages stored, want 2 — the refused sends must store nothing", len(messages))
	}
	if _, err := g.chat.SendMessage(chat.ID, "次の質問"); err != nil {
		t.Fatalf("SendMessage after the turn: %v", err)
	}
}

// TestChatTurnMemoryFailureLeavesNoUserMessage covers TASK-83 AC #3: when storing
// a memory fails, the turn fails before the user message is stored and before a
// stream would start, so no user message is left without a reply.
func TestChatTurnMemoryFailureLeavesNoUserMessage(t *testing.T) {
	srv := failingLLMServer(t)
	g := newServiceGraph(t, srv.URL)

	project, err := g.projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	chat, err := g.chats.CreateChat(repository.CreateChatInput{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if _, err := g.db.Exec(`CREATE TRIGGER fail_memory_insert BEFORE INSERT ON memories
		BEGIN SELECT RAISE(ABORT, 'memory store failed'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	// A durable procedural cue, so the turn tries to store a memory.
	content := "今後は必ず日本語で回答してください。"
	if _, err := g.chat.SendMessage(chat.ID, content); err == nil {
		t.Fatal("SendMessage: want the memory failure, got nil")
	}
	started := false
	if _, err := g.chat.SendMessageStream(chat.ID, content, func() { started = true }, func(string) {}); err == nil {
		t.Fatal("SendMessageStream: want the memory failure, got nil")
	}
	if started {
		t.Fatal("the stream must not start when the memory store fails")
	}

	messages, err := g.chats.ListMessages(chat.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("%d messages stored, want none: %+v", len(messages), messages)
	}
}
