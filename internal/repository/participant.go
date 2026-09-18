package repository

import (
	"database/sql"
	"errors"
	"strings"

	"snzstudio/internal/model"
	"snzstudio/internal/util"
)

// ParticipantRepository stores the speakers of a multi-agent chat
// (docs/multi-agent-chat-design.md §3). A participant is never hard-deleted:
// removal from the roster stamps deleted_at, so the chat has two participant
// sets and this repository exposes one method per set — ListRoster for the
// enrolled ones and ListAll for name resolution over the whole history.
type ParticipantRepository struct {
	db *sql.DB
}

// NewParticipantRepository constructs a ParticipantRepository over the shared DB.
func NewParticipantRepository(db *sql.DB) *ParticipantRepository {
	return &ParticipantRepository{db: db}
}

const participantColumns = `id, chat_id, display_name, role_prompt, base_url, model_name, sort_order, created_at, deleted_at`

// participantOrder is the cycle round_robin walks, so it must be total: equal
// sort_order values (two participants added in the same batch, or a row whose
// order was never set) still have to yield one stable successor.
const participantOrder = `ORDER BY sort_order ASC, created_at ASC, id ASC`

func scanParticipant(s scanner) (model.Participant, error) {
	var (
		p         model.Participant
		deletedAt sql.NullString
	)
	if err := s.Scan(&p.ID, &p.ChatID, &p.DisplayName, &p.RolePrompt, &p.BaseURL, &p.ModelName, &p.SortOrder, &p.CreatedAt, &deletedAt); err != nil {
		return p, err
	}
	p.DeletedAt = strPtr(deletedAt)
	return p, nil
}

func (r *ParticipantRepository) listParticipants(query string, args ...any) ([]model.Participant, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	participants := []model.Participant{}
	for rows.Next() {
		p, err := scanParticipant(rows)
		if err != nil {
			return nil, err
		}
		participants = append(participants, p)
	}
	return participants, rows.Err()
}

// ListRoster returns the chat's roster: the participants still enrolled
// (deleted_at IS NULL), in turn order. This is what round_robin cycles and what
// the organisation panel edits.
func (r *ParticipantRepository) ListRoster(chatID string) ([]model.Participant, error) {
	return r.listParticipants(
		"SELECT "+participantColumns+" FROM participants WHERE chat_id = ? AND deleted_at IS NULL "+participantOrder,
		chatID)
}

// ListAll returns every participant row of the chat, removed ones included, in
// the same order. Callers that resolve a past message's speaker must use this
// set: messages.participant_id keeps pointing at a removed participant, and its
// display name and model name still have to be shown.
func (r *ParticipantRepository) ListAll(chatID string) ([]model.Participant, error) {
	return r.listParticipants(
		"SELECT "+participantColumns+" FROM participants WHERE chat_id = ? "+participantOrder,
		chatID)
}

// GetParticipant returns the participant, removed ones included, or (nil, nil)
// if no such row exists. The result carries chat_id, which is what lets a
// caller reject a participant named under a different chat (design §3).
func (r *ParticipantRepository) GetParticipant(participantID string) (*model.Participant, error) {
	p, err := scanParticipant(r.db.QueryRow("SELECT "+participantColumns+" FROM participants WHERE id = ?", participantID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreateParticipantInput carries the fields for CreateParticipant.
type CreateParticipantInput struct {
	ChatID      string
	DisplayName string
	RolePrompt  string
	BaseURL     string
	ModelName   string
}

// CreateParticipant appends a participant to the chat's roster, assigning the
// next sort_order (max+1 within the chat, counting removed rows so a removal
// never makes a later addition collide with an existing order).
func (r *ParticipantRepository) CreateParticipant(input CreateParticipantInput) (model.Participant, error) {
	var maxSort sql.NullInt64
	if err := r.db.QueryRow("SELECT MAX(sort_order) FROM participants WHERE chat_id = ?", input.ChatID).Scan(&maxSort); err != nil {
		return model.Participant{}, err
	}
	next := 0
	if maxSort.Valid {
		next = int(maxSort.Int64) + 1
	}

	p := model.Participant{
		ID:          util.NewID("participant"),
		ChatID:      input.ChatID,
		DisplayName: strings.TrimSpace(input.DisplayName),
		RolePrompt:  strings.TrimSpace(input.RolePrompt),
		BaseURL:     strings.TrimSpace(input.BaseURL),
		ModelName:   strings.TrimSpace(input.ModelName),
		SortOrder:   next,
		CreatedAt:   util.NowISO(),
	}
	if _, err := r.db.Exec(`
		INSERT INTO participants (id, chat_id, display_name, role_prompt, base_url, model_name, sort_order, created_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		p.ID, p.ChatID, p.DisplayName, p.RolePrompt, p.BaseURL, p.ModelName, p.SortOrder, p.CreatedAt); err != nil {
		return model.Participant{}, err
	}
	return p, nil
}

// UpdateParticipantInput carries the fields for UpdateParticipant. A nil field
// is left untouched, matching the PATCH route that edits one setting at a time.
type UpdateParticipantInput struct {
	ParticipantID string
	DisplayName   *string
	RolePrompt    *string
	BaseURL       *string
	ModelName     *string
	SortOrder     *int
}

// UpdateParticipant applies the given fields and returns the updated row, or
// (nil, nil) if the participant does not exist. Removed participants are
// updatable here too: whether editing one is meaningful is the service layer's
// call, and this layer has no reason to make the row unreachable.
func (r *ParticipantRepository) UpdateParticipant(input UpdateParticipantInput) (*model.Participant, error) {
	res, err := r.db.Exec(`
		UPDATE participants
		SET display_name = COALESCE(?, display_name),
		    role_prompt = COALESCE(?, role_prompt),
		    base_url = COALESCE(?, base_url),
		    model_name = COALESCE(?, model_name),
		    sort_order = COALESCE(?, sort_order)
		WHERE id = ?`,
		trimmedPtrArg(input.DisplayName), trimmedPtrArg(input.RolePrompt), trimmedPtrArg(input.BaseURL),
		trimmedPtrArg(input.ModelName), ptrArg(input.SortOrder), input.ParticipantID)
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
	return r.GetParticipant(input.ParticipantID)
}

// RemoveParticipant takes the participant off the roster by stamping
// deleted_at, and returns the updated row (or nil if it does not exist). The
// row survives so past messages keep resolving to a name and round_robin keeps
// a cycle position to advance from (design §3). An already-removed participant
// keeps its original deleted_at, so a repeated call is a no-op.
func (r *ParticipantRepository) RemoveParticipant(participantID string) (*model.Participant, error) {
	if _, err := r.db.Exec("UPDATE participants SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL", util.NowISO(), participantID); err != nil {
		return nil, err
	}
	return r.GetParticipant(participantID)
}
