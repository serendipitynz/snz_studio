package httpapi

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func pinMessage(t *testing.T, h http.Handler, messageID string, pinned bool) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(t, h, "PATCH", "/api/messages/"+messageID+"/pinned", map[string]any{"pinned": pinned})
}

func listedPins(t *testing.T, h http.Handler, chatID string) map[string]bool {
	t.Helper()
	var messages []struct {
		ID     string `json:"id"`
		Pinned bool   `json:"pinned"`
	}
	unmarshalField(t, decodeJSONMap(t, doJSON(t, h, "GET", "/api/chats/"+chatID, nil)), "messages", &messages)
	pins := map[string]bool{}
	for _, m := range messages {
		pins[m.ID] = m.Pinned
	}
	return pins
}

// TestMultiAgentPinMessage covers TASK-105 AC #2: a pin goes on and comes off,
// and the transcript the view reads carries it.
func TestMultiAgentPinMessage(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Pin Project")
	chatID := createMultiAgentChat(t, h, projectID, "")
	first := postIntervention(t, h, chatID, "前提: 宝は北の塔にある")
	second := postIntervention(t, h, chatID, "雑談")

	if pins := listedPins(t, h, chatID); pins[first] || pins[second] {
		t.Fatalf("pins = %v, want every message unpinned by default", pins)
	}

	rec := pinMessage(t, h, first, true)
	wantStatus(t, rec, http.StatusOK)
	var message struct {
		ID     string `json:"id"`
		Pinned bool   `json:"pinned"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "message", &message)
	if message.ID != first || !message.Pinned {
		t.Fatalf("response message = %+v, want %s pinned", message, first)
	}
	if pins := listedPins(t, h, chatID); !pins[first] || pins[second] {
		t.Fatalf("pins = %v, want only %s pinned", pins, first)
	}

	wantStatus(t, pinMessage(t, h, first, false), http.StatusOK)
	if pins := listedPins(t, h, chatID); pins[first] {
		t.Fatalf("pins = %v, want the pin taken off", pins)
	}

	// A temporary chat writes nothing outside itself by pinning, so it may pin.
	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+chatID+"/temporary", map[string]any{"isTemporary": true}), http.StatusOK)
	wantStatus(t, pinMessage(t, h, second, true), http.StatusOK)
}

func TestMultiAgentPinMessageRefusals(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Pin Refusal Project")
	chatID := createMultiAgentChat(t, h, projectID, "")
	messageID := postIntervention(t, h, chatID, "発言")

	wantError(t, doJSON(t, h, "PATCH", "/api/messages/"+messageID+"/pinned", map[string]any{"pinned": "yes"}),
		http.StatusBadRequest, "pinned must be a boolean")
	wantError(t, doJSON(t, h, "PATCH", "/api/messages/"+messageID+"/pinned", map[string]any{}),
		http.StatusBadRequest, "pinned must be a boolean")
	wantError(t, pinMessage(t, h, "msg_missing", true), http.StatusNotFound, "message not found")

	assistantChatID := createChat(t, h, projectID)
	rec := doJSON(t, h, "POST", "/api/chats/"+assistantChatID+"/messages", map[string]any{"content": "Hello"})
	wantStatus(t, rec, http.StatusCreated)
	var reply struct {
		ID string `json:"id"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "message", &reply)
	wantError(t, pinMessage(t, h, reply.ID, true), http.StatusBadRequest, "chat is not a multi-agent chat")
}

// TestMultiAgentPinDuringTurn covers the second half of TASK-105 AC #2: the pin
// does not wait for the chat's turn slot, so it lands while a turn is in flight
// (design §4.9.4), and the turn's own store leaves it in place.
func TestMultiAgentPinDuringTurn(t *testing.T) {
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
	projectID := createProject(t, h, "Pin Turn Project")
	chatID := createMultiAgentChat(t, h, projectID, "round_robin")
	addParticipant(t, h, chatID, "Alice", llm.URL+"/v1")
	addParticipant(t, h, chatID, "Bob", llm.URL+"/v1")
	messageID := postIntervention(t, h, chatID, "前提: 宝は北の塔にある")

	turnDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		turnDone <- doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{})
	}()

	<-entered // the turn holds the chat's slot and is mid-completion
	wantStatus(t, pinMessage(t, h, messageID, true), http.StatusOK)
	close(release)
	wantStatus(t, <-turnDone, http.StatusOK)

	if pins := listedPins(t, h, chatID); len(pins) != 2 || !pins[messageID] {
		t.Fatalf("pins = %v, want the pin made during the turn kept beside the turn's utterance", pins)
	}
}
