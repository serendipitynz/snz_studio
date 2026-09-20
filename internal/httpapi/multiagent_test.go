package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// multiAgentLLM stands in for a participant's LM Studio endpoint: it answers
// /models (which CheckConnection accepts) and streams one fixed reply per
// completion. hold, when set, blocks inside the first completion so a test can
// keep a turn in flight while it fires the next request.
func newMultiAgentLLM(t *testing.T, hold func()) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/"):
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/models"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[]}`)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			if hold != nil {
				hold()
			}
			payload, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]string{"content": "発言します"}}},
			})
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: "+string(payload)+"\n\ndata: [DONE]\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func createMultiAgentChat(t *testing.T, h http.Handler, projectID, turnRule string) string {
	t.Helper()
	rec := doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats",
		map[string]any{"title": "Debate", "kind": "multi_agent"})
	wantStatus(t, rec, http.StatusCreated)
	var chat struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"`
		TurnRule string `json:"turnRule"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "chat", &chat)
	if chat.Kind != "multi_agent" {
		t.Fatalf("created chat kind = %q, want multi_agent", chat.Kind)
	}
	if turnRule != "" && turnRule != chat.TurnRule {
		wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+chat.ID, map[string]any{"turnRule": turnRule}), http.StatusOK)
	}
	return chat.ID
}

func addParticipant(t *testing.T, h http.Handler, chatID, displayName, baseURL string) string {
	t.Helper()
	rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/participants", map[string]any{
		"displayName": displayName,
		"rolePrompt":  displayName + " の役割",
		"baseUrl":     baseURL,
		"modelName":   "model-" + strings.ToLower(displayName),
	})
	wantStatus(t, rec, http.StatusCreated)
	var participant struct {
		ID string `json:"id"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "participant", &participant)
	return participant.ID
}

func doneFrame(t *testing.T, rec *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	events := parseSSE(t, rec.Body.String())
	done, ok := events["done"]
	if !ok {
		t.Fatalf("expected a done event, got events=%v (body=%s)", mapEventNames(events), rec.Body.String())
	}
	payload := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(done), &payload); err != nil {
		t.Fatalf("decode done payload: %v", err)
	}
	return payload
}

// TestMultiAgentTurnStream is the HTTP half of AC #1: a chat created with
// kind=multi_agent, two participants registered over the API, and a turn that
// streams its deltas and stores the speech under the speaker's participant id.
func TestMultiAgentTurnStream(t *testing.T) {
	llm := newMultiAgentLLM(t, nil)
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Debate Project")
	chatID := createMultiAgentChat(t, h, projectID, "round_robin")
	alice := addParticipant(t, h, chatID, "Alice", llm.URL+"/v1")
	bob := addParticipant(t, h, chatID, "Bob", llm.URL+"/v1")

	rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{})
	wantStatus(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	events := parseSSE(t, rec.Body.String())
	if _, ok := events["delta"]; !ok {
		t.Fatalf("expected a delta event, got events=%v", mapEventNames(events))
	}

	payload := doneFrame(t, rec)
	var spoken struct {
		Role          string  `json:"role"`
		Content       string  `json:"content"`
		ParticipantID *string `json:"participantId"`
	}
	if err := json.Unmarshal(payload["message"], &spoken); err != nil {
		t.Fatalf("decode done.message: %v", err)
	}
	if spoken.Role != "assistant" || spoken.Content != "発言します" {
		t.Fatalf("stored turn = %+v, want an assistant message carrying the streamed text", spoken)
	}
	if spoken.ParticipantID == nil || *spoken.ParticipantID != alice {
		t.Fatalf("first speaker = %v, want Alice (%s)", spoken.ParticipantID, alice)
	}
	if _, ok := payload["participants"]; !ok {
		t.Fatalf("done payload missing participants, so the client cannot name the speaker")
	}

	// The cycle is derived from the transcript, so the second turn moves on.
	rec = doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{})
	wantStatus(t, rec, http.StatusOK)
	if err := json.Unmarshal(doneFrame(t, rec)["message"], &spoken); err != nil {
		t.Fatalf("decode done.message: %v", err)
	}
	if spoken.ParticipantID == nil || *spoken.ParticipantID != bob {
		t.Fatalf("second speaker = %v, want Bob (%s)", spoken.ParticipantID, bob)
	}
}

// TestMultiAgentTurnConflict covers AC #3: while a turn is mid-completion, a
// second request for the same chat is refused with a real 409 status — not an
// SSE error frame on a 200, which is why the stream is opened only once the
// first delta arrives.
func TestMultiAgentTurnConflict(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	llm := newMultiAgentLLM(t, func() {
		once.Do(func() {
			entered <- struct{}{}
			<-release
		})
	})
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Conflict Project")
	chatID := createMultiAgentChat(t, h, projectID, "round_robin")
	addParticipant(t, h, chatID, "Alice", llm.URL+"/v1")
	addParticipant(t, h, chatID, "Bob", llm.URL+"/v1")

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstDone <- doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{})
	}()

	<-entered // the first turn holds the chat and has stored nothing yet
	overlapping := doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{})
	wantStatus(t, overlapping, http.StatusConflict)
	if ct := overlapping.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("refusal content-type = %q, want JSON", ct)
	}
	close(release)
	wantStatus(t, <-firstDone, http.StatusOK)

	// The refused request must not have spoken, and the lock must be gone with the
	// turn that held it.
	detail := decodeJSONMap(t, doJSON(t, h, "GET", "/api/chats/"+chatID, nil))
	var messages []json.RawMessage
	unmarshalField(t, detail, "messages", &messages)
	if len(messages) != 1 {
		t.Fatalf("%d messages stored, want 1 — the refused turn must not speak", len(messages))
	}
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{}), http.StatusOK)
}

// TestMultiAgentParticipantIDScope covers AC #4: a manual nomination is checked
// against the chat in the route (a foreign participant is a 404), while the flat
// participant routes have no second chat to compare with and 404 only on a
// participant that does not exist.
func TestMultiAgentParticipantIDScope(t *testing.T) {
	llm := newMultiAgentLLM(t, nil)
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Scope Project")

	chatID := createMultiAgentChat(t, h, projectID, "manual")
	mine := addParticipant(t, h, chatID, "Alice", llm.URL+"/v1")
	otherChatID := createMultiAgentChat(t, h, projectID, "manual")
	foreign := addParticipant(t, h, otherChatID, "Carol", llm.URL+"/v1")

	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream",
		map[string]any{"participantId": mine}), http.StatusOK)
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream",
		map[string]any{"participantId": foreign}), http.StatusNotFound)
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream",
		map[string]any{"participantId": "participant_missing"}), http.StatusNotFound)
	// manual leaves the choice to the caller, so an unnamed speaker is a bad
	// request rather than a silent round-robin pick.
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{}), http.StatusBadRequest)

	// The flat routes act on the participant's own chat, so a participant of
	// another chat is an ordinary target here.
	wantStatus(t, doJSON(t, h, "PATCH", "/api/participants/"+foreign, map[string]any{"displayName": "Carol II"}), http.StatusOK)
	wantStatus(t, doJSON(t, h, "DELETE", "/api/participants/"+foreign, nil), http.StatusOK)
	wantError(t, doJSON(t, h, "PATCH", "/api/participants/participant_missing", map[string]any{"displayName": "x"}),
		http.StatusNotFound, "participant not found")
	wantError(t, doJSON(t, h, "DELETE", "/api/participants/participant_missing", nil),
		http.StatusNotFound, "participant not found")
}

// TestMultiAgentParticipantsCRUD walks the four participant routes, including
// what removal leaves behind: the row stays listed with deletedAt set, because
// past messages still have to resolve to a speaker.
func TestMultiAgentParticipantsCRUD(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Roster Project")
	chatID := createMultiAgentChat(t, h, projectID, "")

	wantError(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/participants", map[string]any{"displayName": "  "}),
		http.StatusBadRequest, "displayName is required")
	wantError(t, doJSON(t, h, "GET", "/api/chats/chat_missing/participants", nil),
		http.StatusNotFound, "chat not found")

	// A single-assistant chat has no roster to keep: kind is fixed at creation, so
	// the rows could never come into play.
	assistantChatID := createChat(t, h, projectID)
	wantError(t, doJSON(t, h, "POST", "/api/chats/"+assistantChatID+"/participants", map[string]any{"displayName": "Alice"}),
		http.StatusBadRequest, "chat is not a multi-agent chat")

	alice := addParticipant(t, h, chatID, "Alice", "http://127.0.0.1:1/v1")
	addParticipant(t, h, chatID, "Bob", "http://127.0.0.1:1/v1")

	type participant struct {
		ID          string  `json:"id"`
		DisplayName string  `json:"displayName"`
		RolePrompt  string  `json:"rolePrompt"`
		SortOrder   int     `json:"sortOrder"`
		DeletedAt   *string `json:"deletedAt"`
	}
	list := func() []participant {
		t.Helper()
		rec := doJSON(t, h, "GET", "/api/chats/"+chatID+"/participants", nil)
		wantStatus(t, rec, http.StatusOK)
		var out []participant
		unmarshalField(t, decodeJSONMap(t, rec), "participants", &out)
		return out
	}

	roster := list()
	if len(roster) != 2 || roster[0].DisplayName != "Alice" || roster[1].DisplayName != "Bob" {
		t.Fatalf("roster = %+v, want Alice then Bob in turn order", roster)
	}

	wantError(t, doJSON(t, h, "PATCH", "/api/participants/"+alice, map[string]any{"sortOrder": "second"}),
		http.StatusBadRequest, "sortOrder must be an integer")
	// A fractional order is refused rather than truncated: 1.9 stored as 1 would
	// put the participant somewhere other than where the caller asked.
	wantError(t, doJSON(t, h, "PATCH", "/api/participants/"+alice, map[string]any{"sortOrder": 1.9}),
		http.StatusBadRequest, "sortOrder must be an integer")
	// Creation refuses a blank name, so the update must too — the repository
	// trims what it stores, and an empty name leaves the speaker unlabelled.
	wantError(t, doJSON(t, h, "PATCH", "/api/participants/"+alice, map[string]any{"displayName": "   "}),
		http.StatusBadRequest, "displayName must not be empty")
	// An absent field is left alone, so renaming does not blank the role prompt.
	wantStatus(t, doJSON(t, h, "PATCH", "/api/participants/"+alice, map[string]any{"displayName": "Alice II"}), http.StatusOK)
	roster = list()
	if roster[0].DisplayName != "Alice II" || roster[0].RolePrompt != "Alice の役割" {
		t.Fatalf("after rename = %+v, want the role prompt untouched", roster[0])
	}

	// An accepted integer moves the participant in the cycle, which is what the
	// rejections above are protecting.
	wantStatus(t, doJSON(t, h, "PATCH", "/api/participants/"+alice, map[string]any{"sortOrder": 3}), http.StatusOK)
	roster = list()
	if roster[1].ID != alice || roster[1].SortOrder != 3 {
		t.Fatalf("after reorder = %+v, want Alice last with sortOrder 3", roster)
	}

	wantStatus(t, doJSON(t, h, "DELETE", "/api/participants/"+alice, nil), http.StatusOK)
	roster = list()
	if len(roster) != 2 {
		t.Fatalf("%d participants listed after removal, want 2 — the row survives for name resolution", len(roster))
	}
	removed := roster[1]
	if removed.ID != alice || removed.DeletedAt == nil {
		t.Fatalf("removed participant = %+v, want alice with deletedAt set", removed)
	}
}

// TestMultiAgentChatSettings covers the PATCH extension of design §5: turnRule
// and scenePrompt are accepted for a multi-agent chat only, and each field moves
// on its own.
func TestMultiAgentChatSettings(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Settings Project")
	chatID := createMultiAgentChat(t, h, projectID, "")

	wantError(t, doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats", map[string]any{"kind": "swarm"}),
		http.StatusBadRequest, `kind must be "assistant" or "multi_agent"`)
	wantError(t, doJSON(t, h, "PATCH", "/api/chats/"+chatID, map[string]any{"turnRule": "auction"}),
		http.StatusBadRequest, `turnRule must be "round_robin" or "manual"`)

	rec := doJSON(t, h, "PATCH", "/api/chats/"+chatID, map[string]any{"scenePrompt": "論題: ローカル LLM の是非"})
	wantStatus(t, rec, http.StatusOK)
	var chat struct {
		Title       string `json:"title"`
		TurnRule    string `json:"turnRule"`
		ScenePrompt string `json:"scenePrompt"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "chat", &chat)
	if chat.ScenePrompt == "" || chat.Title != "Debate" || chat.TurnRule != "round_robin" {
		t.Fatalf("after scene update = %+v, want only the scene prompt changed", chat)
	}

	rec = doJSON(t, h, "PATCH", "/api/chats/"+chatID, map[string]any{"title": "Renamed", "turnRule": "manual"})
	wantStatus(t, rec, http.StatusOK)
	unmarshalField(t, decodeJSONMap(t, rec), "chat", &chat)
	if chat.Title != "Renamed" || chat.TurnRule != "manual" || chat.ScenePrompt == "" {
		t.Fatalf("after combined update = %+v, want title and turn rule changed and the scene kept", chat)
	}

	assistantChatID := createChat(t, h, projectID)
	wantError(t, doJSON(t, h, "PATCH", "/api/chats/"+assistantChatID, map[string]any{"turnRule": "manual"}),
		http.StatusBadRequest, "turnRule and scenePrompt apply to multi-agent chats only")
	wantError(t, doJSON(t, h, "PATCH", "/api/chats/chat_missing", map[string]any{"title": "x"}),
		http.StatusNotFound, "chat not found")
}

// TestMultiAgentMessagesAreStoredNotAnswered covers AC #2: in a multi-agent chat
// both message routes record the human's intervention and generate nothing,
// while a single-assistant chat keeps answering as before.
func TestMultiAgentMessagesAreStoredNotAnswered(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Intervention Project")
	chatID := createMultiAgentChat(t, h, projectID, "")

	rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages", map[string]any{"content": "その論点を足して"})
	wantStatus(t, rec, http.StatusCreated)
	body := decodeJSONMap(t, rec)
	var stored struct {
		Role          string  `json:"role"`
		Content       string  `json:"content"`
		ParticipantID *string `json:"participantId"`
	}
	unmarshalField(t, body, "message", &stored)
	if stored.Role != "user" || stored.Content != "その論点を足して" || stored.ParticipantID != nil {
		t.Fatalf("stored message = %+v, want the human's own message with no participant", stored)
	}

	type listed struct {
		Role string `json:"role"`
	}
	var messages []listed
	unmarshalField(t, body, "messages", &messages)
	if len(messages) != 1 || messages[0].Role != "user" {
		t.Fatalf("messages = %+v, want the intervention alone with no generated reply", messages)
	}

	rec = doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages/stream", map[string]any{"content": "もう一つ"})
	wantStatus(t, rec, http.StatusOK)
	events := parseSSE(t, rec.Body.String())
	if _, ok := events["delta"]; ok {
		t.Fatalf("a multi-agent intervention must not stream generated deltas (body=%s)", rec.Body.String())
	}
	if err := json.Unmarshal(doneFrame(t, rec)["messages"], &messages); err != nil {
		t.Fatalf("decode done.messages: %v", err)
	}
	if len(messages) != 2 || messages[1].Role != "user" {
		t.Fatalf("messages = %+v, want two human messages and no assistant turn", messages)
	}

	// The single-assistant path is untouched: the dead endpoint still yields a
	// stored fallback answer.
	assistantChatID := createChat(t, h, projectID)
	rec = doJSON(t, h, "POST", "/api/chats/"+assistantChatID+"/messages", map[string]any{"content": "Hello"})
	wantStatus(t, rec, http.StatusCreated)
	unmarshalField(t, decodeJSONMap(t, rec), "message", &stored)
	if stored.Role != "assistant" {
		t.Fatalf("assistant chat reply role = %q, want assistant", stored.Role)
	}
}

// TestMultiAgentPresets covers the preset half of TASK-5 AC #1: the bundled
// list is served, a presetId creates the chat with the preset's rule, scene and
// roster in preset order, and an inline preset goes through the same parser.
func TestMultiAgentPresets(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Preset Project")

	rec := doJSON(t, h, "GET", "/api/multi-agent-presets", nil)
	wantStatus(t, rec, http.StatusOK)
	var presets []struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		Group        string `json:"group"`
		TurnRule     string `json:"turnRule"`
		ScenePrompt  string `json:"scenePrompt"`
		Participants []struct {
			DisplayName string `json:"displayName"`
		} `json:"participants"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "presets", &presets)
	var debate *struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		Group        string `json:"group"`
		TurnRule     string `json:"turnRule"`
		ScenePrompt  string `json:"scenePrompt"`
		Participants []struct {
			DisplayName string `json:"displayName"`
		} `json:"participants"`
	}
	for i := range presets {
		if presets[i].ID == "debate" {
			debate = &presets[i]
		}
	}
	if debate == nil {
		t.Fatalf("preset list lacks debate: %+v", presets)
	}

	// Bundled preset by id, with an empty title: the preset's title is used.
	rec = doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats",
		map[string]any{"kind": "multi_agent", "presetId": "debate"})
	wantStatus(t, rec, http.StatusCreated)
	var chat struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Kind        string `json:"kind"`
		TurnRule    string `json:"turnRule"`
		ScenePrompt string `json:"scenePrompt"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "chat", &chat)
	if chat.Kind != "multi_agent" || chat.Title != debate.Title || chat.TurnRule != debate.TurnRule || chat.ScenePrompt != debate.ScenePrompt {
		t.Fatalf("chat from preset = %+v, want the preset's title, rule and scene", chat)
	}

	rec = doJSON(t, h, "GET", "/api/chats/"+chat.ID+"/participants", nil)
	wantStatus(t, rec, http.StatusOK)
	var roster []struct {
		DisplayName string  `json:"displayName"`
		RolePrompt  string  `json:"rolePrompt"`
		BaseURL     string  `json:"baseUrl"`
		SortOrder   int     `json:"sortOrder"`
		DeletedAt   *string `json:"deletedAt"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "participants", &roster)
	if len(roster) != len(debate.Participants) {
		t.Fatalf("roster has %d participants, want %d", len(roster), len(debate.Participants))
	}
	for i, p := range roster {
		if p.DisplayName != debate.Participants[i].DisplayName || p.SortOrder != i || p.RolePrompt == "" || p.BaseURL != "" || p.DeletedAt != nil {
			t.Fatalf("roster[%d] = %+v, want %q in preset order with a role prompt and no endpoint", i, p, debate.Participants[i].DisplayName)
		}
	}

	// An explicit title wins over the preset's.
	rec = doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats",
		map[string]any{"kind": "multi_agent", "presetId": "debate", "title": "宿題ディベート"})
	wantStatus(t, rec, http.StatusCreated)
	unmarshalField(t, decodeJSONMap(t, rec), "chat", &chat)
	if chat.Title != "宿題ディベート" {
		t.Fatalf("title = %q, want the explicit title", chat.Title)
	}

	// Inline preset, as the file import sends it.
	inline := map[string]any{
		"title":       "自作の対話",
		"turnRule":    "manual",
		"scenePrompt": "静かな部屋。",
		"participants": []map[string]any{
			{"displayName": "甲", "rolePrompt": "あなたは甲です。"},
			{"displayName": "乙", "rolePrompt": "あなたは乙です。"},
		},
	}
	rec = doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats",
		map[string]any{"kind": "multi_agent", "preset": inline})
	wantStatus(t, rec, http.StatusCreated)
	unmarshalField(t, decodeJSONMap(t, rec), "chat", &chat)
	if chat.Title != "自作の対話" || chat.TurnRule != "manual" || chat.ScenePrompt != "静かな部屋。" {
		t.Fatalf("chat from inline preset = %+v", chat)
	}
	rec = doJSON(t, h, "GET", "/api/chats/"+chat.ID+"/participants", nil)
	wantStatus(t, rec, http.StatusOK)
	unmarshalField(t, decodeJSONMap(t, rec), "participants", &roster)
	if len(roster) != 2 || roster[0].DisplayName != "甲" || roster[1].DisplayName != "乙" {
		t.Fatalf("inline roster = %+v, want 甲 then 乙", roster)
	}

	// Refusals.
	wantError(t, doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats",
		map[string]any{"kind": "multi_agent", "presetId": "no-such-preset"}),
		http.StatusNotFound, "preset not found")
	wantError(t, doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats",
		map[string]any{"presetId": "debate"}),
		http.StatusBadRequest, "presetId and preset apply to multi-agent chats only")
	wantError(t, doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats",
		map[string]any{"kind": "multi_agent", "presetId": "debate", "preset": inline}),
		http.StatusBadRequest, "specify either presetId or preset, not both")
	wantError(t, doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats",
		map[string]any{"kind": "multi_agent", "preset": map[string]any{"title": "x", "participants": []any{}}}),
		http.StatusBadRequest, "preset: invalid preset: participants must have at least two entries")
	wantError(t, doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats",
		map[string]any{"kind": "multi_agent", "preset": "not an object"}),
		http.StatusBadRequest, "preset: invalid preset: json: cannot unmarshal string into Go value of type preset.MultiAgentPreset")

	// A refused preset leaves no chat behind.
	rec = doJSON(t, h, "GET", "/api/projects/"+projectID, nil)
	wantStatus(t, rec, http.StatusOK)
	var chats []json.RawMessage
	unmarshalField(t, decodeJSONMap(t, rec), "chats", &chats)
	if len(chats) != 3 {
		t.Fatalf("project has %d chats, want the 3 created above", len(chats))
	}
}

// presetChatShape is what applying a preset has to produce, whether the preset
// was named at creation or applied afterwards: the two are compared field by
// field, which is TASK-14 AC #2.
type presetChatShape struct {
	Title        string `json:"title"`
	TurnRule     string `json:"turnRule"`
	ScenePrompt  string `json:"scenePrompt"`
	Participants []struct {
		DisplayName string  `json:"displayName"`
		RolePrompt  string  `json:"rolePrompt"`
		BaseURL     string  `json:"baseUrl"`
		ModelName   string  `json:"modelName"`
		SortOrder   int     `json:"sortOrder"`
		DeletedAt   *string `json:"deletedAt"`
	} `json:"participants"`
}

func readPresetChatShape(t *testing.T, h http.Handler, chatID string) presetChatShape {
	t.Helper()
	var shape presetChatShape
	rec := doJSON(t, h, "GET", "/api/chats/"+chatID, nil)
	wantStatus(t, rec, http.StatusOK)
	unmarshalField(t, decodeJSONMap(t, rec), "chat", &shape)
	rec = doJSON(t, h, "GET", "/api/chats/"+chatID+"/participants", nil)
	wantStatus(t, rec, http.StatusOK)
	unmarshalField(t, decodeJSONMap(t, rec), "participants", &shape.Participants)
	return shape
}

func createEmptyMultiAgentChat(t *testing.T, h http.Handler, projectID, title string) string {
	t.Helper()
	rec := doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats",
		map[string]any{"kind": "multi_agent", "title": title})
	wantStatus(t, rec, http.StatusCreated)
	var chat struct {
		ID string `json:"id"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "chat", &chat)
	return chat.ID
}

// TestMultiAgentPresetAppliedAfterCreation covers TASK-14: a multi-agent chat
// that has not been spoken in yet takes a preset from the organisation panel,
// the result is indistinguishable from having named the preset at creation
// (AC #2), and a chat with a message refuses it (AC #3).
func TestMultiAgentPresetAppliedAfterCreation(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Apply Preset Project")

	rec := doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats",
		map[string]any{"kind": "multi_agent", "presetId": "debate"})
	wantStatus(t, rec, http.StatusCreated)
	var created struct {
		ID string `json:"id"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "chat", &created)
	atCreation := readPresetChatShape(t, h, created.ID)

	// The sidebar's "+" makes exactly this chat: multi-agent, untitled, empty
	// roster. A participant typed in by hand is added first, so the assertions
	// below show the roster replaced rather than appended to (AC #4).
	chatID := createEmptyMultiAgentChat(t, h, projectID, "")
	addParticipant(t, h, chatID, "下書きの参加者", "http://127.0.0.1:1")

	rec = doJSON(t, h, "POST", "/api/chats/"+chatID+"/preset", map[string]any{"presetId": "debate"})
	wantStatus(t, rec, http.StatusOK)
	applied := readPresetChatShape(t, h, chatID)
	if applied.Title != atCreation.Title || applied.TurnRule != atCreation.TurnRule || applied.ScenePrompt != atCreation.ScenePrompt {
		t.Fatalf("applied chat = %+v, want the same title, rule and scene as %+v", applied, atCreation)
	}
	if len(applied.Participants) != len(atCreation.Participants) {
		t.Fatalf("applied roster has %d participants, want %d (the hand-typed one must be gone)",
			len(applied.Participants), len(atCreation.Participants))
	}
	for i := range applied.Participants {
		if applied.Participants[i] != atCreation.Participants[i] {
			t.Fatalf("applied roster[%d] = %+v, want %+v", i, applied.Participants[i], atCreation.Participants[i])
		}
	}

	// The response carries the applied state, so the panel does not have to re-read it.
	body := decodeJSONMap(t, rec)
	var responded presetChatShape
	unmarshalField(t, body, "chat", &responded)
	unmarshalField(t, body, "participants", &responded.Participants)
	if responded.TurnRule != applied.TurnRule || len(responded.Participants) != len(applied.Participants) {
		t.Fatalf("apply response = %+v, want the stored chat and roster", responded)
	}

	// A title the user gave is kept; only an empty one takes the preset's.
	namedID := createEmptyMultiAgentChat(t, h, projectID, "宿題ディベート")
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+namedID+"/preset", map[string]any{"presetId": "debate"}), http.StatusOK)
	if named := readPresetChatShape(t, h, namedID); named.Title != "宿題ディベート" {
		t.Fatalf("title after applying = %q, want the title the chat already had", named.Title)
	}

	// An imported preset goes through the same parser as a bundled one (AC #1).
	inlineID := createEmptyMultiAgentChat(t, h, projectID, "")
	inline := map[string]any{
		"title":       "自作の対話",
		"turnRule":    "manual",
		"scenePrompt": "静かな部屋。",
		"participants": []map[string]any{
			{"displayName": "甲", "rolePrompt": "あなたは甲です。"},
			{"displayName": "乙", "rolePrompt": "あなたは乙です。"},
		},
	}
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+inlineID+"/preset", map[string]any{"preset": inline}), http.StatusOK)
	fromFile := readPresetChatShape(t, h, inlineID)
	if fromFile.Title != "自作の対話" || fromFile.TurnRule != "manual" || fromFile.ScenePrompt != "静かな部屋。" {
		t.Fatalf("chat from an imported preset = %+v", fromFile)
	}
	if len(fromFile.Participants) != 2 || fromFile.Participants[0].DisplayName != "甲" || fromFile.Participants[1].SortOrder != 1 {
		t.Fatalf("imported roster = %+v, want 甲 then 乙 in preset order", fromFile.Participants)
	}

	// Refusals.
	wantError(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/preset", map[string]any{}),
		http.StatusBadRequest, "presetId or preset is required")
	wantError(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/preset",
		map[string]any{"presetId": "debate", "preset": inline}),
		http.StatusBadRequest, "specify either presetId or preset, not both")
	wantError(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/preset", map[string]any{"presetId": "no-such-preset"}),
		http.StatusNotFound, "preset not found")
	wantError(t, doJSON(t, h, "POST", "/api/chats/"+createChat(t, h, projectID)+"/preset",
		map[string]any{"presetId": "debate"}),
		http.StatusBadRequest, "chat is not a multi-agent chat")

	// Once the conversation has a message, the preset is refused and the roster
	// it would have replaced is left as it is (AC #3).
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages", map[string]any{"content": "始めましょう"}), http.StatusCreated)
	wantError(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/preset", map[string]any{"presetId": "twenty-questions"}),
		http.StatusConflict, "a preset applies only while the conversation has no messages")
	if after := readPresetChatShape(t, h, chatID); after.ScenePrompt != applied.ScenePrompt || len(after.Participants) != len(applied.Participants) {
		t.Fatalf("refused apply changed the chat: %+v", after)
	}
}
