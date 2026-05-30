package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
