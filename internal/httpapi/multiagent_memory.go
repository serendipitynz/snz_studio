// multiagent_memory.go serves the one write path from a multi-agent chat to the
// project's memories (docs/multi-agent-chat-design.md §4.4): the human picks an
// utterance and saves it. Nothing here runs on its own — a multi-agent chat has
// no automatic extraction, so a memory exists only because someone chose it.
// Both routes hang off a flat message id, as the review routes do.
package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"snzstudio/internal/model"
	"snzstudio/internal/repository"
	"snzstudio/internal/service"
)

// memorySourceMultiAgent is the memories.source value of a memory saved from a
// multi-agent utterance. It is spelled like the chat kind so the organizer's
// payload and the project page's badge read as "from a multi-agent chat".
const memorySourceMultiAgent = "multi_agent"

// handleGetMessageMemoryDraft returns what the save dialog opens with: the
// utterance as written and the kind the extraction rules would have given it.
// The kind is inferred here rather than in the browser so the cue table has one
// home.
func (s *Server) handleGetMessageMemoryDraft(w http.ResponseWriter, r *http.Request) {
	message, _, ok := s.requireMemorySaveableMessage(w, r.PathValue("messageId"))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"draft": map[string]string{
			"content": message.Content,
			"kind":    service.InferMemoryKind(message.Content),
		},
	})
}

// handleSaveMessageMemory stores the edited draft as a project memory. It goes
// through the same repository call and embedding sync as the manual and
// automatic paths, so the memory is searchable and organizable like any other;
// only its source tells it apart. locked defaults to true as for a manual
// memory: the human chose the wording, so the organizer should not rewrite it.
func (s *Server) handleSaveMessageMemory(w http.ResponseWriter, r *http.Request) {
	_, chat, ok := s.requireMemorySaveableMessage(w, r.PathValue("messageId"))
	if !ok {
		return
	}
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	content := strings.TrimSpace(bodyString(m, "content"))
	if content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	kind := bodyString(m, "kind")
	if kind == "" {
		kind = service.InferMemoryKind(content)
	}
	if kind != "semantic" && kind != "procedural" && kind != "episodic" {
		writeError(w, http.StatusBadRequest, "invalid memory kind")
		return
	}
	locked := true
	if b, ok := bodyBool(m, "locked"); ok && !b {
		locked = false
	}

	sourceChatID := chat.ID
	memory, err := s.memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID:    chat.ProjectID,
		Kind:         kind,
		Title:        service.GenerateMemoryTitle(content, kind),
		Content:      content,
		SourceChatID: &sourceChatID,
		Source:       memorySourceMultiAgent,
		Locked:       locked,
	})
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.embeddingSync.SyncMemories([]string{memory.ID}); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"memory": memory})
}

// requireMemorySaveableMessage resolves the message and its chat for both
// routes and writes the refusal when the pair cannot be saved from: a missing
// message or chat is 404, a single-assistant chat is 400 (it has its own memory
// paths), and a temporary chat is 409 — the request is well formed and the same
// body is accepted once the flag is cleared, so it is the chat's state that
// refuses it, as with an overlapping turn. The draft is refused on the same
// terms so it never offers a save the second route would reject.
func (s *Server) requireMemorySaveableMessage(w http.ResponseWriter, messageID string) (*model.Message, *model.Chat, bool) {
	message, err := s.chats.GetMessage(messageID)
	if err != nil {
		fail(w, err)
		return nil, nil, false
	}
	if message == nil {
		writeError(w, http.StatusNotFound, "message not found")
		return nil, nil, false
	}
	chat, err := s.chats.GetChat(message.ChatID)
	if err != nil {
		fail(w, err)
		return nil, nil, false
	}
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat not found")
		return nil, nil, false
	}
	if chat.Kind != model.ChatKindMultiAgent {
		writeError(w, http.StatusBadRequest, "chat is not a multi-agent chat")
		return nil, nil, false
	}
	if chat.IsTemporary {
		writeError(w, http.StatusConflict, "a temporary chat does not write project memories")
		return nil, nil, false
	}
	return message, chat, true
}

// handleDraftConclusion generates what the save dialog opens with when the
// human saves a conversation's outcome rather than one utterance: the default
// model's draft of what was decided and what is still open, over the whole
// conversation or from fromMessageId to the latest (design §4.4). It is a POST
// because every call runs the model, and it is allowed in a temporary chat —
// reading the draft writes nothing; only the save route refuses there.
//
// anchorMessageId is the last utterance of the range, which the dialog saves
// through: the save route takes a message id only to resolve its chat.
func (s *Server) handleDraftConclusion(w http.ResponseWriter, r *http.Request) {
	chat, ok := s.requireMultiAgentChat(w, r.PathValue("chatId"))
	if !ok {
		return
	}
	m, ok := decodeBody(w, r)
	if !ok {
		return
	}
	messages, err := s.chats.ListMessages(chat.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if fromID := bodyString(m, "fromMessageId"); fromID != "" {
		start := -1
		for i, message := range messages {
			if message.ID == fromID {
				start = i
				break
			}
		}
		if start < 0 {
			writeError(w, http.StatusNotFound, "message not found in this chat")
			return
		}
		messages = messages[start:]
	}
	participants, err := s.participants.ListAll(chat.ID)
	if err != nil {
		fail(w, err)
		return
	}

	content, err := s.summary.DraftConclusion(messages, participants)
	var tooLong *service.ConclusionTooLongError
	switch {
	case errors.Is(err, service.ErrNothingToConclude):
		writeError(w, http.StatusConflict, "the conversation has no utterances to summarize")
		return
	case errors.As(err, &tooLong):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": tooLong.Error(),
			"chars": tooLong.Chars,
			"limit": tooLong.Limit,
		})
		return
	case err != nil:
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"draft":           map[string]string{"content": content, "kind": "semantic"},
		"anchorMessageId": messages[len(messages)-1].ID,
		"messageCount":    len(messages),
	})
}
