package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"snzstudio/internal/model"
	"snzstudio/internal/util"
)

// ChatRepository ports backend/src/repositories/chatRepository.ts.
type ChatRepository struct {
	db *sql.DB
}

// NewChatRepository constructs a ChatRepository over the shared DB.
func NewChatRepository(db *sql.DB) *ChatRepository {
	return &ChatRepository{db: db}
}

const (
	chatColumns    = `id, project_id, title, is_temporary, kind, turn_rule, scene_prompt, facilitator_participant_id, state_sheet, created_at, updated_at`
	messageColumns = `id, chat_id, role, content, created_at, response_ms, output_tokens, tokens_per_second, model_name, participant_id, addressed_participant_ids`
	summaryColumns = `chat_id, summary, updated_at`
	referenceCols  = `id, assistant_message_id, source_type, source_id, label, excerpt, score, created_at`
)

func scanChat(s scanner) (model.Chat, error) {
	var (
		c           model.Chat
		isTemporary int64
	)
	if err := s.Scan(&c.ID, &c.ProjectID, &c.Title, &isTemporary, &c.Kind, &c.TurnRule, &c.ScenePrompt, &c.FacilitatorID, &c.StateSheet, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return c, err
	}
	c.IsTemporary = isTemporary != 0
	return c, nil
}

func scanMessage(s scanner) (model.Message, error) {
	var (
		m             model.Message
		respMs        sql.NullInt64
		outTok        sql.NullInt64
		tps           sql.NullFloat64
		modelName     sql.NullString
		participantID sql.NullString
		addressees    string
	)
	if err := s.Scan(&m.ID, &m.ChatID, &m.Role, &m.Content, &m.CreatedAt, &respMs, &outTok, &tps, &modelName, &participantID, &addressees); err != nil {
		return m, err
	}
	if err := json.Unmarshal([]byte(addressees), &m.AddressedParticipantIDs); err != nil {
		return m, fmt.Errorf("message %s: addressed_participant_ids: %w", m.ID, err)
	}
	m.ResponseMs = int64Ptr(respMs)
	m.OutputTokens = int64Ptr(outTok)
	m.TokensPerSecond = float64Ptr(tps)
	m.ModelName = strPtr(modelName)
	m.ParticipantID = strPtr(participantID)
	return m, nil
}

func scanSummary(s scanner) (model.ChatSummary, error) {
	var summary model.ChatSummary
	err := s.Scan(&summary.ChatID, &summary.Summary, &summary.UpdatedAt)
	return summary, err
}

func scanReference(s scanner) (model.AssistantMessageReference, error) {
	var ref model.AssistantMessageReference
	err := s.Scan(&ref.ID, &ref.AssistantMessageID, &ref.SourceType, &ref.SourceID, &ref.Label, &ref.Excerpt, &ref.Score, &ref.CreatedAt)
	return ref, err
}

// ListByProject returns a project's chats, most recently updated first.
func (r *ChatRepository) ListByProject(projectID string) ([]model.Chat, error) {
	rows, err := r.db.Query("SELECT "+chatColumns+" FROM chats WHERE project_id = ? ORDER BY updated_at DESC, created_at DESC", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chats := []model.Chat{}
	for rows.Next() {
		c, err := scanChat(rows)
		if err != nil {
			return nil, err
		}
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

// ListRecent returns up to limit chats across every project, most recently
// updated first, each with its project's title.
func (r *ChatRepository) ListRecent(limit int) ([]model.RecentChat, error) {
	rows, err := r.db.Query(`
		SELECT c.id, c.project_id, c.title, c.is_temporary, c.kind, c.turn_rule, c.scene_prompt, c.facilitator_participant_id, c.state_sheet, c.created_at, c.updated_at, p.title
		FROM chats c
		JOIN projects p ON p.id = c.project_id
		ORDER BY c.updated_at DESC, c.created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chats := []model.RecentChat{}
	for rows.Next() {
		var (
			c           model.RecentChat
			isTemporary int64
		)
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Title, &isTemporary, &c.Kind, &c.TurnRule, &c.ScenePrompt, &c.FacilitatorID, &c.StateSheet, &c.CreatedAt, &c.UpdatedAt, &c.ProjectTitle); err != nil {
			return nil, err
		}
		c.IsTemporary = isTemporary != 0
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

// GetChat returns the chat, or (nil, nil) if it does not exist.
func (r *ChatRepository) GetChat(chatID string) (*model.Chat, error) {
	c, err := scanChat(r.db.QueryRow("SELECT "+chatColumns+" FROM chats WHERE id = ?", chatID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CreateChatInput carries the fields for CreateChat. Kind, TurnRule,
// ScenePrompt and StateSheet are optional: an empty Kind/TurnRule falls back to the column
// defaults, so existing callers keep creating single-assistant chats. The
// facilitator is not among them — it names a participant, and a chat has no
// roster until after it exists (UpdateMultiAgentSettings sets it).
type CreateChatInput struct {
	ProjectID   string
	Title       string
	IsTemporary bool
	Kind        string
	TurnRule    string
	ScenePrompt string
	StateSheet  string
}

// CreateChat inserts a chat and seeds an empty summary row. Mirrors createChat.
func (r *ChatRepository) CreateChat(input CreateChatInput) (model.Chat, error) {
	now := util.NowISO()
	kind := input.Kind
	if kind == "" {
		kind = model.ChatKindAssistant
	}
	turnRule := input.TurnRule
	if turnRule == "" {
		turnRule = model.TurnRuleRoundRobin
	}
	c := model.Chat{
		ID:          util.NewID("chat"),
		ProjectID:   input.ProjectID,
		Title:       strings.TrimSpace(input.Title),
		IsTemporary: input.IsTemporary,
		Kind:        kind,
		TurnRule:    turnRule,
		ScenePrompt: input.ScenePrompt,
		StateSheet:  input.StateSheet,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := r.db.Exec(`
		INSERT INTO chats (id, project_id, title, is_temporary, kind, turn_rule, scene_prompt, state_sheet, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProjectID, c.Title, boolToInt(c.IsTemporary), c.Kind, c.TurnRule, c.ScenePrompt, c.StateSheet, c.CreatedAt, c.UpdatedAt); err != nil {
		return model.Chat{}, err
	}
	if err := r.UpsertSummary(c.ID, ""); err != nil {
		return model.Chat{}, err
	}
	return c, nil
}

// UpdateChatTitle updates the title, returning (nil, nil) if the chat does not
// exist.
func (r *ChatRepository) UpdateChatTitle(chatID, title string) (*model.Chat, error) {
	res, err := r.db.Exec("UPDATE chats SET title = ?, updated_at = ? WHERE id = ?", strings.TrimSpace(title), util.NowISO(), chatID)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return r.GetChat(chatID)
}

// SetTemporary toggles a chat's temporary flag, returning (nil, nil) if the chat
// does not exist.
func (r *ChatRepository) SetTemporary(chatID string, isTemporary bool) (*model.Chat, error) {
	res, err := r.db.Exec("UPDATE chats SET is_temporary = ?, updated_at = ? WHERE id = ?", boolToInt(isTemporary), util.NowISO(), chatID)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return r.GetChat(chatID)
}

// MultiAgentSettings carries the fields UpdateMultiAgentSettings writes. A nil
// field is left untouched.
type MultiAgentSettings struct {
	TurnRule      *string
	ScenePrompt   *string
	FacilitatorID *string
	StateSheet    *string
}

// UpdateMultiAgentSettings updates a multi-agent chat's turn rule, scene prompt,
// facilitator and/or shared state sheet, and returns (nil, nil) if the chat does
// not exist. The update is partial because PATCH /api/chats/{chatId} accepts any
// of the fields on its own (design §5).
func (r *ChatRepository) UpdateMultiAgentSettings(chatID string, settings MultiAgentSettings) (*model.Chat, error) {
	res, err := r.db.Exec(`
		UPDATE chats
		SET turn_rule = COALESCE(?, turn_rule),
		    scene_prompt = COALESCE(?, scene_prompt),
		    facilitator_participant_id = COALESCE(?, facilitator_participant_id),
		    state_sheet = COALESCE(?, state_sheet),
		    updated_at = ?
		WHERE id = ?`,
		ptrArg(settings.TurnRule), ptrArg(settings.ScenePrompt), ptrArg(settings.FacilitatorID), ptrArg(settings.StateSheet),
		util.NowISO(), chatID)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return r.GetChat(chatID)
}

// DeleteChat removes a chat (messages/summary/references cascade), returning the
// deleted record (or nil if it did not exist).
func (r *ChatRepository) DeleteChat(chatID string) (*model.Chat, error) {
	chat, err := r.GetChat(chatID)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, nil
	}
	if _, err := r.db.Exec("DELETE FROM chats WHERE id = ?", chatID); err != nil {
		return nil, err
	}
	return chat, nil
}

// AddMessageInput carries the fields for AddMessage. ParticipantID is set only
// for a multi-agent participant's turn; AddressedParticipantIDs only for a
// multi-agent message that called on someone (nil stores as no call).
type AddMessageInput struct {
	ChatID          string
	Role            string
	Content         string
	ResponseMs      *int64
	OutputTokens    *int64
	TokensPerSecond *float64
	ModelName       *string
	ParticipantID   *string

	AddressedParticipantIDs []string
}

// AddMessage inserts a message and bumps the chat's updated_at. Mirrors addMessage.
func (r *ChatRepository) AddMessage(input AddMessageInput) (model.Message, error) {
	return r.AddMessageWithReferences(input, nil)
}

// AddMessageWithReferences inserts a message together with its references in one
// transaction, so either both are stored or neither is. A multi-agent turn needs
// that: its speaker order is read from the stored messages, so an utterance whose
// references failed to store must not remain as a turn that happened.
func (r *ChatRepository) AddMessageWithReferences(input AddMessageInput, references []ReferenceInput) (model.Message, error) {
	now := util.NowISO()
	m := model.Message{
		ID:              util.NewID("msg"),
		ChatID:          input.ChatID,
		Role:            input.Role,
		Content:         strings.TrimSpace(input.Content),
		CreatedAt:       now,
		ResponseMs:      input.ResponseMs,
		OutputTokens:    input.OutputTokens,
		TokensPerSecond: input.TokensPerSecond,
		ModelName:       input.ModelName,
		ParticipantID:   input.ParticipantID,

		AddressedParticipantIDs: input.AddressedParticipantIDs,
	}
	if m.AddressedParticipantIDs == nil {
		m.AddressedParticipantIDs = []string{}
	}
	addressees, err := json.Marshal(m.AddressedParticipantIDs)
	if err != nil {
		return model.Message{}, err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return model.Message{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		INSERT INTO messages (id, chat_id, role, content, created_at, response_ms, output_tokens, tokens_per_second, model_name, participant_id, addressed_participant_ids)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ChatID, m.Role, m.Content, m.CreatedAt,
		ptrArg(m.ResponseMs), ptrArg(m.OutputTokens), ptrArg(m.TokensPerSecond), ptrArg(m.ModelName), ptrArg(m.ParticipantID), string(addressees)); err != nil {
		return model.Message{}, err
	}
	if err := insertReferences(tx, m.ID, references); err != nil {
		return model.Message{}, err
	}
	if _, err := tx.Exec("UPDATE chats SET updated_at = ? WHERE id = ?", util.NowISO(), m.ChatID); err != nil {
		return model.Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Message{}, err
	}
	return m, nil
}

// UpdateMessageContent overwrites a message's content (used during streaming),
// returning (nil, nil) if the message does not exist. Content is stored verbatim
// (no trimming), matching the TS.
func (r *ChatRepository) UpdateMessageContent(messageID, content string) (*model.Message, error) {
	res, err := r.db.Exec("UPDATE messages SET content = ? WHERE id = ?", content, messageID)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return r.GetMessage(messageID)
}

// FinalizeMessageInput carries the fields for FinalizeMessage.
type FinalizeMessageInput struct {
	MessageID       string
	Content         string
	ResponseMs      *int64
	OutputTokens    *int64
	TokensPerSecond *float64
	ModelName       *string
}

// FinalizeMessage writes the final content and metrics for a streamed assistant
// message, returning (nil, nil) if the message does not exist. model_name is kept
// when the input is nil (COALESCE). Mirrors finalizeMessage.
func (r *ChatRepository) FinalizeMessage(input FinalizeMessageInput) (*model.Message, error) {
	res, err := r.db.Exec(`
		UPDATE messages
		SET content = ?, response_ms = ?, output_tokens = ?, tokens_per_second = ?, model_name = COALESCE(?, model_name)
		WHERE id = ?`,
		input.Content, ptrArg(input.ResponseMs), ptrArg(input.OutputTokens), ptrArg(input.TokensPerSecond), ptrArg(input.ModelName), input.MessageID)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return r.GetMessage(input.MessageID)
}

// ListMessages returns a chat's messages in chronological order.
func (r *ChatRepository) ListMessages(chatID string) ([]model.Message, error) {
	rows, err := r.db.Query("SELECT "+messageColumns+" FROM messages WHERE chat_id = ? ORDER BY created_at ASC", chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []model.Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// GetMessage returns the message, or (nil, nil) if it does not exist.
func (r *ChatRepository) GetMessage(messageID string) (*model.Message, error) {
	m, err := scanMessage(r.db.QueryRow("SELECT "+messageColumns+" FROM messages WHERE id = ?", messageID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListRecentMessages returns the most recent messages in chronological order.
// limit defaults to 8 when not positive. Mirrors listRecentMessages(chatId, limit=8).
func (r *ChatRepository) ListRecentMessages(chatID string, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 8
	}
	rows, err := r.db.Query("SELECT "+messageColumns+" FROM messages WHERE chat_id = ? ORDER BY created_at DESC LIMIT ?", chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []model.Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse to chronological order, matching rows.reverse() in the TS.
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

// GetSummary returns the chat's summary row, or (nil, nil) if absent.
func (r *ChatRepository) GetSummary(chatID string) (*model.ChatSummary, error) {
	s, err := scanSummary(r.db.QueryRow("SELECT "+summaryColumns+" FROM chat_summaries WHERE chat_id = ?", chatID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSummariesByProject returns chat summaries for a project, optionally
// excluding temporary chats. Mirrors listSummariesByProject(projectId, includeTemporary=true).
func (r *ChatRepository) ListSummariesByProject(projectID string, includeTemporary bool) ([]model.ChatSummary, error) {
	rows, err := r.db.Query(`
		SELECT s.chat_id, s.summary, s.updated_at
		FROM chat_summaries s
		JOIN chats c ON c.id = s.chat_id
		WHERE c.project_id = ?
		  AND (? = 1 OR c.is_temporary = 0)
		ORDER BY s.updated_at DESC`,
		projectID, boolToInt(includeTemporary))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	summaries := []model.ChatSummary{}
	for rows.Next() {
		s, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

// UpsertSummary inserts or updates a chat's summary. Mirrors upsertSummary.
func (r *ChatRepository) UpsertSummary(chatID, summary string) error {
	_, err := r.db.Exec(`
		INSERT INTO chat_summaries (chat_id, summary, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET summary = excluded.summary, updated_at = excluded.updated_at`,
		chatID, summary, util.NowISO())
	return err
}

// ReferenceInput is a single assistant-message reference to persist (without the
// generated id/createdAt/assistantMessageId).
type ReferenceInput struct {
	SourceType string
	SourceID   string
	Label      string
	Excerpt    string
	Score      float64
}

// ReplaceAssistantReferences swaps all references for an assistant message.
// Mirrors replaceAssistantReferences.
func (r *ChatRepository) ReplaceAssistantReferences(assistantMessageID string, references []ReferenceInput) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM assistant_message_references WHERE assistant_message_id = ?", assistantMessageID); err != nil {
		return err
	}
	if err := insertReferences(tx, assistantMessageID, references); err != nil {
		return err
	}
	return tx.Commit()
}

func insertReferences(tx *sql.Tx, assistantMessageID string, references []ReferenceInput) error {
	for _, ref := range references {
		if _, err := tx.Exec(`
			INSERT INTO assistant_message_references (id, assistant_message_id, source_type, source_id, label, excerpt, score, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			util.NewID("ref"), assistantMessageID, ref.SourceType, ref.SourceID, ref.Label, ref.Excerpt, ref.Score, util.NowISO()); err != nil {
			return err
		}
	}
	return nil
}

// GetMessagesWithReferences returns a chat's messages, each with its references
// (ordered by score desc, created_at asc) attached. Mirrors getMessagesWithReferences.
func (r *ChatRepository) GetMessagesWithReferences(chatID string) ([]model.MessageWithReferences, error) {
	messages, err := r.ListMessages(chatID)
	if err != nil {
		return nil, err
	}

	var assistantIDs []string
	for _, m := range messages {
		if m.Role == "assistant" {
			assistantIDs = append(assistantIDs, m.ID)
		}
	}

	refsByMessage := map[string][]model.AssistantMessageReference{}
	if len(assistantIDs) > 0 {
		args := make([]any, len(assistantIDs))
		for i, id := range assistantIDs {
			args[i] = id
		}
		rows, err := r.db.Query("SELECT "+referenceCols+" FROM assistant_message_references WHERE assistant_message_id IN ("+inPlaceholders(len(assistantIDs))+") ORDER BY score DESC, created_at ASC", args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			ref, err := scanReference(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			refsByMessage[ref.AssistantMessageID] = append(refsByMessage[ref.AssistantMessageID], ref)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	out := make([]model.MessageWithReferences, 0, len(messages))
	for _, m := range messages {
		refs := refsByMessage[m.ID]
		if refs == nil {
			refs = []model.AssistantMessageReference{}
		}
		out = append(out, model.MessageWithReferences{Message: m, References: refs})
	}
	return out, nil
}
