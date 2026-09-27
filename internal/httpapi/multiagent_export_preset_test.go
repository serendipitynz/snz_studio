package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// exportedLineUp is everything TASK-62 AC #2 says a chat recreated from its
// exported preset must match: the chat's settings and state, and the roster in
// order with each participant's endpoint and model.
type exportedLineUp struct {
	Title         string          `json:"title"`
	TurnRule      string          `json:"turnRule"`
	ScenePrompt   string          `json:"scenePrompt"`
	StateSheet    string          `json:"stateSheet"`
	Commands      json.RawMessage `json:"commands"`
	FacilitatorID string          `json:"facilitatorId"`
	Participants  []struct {
		ID                      string  `json:"id"`
		DisplayName             string  `json:"displayName"`
		RolePrompt              string  `json:"rolePrompt"`
		BaseURL                 string  `json:"baseUrl"`
		ModelName               string  `json:"modelName"`
		ReceivesProjectMaterial bool    `json:"receivesProjectMaterial"`
		StateSheet              string  `json:"stateSheet"`
		DeletedAt               *string `json:"deletedAt"`
	} `json:"participants"`
}

// comparable drops what differs between two chats by construction (ids) and
// keeps the facilitator as the roster position it points at.
func (l exportedLineUp) comparable() map[string]any {
	facilitator := -1
	type entry struct {
		DisplayName, RolePrompt, BaseURL, ModelName, StateSheet string
		ReceivesProjectMaterial                                 bool
	}
	var roster []entry
	for _, p := range l.Participants {
		if p.DeletedAt != nil {
			continue
		}
		if p.ID == l.FacilitatorID {
			facilitator = len(roster)
		}
		roster = append(roster, entry{p.DisplayName, p.RolePrompt, p.BaseURL, p.ModelName, p.StateSheet, p.ReceivesProjectMaterial})
	}
	return map[string]any{
		"title":       l.Title,
		"turnRule":    l.TurnRule,
		"scenePrompt": l.ScenePrompt,
		"stateSheet":  l.StateSheet,
		"commands":    string(l.Commands),
		"facilitator": facilitator,
		"roster":      roster,
	}
}

func readLineUp(t *testing.T, h http.Handler, chatID string) exportedLineUp {
	t.Helper()
	var lineUp exportedLineUp
	rec := doJSON(t, h, "GET", "/api/chats/"+chatID, nil)
	wantStatus(t, rec, http.StatusOK)
	unmarshalField(t, decodeJSONMap(t, rec), "chat", &lineUp)
	rec = doJSON(t, h, "GET", "/api/chats/"+chatID+"/participants", nil)
	wantStatus(t, rec, http.StatusOK)
	unmarshalField(t, decodeJSONMap(t, rec), "participants", &lineUp.Participants)
	return lineUp
}

func exportPreset(t *testing.T, h http.Handler, chatID string) map[string]any {
	t.Helper()
	rec := doJSON(t, h, "GET", "/api/chats/"+chatID+"/export/preset", nil)
	wantStatus(t, rec, http.StatusOK)
	var exported map[string]any
	unmarshalField(t, decodeJSONMap(t, rec), "preset", &exported)
	return exported
}

// TestMultiAgentPresetExportRoundTrip is TASK-62 AC #1 and #2: a chat that has
// been spoken in, with every field the export carries set to something other
// than its default, is exported, and the file applied at creation and to an
// empty existing chat gives back the same line-up.
func TestMultiAgentPresetExportRoundTrip(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Export Preset Project")
	source := createMultiAgentChat(t, h, projectID, "facilitator_alternating")

	gm := addParticipant(t, h, source, "GM", "http://192.168.0.10:1234/v1")
	player := addParticipant(t, h, source, "プレイヤー", "http://192.168.0.11:1234/v1")
	removed := addParticipant(t, h, source, "途中で抜けた人", "http://192.168.0.12:1234/v1")
	// No endpoint: the export has to leave the pair empty, not invent one.
	rec := doJSON(t, h, "POST", "/api/chats/"+source+"/participants", map[string]any{"displayName": "既定で話す人"})
	wantStatus(t, rec, http.StatusCreated)

	wantStatus(t, doJSON(t, h, "PATCH", "/api/participants/"+player, map[string]any{
		"receivesProjectMaterial": false,
		"stateSheet":              "HP: 12\n所持品: ロープ",
	}), http.StatusOK)
	wantStatus(t, doJSON(t, h, "PATCH", "/api/participants/"+gm, map[string]any{"stateSheet": "残りの手札: 3"}), http.StatusOK)
	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+source, map[string]any{
		"scenePrompt":   "嵐の夜の山小屋。",
		"stateSheet":    "場所: 山小屋\n時刻: 深夜",
		"commands":      map[string]any{"roll": map[string]any{"target": 12}},
		"facilitatorId": gm,
	}), http.StatusOK)
	wantStatus(t, doJSON(t, h, "DELETE", "/api/participants/"+removed, nil), http.StatusOK)
	// The export is offered whether or not the conversation has started.
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+source+"/messages", map[string]any{"content": "始めよう"}), http.StatusCreated)

	exported := exportPreset(t, h, source)
	for _, key := range []string{"id", "group", "description", "diceTarget"} {
		if _, ok := exported[key]; ok {
			t.Errorf("exported preset carries %q, which the import does not read: %v", key, exported)
		}
	}
	raw, _ := json.Marshal(exported)
	if strings.Contains(string(raw), "途中で抜けた人") || strings.Contains(string(raw), "始めよう") {
		t.Fatalf("exported preset carries the removed participant or the transcript: %s", raw)
	}

	want := readLineUp(t, h, source).comparable()
	if roster := want["roster"]; reflect.ValueOf(roster).Len() != 3 {
		t.Fatalf("source roster = %+v, want the three participants left on it", roster)
	}

	// Applied at creation, with no title of its own: the preset's title names it.
	rec = doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats",
		map[string]any{"kind": "multi_agent", "preset": exported})
	wantStatus(t, rec, http.StatusCreated)
	var created struct {
		ID string `json:"id"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "chat", &created)
	if got := readLineUp(t, h, created.ID).comparable(); !reflect.DeepEqual(got, want) {
		t.Fatalf("chat created from the export =\n%+v\nwant\n%+v", got, want)
	}

	// Applied to an untitled chat that has not been spoken in, over a roster
	// typed in by hand.
	existing := createEmptyMultiAgentChat(t, h, projectID, "")
	addParticipant(t, h, existing, "下書きの参加者", "http://127.0.0.1:1")
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+existing+"/preset", map[string]any{"preset": exported}), http.StatusOK)
	if got := readLineUp(t, h, existing).comparable(); !reflect.DeepEqual(got, want) {
		t.Fatalf("chat the export was applied to =\n%+v\nwant\n%+v", got, want)
	}

	// Exporting the copy gives the same file: nothing is lost or added per trip.
	if again := exportPreset(t, h, created.ID); !reflect.DeepEqual(again, exported) {
		t.Fatalf("export of the copy =\n%v\nwant\n%v", again, exported)
	}
}

// TestMultiAgentPresetExportRefusals covers the chats whose line-up would not
// load back (409, the owner's call when TASK-62 started) and the route's own
// refusals.
func TestMultiAgentPresetExportRefusals(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Export Refusal Project")

	lone := createMultiAgentChat(t, h, projectID, "")
	addParticipant(t, h, lone, "ひとり", "")
	wantError(t, doJSON(t, h, "GET", "/api/chats/"+lone+"/export/preset", nil),
		http.StatusConflict, "preset: invalid preset: participants must have at least two entries")

	// facilitator_alternating whose facilitator has left the roster: the chat
	// degrades to round_robin (§4.5), but the preset has to name one.
	orphaned := createMultiAgentChat(t, h, projectID, "facilitator_alternating")
	host := addParticipant(t, h, orphaned, "司会", "")
	addParticipant(t, h, orphaned, "甲", "")
	addParticipant(t, h, orphaned, "乙", "")
	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+orphaned, map[string]any{"facilitatorId": host}), http.StatusOK)
	wantStatus(t, doJSON(t, h, "DELETE", "/api/participants/"+host, nil), http.StatusOK)
	wantError(t, doJSON(t, h, "GET", "/api/chats/"+orphaned+"/export/preset", nil),
		http.StatusConflict, "preset: invalid preset: turnRule \"facilitator_alternating\" needs exactly one participant with facilitator: true")

	// A facilitator id left over from a rule that read it is not carried into a
	// rule that refuses the mark.
	stale := createMultiAgentChat(t, h, projectID, "weighted")
	lead := addParticipant(t, h, stale, "聞き手", "")
	addParticipant(t, h, stale, "語り手", "")
	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+stale, map[string]any{"facilitatorId": lead}), http.StatusOK)
	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+stale, map[string]any{"turnRule": "round_robin"}), http.StatusOK)
	raw, _ := json.Marshal(exportPreset(t, h, stale))
	if strings.Contains(string(raw), "facilitator") {
		t.Fatalf("round_robin export carries a facilitator mark: %s", raw)
	}

	rec := doJSON(t, h, "POST", "/api/projects/"+projectID+"/chats", map[string]any{"title": "単独"})
	wantStatus(t, rec, http.StatusCreated)
	var assistant struct {
		ID string `json:"id"`
	}
	unmarshalField(t, decodeJSONMap(t, rec), "chat", &assistant)
	wantError(t, doJSON(t, h, "GET", "/api/chats/"+assistant.ID+"/export/preset", nil),
		http.StatusBadRequest, "chat is not a multi-agent chat")
	wantError(t, doJSON(t, h, "GET", "/api/chats/no-such-chat/export/preset", nil),
		http.StatusNotFound, "chat not found")
}
