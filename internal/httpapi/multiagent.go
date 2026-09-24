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
// participant be created before its endpoint has been picked. An absent
// receivesProjectMaterial leaves the participant reading the project's material,
// which is the default a roster is built from (design §4.4).
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
	stateSheet := strings.TrimSpace(bodyString(m, "stateSheet"))
	if !stateSheetFits(w, &stateSheet, model.ParticipantStateSheetMaxRunes) {
		return
	}

	participant, err := s.participants.CreateParticipant(repository.CreateParticipantInput{
		ChatID:                  chat.ID,
		DisplayName:             displayName,
		RolePrompt:              bodyString(m, "rolePrompt"),
		BaseURL:                 bodyString(m, "baseUrl"),
		ModelName:               bodyString(m, "modelName"),
		ReceivesProjectMaterial: bodyBoolPtr(m, "receivesProjectMaterial"),
		StateSheet:              stateSheet,
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
	stateSheet := trimmedBodyStringPtr(m, "stateSheet")
	if !stateSheetFits(w, stateSheet, model.ParticipantStateSheetMaxRunes) {
		return
	}

	participant, err := s.participants.UpdateParticipant(repository.UpdateParticipantInput{
		ParticipantID:           r.PathValue("participantId"),
		DisplayName:             displayName,
		RolePrompt:              bodyStringPtr(m, "rolePrompt"),
		BaseURL:                 bodyStringPtr(m, "baseUrl"),
		ModelName:               bodyStringPtr(m, "modelName"),
		SortOrder:               sortOrder,
		ReceivesProjectMaterial: bodyBoolPtr(m, "receivesProjectMaterial"),
		StateSheet:              stateSheet,
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

// handleRunTurnStream runs one turn and streams it (design §4.1, §5, §4.6.6).
// The SSE writer is created when the engine announces the speaker instead of up
// front: every way a turn can be refused — a turn already running (409), an
// unknown or foreign participant (404), a rule violation (400), an endpoint that
// does not answer (502) — happens before that, and opening the stream earlier
// would have already committed the response to 200. Everything that fails after
// the announcement (generation, storing the message) is reported inside the
// stream, whether or not a delta had arrived.
func (s *Server) handleRunTurnStream(w http.ResponseWriter, r *http.Request) {
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	chatID := r.PathValue("chatId")
	participantID := bodyString(m, "participantId")

	var sse *SSEWriter
	var sseErr error
	message, turnErr := s.turnEngine.RunTurn(chatID, participantID, func(choice service.SpeakerChoice) {
		sse, sseErr = NewSSEWriter(w)
		if sse != nil {
			_ = sse.Event("speaker", choice)
		}
	}, func(chunk string) {
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
		_ = sse.Event("error", map[string]string{"message": turnErr.Error()})
		return
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

// createPresetParticipants creates the preset's participants in preset order,
// which is the round_robin order (§2), and returns the id the preset's
// facilitator mark resolved to (empty when the preset marks none). Endpoint and
// model are left empty, so a turn runs against the workspace endpoint until the
// organisation panel assigns one. receivesProjectMaterial is preset data, because
// a preset is what expresses a line-up where one speaker knows what the others
// must not.
func (s *Server) createPresetParticipants(chatID string, p *preset.MultiAgentPreset) (string, error) {
	facilitatorID := ""
	for _, participant := range p.Participants {
		created, err := s.participants.CreateParticipant(repository.CreateParticipantInput{
			ChatID:                  chatID,
			DisplayName:             participant.DisplayName,
			RolePrompt:              participant.RolePrompt,
			ReceivesProjectMaterial: participant.ReceivesProjectMaterial,
			StateSheet:              participant.StateSheet,
		})
		if err != nil {
			return "", err
		}
		if participant.Facilitator {
			facilitatorID = created.ID
		}
	}
	return facilitatorID, nil
}

// applyPresetRoster is createPresetParticipants for a chat that was just created
// from the preset, followed by the facilitator the roster resolved to. Chat
// creation and the roster are not one transaction (the repositories expose
// none), so a roster that fails midway takes its chat with it rather than
// leaving a multi-agent chat with a partial roster behind.
func (s *Server) applyPresetRoster(chatID string, p *preset.MultiAgentPreset) (*model.Chat, error) {
	facilitatorID, err := s.createPresetParticipants(chatID, p)
	if err == nil {
		return s.chats.UpdateMultiAgentSettings(chatID, repository.MultiAgentSettings{FacilitatorID: &facilitatorID})
	}
	if _, deleteErr := s.chats.DeleteChat(chatID); deleteErr != nil {
		log.Printf("[preset] chat %s kept with a partial roster: %v", chatID, deleteErr)
	}
	return nil, err
}

// The refusals handleApplyMultiAgentPreset raises from inside the turn exclusion,
// where it can report only an error.
var (
	errChatAlreadySpoken = errors.New("a preset applies only while the conversation has no messages")
	errChatVanished      = errors.New("chat not found")
)

// handleApplyMultiAgentPreset applies a preset to a multi-agent chat that exists
// already, which is allowed only while the chat has no messages. A chat can now
// be created with an empty roster (the sidebar's "+"), so the preset has to be
// pickable after creation; a conversation that has already been spoken in is
// refused instead, because replacing the roster, the turn rule and the scene
// would leave the transcript referring to a cast the chat no longer has.
func (s *Server) handleApplyMultiAgentPreset(w http.ResponseWriter, r *http.Request) {
	chat, ok := s.requireMultiAgentChat(w, r.PathValue("chatId"))
	if !ok {
		return
	}
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	chosen, ok := s.presetFromBody(w, m, chat.Kind)
	if !ok {
		return
	}
	if chosen == nil {
		writeError(w, http.StatusBadRequest, "presetId or preset is required")
		return
	}

	// The emptiness check and the replacement it guards run under both of the
	// chat's exclusions, because either kind of write would otherwise slip between
	// them. A turn picks its speaker from the roster and stores its message only
	// when it finishes, so a turn in flight would store a message pointing at a
	// participant this route has since deleted — a speaker the transcript can no
	// longer name, which is what §3's logical removal exists to prevent. A human
	// intervention needs no roster at all, so it can land straight after the check
	// and leave the preset applied to a conversation that has been spoken in.
	var updated *model.Chat
	applyErr := s.turnEngine.WithTurnExcluded(chat.ID, func() error {
		return s.turnEngine.WithMessageWrite(chat.ID, func() error {
			var err error
			updated, err = s.applyPresetToChat(chat.ID, chosen)
			return err
		})
	})
	switch {
	case errors.Is(applyErr, errChatVanished):
		writeError(w, http.StatusNotFound, applyErr.Error())
		return
	// 409 rather than 400 for both: the request is well formed and the same body
	// would be accepted on the same chat a moment earlier — it is the chat's state
	// that rules it out, which is what 409 says (as it already does for an
	// overlapping turn).
	case errors.Is(applyErr, errChatAlreadySpoken), errors.Is(applyErr, service.ErrTurnInProgress):
		writeError(w, http.StatusConflict, applyErr.Error())
		return
	case applyErr != nil:
		fail(w, applyErr)
		return
	}

	participants, err := s.participants.ListAll(chat.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chat": updated, "participants": participants})
}

// applyPresetToChat is the state change handleApplyMultiAgentPreset makes, run
// by its caller under the chat's turn and message-write exclusions — the
// emptiness check included, since it is what the rest of this depends on.
func (s *Server) applyPresetToChat(chatID string, p *preset.MultiAgentPreset) (*model.Chat, error) {
	spoken, err := s.chats.ListRecentMessages(chatID, 1)
	if err != nil {
		return nil, err
	}
	if len(spoken) > 0 {
		return nil, errChatAlreadySpoken
	}

	// The roster is replaced rather than appended to, and the rows are deleted
	// outright: applying a preset must leave the chat as if it had been created
	// from that preset, and a kept row would both show up as a removed participant
	// and push the new roster's sort_order past it.
	if err := s.participants.DeleteRoster(chatID); err != nil {
		return nil, err
	}
	// The shared state sheet is written even when the preset carries none, for the
	// same reason the facilitator is below: it describes the line-up being
	// replaced, and an empty preset value must clear it.
	chat, err := s.chats.UpdateMultiAgentSettings(chatID, repository.MultiAgentSettings{
		TurnRule:    &p.TurnRule,
		ScenePrompt: &p.ScenePrompt,
		StateSheet:  &p.StateSheet,
	})
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, errChatVanished
	}
	// Same rule as creation: the preset names the chat only when nothing else has.
	// A chat created from the sidebar has an empty title, which is the case this
	// route exists for.
	if strings.TrimSpace(chat.Title) == "" {
		chat, err = s.chats.UpdateChatTitle(chatID, p.Title)
		if err != nil {
			return nil, err
		}
		if chat == nil {
			return nil, errChatVanished
		}
	}
	// A failure partway through leaves a partial roster, which creation avoids by
	// deleting the chat (applyPresetRoster). Here the chat predates the preset and
	// may be the only thing the user has, so it is kept: the transcript is still
	// empty, so this same route stays open and re-applying clears the partial
	// roster and starts over.
	facilitatorID, err := s.createPresetParticipants(chatID, p)
	if err != nil {
		return nil, err
	}
	// Written whether or not the preset marks one: the roster it replaced is gone,
	// so leaving the previous facilitator in place would point the setting at a
	// participant this chat no longer has.
	chat, err = s.chats.UpdateMultiAgentSettings(chatID, repository.MultiAgentSettings{FacilitatorID: &facilitatorID})
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, errChatVanished
	}
	return chat, nil
}

// storeHumanMessage records the human's intervention in a multi-agent chat: the
// message and whom it calls on, with none of the memory extraction, retrieval or
// summary work a single-assistant turn does (design §4.4). The call is detected
// here, at store time, for the same reason a turn's is (§4.6.5): without it "A,
// tell us more" never reaches A under the weighted rule. It takes the chat's
// message-write lock, which is what keeps it from landing inside an apply that
// has already found the conversation empty; a turn in flight does not hold that
// lock, so speaking mid-turn still goes straight through.
func (s *Server) storeHumanMessage(chatID, content string) (*model.Message, error) {
	var message model.Message
	err := s.turnEngine.WithMessageWrite(chatID, func() error {
		roster, err := s.participants.ListRoster(chatID)
		if err != nil {
			return err
		}
		stored, err := s.chats.AddMessage(repository.AddMessageInput{
			ChatID:  chatID,
			Role:    "user",
			Content: content,

			AddressedParticipantIDs: service.DetectHumanAddressees(content, roster),
		})
		message = stored
		return err
	})
	if err != nil {
		return nil, err
	}
	return &message, nil
}
