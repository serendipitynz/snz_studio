package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"snzstudio/internal/service"
)

// conclusionRequest is what the default-model stub saw of a conclusion call.
type conclusionRequest struct {
	Model    string
	System   string
	UserText string
}

// newConclusionLLM stands in for the workspace's default model: it answers a
// non-streaming completion with reply and records each request.
func newConclusionLLM(t *testing.T, reply string) (*httptest.Server, func() []conclusionRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []conclusionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		req := conclusionRequest{Model: body.Model}
		for _, m := range body.Messages {
			switch m.Role {
			case "system":
				req.System = m.Content
			case "user":
				req.UserText = m.Content
			}
		}
		mu.Lock()
		seen = append(seen, req)
		mu.Unlock()
		payload, _ := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": reply}}},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []conclusionRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]conclusionRequest(nil), seen...)
	}
}

func pointDefaultLLMAt(t *testing.T, s *Server, baseURL string) {
	t.Helper()
	editable := s.cfg.GetEditable()
	editable.LLMBaseURL = baseURL
	editable.LLMModel = "default-model"
	if _, err := s.cfg.UpdateEditable(editable); err != nil {
		t.Fatalf("UpdateEditable: %v", err)
	}
}

type conclusionDraftResponse struct {
	Draft struct {
		Content string `json:"content"`
		Kind    string `json:"kind"`
	} `json:"draft"`
	AnchorMessageID string `json:"anchorMessageId"`
	MessageCount    int    `json:"messageCount"`
}

func postConclusionDraft(t *testing.T, h http.Handler, chatID string, body map[string]any, want int) conclusionDraftResponse {
	t.Helper()
	rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/conclusion-draft", body)
	wantStatus(t, rec, want)
	var out conclusionDraftResponse
	if want == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode conclusion draft: %v", err)
		}
	}
	return out
}

// TestMultiAgentConclusionDraft covers TASK-27 AC #1, #3 and #4: a draft is
// generated over the whole conversation or from a chosen utterance, by the
// workspace's default model rather than a participant's, with a prompt that
// separates decided from open and keeps role-driven objections out of the
// decisions; nothing is written to chat_summaries or to the memories.
func TestMultiAgentConclusionDraft(t *testing.T) {
	const reply = "決定:\n- Go で書く — 単一バイナリで配れるため\n未決:\n- なし"
	participantLLM := newMultiAgentLLM(t, nil)
	defaultLLM, seen := newConclusionLLM(t, reply)
	s := newTestServer(t)
	pointDefaultLLMAt(t, s, defaultLLM.URL+"/v1")
	h := s.Handler()

	projectID := createProject(t, h, "Conclusion Project")
	chatID := createMultiAgentChat(t, h, projectID, "round_robin")
	addParticipant(t, h, chatID, "Alice", participantLLM.URL+"/v1")
	addParticipant(t, h, chatID, "Bob", participantLLM.URL+"/v1")
	postIntervention(t, h, chatID, "言語を決めたい。候補は Go と Rust。")
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{}), http.StatusOK)
	lastID := postIntervention(t, h, chatID, "Go を採用します。")

	whole := postConclusionDraft(t, h, chatID, map[string]any{}, http.StatusOK)
	if whole.Draft.Content != reply || whole.Draft.Kind != "semantic" {
		t.Fatalf("draft = %+v, want the model's reply under the semantic kind", whole.Draft)
	}
	if whole.AnchorMessageID != lastID || whole.MessageCount != 3 {
		t.Fatalf("anchor = %q count = %d, want the latest message %q over 3 messages", whole.AnchorMessageID, whole.MessageCount, lastID)
	}

	requests := seen()
	if len(requests) != 1 {
		t.Fatalf("default model saw %d requests, want 1", len(requests))
	}
	req := requests[0]
	if req.Model != "default-model" {
		t.Fatalf("conclusion ran on %q, want the workspace default model", req.Model)
	}
	for _, want := range []string{"決定:", "未決:", "役割", "決定に入れない"} {
		if !strings.Contains(req.System, want) {
			t.Fatalf("system prompt lacks %q:\n%s", want, req.System)
		}
	}
	for _, want := range []string{"ユーザー: 言語を決めたい", "Alice: 発言します", "Alice: Alice の役割", "ユーザー: Go を採用します。"} {
		if !strings.Contains(req.UserText, want) {
			t.Fatalf("user input lacks %q:\n%s", want, req.UserText)
		}
	}
	// Bob never spoke in the range, so his role would only be noise.
	if strings.Contains(req.UserText, "Bob") {
		t.Fatalf("user input names a participant who did not speak:\n%s", req.UserText)
	}

	fromLast := postConclusionDraft(t, h, chatID, map[string]any{"fromMessageId": lastID}, http.StatusOK)
	if fromLast.MessageCount != 1 || fromLast.AnchorMessageID != lastID {
		t.Fatalf("from the last message: %+v, want only that message", fromLast)
	}
	if text := seen()[1].UserText; strings.Contains(text, "言語を決めたい") {
		t.Fatalf("a range starting later still carried the first message:\n%s", text)
	}

	rec := doJSON(t, h, "GET", "/api/chats/"+chatID, nil)
	wantStatus(t, rec, http.StatusOK)
	var summary struct {
		Summary string `json:"summary"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "summary", &summary)
	if summary.Summary != "" {
		t.Fatalf("chat summary = %q, want it empty: a conclusion draft must not touch chat_summaries", summary.Summary)
	}
	if memories := listProjectMemories(t, h, projectID); len(memories) != 0 {
		t.Fatalf("project memories = %+v, want none until the human saves", memories)
	}

	// The dialog saves the edited draft through the range's last utterance.
	rec = doJSON(t, h, "POST", "/api/messages/"+whole.AnchorMessageID+"/memory", map[string]any{
		"content": "Go で書く。",
		"kind":    whole.Draft.Kind,
	})
	wantStatus(t, rec, http.StatusCreated)
	var saved listedMemory
	unmarshalField(t, decodeJSONMap(t, rec), "memory", &saved)
	if saved.Kind != "semantic" || saved.Source != "multi_agent" || saved.SourceChatID == nil || *saved.SourceChatID != chatID {
		t.Fatalf("saved conclusion = %+v, want a semantic multi_agent memory pointing at the chat", saved)
	}
}

// TestMultiAgentConclusionDraftLimits covers AC #5 and #7 on the server side,
// plus the refusals: a temporary chat still gets its draft but the save through
// the anchor is refused; a conversation over the limit is refused with the
// counted length instead of being clipped, and a later starting utterance
// brings it under.
func TestMultiAgentConclusionDraftLimits(t *testing.T) {
	defaultLLM, seen := newConclusionLLM(t, "決定:\n- なし\n未決:\n- なし")
	s := newTestServer(t)
	pointDefaultLLMAt(t, s, defaultLLM.URL+"/v1")
	h := s.Handler()
	projectID := createProject(t, h, "Limit Project")
	chatID := createMultiAgentChat(t, h, projectID, "")

	wantError(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/conclusion-draft", nil),
		http.StatusConflict, "the conversation has no utterances to summarize")

	postIntervention(t, h, chatID, strings.Repeat("あ", service.ConclusionCharLimit))
	lateID := postIntervention(t, h, chatID, "ここから先を要約してほしい。")

	rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/conclusion-draft", map[string]any{})
	wantStatus(t, rec, http.StatusUnprocessableEntity)
	var over struct {
		Chars int `json:"chars"`
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &over); err != nil {
		t.Fatalf("decode 422 body: %v", err)
	}
	if over.Limit != service.ConclusionCharLimit || over.Chars <= over.Limit {
		t.Fatalf("422 body = %+v, want the counted length over the limit", over)
	}
	if len(seen()) != 0 {
		t.Fatal("an over-limit conversation reached the model")
	}

	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+chatID+"/temporary", map[string]any{"isTemporary": true}), http.StatusOK)
	draft := postConclusionDraft(t, h, chatID, map[string]any{"fromMessageId": lateID}, http.StatusOK)
	wantError(t, doJSON(t, h, "POST", "/api/messages/"+draft.AnchorMessageID+"/memory", map[string]any{"content": draft.Draft.Content}),
		http.StatusConflict, "a temporary chat does not write project memories")

	wantError(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/conclusion-draft", map[string]any{"fromMessageId": "msg_missing"}),
		http.StatusNotFound, "message not found in this chat")
	assistantChatID := createChat(t, h, projectID)
	wantError(t, doJSON(t, h, "POST", "/api/chats/"+assistantChatID+"/conclusion-draft", nil),
		http.StatusBadRequest, "chat is not a multi-agent chat")
}
