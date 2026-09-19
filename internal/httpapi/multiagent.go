// multiagent.go serves the multi-agent chat routes of
// docs/multi-agent-chat-design.md §5: the participants CRUD and the one-turn SSE
// endpoint. Participant reads and writes go straight to the repository, as the
// memory and document routes do; only a turn needs a service, because a turn is
// where the turn rule, the endpoint check and the exclusion live.
package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"snzstudio/internal/model"
	"snzstudio/internal/preset"
	"snzstudio/internal/repository"
	"snzstudio/internal/service"
)

// handleListParticipants returns every participant row of the chat, removed ones
// included, each carrying deletedAt. One shape serves both readers: the
// organisation panel filters to the roster, while the spectator view needs the
// removed rows to name the speaker of an older message (design §3).
func (s *Server) handleListParticipants(w http.ResponseWriter, r *http.Request) {
	chat, ok := s.requireMultiAgentChat(w, r.PathValue("chatId"))
	if !ok {
		return
	}
	participants, err := s.participants.ListAll(chat.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"participants": participants})
}

// handleCreateParticipant appends a participant to the chat's roster. Only the
// display name is required: an empty baseUrl or modelName falls back to the
// workspace's configured endpoint when the turn runs, which is what lets a
// participant be created before its endpoint has been picked.
func (s *Server) handleCreateParticipant(w http.ResponseWriter, r *http.Request) {
	chat, ok := s.requireMultiAgentChat(w, r.PathValue("chatId"))
	if !ok {
		return
	}
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	displayName := strings.TrimSpace(bodyString(m, "displayName"))
	if displayName == "" {
		writeError(w, http.StatusBadRequest, "displayName is required")
		return
	}

	participant, err := s.participants.CreateParticipant(repository.CreateParticipantInput{
		ChatID:      chat.ID,
		DisplayName: displayName,
		RolePrompt:  bodyString(m, "rolePrompt"),
		BaseURL:     bodyString(m, "baseUrl"),
		ModelName:   bodyString(m, "modelName"),
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"participant": participant})
}

// handleUpdateParticipant edits the fields the body carries. The route takes a
// flat participant id, so the participant's own chat_id is the only chat in
// play and there is nothing to cross-check: a missing row is the sole 404
// (design §3).
func (s *Server) handleUpdateParticipant(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	sortOrder, ok := bodyIntPtr(m, "sortOrder")
	if !ok {
		writeError(w, http.StatusBadRequest, "sortOrder must be an integer")
		return
	}
	// The repository trims what it stores, so a whitespace-only name would be
	// persisted as an empty one — a speaker the transcript cannot label and a
	// prompt that tells the model to speak as nobody. Creation already refuses it.
	displayName := bodyStringPtr(m, "displayName")
	if displayName != nil && strings.TrimSpace(*displayName) == "" {
		writeError(w, http.StatusBadRequest, "displayName must not be empty")
		return
	}

	participant, err := s.participants.UpdateParticipant(repository.UpdateParticipantInput{
		ParticipantID: r.PathValue("participantId"),
		DisplayName:   displayName,
		RolePrompt:    bodyStringPtr(m, "rolePrompt"),
		BaseURL:       bodyStringPtr(m, "baseUrl"),
		ModelName:     bodyStringPtr(m, "modelName"),
		SortOrder:     sortOrder,
	})
	if err != nil {
		fail(w, err)
		return
	}
	if participant == nil {
		writeError(w, http.StatusNotFound, "participant not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"participant": participant})
}

// handleRemoveParticipant takes the participant off the roster. The row is kept
// (deleted_at is stamped) so past messages keep resolving to a speaker, so the
// removed participant is returned rather than a bare ok.
func (s *Server) handleRemoveParticipant(w http.ResponseWriter, r *http.Request) {
	participant, err := s.participants.RemoveParticipant(r.PathValue("participantId"))
	if err != nil {
		fail(w, err)
		return
	}
	if participant == nil {
		writeError(w, http.StatusNotFound, "participant not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "participant": participant})
}

// handleRunTurnStream runs one turn and streams it (design §4.1, §5). The SSE
// writer is created on the first delta instead of up front: every way a turn can
// be refused — a turn already running (409), an unknown or foreign participant
// (404), a rule violation (400) — happens before the model produces anything, and
// opening the stream earlier would have already committed the response to 200.
func (s *Server) handleRunTurnStream(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	chatID := r.PathValue("chatId")
	participantID := bodyString(m, "participantId")

	var sse *SSEWriter
	var sseErr error
	message, turnErr := s.turnEngine.RunTurn(chatID, participantID, func(chunk string) {
		if sse == nil && sseErr == nil {
			sse, sseErr = NewSSEWriter(w)
		}
		if sse != nil {
			_ = sse.Event("delta", map[string]string{"content": chunk})
		}
	})
	if sseErr != nil {
		fail(w, sseErr)
		return
	}
	if turnErr != nil {
		if sse == nil {
			status, message := turnErrorResponse(turnErr)
			writeError(w, status, message)
			return
		}
		// The turn broke down mid-stream, so the status is already 200 and the
		// failure can only be reported inside the stream.
		_ = sse.Event("error", map[string]string{"message": turnErr.Error()})
		return
	}

	// A turn that produced no delta (an empty completion) still has to answer, so
	// the writer is opened here if the callback never did.
	if sse == nil {
		sse, sseErr = NewSSEWriter(w)
		if sseErr != nil {
			fail(w, sseErr)
			return
		}
	}

	chat, err := s.chats.GetChat(chatID)
	if err != nil {
		_ = sse.Event("error", map[string]string{"message": err.Error()})
		return
	}
	messages, err := s.chats.GetMessagesWithReferences(chatID)
	if err != nil {
		_ = sse.Event("error", map[string]string{"message": err.Error()})
		return
	}
	participants, err := s.participants.ListAll(chatID)
	if err != nil {
		_ = sse.Event("error", map[string]string{"message": err.Error()})
		return
	}
	_ = sse.Event("done", map[string]any{
		"chat":         chat,
		"message":      message,
		"messages":     messages,
		"participants": participants,
	})
}

// turnErrorResponse maps a turn's refusal to its HTTP status. The statuses are
// what the design pins down (409 for an overlapping turn, 404 for a participant
// that is not this chat's), so they are matched on the sentinels rather than on
// message text.
func turnErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrTurnInProgress):
		return http.StatusConflict, err.Error()
	case errors.Is(err, service.ErrChatNotFound), errors.Is(err, service.ErrParticipantNotInChat):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, service.ErrNotMultiAgentChat),
		errors.Is(err, service.ErrRosterEmpty),
		errors.Is(err, service.ErrManualParticipantRequired),
		errors.Is(err, service.ErrParticipantNotNameable),
		errors.Is(err, service.ErrParticipantRemoved):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, service.ErrEndpointUnavailable):
		// The participant's own endpoint refused the model, so the failure is
		// upstream of this server rather than a fault in the request.
		return http.StatusBadGateway, err.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}

// requireMultiAgentChat resolves the chat of a participants route. kind is fixed
// at creation, so a single-assistant chat can never acquire a meaningful roster
// and is rejected here rather than being allowed to collect rows nothing reads.
func (s *Server) requireMultiAgentChat(w http.ResponseWriter, chatID string) (*model.Chat, bool) {
	chat, err := s.chats.GetChat(chatID)
	if err != nil {
		fail(w, err)
		return nil, false
	}
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat not found")
		return nil, false
	}
	if chat.Kind != model.ChatKindMultiAgent {
		writeError(w, http.StatusBadRequest, "chat is not a multi-agent chat")
		return nil, false
	}
	return chat, true
}

// handleListMultiAgentPresets lists the bundled presets for the creation form
// (design §6). Imported presets are not listed: they are applied once, from the
// file, and leave no record of their own.
func (s *Server) handleListMultiAgentPresets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"presets": preset.Bundled()})
}

// presetFromBody reads the preset a chat creation names: presetId picks a
// bundled one, preset carries an imported JSON inline. Both end in the same
// parser, so an imported file is held to exactly what a bundled one is. The
// second return value is false once a refusal has been written.
func (s *Server) presetFromBody(w http.ResponseWriter, m map[string]any, kind string) (*preset.MultiAgentPreset, bool) {
	presetID := strings.TrimSpace(bodyString(m, "presetId"))
	inline, hasInline := m["preset"]
	if presetID == "" && !hasInline {
		return nil, true
	}
	if kind != model.ChatKindMultiAgent {
		writeError(w, http.StatusBadRequest, "presetId and preset apply to multi-agent chats only")
		return nil, false
	}
	if presetID != "" && hasInline {
		writeError(w, http.StatusBadRequest, "specify either presetId or preset, not both")
		return nil, false
	}
	if presetID != "" {
		p, ok := preset.Find(presetID)
		if !ok {
			writeError(w, http.StatusNotFound, "preset not found")
			return nil, false
		}
		return p, true
	}

	// The body was decoded into a generic map, so the preset object is re-encoded
	// for the parser rather than given a second decoding path of its own.
	raw, err := json.Marshal(inline)
	if err != nil {
		writeError(w, http.StatusBadRequest, "preset must be a JSON object")
		return nil, false
	}
	p, err := preset.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return p, true
}

// applyPresetRoster creates the preset's participants in preset order, which is
// the round_robin order (§2). Endpoint and model are left empty, so a turn runs
// against the workspace endpoint until the organisation panel assigns one.
// Chat creation and the roster are not one transaction (the repositories expose
// none), so a roster that fails midway takes its chat with it rather than
// leaving a multi-agent chat with a partial roster behind.
func (s *Server) applyPresetRoster(chatID string, p *preset.MultiAgentPreset) error {
	for _, participant := range p.Participants {
		_, err := s.participants.CreateParticipant(repository.CreateParticipantInput{
			ChatID:      chatID,
			DisplayName: participant.DisplayName,
			RolePrompt:  participant.RolePrompt,
		})
		if err == nil {
			continue
		}
		if _, deleteErr := s.chats.DeleteChat(chatID); deleteErr != nil {
			log.Printf("[preset] chat %s kept with a partial roster: %v", chatID, deleteErr)
		}
		return err
	}
	return nil
}

// storeHumanMessage records the human's intervention in a multi-agent chat: the
// message only, with none of the memory extraction, retrieval or summary work a
// single-assistant turn does (design §4.4).
func (s *Server) storeHumanMessage(chatID, content string) (*model.Message, error) {
	message, err := s.chats.AddMessage(repository.AddMessageInput{
		ChatID:  chatID,
		Role:    "user",
		Content: content,
	})
	if err != nil {
		return nil, err
	}
	return &message, nil
}
