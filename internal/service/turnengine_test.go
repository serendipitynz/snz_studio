package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/model"
	"snzstudio/internal/repository"
)

// capturedRequest is the part of a /chat/completions body the prompt-mapping
// assertions read back.
type capturedRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

// turnLLMServer stands in for a participant's LM Studio endpoint: it streams a
// fixed reply and records every prompt it was sent. It answers /models with an
// empty list (which CheckConnection accepts for any model) and 404s the native
// /api/v1 routes, so the endpoint check exercises its non-LM-Studio path.
type turnLLMServer struct {
	*httptest.Server
	reply     string
	onRequest func()

	mu       sync.Mutex
	requests []capturedRequest
}

func newTurnLLMServer(t *testing.T, reply string, onRequest func()) *turnLLMServer {
	t.Helper()
	s := &turnLLMServer{reply: reply, onRequest: onRequest}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/"):
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/models"):
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[]}`)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			var body capturedRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.mu.Lock()
			s.requests = append(s.requests, body)
			s.mu.Unlock()
			if s.onRequest != nil {
				s.onRequest()
			}
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, sseReply(s.reply))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *turnLLMServer) captured() []capturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]capturedRequest(nil), s.requests...)
}

func sseReply(content string) string {
	payload, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]string{"content": content}}},
	})
	return "data: " + string(payload) + "\n\ndata: [DONE]\n\n"
}

// turnGraph wires the repositories a turn needs against one DB, plus the engine.
type turnGraph struct {
	projects     *repository.ProjectRepository
	chats        *repository.ChatRepository
	participants *repository.ParticipantRepository
	engine       *TurnEngine
}

func newTurnGraph(t *testing.T) *turnGraph {
	t.Helper()
	d := newServiceTestDB(t)
	cfg := testConfig(config.Settings{
		Editable:     config.Editable{LLMBaseURL: "http://unused.invalid/v1", LLMModel: "default-model"},
		LLMTimeoutMs: 5000,
	})
	projects := repository.NewProjectRepository(d)
	chats := repository.NewChatRepository(d)
	participants := repository.NewParticipantRepository(d)
	return &turnGraph{
		projects:     projects,
		chats:        chats,
		participants: participants,
		engine:       NewTurnEngine(chats, participants, NewLLMClient(cfg), cfg),
	}
}

// newMultiAgentChat creates a multi-agent chat with the given turn rule and one
// participant per display name, all pointing at baseURL.
func (g *turnGraph) newMultiAgentChat(t *testing.T, turnRule, scenePrompt, baseURL string, names ...string) (model.Chat, []model.Participant) {
	t.Helper()
	project, err := g.projects.CreateProject(repository.CreateProjectInput{Title: "Debate"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	chat, err := g.chats.CreateChat(repository.CreateChatInput{
		ProjectID:   project.ID,
		Kind:        model.ChatKindMultiAgent,
		TurnRule:    turnRule,
		ScenePrompt: scenePrompt,
	})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	roster := make([]model.Participant, 0, len(names))
	for _, name := range names {
		p, err := g.participants.CreateParticipant(repository.CreateParticipantInput{
			ChatID:      chat.ID,
			DisplayName: name,
			RolePrompt:  name + " の役割プロンプト",
			BaseURL:     baseURL,
			ModelName:   "model-" + strings.ToLower(name),
		})
		if err != nil {
			t.Fatalf("CreateParticipant(%s): %v", name, err)
		}
		roster = append(roster, p)
	}
	return chat, roster
}

func (g *turnGraph) addMessage(t *testing.T, chatID, role, content string, participantID *string) {
	t.Helper()
	if _, err := g.chats.AddMessage(repository.AddMessageInput{
		ChatID:        chatID,
		Role:          role,
		Content:       content,
		ParticipantID: participantID,
	}); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
}

// TestTurnEngineRoundRobinCycle covers AC #1: the next speaker follows from the
// stored transcript alone, so the cycle survives without progression state on
// the server. It also covers the saved shape of a turn (AC #3).
func TestTurnEngineRoundRobinCycle(t *testing.T) {
	srv := newTurnLLMServer(t, "発言です", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "論題: ローカル LLM", srv.URL, "Alice", "Bob", "Carol")

	want := []model.Participant{roster[0], roster[1], roster[2], roster[0]}
	for i, expected := range want {
		message, err := g.engine.RunTurn(chat.ID, "", nil)
		if err != nil {
			t.Fatalf("RunTurn #%d: %v", i+1, err)
		}
		if message.ParticipantID == nil || *message.ParticipantID != expected.ID {
			t.Fatalf("turn #%d spoke by %v, want %s (%s)", i+1, message.ParticipantID, expected.ID, expected.DisplayName)
		}
		if message.Role != "assistant" || message.Content != "発言です" {
			t.Fatalf("turn #%d stored as role=%q content=%q", i+1, message.Role, message.Content)
		}
		if message.ModelName == nil || *message.ModelName != expected.ModelName {
			t.Fatalf("turn #%d model = %v, want %s", i+1, message.ModelName, expected.ModelName)
		}
		if message.ResponseMs == nil || message.OutputTokens == nil || message.TokensPerSecond == nil {
			t.Fatalf("turn #%d did not store generation metrics", i+1)
		}
	}

	// Every turn must have gone to the speaker's own endpoint and model.
	requests := srv.captured()
	if len(requests) != len(want) {
		t.Fatalf("%d completions, want %d", len(requests), len(want))
	}
	for i, req := range requests {
		if req.Model != want[i].ModelName {
			t.Fatalf("completion #%d used model %q, want %q", i+1, req.Model, want[i].ModelName)
		}
	}
}

// TestTurnEnginePromptMapping covers AC #3: the prompt is the speaker's own view
// of the conversation (design §4.3).
func TestTurnEnginePromptMapping(t *testing.T) {
	srv := newTurnLLMServer(t, "返答", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleManual, "論題: 地方移住の是非", srv.URL, "Alice", "Bob")
	alice, bob := roster[0], roster[1]

	g.addMessage(t, chat.ID, "user", "では始めてください", nil)
	g.addMessage(t, chat.ID, "assistant", "賛成の立場から述べます", &alice.ID)
	g.addMessage(t, chat.ID, "assistant", "反対の立場から述べます", &bob.ID)
	g.addMessage(t, chat.ID, "assistant", "", &alice.ID) // 中断された空の発言は写像しない

	if _, err := g.engine.RunTurn(chat.ID, alice.ID, nil); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	requests := srv.captured()
	if len(requests) != 1 {
		t.Fatalf("%d completions, want 1", len(requests))
	}
	msgs := requests[0].Messages
	if len(msgs) != 5 {
		t.Fatalf("prompt has %d messages, want 5: %+v", len(msgs), msgs)
	}

	system := msgs[0]
	if system.Role != "system" {
		t.Fatalf("first prompt message role = %q, want system", system.Role)
	}
	for _, fragment := range []string{"論題: 地方移住の是非", alice.RolePrompt, "あなたは「Alice」としてのみ発言する", "Alice, Bob"} {
		if !strings.Contains(system.Content, fragment) {
			t.Fatalf("system prompt missing %q:\n%s", fragment, system.Content)
		}
	}

	// 人間と他参加者は user (「表示名: 本文」)、自分の過去発言は assistant (素の本文)。
	want := []chatMessage{
		{Role: "user", Content: "ユーザー: では始めてください"},
		{Role: "assistant", Content: "賛成の立場から述べます"},
		{Role: "user", Content: "Bob: 反対の立場から述べます"},
	}
	for i, expected := range want {
		if msgs[i+1] != expected {
			t.Fatalf("history[%d] = %+v, want %+v", i, msgs[i+1], expected)
		}
	}
	if last := msgs[len(msgs)-1]; last.Role != "user" || !strings.Contains(last.Content, "Alice") {
		t.Fatalf("turn cue = %+v, want a user message naming Alice", last)
	}
}

// TestTurnEngineManualNomination covers AC #2: the named participant takes the
// turn, even when round_robin would have chosen another one.
func TestTurnEngineManualNomination(t *testing.T) {
	srv := newTurnLLMServer(t, "指名された発言", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleManual, "", srv.URL, "Alice", "Bob", "Carol")
	carol := roster[2]

	message, err := g.engine.RunTurn(chat.ID, carol.ID, nil)
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if message.ParticipantID == nil || *message.ParticipantID != carol.ID {
		t.Fatalf("spoke by %v, want %s (Carol)", message.ParticipantID, carol.ID)
	}

	if _, err := g.engine.RunTurn(chat.ID, "", nil); !errors.Is(err, ErrManualParticipantRequired) {
		t.Fatalf("manual turn without a participant = %v, want ErrManualParticipantRequired", err)
	}
}

// TestTurnEngineRejectsForeignParticipant covers AC #6: a participantId outside
// the chat runs no turn (design §3), and neither does an id nobody owns or one
// that was removed from the roster.
func TestTurnEngineRejectsForeignParticipant(t *testing.T) {
	srv := newTurnLLMServer(t, "発言", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleManual, "", srv.URL, "Alice", "Bob")
	otherChat, otherRoster := g.newMultiAgentChat(t, model.TurnRuleManual, "", srv.URL, "Dave")

	if _, err := g.engine.RunTurn(chat.ID, otherRoster[0].ID, nil); !errors.Is(err, ErrParticipantNotInChat) {
		t.Fatalf("foreign participant = %v, want ErrParticipantNotInChat", err)
	}
	if _, err := g.engine.RunTurn(chat.ID, "participant-does-not-exist", nil); !errors.Is(err, ErrParticipantNotInChat) {
		t.Fatalf("unknown participant = %v, want ErrParticipantNotInChat", err)
	}
	if _, err := g.participants.RemoveParticipant(roster[1].ID); err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}
	if _, err := g.engine.RunTurn(chat.ID, roster[1].ID, nil); !errors.Is(err, ErrParticipantRemoved) {
		t.Fatalf("removed participant = %v, want ErrParticipantRemoved", err)
	}

	for _, id := range []string{chat.ID, otherChat.ID} {
		messages, err := g.chats.ListMessages(id)
		if err != nil {
			t.Fatalf("ListMessages: %v", err)
		}
		if len(messages) != 0 {
			t.Fatalf("chat %s stored %d messages, want none", id, len(messages))
		}
	}
	if len(srv.captured()) != 0 {
		t.Fatalf("a rejected nomination still called the endpoint")
	}
}

// TestTurnEngineRoundRobinAfterRemoval covers AC #5: when the last speaker has
// been removed the cycle has no position to advance from, so it restarts at the
// head of the roster instead of stalling.
func TestTurnEngineRoundRobinAfterRemoval(t *testing.T) {
	srv := newTurnLLMServer(t, "発言", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "", srv.URL, "Alice", "Bob", "Carol")
	alice, bob := roster[0], roster[1]

	g.addMessage(t, chat.ID, "assistant", "Bob の最後の発言", &bob.ID)
	if _, err := g.participants.RemoveParticipant(bob.ID); err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}

	message, err := g.engine.RunTurn(chat.ID, "", nil)
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if message.ParticipantID == nil || *message.ParticipantID != alice.ID {
		t.Fatalf("spoke by %v, want %s (Alice, the roster head)", message.ParticipantID, alice.ID)
	}

	// The removed participant's past message is still attributed by name, since
	// the prompt resolves speakers from every row rather than from the roster.
	requests := srv.captured()
	if len(requests) != 1 {
		t.Fatalf("%d completions, want 1", len(requests))
	}
	if got := requests[0].Messages[1]; got.Content != "Bob: Bob の最後の発言" {
		t.Fatalf("history[0] = %+v, want Bob's line attributed by name", got)
	}
}

// TestTurnEngineConcurrentTurns covers AC #4: two overlapping requests would read
// the same last message and make the same participant speak twice, so the second
// one is refused while the first holds the chat's turn.
func TestTurnEngineConcurrentTurns(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	// Only the first completion is held open; the turn after the release has to
	// run through, so the hook must not block a second time.
	var holdOnce sync.Once
	srv := newTurnLLMServer(t, "発言", func() {
		holdOnce.Do(func() {
			entered <- struct{}{}
			<-release
		})
	})
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "", srv.URL, "Alice", "Bob")

	type turnResult struct {
		message *model.Message
		err     error
	}
	first := make(chan turnResult, 1)
	go func() {
		message, err := g.engine.RunTurn(chat.ID, "", nil)
		first <- turnResult{message, err}
	}()

	<-entered // the first turn is now mid-completion, before it has stored anything
	if _, err := g.engine.RunTurn(chat.ID, "", nil); !errors.Is(err, ErrTurnInProgress) {
		t.Fatalf("overlapping turn = %v, want ErrTurnInProgress", err)
	}
	close(release)

	got := <-first
	if got.err != nil {
		t.Fatalf("first turn: %v", got.err)
	}
	if got.message.ParticipantID == nil || *got.message.ParticipantID != roster[0].ID {
		t.Fatalf("first turn spoke by %v, want %s (Alice)", got.message.ParticipantID, roster[0].ID)
	}

	messages, err := g.chats.ListMessages(chat.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("%d messages stored, want 1 — the refused turn must not speak", len(messages))
	}

	// The lock is per chat and released with the turn, so the cycle continues.
	next, err := g.engine.RunTurn(chat.ID, "", nil)
	if err != nil {
		t.Fatalf("turn after release: %v", err)
	}
	if next.ParticipantID == nil || *next.ParticipantID != roster[1].ID {
		t.Fatalf("next turn spoke by %v, want %s (Bob)", next.ParticipantID, roster[1].ID)
	}
}

// TestTurnEngineRejectsNonMultiAgentChat keeps the single-assistant chats out of
// this path: their generation stays with ChatService (design §4.4).
func TestTurnEngineRejectsNonMultiAgentChat(t *testing.T) {
	srv := newTurnLLMServer(t, "発言", nil)
	g := newTurnGraph(t)
	project, err := g.projects.CreateProject(repository.CreateProjectInput{Title: "Plain"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	chat, err := g.chats.CreateChat(repository.CreateChatInput{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	if _, err := g.engine.RunTurn(chat.ID, "", nil); !errors.Is(err, ErrNotMultiAgentChat) {
		t.Fatalf("assistant chat = %v, want ErrNotMultiAgentChat", err)
	}
	if _, err := g.engine.RunTurn("chat-does-not-exist", "", nil); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("missing chat = %v, want ErrChatNotFound", err)
	}

	// A round_robin chat with an empty roster has nobody to speak.
	multiAgent, _ := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "", srv.URL)
	if _, err := g.engine.RunTurn(multiAgent.ID, "", nil); !errors.Is(err, ErrRosterEmpty) {
		t.Fatalf("empty roster = %v, want ErrRosterEmpty", err)
	}
	// Naming a speaker under round_robin is a mismatch, not a silent override.
	other, otherRoster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "", srv.URL, "Alice")
	if _, err := g.engine.RunTurn(other.ID, otherRoster[0].ID, nil); !errors.Is(err, ErrParticipantNotNameable) {
		t.Fatalf("round_robin nomination = %v, want ErrParticipantNotNameable", err)
	}
}
