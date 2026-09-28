package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"snzstudio/internal/model"
	"snzstudio/internal/service/commands"
)

type listedMemory struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"`
	Title         string  `json:"title"`
	Content       string  `json:"content"`
	Source        string  `json:"source"`
	SourceChatID  *string `json:"sourceChatId"`
	Locked        bool    `json:"locked"`
	SharedWithAll bool    `json:"sharedWithAll"`
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
	// Every participant heard the utterance this memory was made from, so it
	// starts in the common project material (design §4.4, TASK-31).
	if !saved.SharedWithAll {
		t.Fatalf("saved memory = %+v, want it shared with every participant", saved)
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

// TestMultiAgentMessageMemoryDraftRolls covers TASK-64 AC #1 and #2: the draft
// carries the message's rolls after its body, one 🎲 line each as the markdown
// export writes them, so a roll-only message no longer opens an empty draft.
// The kind stays inferred from the body alone — both actions below carry a
// procedural cue that must not decide it — and a recorded roll stays in the
// draft after /roll is disabled, as it stays on the chip.
func TestMultiAgentMessageMemoryDraftRolls(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Memory Roll Project")
	chatID := createMultiAgentChat(t, h, projectID, "")
	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+chatID, map[string]any{"commands": map[string]any{"roll": map[string]any{"target": 12}}}), http.StatusOK)

	postRoll := func(content string) (string, model.DiceRoll) {
		t.Helper()
		rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages", map[string]any{"content": content})
		wantStatus(t, rec, http.StatusCreated)
		var message struct {
			ID        string           `json:"id"`
			DiceRolls []model.DiceRoll `json:"diceRolls"`
		}
		unmarshalField(t, decodeJSONMap(t, rec), "message", &message)
		if len(message.DiceRolls) != 1 {
			t.Fatalf("%q stored rolls %+v, want one", content, message.DiceRolls)
		}
		return message.ID, message.DiceRolls[0]
	}
	getDraft := func(messageID string) (content, kind string) {
		t.Helper()
		rec := doJSON(t, h, "GET", "/api/messages/"+messageID+"/memory-draft", nil)
		wantStatus(t, rec, http.StatusOK)
		var draft struct {
			Content string `json:"content"`
			Kind    string `json:"kind"`
		}
		unmarshalField(t, decodeJSONMap(t, rec), "draft", &draft)
		return draft.Content, draft.Kind
	}

	rollOnlyID, roll := postRoll("/roll 2d6 目標7 毎回の成功判定")
	wantRollOnly := "🎲 " + commands.DiceRollLine(roll)
	if !strings.Contains(wantRollOnly, "毎回の成功判定 — 2d6 → ") || !strings.Contains(wantRollOnly, "（目標 7、") {
		t.Fatalf("roll line = %q, want the action, the expression and the target", wantRollOnly)
	}
	if content, kind := getDraft(rollOnlyID); content != wantRollOnly || kind != "episodic" {
		t.Fatalf("roll-only draft = %q (%s), want %q under the default episodic kind", content, kind, wantRollOnly)
	}

	withBodyID, roll := postRoll("このプロジェクトは Go で構成します。\n/roll 1d20+3 必ず跳ぶ")
	wantWithBody := "このプロジェクトは Go で構成します。\n\n🎲 " + commands.DiceRollLine(roll)
	if content, kind := getDraft(withBodyID); content != wantWithBody || kind != "semantic" {
		t.Fatalf("draft = %q (%s), want %q under the body's semantic kind", content, kind, wantWithBody)
	}

	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+chatID, map[string]any{"commands": map[string]any{}}), http.StatusOK)
	if content, _ := getDraft(rollOnlyID); content != wantRollOnly {
		t.Fatalf("draft after disabling /roll = %q, want the recorded roll kept", content)
	}
}

// TestMultiAgentInterventionEffects covers TASK-36 on the intervention routes:
// the effect reaches the sheet and the response carries the chat and the roster
// as they are after it, the memory draft carries the effect as a 📝 line, and
// an effect command that cannot be read is 400 on both routes.
func TestMultiAgentInterventionEffects(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Effect Project")
	chatID := createMultiAgentChat(t, h, projectID, "")
	rec := doJSON(t, h, "POST", "/api/chats/"+chatID+"/participants", map[string]any{"displayName": "レン (斥候)", "stateSheet": "HP: 7/10"})
	wantStatus(t, rec, http.StatusCreated)
	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+chatID, map[string]any{
		"commands":   map[string]any{"add": map[string]any{}, "use": map[string]any{}, "set": map[string]any{}},
		"stateSheet": "場所: 入口",
	}), http.StatusOK)

	rec = doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages", map[string]any{"content": "奥へ進む。\n/set 共通 場所 第二坑道"})
	wantStatus(t, rec, http.StatusCreated)
	var response struct {
		Message struct {
			ID           string              `json:"id"`
			StateEffects []model.StateEffect `json:"stateEffects"`
		} `json:"message"`
		Chat         model.Chat          `json:"chat"`
		Participants []model.Participant `json:"participants"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Chat.StateSheet != "場所: 第二坑道" || len(response.Message.StateEffects) != 1 || !response.Message.StateEffects[0].Applied {
		t.Fatalf("response = %+v, want the place set and the chat read after it", response)
	}

	rec = doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages", map[string]any{"content": "/add レン HP -3"})
	wantStatus(t, rec, http.StatusCreated)
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Participants) != 1 || response.Participants[0].StateSheet != "HP: 4/10" {
		t.Fatalf("participants = %+v, want レン's sheet as the effect left it", response.Participants)
	}
	draft := doJSON(t, h, "GET", "/api/messages/"+response.Message.ID+"/memory-draft", nil)
	wantStatus(t, draft, http.StatusOK)
	if body := draft.Body.String(); !strings.Contains(body, "📝 レン (斥候) HP -3: 7/10 → 4/10") {
		t.Fatalf("draft = %s, want the effect as a 📝 line", body)
	}

	for _, route := range []string{"/messages", "/messages/stream"} {
		rec := doJSON(t, h, "POST", "/api/chats/"+chatID+route, map[string]any{"content": "/add ガルド HP -3"})
		wantStatus(t, rec, http.StatusBadRequest)
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
