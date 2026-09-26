package repository

import (
	"database/sql"
	"errors"
	"strings"

	"snzstudio/internal/model"
	"snzstudio/internal/util"
)

// ErrProjectReorderMismatch is returned by ReorderProjects when the supplied IDs
// do not exactly match the set of existing project IDs. It mirrors the TS code
// path that returned null for that case so the HTTP layer can map it to 400.
var ErrProjectReorderMismatch = errors.New("repository: reorder ids do not match existing projects")

// ProjectRepository ports backend/src/repositories/projectRepository.ts.
type ProjectRepository struct {
	db *sql.DB
}

// NewProjectRepository constructs a ProjectRepository over the shared DB.
func NewProjectRepository(db *sql.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

// projectSelect joins chats to derive chat_count and last_activity_at, matching
// mapProject's columns. The timestamps are fixed-width UTC ISO strings, so the
// scalar MAX compares them in time order.
const projectSelect = `
	SELECT p.id, p.title, p.description, p.system_prompt, p.sort_order, p.created_at, p.updated_at, COUNT(c.id) AS chat_count,
		MAX(p.updated_at, COALESCE(MAX(c.updated_at), '')) AS last_activity_at
	FROM projects p
	LEFT JOIN chats c ON c.project_id = p.id`

func scanProject(s scanner) (model.Project, error) {
	var p model.Project
	err := s.Scan(&p.ID, &p.Title, &p.Description, &p.SystemPrompt, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt, &p.ChatCount, &p.LastActivityAt)
	return p, err
}

// ListProjects returns all projects ordered by sort_order then creation time.
func (r *ProjectRepository) ListProjects() ([]model.Project, error) {
	rows, err := r.db.Query(projectSelect + `
		GROUP BY p.id
		ORDER BY p.sort_order ASC, p.created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projects := []model.Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// GetProject returns the project, or (nil, nil) if it does not exist.
func (r *ProjectRepository) GetProject(projectID string) (*model.Project, error) {
	row := r.db.QueryRow(projectSelect+`
		WHERE p.id = ?
		GROUP BY p.id`, projectID)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreateProjectInput carries the fields for CreateProject.
type CreateProjectInput struct {
	Title        string
	Description  string
	SystemPrompt string
}

// CreateProject inserts a project, assigning the next sort_order (max+1).
func (r *ProjectRepository) CreateProject(input CreateProjectInput) (model.Project, error) {
	var maxSort sql.NullInt64
	if err := r.db.QueryRow("SELECT MAX(sort_order) FROM projects").Scan(&maxSort); err != nil {
		return model.Project{}, err
	}
	next := -1
	if maxSort.Valid {
		next = int(maxSort.Int64)
	}
	next++

	now := util.NowISO()
	p := model.Project{
		ID:             util.NewID("project"),
		Title:          strings.TrimSpace(input.Title),
		Description:    strings.TrimSpace(input.Description),
		SystemPrompt:   strings.TrimSpace(input.SystemPrompt),
		SortOrder:      next,
		ChatCount:      0,
		CreatedAt:      now,
		UpdatedAt:      now,
		LastActivityAt: now,
	}

	_, err := r.db.Exec(`
		INSERT INTO projects (id, title, description, system_prompt, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Title, p.Description, p.SystemPrompt, p.SortOrder, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return model.Project{}, err
	}
	return p, nil
}

// UpdateProjectTitle updates the title, returning (nil, nil) if the project does
// not exist.
func (r *ProjectRepository) UpdateProjectTitle(projectID, title string) (*model.Project, error) {
	return r.updateField(projectID, "title", strings.TrimSpace(title))
}

// UpdateProjectSystemPrompt updates the system prompt, returning (nil, nil) if
// the project does not exist.
func (r *ProjectRepository) UpdateProjectSystemPrompt(projectID, systemPrompt string) (*model.Project, error) {
	return r.updateField(projectID, "system_prompt", strings.TrimSpace(systemPrompt))
}

func (r *ProjectRepository) updateField(projectID, column, value string) (*model.Project, error) {
	res, err := r.db.Exec("UPDATE projects SET "+column+" = ?, updated_at = ? WHERE id = ?", value, util.NowISO(), projectID)
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
	return r.GetProject(projectID)
}

// DeleteProject deletes the project (chats/documents/memories cascade), returning
// whether a row was removed.
func (r *ProjectRepository) DeleteProject(projectID string) (bool, error) {
	res, err := r.db.Exec("DELETE FROM projects WHERE id = ?", projectID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ReorderProjects rewrites sort_order to match the order of projectIDs and
// returns the reordered list. It returns ErrProjectReorderMismatch if the IDs do
// not exactly cover the existing project set.
func (r *ProjectRepository) ReorderProjects(projectIDs []string) ([]model.Project, error) {
	existing, err := r.existingProjectIDs()
	if err != nil {
		return nil, err
	}
	if len(existing) != len(projectIDs) {
		return nil, ErrProjectReorderMismatch
	}
	for _, id := range projectIDs {
		if !existing[id] {
			return nil, ErrProjectReorderMismatch
		}
	}

	now := util.NowISO()
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for i, id := range projectIDs {
		if _, err := tx.Exec("UPDATE projects SET sort_order = ?, updated_at = ? WHERE id = ?", i, now, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.ListProjects()
}

func (r *ProjectRepository) existingProjectIDs() (map[string]bool, error) {
	rows, err := r.db.Query("SELECT id FROM projects")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}
