package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"snzstudio/internal/model"
	"snzstudio/internal/search"
	"snzstudio/internal/util"
)

// MemoryRepository ports backend/src/repositories/memoryRepository.ts.
type MemoryRepository struct {
	db *sql.DB
}

// NewMemoryRepository constructs a MemoryRepository over the shared DB.
func NewMemoryRepository(db *sql.DB) *MemoryRepository {
	return &MemoryRepository{db: db}
}

const memoryColumns = `id, project_id, kind, title, content, source_chat_id, created_at, updated_at, source, locked, shared_with_all`

func scanMemory(s scanner) (model.Memory, error) {
	var (
		m             model.Memory
		sourceChat    sql.NullString
		source        sql.NullString
		locked        int64
		sharedWithAll int64
	)
	if err := s.Scan(&m.ID, &m.ProjectID, &m.Kind, &m.Title, &m.Content, &sourceChat, &m.CreatedAt, &m.UpdatedAt, &source, &locked, &sharedWithAll); err != nil {
		return m, err
	}
	m.SharedWithAll = sharedWithAll != 0
	m.SourceChatID = strPtr(sourceChat)
	if source.Valid && source.String != "" {
		m.Source = source.String
	} else {
		m.Source = "manual"
	}
	m.Locked = locked != 0
	return m, nil
}

// GetMemory returns the memory, or (nil, nil) if it does not exist.
func (r *MemoryRepository) GetMemory(memoryID string) (*model.Memory, error) {
	m, err := scanMemory(r.db.QueryRow("SELECT "+memoryColumns+" FROM memories WHERE id = ?", memoryID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListByProject returns a project's memories, locked first then most recent.
func (r *MemoryRepository) ListByProject(projectID string) ([]model.Memory, error) {
	return r.queryMemories("SELECT "+memoryColumns+" FROM memories WHERE project_id = ? ORDER BY locked DESC, updated_at DESC, created_at DESC", projectID)
}

// ListByProjectAndKind returns a project's memories of a given kind.
func (r *MemoryRepository) ListByProjectAndKind(projectID, kind string) ([]model.Memory, error) {
	return r.queryMemories("SELECT "+memoryColumns+" FROM memories WHERE project_id = ? AND kind = ? ORDER BY locked DESC, updated_at DESC", projectID, kind)
}

func (r *MemoryRepository) queryMemories(query string, args ...any) ([]model.Memory, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	memories := []model.Memory{}
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

// CreateMemoryInput carries the fields for CreateMemory. Source defaults to
// "manual" when empty.
//
// There is deliberately no SharedWithAll field: whether a new memory is common
// project material follows from Source (SharedWithAllDefault), and the human
// changes it afterwards from the memory list.
type CreateMemoryInput struct {
	ProjectID    string
	Kind         string
	Title        string
	Content      string
	SourceChatID *string
	Source       string
	Locked       bool
}

// SharedWithAllDefault reports whether a memory of this source starts in the
// common project material (design §4.4). Only "multi_agent" does: it was saved
// from an utterance of the multi-agent conversation (TASK-19), which every
// participant heard, so withholding it from a speaker hides nothing. Every other
// source — a manually added memory, one extracted from a single-assistant chat,
// one the organizer rewrote out of several — was never spoken in the
// conversation, so it stays closed until the human says otherwise.
func SharedWithAllDefault(source string) bool {
	return source == "multi_agent"
}

// CreateMemory inserts a memory and its pre-tokenized FTS row. Mirrors createMemory.
func (r *MemoryRepository) CreateMemory(input CreateMemoryInput) (model.Memory, error) {
	source := input.Source
	if source == "" {
		source = "manual"
	}
	now := util.NowISO()
	m := model.Memory{
		ID:            util.NewID("memory"),
		ProjectID:     input.ProjectID,
		Kind:          input.Kind,
		Title:         strings.TrimSpace(input.Title),
		Content:       strings.TrimSpace(input.Content),
		SourceChatID:  input.SourceChatID,
		Source:        source,
		Locked:        input.Locked,
		SharedWithAll: SharedWithAllDefault(source),
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	tx, err := r.db.Begin()
	if err != nil {
		return model.Memory{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		INSERT INTO memories (id, project_id, kind, title, content, source_chat_id, source, locked, shared_with_all, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ProjectID, m.Kind, m.Title, m.Content, ptrArg(m.SourceChatID), m.Source, boolToInt(m.Locked),
		boolToInt(m.SharedWithAll), m.CreatedAt, m.UpdatedAt); err != nil {
		return model.Memory{}, err
	}
	if _, err := tx.Exec(
		"INSERT INTO memories_fts (project_id, memory_id, kind, title, content) VALUES (?, ?, ?, ?, ?)",
		m.ProjectID, m.ID, search.BuildSearchText(m.Kind), search.BuildSearchText(m.Title), search.BuildSearchText(m.Content)); err != nil {
		return model.Memory{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Memory{}, err
	}
	return m, nil
}

// UpdateMemoryInput carries the fields for UpdateMemory. Locked is optional: nil
// keeps the current value (COALESCE). There is no SharedWithAll field because
// UpdateMemory always clears it — see below.
type UpdateMemoryInput struct {
	MemoryID string
	Kind     string
	Title    string
	Content  string
	Locked   *bool
}

// UpdateMemory updates a memory and replaces its FTS row, returning (nil, nil) if
// the memory does not exist. Mirrors updateMemory.
//
// It also takes the memory out of the common project material. Its only caller is
// the organizer, which rewrites a memory's content by folding other memories into
// it — including ones nobody shared. Keeping the flag would let an unlocked
// shared memory come back holding a withheld one's contents and still reach the
// speakers it was hiding from (design §4.4). Clearing it loses a sharing decision
// the human can restore with one click, where the leak cannot be taken back.
func (r *MemoryRepository) UpdateMemory(input UpdateMemoryInput) (*model.Memory, error) {
	var projectID string
	err := r.db.QueryRow("SELECT project_id FROM memories WHERE id = ?", input.MemoryID).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(input.Title)
	content := strings.TrimSpace(input.Content)
	var lockedArg any
	if input.Locked != nil {
		lockedArg = boolToInt(*input.Locked)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		UPDATE memories
		SET kind = ?, title = ?, content = ?, locked = COALESCE(?, locked), shared_with_all = 0, updated_at = ?
		WHERE id = ?`,
		input.Kind, title, content, lockedArg, util.NowISO(), input.MemoryID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec("DELETE FROM memories_fts WHERE memory_id = ?", input.MemoryID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		"INSERT INTO memories_fts (project_id, memory_id, kind, title, content) VALUES (?, ?, ?, ?, ?)",
		projectID, input.MemoryID, search.BuildSearchText(input.Kind), search.BuildSearchText(title), search.BuildSearchText(content)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetMemory(input.MemoryID)
}

// SetMemoryLocked toggles a memory's locked flag, returning (nil, nil) if the
// memory does not exist.
func (r *MemoryRepository) SetMemoryLocked(memoryID string, locked bool) (*model.Memory, error) {
	res, err := r.db.Exec("UPDATE memories SET locked = ?, updated_at = ? WHERE id = ?", boolToInt(locked), util.NowISO(), memoryID)
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
	return r.GetMemory(memoryID)
}

// SetMemorySharedWithAll puts a memory into the common project material or takes
// it out, returning (nil, nil) if the memory does not exist.
func (r *MemoryRepository) SetMemorySharedWithAll(memoryID string, sharedWithAll bool) (*model.Memory, error) {
	res, err := r.db.Exec("UPDATE memories SET shared_with_all = ?, updated_at = ? WHERE id = ?",
		boolToInt(sharedWithAll), util.NowISO(), memoryID)
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
	return r.GetMemory(memoryID)
}

// DeleteMemory removes a memory and its FTS row, returning the deleted record (or
// nil if it did not exist).
func (r *MemoryRepository) DeleteMemory(memoryID string) (*model.Memory, error) {
	m, err := r.GetMemory(memoryID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM memories_fts WHERE memory_id = ?", memoryID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec("DELETE FROM memories WHERE id = ?", memoryID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return m, nil
}

// HasSimilarMemory reports whether a memory with the same (case-insensitive)
// title and content already exists in the project. Comparison uses SQLite's
// lower(), matching the TS query (ASCII-only folding, which is a no-op for
// Japanese). Mirrors hasSimilarMemory.
func (r *MemoryRepository) HasSimilarMemory(projectID, title, content string) (bool, error) {
	var id string
	err := r.db.QueryRow(`
		SELECT id FROM memories
		WHERE project_id = ? AND lower(title) = lower(?) AND lower(content) = lower(?)
		LIMIT 1`,
		projectID, strings.TrimSpace(title), strings.TrimSpace(content)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MemoryForEmbedding is the subset of a memory used to compute embeddings.
type MemoryForEmbedding struct {
	ID        string
	ProjectID string
	Kind      string
	Title     string
	Content   string
}

// ListForEmbedding returns memories for embedding. When memoryIDs is non-empty,
// only those memories are returned; otherwise all are. Mirrors listForEmbedding.
func (r *MemoryRepository) ListForEmbedding(memoryIDs []string) ([]MemoryForEmbedding, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if len(memoryIDs) > 0 {
		args := make([]any, len(memoryIDs))
		for i, id := range memoryIDs {
			args[i] = id
		}
		rows, err = r.db.Query("SELECT id, project_id, kind, title, content FROM memories WHERE id IN ("+inPlaceholders(len(memoryIDs))+") ORDER BY created_at ASC", args...)
	} else {
		rows, err = r.db.Query("SELECT id, project_id, kind, title, content FROM memories ORDER BY created_at ASC")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []MemoryForEmbedding{}
	for rows.Next() {
		var m MemoryForEmbedding
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Kind, &m.Title, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MemoryEmbedding is a memory embedding to be persisted.
type MemoryEmbedding struct {
	MemoryID  string
	ProjectID string
	Kind      string
	Embedding []float64
	Model     string
}

// UpsertMemoryEmbeddings inserts or replaces memory embeddings. Mirrors
// upsertMemoryEmbeddings.
func (r *MemoryRepository) UpsertMemoryEmbeddings(rows []MemoryEmbedding) error {
	if len(rows) == 0 {
		return nil
	}
	updatedAt := util.NowISO()
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, row := range rows {
		embedding, err := json.Marshal(row.Embedding)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO memory_embeddings (memory_id, project_id, kind, embedding_json, model, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(memory_id) DO UPDATE SET
				project_id = excluded.project_id,
				kind = excluded.kind,
				embedding_json = excluded.embedding_json,
				model = excluded.model,
				updated_at = excluded.updated_at`,
			row.MemoryID, row.ProjectID, row.Kind, string(embedding), row.Model, updatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// HasEmbeddingsForModel reports whether any memory embedding is stored for the given
// model id. Paired with the document equivalent as the internal sidecar's run-once
// rebuild guard.
func (r *MemoryRepository) HasEmbeddingsForModel(model string) (bool, error) {
	var exists int
	if err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM memory_embeddings WHERE model = ?)`, model).Scan(&exists); err != nil {
		return false, err
	}
	return exists == 1, nil
}

// RebuildSearchIndex rebuilds memories_fts from scratch with current tokenization.
// Mirrors rebuildSearchIndex.
func (r *MemoryRepository) RebuildSearchIndex() error {
	type rec struct {
		projectID, id, kind, title, content string
	}
	rows, err := r.db.Query("SELECT project_id, id, kind, title, content FROM memories ORDER BY created_at ASC")
	if err != nil {
		return err
	}
	var recs []rec
	for rows.Next() {
		var x rec
		if err := rows.Scan(&x.projectID, &x.id, &x.kind, &x.title, &x.content); err != nil {
			rows.Close()
			return err
		}
		recs = append(recs, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM memories_fts"); err != nil {
		return err
	}
	for _, x := range recs {
		if _, err := tx.Exec(
			"INSERT INTO memories_fts (project_id, memory_id, kind, title, content) VALUES (?, ?, ?, ?, ?)",
			x.projectID, x.id, search.BuildSearchText(x.kind), search.BuildSearchText(x.title), search.BuildSearchText(x.content)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
