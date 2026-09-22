package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

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
	documents    *repository.DocumentRepository
	memories     *repository.MemoryRepository
	cfg          *config.Config
	material     *ContextService
	engine       *TurnEngine
}

func newTurnGraph(t *testing.T) *turnGraph {
	t.Helper()
	d := newServiceTestDB(t)
	cfg := testConfig(config.Settings{
		Editable:     config.Editable{LLMBaseURL: "http://unused.invalid/v1", LLMModel: "default-model"},
		LLMTimeoutMs: 5000,
	})
	return newTurnGraphWithConfig(t, d, cfg)
}

// newTurnGraphWithConfig wires the engine the way NewServer does, with the
// ContextService as the project material assembler over a retrieval that follows cfg's
// embedding settings (FTS-only when no embedding model is configured).
func newTurnGraphWithConfig(t *testing.T, d *sql.DB, cfg *config.Config) *turnGraph {
	t.Helper()
	projects := repository.NewProjectRepository(d)
	chats := repository.NewChatRepository(d)
	participants := repository.NewParticipantRepository(d)
	documents := repository.NewDocumentRepository(d)
	memories := repository.NewMemoryRepository(d)
	retrieval := NewRetrievalService(d, NewEmbeddingClient(cfg))
	material := NewContextService(projects, chats, documents, memories, retrieval)
	return &turnGraph{
		projects:     projects,
		chats:        chats,
		participants: participants,
		documents:    documents,
		memories:     memories,
		cfg:          cfg,
		material:     material,
		engine:       NewTurnEngine(chats, participants, NewLLMClient(cfg), cfg, material),
	}
}

// newMultiAgentChat creates a multi-agent chat with the given turn rule and one
// participant per display name, all pointing at baseURL.
func (g *turnGraph) newMultiAgentChat(t *testing.T, turnRule, scenePrompt, baseURL string, names ...string) (model.Chat, []model.Participant) {
	t.Helper()
	project := g.newProject(t, repository.CreateProjectInput{Title: "Debate"})
	return g.newMultiAgentChatInProject(t, project.ID, turnRule, scenePrompt, baseURL, names...)
}

func (g *turnGraph) newProject(t *testing.T, input repository.CreateProjectInput) model.Project {
	t.Helper()
	project, err := g.projects.CreateProject(input)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return project
}

func (g *turnGraph) newMultiAgentChatInProject(t *testing.T, projectID, turnRule, scenePrompt, baseURL string, names ...string) (model.Chat, []model.Participant) {
	t.Helper()
	chat, err := g.chats.CreateChat(repository.CreateChatInput{
		ProjectID:   projectID,
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
		message, err := g.engine.RunTurn(chat.ID, "", nil, nil)
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

	if _, err := g.engine.RunTurn(chat.ID, alice.ID, nil, nil); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	requests := srv.captured()
	if len(requests) != 1 {
		t.Fatalf("%d completions, want 1", len(requests))
	}
	msgs := requests[0].Messages
	if len(msgs) != 4 {
		t.Fatalf("prompt has %d messages, want 4: %+v", len(msgs), msgs)
	}

	system := msgs[0]
	if system.Role != "system" {
		t.Fatalf("first prompt message role = %q, want system", system.Role)
	}
	for _, fragment := range []string{
		"論題: 地方移住の是非",
		alice.RolePrompt,
		"あなたは「Alice」としてのみ発言する",
		"Alice, Bob",
		"直前の発言のどこに反応しているか",
		"発言の長さは場面設定の指定に従う",
	} {
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
	// The prompt ends with the utterance to answer. Ending it with the hand-off
	// instead lets a model that reasons before answering read it as an
	// instruction about the conversation and return no content at all.
	if last := msgs[len(msgs)-1]; last.Role != "user" || last.Content != "Bob: 反対の立場から述べます" {
		t.Fatalf("final prompt message = %+v, want Bob's utterance", last)
	}
}

// TestTurnEngineCueWhenNothingToAnswer covers the two turns whose prompt carries
// no utterance for the speaker: the opening turn of an empty transcript, and a
// manual re-nomination of the participant who just spoke. Both fall back to the
// hand-off cue, because the final user slot cannot be left blank.
func TestTurnEngineCueWhenNothingToAnswer(t *testing.T) {
	srv := newTurnLLMServer(t, "発言します", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleManual, "場面設定", srv.URL, "Alice", "Bob")
	alice := roster[0]

	for _, label := range []string{"opening", "re-nomination"} {
		if _, err := g.engine.RunTurn(chat.ID, alice.ID, nil, nil); err != nil {
			t.Fatalf("RunTurn (%s): %v", label, err)
		}
	}

	requests := srv.captured()
	if len(requests) != 2 {
		t.Fatalf("%d completions, want 2", len(requests))
	}
	for i, req := range requests {
		last := req.Messages[len(req.Messages)-1]
		if last.Role != "user" || !strings.Contains(last.Content, "Alice") {
			t.Fatalf("turn %d ends with %+v, want the hand-off naming Alice", i+1, last)
		}
	}
}

// TestTurnEngineManualNomination covers AC #2: the named participant takes the
// turn, even when round_robin would have chosen another one.
func TestTurnEngineManualNomination(t *testing.T) {
	srv := newTurnLLMServer(t, "指名された発言", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleManual, "", srv.URL, "Alice", "Bob", "Carol")
	carol := roster[2]

	message, err := g.engine.RunTurn(chat.ID, carol.ID, nil, nil)
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if message.ParticipantID == nil || *message.ParticipantID != carol.ID {
		t.Fatalf("spoke by %v, want %s (Carol)", message.ParticipantID, carol.ID)
	}

	if _, err := g.engine.RunTurn(chat.ID, "", nil, nil); !errors.Is(err, ErrManualParticipantRequired) {
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

	if _, err := g.engine.RunTurn(chat.ID, otherRoster[0].ID, nil, nil); !errors.Is(err, ErrParticipantNotInChat) {
		t.Fatalf("foreign participant = %v, want ErrParticipantNotInChat", err)
	}
	if _, err := g.engine.RunTurn(chat.ID, "participant-does-not-exist", nil, nil); !errors.Is(err, ErrParticipantNotInChat) {
		t.Fatalf("unknown participant = %v, want ErrParticipantNotInChat", err)
	}
	if _, err := g.participants.RemoveParticipant(roster[1].ID); err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}
	if _, err := g.engine.RunTurn(chat.ID, roster[1].ID, nil, nil); !errors.Is(err, ErrParticipantRemoved) {
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

	message, err := g.engine.RunTurn(chat.ID, "", nil, nil)
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

// setFacilitator points the chat's facilitator_alternating rule at one
// participant and returns the updated chat.
func (g *turnGraph) setFacilitator(t *testing.T, chatID, participantID string) model.Chat {
	t.Helper()
	chat, err := g.chats.UpdateMultiAgentSettings(chatID, nil, nil, &participantID)
	if err != nil || chat == nil {
		t.Fatalf("UpdateMultiAgentSettings(facilitator) = %v, %v", chat, err)
	}
	return *chat
}

func (g *turnGraph) runTurns(t *testing.T, chatID string, want []model.Participant) {
	t.Helper()
	for i, expected := range want {
		message, err := g.engine.RunTurn(chatID, "", nil, nil)
		if err != nil {
			t.Fatalf("RunTurn #%d: %v", i+1, err)
		}
		if message.ParticipantID == nil || *message.ParticipantID != expected.ID {
			t.Fatalf("turn #%d spoke by %v, want %s (%s)", i+1, message.ParticipantID, expected.ID, expected.DisplayName)
		}
	}
}

// TestTurnEngineFacilitatorAlternating covers TASK-21 AC #1 and #2: the
// facilitator speaks every other turn, the rest of the roster cycles in
// sort_order between its turns, and the whole order comes out of the stored
// transcript — a second engine over the same database, holding no progression
// state of its own, continues where the first left off.
//
// The facilitator sits in the middle of the roster so the opening turn cannot
// pass by being the roster's head.
func TestTurnEngineFacilitatorAlternating(t *testing.T) {
	srv := newTurnLLMServer(t, "発言", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleFacilitatorAlternating, "灰縁坑の調査", srv.URL, "Player1", "GM", "Player2")
	player1, gm, player2 := roster[0], roster[1], roster[2]
	g.setFacilitator(t, chat.ID, gm.ID)

	g.runTurns(t, chat.ID, []model.Participant{gm, player1, gm})

	g.engine = NewTurnEngine(g.chats, g.participants, NewLLMClient(g.cfg), g.cfg, g.material)
	g.runTurns(t, chat.ID, []model.Participant{player2, gm, player1})
}

// TestTurnEngineFacilitatorAnswersIntervention covers TASK-21 AC #3: the human's
// intervention carries no participant_id, and under this rule the facilitator is
// the one who answers it. The cycle of the others is not reset by it — the
// player who had not spoken yet is still next after the facilitator.
func TestTurnEngineFacilitatorAnswersIntervention(t *testing.T) {
	srv := newTurnLLMServer(t, "発言", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleFacilitatorAlternating, "", srv.URL, "GM", "Player1", "Player2")
	gm, player1, player2 := roster[0], roster[1], roster[2]
	g.setFacilitator(t, chat.ID, gm.ID)

	g.addMessage(t, chat.ID, "assistant", "坑道の入口に立っています", &gm.ID)
	g.addMessage(t, chat.ID, "assistant", "足跡を調べます", &player1.ID)
	g.addMessage(t, chat.ID, "user", "（GM へ）そろそろ物音を", nil)

	g.runTurns(t, chat.ID, []model.Participant{gm, player2})
}

// TestTurnEngineFacilitatorFallsBackToRoundRobin covers TASK-21 AC #3: a
// facilitator that has left the roster, and one never chosen, both leave the
// rule without the participant it needs, and the turn proceeds in roster order
// rather than failing — an auto-advancing conversation must not stop on a state
// a removal can reach at any time.
func TestTurnEngineFacilitatorFallsBackToRoundRobin(t *testing.T) {
	srv := newTurnLLMServer(t, "発言", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleFacilitatorAlternating, "", srv.URL, "GM", "Player1", "Player2")
	gm, player1, player2 := roster[0], roster[1], roster[2]
	g.setFacilitator(t, chat.ID, gm.ID)

	g.addMessage(t, chat.ID, "assistant", "坑道の入口に立っています", &gm.ID)
	g.addMessage(t, chat.ID, "assistant", "足跡を調べます", &player1.ID)
	if _, err := g.participants.RemoveParticipant(gm.ID); err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}

	// round_robin's own derivation over the roster the removal left: Player1 spoke
	// last, so Player2 follows, and the cycle wraps back to Player1.
	g.runTurns(t, chat.ID, []model.Participant{player2, player1})

	unset, _ := g.newMultiAgentChat(t, model.TurnRuleFacilitatorAlternating, "", srv.URL, "Alice", "Bob")
	message, err := g.engine.RunTurn(unset.ID, "", nil, nil)
	if err != nil {
		t.Fatalf("RunTurn(no facilitator): %v", err)
	}
	if message.ParticipantID == nil {
		t.Fatalf("no facilitator: turn stored without a speaker")
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
		message, err := g.engine.RunTurn(chat.ID, "", nil, nil)
		first <- turnResult{message, err}
	}()

	<-entered // the first turn is now mid-completion, before it has stored anything
	if _, err := g.engine.RunTurn(chat.ID, "", nil, nil); !errors.Is(err, ErrTurnInProgress) {
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
	next, err := g.engine.RunTurn(chat.ID, "", nil, nil)
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

	if _, err := g.engine.RunTurn(chat.ID, "", nil, nil); !errors.Is(err, ErrNotMultiAgentChat) {
		t.Fatalf("assistant chat = %v, want ErrNotMultiAgentChat", err)
	}
	if _, err := g.engine.RunTurn("chat-does-not-exist", "", nil, nil); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("missing chat = %v, want ErrChatNotFound", err)
	}

	// A round_robin chat with an empty roster has nobody to speak.
	multiAgent, _ := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "", srv.URL)
	if _, err := g.engine.RunTurn(multiAgent.ID, "", nil, nil); !errors.Is(err, ErrRosterEmpty) {
		t.Fatalf("empty roster = %v, want ErrRosterEmpty", err)
	}
	// Naming a speaker under round_robin is a mismatch, not a silent override.
	other, otherRoster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "", srv.URL, "Alice")
	if _, err := g.engine.RunTurn(other.ID, otherRoster[0].ID, nil, nil); !errors.Is(err, ErrParticipantNotNameable) {
		t.Fatalf("round_robin nomination = %v, want ErrParticipantNotNameable", err)
	}
}

// modelListingServer answers /v1/models with a real, non-empty list the way a
// live LM Studio does, so the pre-turn check has to match a model name against
// it rather than taking the empty-list shortcut turnLLMServer relies on.
func modelListingServer(t *testing.T, models ...string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/"):
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/models"):
			entries := make([]map[string]string, 0, len(models))
			for _, name := range models {
				entries = append(entries, map[string]string{"id": name})
			}
			payload, err := json.Marshal(map[string]any{"data": entries})
			if err != nil {
				t.Errorf("marshal model list: %v", err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(payload)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, sseReply("発言"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

// TestTurnEngineInheritedTargetInFailure covers TASK-11: a participant may leave
// the endpoint or the model blank to inherit the workspace setting, and the two
// inherit independently — so a participant can run its own endpoint with the
// workspace model. When that combination is refused, the error has to name the
// values the turn actually used; naming the participant's raw fields would print
// a blank exactly where the inherited value was, leaving the reader with an
// endpoint failure that names no model at all.
func TestTurnEngineInheritedTargetInFailure(t *testing.T) {
	endpoint := modelListingServer(t, "qwen3-8b")
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "論題", endpoint.URL, "Alice")
	if _, err := g.participants.UpdateParticipant(repository.UpdateParticipantInput{
		ParticipantID: roster[0].ID,
		ModelName:     strPtr(""),
	}); err != nil {
		t.Fatalf("UpdateParticipant: %v", err)
	}

	_, err := g.engine.RunTurn(chat.ID, "", nil, nil)
	if !errors.Is(err, ErrEndpointUnavailable) {
		t.Fatalf("blank model against an endpoint without the workspace model = %v, want ErrEndpointUnavailable", err)
	}
	// "default-model" is the workspace model newTurnGraph configures, and the
	// participant's own endpoint is where it was tried.
	if !strings.Contains(err.Error(), "default-model") || !strings.Contains(err.Error(), endpoint.URL) {
		t.Fatalf("error %q does not name the effective model and endpoint", err)
	}
}

// TestTurnEngineInheritedModelRuns is the other half: the same participant with
// a blank model runs when its own endpoint does serve the workspace model, and
// the turn is recorded under that model rather than under a blank name.
func TestTurnEngineInheritedModelRuns(t *testing.T) {
	endpoint := modelListingServer(t, "qwen3-8b", "default-model")
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "論題", endpoint.URL, "Alice")
	if _, err := g.participants.UpdateParticipant(repository.UpdateParticipantInput{
		ParticipantID: roster[0].ID,
		ModelName:     strPtr(""),
	}); err != nil {
		t.Fatalf("UpdateParticipant: %v", err)
	}

	message, err := g.engine.RunTurn(chat.ID, "", nil, nil)
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if message.ModelName == nil || *message.ModelName != "default-model" {
		t.Fatalf("stored model = %v, want the inherited workspace model", message.ModelName)
	}
}

// TestTurnEngineInheritedTargetSurvivesConfigChange covers the window between
// the pre-turn check and the completion. A participant that inherits both fields
// resolves them from one settings snapshot, so a Configuration save landing
// mid-turn cannot send the turn to a combination the check never approved.
func TestTurnEngineInheritedTargetSurvivesConfigChange(t *testing.T) {
	g := newTurnGraph(t)
	var endpointURL string
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/"):
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/models"):
			// The pre-turn check is reading the model list right now; save a
			// different workspace model while it does, the way the Configuration
			// screen can at any moment.
			if _, err := g.cfg.UpdateEditable(config.Editable{
				LLMBaseURL: endpointURL,
				LLMModel:   "swapped-in-mid-turn",
			}); err != nil {
				t.Errorf("UpdateEditable: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"default-model"}]}`)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			var body capturedRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode completion body: %v", err)
				return
			}
			if body.Model != "default-model" {
				t.Errorf("completion ran model %q, want the model the pre-turn check approved", body.Model)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, sseReply("発言"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(endpoint.Close)
	endpointURL = endpoint.URL

	if _, err := g.cfg.UpdateEditable(config.Editable{LLMBaseURL: endpoint.URL, LLMModel: "default-model"}); err != nil {
		t.Fatalf("UpdateEditable: %v", err)
	}
	// Both fields blank, so both are inherited.
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "論題", "", "Alice")
	if _, err := g.participants.UpdateParticipant(repository.UpdateParticipantInput{
		ParticipantID: roster[0].ID,
		ModelName:     strPtr(""),
	}); err != nil {
		t.Fatalf("UpdateParticipant: %v", err)
	}

	message, err := g.engine.RunTurn(chat.ID, "", nil, nil)
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if message.ModelName == nil || *message.ModelName != "default-model" {
		t.Fatalf("stored model = %v, want the model resolved before the check", message.ModelName)
	}
}

// TestMessageWriteExclusion covers what the message-write lock is for: a human
// message and a preset apply on the same chat cannot interleave, while a turn in
// flight still lets a human speak (design §4.4).
func TestMessageWriteExclusion(t *testing.T) {
	engine := newTurnGraph(t).engine

	held := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan struct{})
	go func() {
		_ = engine.WithMessageWrite("chat-1", func() error {
			close(held)
			<-release
			return nil
		})
		close(firstDone)
	}()

	<-held
	second := make(chan struct{})
	go func() {
		_ = engine.WithMessageWrite("chat-1", func() error { return nil })
		close(second)
	}()
	select {
	case <-second:
		t.Fatal("a second message write ran while the first held the chat's lock")
	case <-time.After(50 * time.Millisecond):
	}

	// Another chat is never blocked by it, and neither is a turn: the lock is per
	// chat and separate from the turn slot.
	if err := engine.WithMessageWrite("chat-2", func() error { return nil }); err != nil {
		t.Fatalf("message write on another chat = %v, want nil", err)
	}
	if !engine.acquireTurn("chat-1") {
		t.Fatal("a turn could not start while a message write held chat-1 — speaking mid-turn must stay possible in both directions")
	}
	engine.releaseTurn("chat-1")

	close(release)
	<-firstDone
	<-second
}

// TestMessageWriteIsNotTheTurnSlot pins the half that keeps the fix from
// regressing intervention: a turn holds its own slot for as long as a completion
// takes, and a human message must not wait for it.
func TestMessageWriteIsNotTheTurnSlot(t *testing.T) {
	engine := newTurnGraph(t).engine
	if !engine.acquireTurn("chat-1") {
		t.Fatal("could not take the turn slot")
	}
	defer engine.releaseTurn("chat-1")

	done := make(chan struct{})
	go func() {
		_ = engine.WithMessageWrite("chat-1", func() error { return nil })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("a human message waited on a turn in flight")
	}
}

// TestTurnEngineWeighted covers TASK-34 AC #1, #6 and #7 end to end: under the
// weighted rule the directive is asked for in the reminder, stripped from the
// stored utterance and resolved to participant ids at store time, and the call it
// made decides the next turn. The speaker event carries the breakdown.
func TestTurnEngineWeighted(t *testing.T) {
	srv := newTurnLLMServer(t, "扉が開いた。\n[次: Bob]", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleWeighted, "", srv.URL, "GM", "Alice", "Bob")
	gm, bob := roster[0], roster[2]
	g.setFacilitator(t, chat.ID, gm.ID)

	var choices []SpeakerChoice
	onSpeaker := func(choice SpeakerChoice) { choices = append(choices, choice) }

	// Nobody has spoken, so everyone weighs 1 and sort_order takes GM.
	first, err := g.engine.RunTurn(chat.ID, "", onSpeaker, nil)
	if err != nil {
		t.Fatalf("RunTurn #1: %v", err)
	}
	if *first.ParticipantID != gm.ID || first.Content != "扉が開いた。" || !reflect.DeepEqual(first.AddressedParticipantIDs, []string{bob.ID}) {
		t.Fatalf("turn #1 = (%s, %q, %v), want GM calling Bob with the directive stripped", *first.ParticipantID, first.Content, first.AddressedParticipantIDs)
	}

	// GM called on Bob, so Bob's ×1.2 beats Alice's 1.0. Bob's own directive
	// names himself and is dropped, but the line is still stripped.
	second, err := g.engine.RunTurn(chat.ID, "", onSpeaker, nil)
	if err != nil {
		t.Fatalf("RunTurn #2: %v", err)
	}
	if *second.ParticipantID != bob.ID || second.Content != "扉が開いた。" || len(second.AddressedParticipantIDs) != 0 {
		t.Fatalf("turn #2 = (%s, %q, %v), want Bob answering, calling no one", *second.ParticipantID, second.Content, second.AddressedParticipantIDs)
	}

	stored, err := g.chats.ListMessages(chat.ID)
	if err != nil || len(stored) != 2 || !reflect.DeepEqual(stored[0].AddressedParticipantIDs, []string{bob.ID}) {
		t.Fatalf("stored = %+v, %v, want the call read back from the database", stored, err)
	}

	if len(choices) != 2 || len(choices[1].Weights) != len(roster) {
		t.Fatalf("speaker events = %+v, want one weight per participant", choices)
	}
	if w := choices[1].Weights[2]; w.ParticipantID != bob.ID || w.DisplayName != "Bob" || len(w.Factors) != 1 || w.Factors[0].Name != callFactor {
		t.Fatalf("Bob's breakdown = %+v, want the call factor alone", w)
	}

	for i, req := range srv.captured() {
		if !strings.Contains(req.Messages[0].Content, "[次: 表示名]") {
			t.Fatalf("completion #%d system prompt lacks the directive instruction", i+1)
		}
	}
}

// TestTurnEngineExistingRulesIgnoreCalls covers TASK-34 AC #3: a call stored on a
// message moves none of the existing rules, and their prompts do not ask for the
// directive. The directive a model writes anyway is still stripped and stored,
// so a chat switched to weighted later reads it.
func TestTurnEngineExistingRulesIgnoreCalls(t *testing.T) {
	srv := newTurnLLMServer(t, "賛成です。\n[次: Carol]", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "", srv.URL, "Alice", "Bob", "Carol")
	alice, bob, carol := roster[0], roster[1], roster[2]

	g.runTurns(t, chat.ID, []model.Participant{alice, bob, carol, alice})

	stored, err := g.chats.ListMessages(chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored[0].Content != "賛成です。" || !reflect.DeepEqual(stored[0].AddressedParticipantIDs, []string{carol.ID}) {
		t.Fatalf("first turn stored as (%q, %v), want the directive stripped and resolved", stored[0].Content, stored[0].AddressedParticipantIDs)
	}
	for i, req := range srv.captured() {
		if strings.Contains(req.Messages[0].Content, "[次:") {
			t.Fatalf("completion #%d under round_robin asks for the directive", i+1)
		}
	}

	facilitated, roster := g.newMultiAgentChat(t, model.TurnRuleFacilitatorAlternating, "", srv.URL, "GM", "P1", "P2")
	g.setFacilitator(t, facilitated.ID, roster[0].ID)
	g.addMessage(t, facilitated.ID, "user", "P2、どう？", nil)
	g.runTurns(t, facilitated.ID, []model.Participant{roster[0], roster[1], roster[0], roster[2]})
}

// TestTurnEngineDirectiveOnlyUtteranceFails: a reply that is nothing but the
// directive would store an empty utterance, which the engine refuses the way it
// refuses an empty completion — a turn with no content must not enter the
// transcript.
func TestTurnEngineDirectiveOnlyUtteranceFails(t *testing.T) {
	srv := newTurnLLMServer(t, "[次: Bob]", nil)
	g := newTurnGraph(t)
	chat, _ := g.newMultiAgentChat(t, model.TurnRuleWeighted, "", srv.URL, "Alice", "Bob")
	if _, err := g.engine.RunTurn(chat.ID, "", nil, nil); !errors.Is(err, ErrUtteranceOnlyDirective) {
		t.Fatalf("RunTurn = %v, want ErrUtteranceOnlyDirective", err)
	}
	if stored, _ := g.chats.ListMessages(chat.ID); len(stored) != 0 {
		t.Fatalf("stored %d messages, want none", len(stored))
	}
}
