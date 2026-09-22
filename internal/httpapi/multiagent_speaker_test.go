package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseEventOrder lists the event names of a stream in the order they were sent;
// parseSSE keeps only the last frame of each name, which cannot say whether the
// speaker came before the first delta.
func sseEventOrder(raw string) []string {
	var names []string
	for _, frame := range strings.Split(raw, "\n\n") {
		for _, line := range strings.Split(strings.TrimSpace(frame), "\n") {
			if strings.HasPrefix(line, "event:") {
				names = append(names, strings.TrimSpace(strings.TrimPrefix(line, "event:")))
			}
		}
	}
	return names
}

// newFailingCompletionLLM answers the pre-turn check like newMultiAgentLLM but
// breaks the completion: before any delta when streamOneDelta is false, after one
// delta otherwise.
func newFailingCompletionLLM(t *testing.T, streamOneDelta bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/"):
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/models"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[]}`)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			if !streamOneDelta {
				http.Error(w, "model crashed", http.StatusInternalServerError)
				return
			}
			payload, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]string{"content": "途中まで"}}},
			})
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: "+string(payload)+"\n\ndata: {not json\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestMultiAgentTurnAnnouncesSpeaker is TASK-33 AC #1: the stream names the
// speaker the engine picked, once, before the first delta, with the model the
// completion runs on and an empty weight breakdown for a positional rule.
func TestMultiAgentTurnAnnouncesSpeaker(t *testing.T) {
	llm := newMultiAgentLLM(t, nil)
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Speaker Project")
	chatID := createMultiAgentChat(t, h, projectID, "round_robin")
	addParticipant(t, h, chatID, "Alice", llm.URL+"/v1")
	bob := addParticipant(t, h, chatID, "Bob", llm.URL+"/v1")
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{}), http.StatusOK)

	rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{})
	wantStatus(t, rec, http.StatusOK)
	order := sseEventOrder(rec.Body.String())
	if len(order) < 3 || order[0] != "speaker" || order[1] != "delta" || order[len(order)-1] != "done" {
		t.Fatalf("event order = %v, want speaker, delta..., done", order)
	}
	if n := strings.Count(strings.Join(order, ","), "speaker"); n != 1 {
		t.Fatalf("%d speaker events, want 1", n)
	}

	var speaker struct {
		Participant struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
		} `json:"participant"`
		ModelName string            `json:"modelName"`
		Weights   []json.RawMessage `json:"weights"`
	}
	raw := parseSSE(t, rec.Body.String())["speaker"]
	if err := json.Unmarshal([]byte(raw), &speaker); err != nil {
		t.Fatalf("decode speaker: %v (%s)", err, raw)
	}
	if speaker.Participant.ID != bob || speaker.Participant.DisplayName != "Bob" || speaker.ModelName != "model-bob" {
		t.Fatalf("speaker = %+v, want Bob on model-bob", speaker)
	}
	if speaker.Weights == nil || len(speaker.Weights) != 0 {
		t.Fatalf("weights = %s, want an empty array for round_robin", raw)
	}

	var spoken struct {
		ParticipantID *string `json:"participantId"`
	}
	if err := json.Unmarshal(doneFrame(t, rec)["message"], &spoken); err != nil {
		t.Fatalf("decode done.message: %v", err)
	}
	if spoken.ParticipantID == nil || *spoken.ParticipantID != speaker.Participant.ID {
		t.Fatalf("stored speaker = %v, announced %s", spoken.ParticipantID, speaker.Participant.ID)
	}
}

// TestMultiAgentTurnRefusalsKeepTheirStatus is TASK-33 AC #2: announcing the
// speaker opens the stream, so every refusal decided before it must still come
// back as a JSON status rather than an error frame on a 200. The overlapping
// turn's 409 is TestMultiAgentTurnConflict.
func TestMultiAgentTurnRefusalsKeepTheirStatus(t *testing.T) {
	llm := newMultiAgentLLM(t, nil)
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL + "/v1"
	dead.Close()

	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Refusal Project")
	derived := createMultiAgentChat(t, h, projectID, "round_robin")
	alice := addParticipant(t, h, derived, "Alice", llm.URL+"/v1")
	addParticipant(t, h, derived, "Bob", llm.URL+"/v1")
	manual := createMultiAgentChat(t, h, projectID, "manual")
	carol := addParticipant(t, h, manual, "Carol", llm.URL+"/v1")
	removed := addParticipant(t, h, manual, "Dave", llm.URL+"/v1")
	wantStatus(t, doJSON(t, h, "DELETE", "/api/participants/"+removed, nil), http.StatusOK)
	unreachable := createMultiAgentChat(t, h, projectID, "round_robin")
	addParticipant(t, h, unreachable, "Erin", deadURL)
	addParticipant(t, h, unreachable, "Frank", deadURL)
	empty := createMultiAgentChat(t, h, projectID, "round_robin")

	cases := []struct {
		name   string
		chatID string
		body   map[string]any
		status int
	}{
		{"unknown chat", "chat_missing", map[string]any{}, http.StatusNotFound},
		{"foreign participant", manual, map[string]any{"participantId": alice}, http.StatusNotFound},
		{"empty roster", empty, map[string]any{}, http.StatusBadRequest},
		{"manual without a nominee", manual, map[string]any{}, http.StatusBadRequest},
		{"nominee under a derived rule", derived, map[string]any{"participantId": alice}, http.StatusBadRequest},
		{"removed nominee", manual, map[string]any{"participantId": removed}, http.StatusBadRequest},
		{"endpoint unreachable", unreachable, map[string]any{}, http.StatusBadGateway},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := doJSON(t, h, "POST", "/api/chats/"+c.chatID+"/turns/stream", c.body)
			wantStatus(t, rec, c.status)
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("content-type = %q, want JSON (body=%s)", ct, rec.Body.String())
			}
		})
	}

	// The refusals left the chats untouched and able to run a turn.
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+manual+"/turns/stream",
		map[string]any{"participantId": carol}), http.StatusOK)
}

// TestMultiAgentGenerationFailureIsAStreamError is TASK-33 AC #3: a completion
// that fails after the speaker was announced is an error frame on a 200 whether
// or not a delta had arrived. Before TASK-33 the no-delta case was an HTTP 500;
// the change is intended (design §4.6.6), and this pins it.
func TestMultiAgentGenerationFailureIsAStreamError(t *testing.T) {
	for _, c := range []struct {
		name      string
		withDelta bool
		wantOrder []string
	}{
		{"before any delta", false, []string{"speaker", "error"}},
		{"after a delta", true, []string{"speaker", "delta", "error"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			llm := newFailingCompletionLLM(t, c.withDelta)
			h := newTestServer(t).Handler()
			projectID := createProject(t, h, "Failure Project")
			chatID := createMultiAgentChat(t, h, projectID, "round_robin")
			addParticipant(t, h, chatID, "Alice", llm.URL+"/v1")
			addParticipant(t, h, chatID, "Bob", llm.URL+"/v1")

			rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{})
			wantStatus(t, rec, http.StatusOK)
			if order := sseEventOrder(rec.Body.String()); strings.Join(order, ",") != strings.Join(c.wantOrder, ",") {
				t.Fatalf("event order = %v, want %v", order, c.wantOrder)
			}

			// A failed turn must not enter the transcript (design §4.2 step 2).
			var messages []json.RawMessage
			unmarshalField(t, decodeJSONMap(t, doJSON(t, h, "GET", "/api/chats/"+chatID, nil)), "messages", &messages)
			if len(messages) != 0 {
				t.Fatalf("%d messages stored, want 0 after a failed turn", len(messages))
			}
		})
	}
}
