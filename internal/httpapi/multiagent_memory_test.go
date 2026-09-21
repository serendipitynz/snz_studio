package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type listedMemory struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Title        string  `json:"title"`
	Content      string  `json:"content"`
	Source       string  `json:"source"`
	SourceChatID *string `json:"sourceChatId"`
	Locked       bool    `json:"locked"`
}

func listProjectMemories(t *testing.T, h http.Handler, projectID string) []listedMemory {
	t.Helper()
	rec := doJSON(t, h, "GET", "/api/projects/"+projectID, nil)
	wantStatus(t, rec, http.StatusOK)
	var memories []listedMemory
	unmarshalField(t, decodeJSONMap(t, rec), "memories", &memories)
	return memories
}

func postIntervention(t *testing.T, h http.Handler, chatID, content string) string {
	t.Helper()
	rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages", map[string]any{"content": content})
	wantStatus(t, rec, http.StatusCreated)
	var message struct {
		ID string `json:"id"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "message", &message)
	return message.ID
}

// TestMultiAgentSaveMessageMemory covers TASK-19 AC #1 and #2: the draft
// carries the utterance and the kind the extraction rules infer, and the save
// stores the edited draft as a project memory whose source names the chat kind
// and whose sourceChatId points back at the conversation.
func TestMultiAgentSaveMessageMemory(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Memory Project")
	chatID := createMultiAgentChat(t, h, projectID, "")
	messageID := postIntervention(t, h, chatID, "このプロジェクトは Go で構成します。")

	rec := doJSON(t, h, "GET", "/api/messages/"+messageID+"/memory-draft", nil)
	wantStatus(t, rec, http.StatusOK)
	var draft struct {
		Content string `json:"content"`
		Kind    string `json:"kind"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "draft", &draft)
	if draft.Content != "このプロジェクトは Go で構成します。" || draft.Kind != "semantic" {
		t.Fatalf("draft = %+v, want the utterance with the inferred semantic kind", draft)
	}

	wantError(t, doJSON(t, h, "POST", "/api/messages/"+messageID+"/memory", map[string]any{"content": "  "}),
		http.StatusBadRequest, "content is required")
	wantError(t, doJSON(t, h, "POST", "/api/messages/"+messageID+"/memory", map[string]any{"content": "x", "kind": "bogus"}),
		http.StatusBadRequest, "invalid memory kind")
	wantError(t, doJSON(t, h, "GET", "/api/messages/msg_missing/memory-draft", nil),
		http.StatusNotFound, "message not found")

	// The edited wording and the chosen kind win over the draft.
	rec = doJSON(t, h, "POST", "/api/messages/"+messageID+"/memory", map[string]any{
		"content": "常に Go で実装する。",
		"kind":    "procedural",
		"locked":  false,
	})
	wantStatus(t, rec, http.StatusCreated)
	var saved listedMemory
	unmarshalField(t, decodeJSONMap(t, rec), "memory", &saved)
	if saved.Kind != "procedural" || saved.Content != "常に Go で実装する。" || saved.Title != "常に Go で実装する" {
		t.Fatalf("saved memory = %+v, want the edited content under the chosen kind", saved)
	}
	if saved.Source != "multi_agent" || saved.SourceChatID == nil || *saved.SourceChatID != chatID || saved.Locked {
		t.Fatalf("saved memory = %+v, want source multi_agent, sourceChatId %s, locked false", saved, chatID)
	}

	// kind omitted -> inferred; locked omitted -> true, as for a manual memory.
	rec = doJSON(t, h, "POST", "/api/messages/"+messageID+"/memory", map[string]any{"content": "前回のリリースで対応した項目"})
	wantStatus(t, rec, http.StatusCreated)
	unmarshalField(t, decodeJSONMap(t, rec), "memory", &saved)
	if saved.Kind != "episodic" || !saved.Locked {
		t.Fatalf("saved memory = %+v, want the inferred episodic kind and locked by default", saved)
	}

	memories := listProjectMemories(t, h, projectID)
	if len(memories) != 2 {
		t.Fatalf("project memories = %+v, want the two saved ones", memories)
	}
	for _, memory := range memories {
		if memory.Source != "multi_agent" {
			t.Fatalf("listed memory %+v, want source multi_agent", memory)
		}
	}
}

// TestMultiAgentSaveMessageMemoryRefusals covers AC #4 on the server side and
// the chat-kind guard: a temporary multi-agent chat refuses both the draft and
// the save with 409, and a single-assistant message is refused with 400 since
// that chat has its own memory paths.
func TestMultiAgentSaveMessageMemoryRefusals(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Refusal Project")
	chatID := createMultiAgentChat(t, h, projectID, "")
	messageID := postIntervention(t, h, chatID, "覚えておいてほしい事実")

	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+chatID+"/temporary", map[string]any{"isTemporary": true}), http.StatusOK)
	wantError(t, doJSON(t, h, "GET", "/api/messages/"+messageID+"/memory-draft", nil),
		http.StatusConflict, "a temporary chat does not write project memories")
	wantError(t, doJSON(t, h, "POST", "/api/messages/"+messageID+"/memory", map[string]any{"content": "x"}),
		http.StatusConflict, "a temporary chat does not write project memories")

	// Clearing the flag makes the same request acceptable again.
	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+chatID+"/temporary", map[string]any{"isTemporary": false}), http.StatusOK)
	wantStatus(t, doJSON(t, h, "POST", "/api/messages/"+messageID+"/memory", map[string]any{"content": "x"}), http.StatusCreated)

	assistantChatID := createChat(t, h, projectID)
	rec := doJSON(t, h, "POST", "/api/chats/"+assistantChatID+"/messages", map[string]any{"content": "Hello"})
	wantStatus(t, rec, http.StatusCreated)
	var reply struct {
		ID string `json:"id"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "message", &reply)
	wantError(t, doJSON(t, h, "POST", "/api/messages/"+reply.ID+"/memory", map[string]any{"content": "x"}),
		http.StatusBadRequest, "chat is not a multi-agent chat")
}

// TestMultiAgentNeverExtractsMemories covers AC #3: neither a human message
// that carries a durable cue and the "覚えて" trigger nor a participant's
// utterance full of cues produces a memory on its own. The stub endpoint
// answers with sentences the rule-based extractor would store from a
// single-assistant user message.
func TestMultiAgentNeverExtractsMemories(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/"):
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/models"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[]}`)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			payload, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]string{
					"content": "このプロジェクトは Rust で構成します。常に敬語で話してください。",
				}}},
			})
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: "+string(payload)+"\n\ndata: [DONE]\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(llm.Close)

	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "No Extraction Project")
	chatID := createMultiAgentChat(t, h, projectID, "round_robin")
	addParticipant(t, h, chatID, "Alice", llm.URL+"/v1")
	addParticipant(t, h, chatID, "Bob", llm.URL+"/v1")

	postIntervention(t, h, chatID, "このプロジェクトは Go で構成します。今後は日本語で答えて。覚えて。")
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages/stream", map[string]any{"content": "常に簡潔に。メモリに保存して。"}), http.StatusOK)
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{}), http.StatusOK)

	if memories := listProjectMemories(t, h, projectID); len(memories) != 0 {
		t.Fatalf("project memories = %+v, want none: a multi-agent chat must not extract memories", memories)
	}

	// The same cue-laden sentence does produce a memory in a single-assistant
	// chat, so the absence above is the multi-agent path's doing.
	assistantChatID := createChat(t, h, projectID)
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+assistantChatID+"/messages", map[string]any{"content": "このプロジェクトは Go で構成します。"}), http.StatusCreated)
	if memories := listProjectMemories(t, h, projectID); len(memories) == 0 {
		t.Fatal("single-assistant extraction should have stored a memory from the same sentence")
	}
}
