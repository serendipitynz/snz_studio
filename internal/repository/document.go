package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"snzstudio/internal/chunk"
	"snzstudio/internal/doccategory"
	"snzstudio/internal/model"
	"snzstudio/internal/search"
	"snzstudio/internal/util"
)

// DocumentRepository ports backend/src/repositories/documentRepository.ts.
type DocumentRepository struct {
	db *sql.DB
}

// NewDocumentRepository constructs a DocumentRepository over the shared DB.
func NewDocumentRepository(db *sql.DB) *DocumentRepository {
	return &DocumentRepository{db: db}
}

const documentColumns = `id, project_id, type, category, title, note, tags_json, derived_text, content_text, file_path, mime_type, created_at, updated_at`

func scanDocument(s scanner) (model.DocumentRecord, error) {
	var (
		d        model.DocumentRecord
		category sql.NullString
		tagsJSON string
		filePath sql.NullString
		mimeType sql.NullString
	)
	if err := s.Scan(&d.ID, &d.ProjectID, &d.Type, &category, &d.Title, &d.Note, &tagsJSON,
		&d.DerivedText, &d.ContentText, &filePath, &mimeType, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return d, err
	}
	cat := "misc"
	if category.Valid {
		cat = category.String
	}
	if !doccategory.IsValid(cat) {
		cat = "misc"
	}
	d.Category = cat
	d.Tags = decodeTags(tagsJSON)
	d.FilePath = strPtr(filePath)
	d.MimeType = strPtr(mimeType)
	return d, nil
}

// decodeTags mirrors safeJsonParse<string[]>(tags_json, []): invalid or empty
// JSON yields an empty (non-nil) slice.
func decodeTags(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil || tags == nil {
		return []string{}
	}
	return tags
}

// ListByProject returns a project's documents, newest first.
func (r *DocumentRepository) ListByProject(projectID string) ([]model.DocumentRecord, error) {
	rows, err := r.db.Query("SELECT "+documentColumns+" FROM documents WHERE project_id = ? ORDER BY created_at DESC", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	docs := []model.DocumentRecord{}
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// GetDocument returns the document, or (nil, nil) if it does not exist.
func (r *DocumentRepository) GetDocument(documentID string) (*model.DocumentRecord, error) {
	d, err := scanDocument(r.db.QueryRow("SELECT "+documentColumns+" FROM documents WHERE id = ?", documentID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// CreateDocumentInput carries the fields for CreateDocument. Category may be
// empty, in which case it is inferred. Tags is used as-is (the comma-string form
// is parsed by callers via util.ParseTags).
type CreateDocumentInput struct {
	ProjectID   string
	Type        string
	Category    string
	Title       string
	Note        string
	Tags        []string
	DerivedText string
	ContentText string
	FilePath    *string
	MimeType    *string
}

// CreateDocument inserts a document together with its chunk rows and pre-tokenized
// FTS rows, all in one transaction. Mirrors createDocument.
func (r *DocumentRepository) CreateDocument(input CreateDocumentInput) (model.DocumentRecord, error) {
	createdAt := util.NowISO()

	category := input.Category
	if category == "" {
		category = doccategory.Infer(doccategory.Input{
			FileName:    ptrString(input.FilePath),
			Title:       input.Title,
			Note:        input.Note,
			ContentText: input.ContentText,
			DerivedText: input.DerivedText,
		})
	}

	tags := input.Tags
	if tags == nil {
		tags = []string{}
	}

	doc := model.DocumentRecord{
		ID:          util.NewID("doc"),
		ProjectID:   input.ProjectID,
		Type:        input.Type,
		Category:    category,
		Title:       strings.TrimSpace(input.Title),
		Note:        strings.TrimSpace(input.Note),
		Tags:        tags,
		DerivedText: strings.TrimSpace(input.DerivedText),
		ContentText: strings.TrimSpace(input.ContentText),
		FilePath:    input.FilePath,
		MimeType:    input.MimeType,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}

	tagsJSON, err := json.Marshal(doc.Tags)
	if err != nil {
		return model.DocumentRecord{}, err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return model.DocumentRecord{}, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO documents (
			id, project_id, type, category, title, note, tags_json, derived_text, content_text, file_path, mime_type, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		doc.ID, doc.ProjectID, doc.Type, doc.Category, doc.Title, doc.Note, string(tagsJSON),
		doc.DerivedText, doc.ContentText, ptrArg(doc.FilePath), ptrArg(doc.MimeType), doc.CreatedAt, doc.UpdatedAt); err != nil {
		return model.DocumentRecord{}, err
	}

	if err := deleteChunks(tx, doc.ID); err != nil {
		return model.DocumentRecord{}, err
	}

	// Combined search body: note the ", " join here vs the " " join for the FTS
	// tags column below — the two separators differ in the TS source.
	var bodyParts []string
	for _, s := range []string{doc.ContentText, doc.DerivedText, doc.Note, strings.Join(doc.Tags, ", ")} {
		if s != "" {
			bodyParts = append(bodyParts, s)
		}
	}
	chunks := chunk.Document(strings.Join(bodyParts, "\n\n"))

	ftsTitle := search.BuildSearchText(doc.Title)
	ftsNote := search.BuildSearchText(doc.Note)
	ftsTags := search.BuildSearchText(strings.Join(doc.Tags, " "))
	ftsDerived := search.BuildSearchText(doc.DerivedText)

	insertChunk := func(index int, content, ftsContent string) error {
		chunkID := util.NewID("chunk")
		if _, err := tx.Exec(`
			INSERT INTO document_chunks (id, document_id, project_id, chunk_index, content, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			chunkID, doc.ID, doc.ProjectID, index, content, createdAt); err != nil {
			return err
		}
		_, err := tx.Exec(`
			INSERT INTO document_chunks_fts (project_id, document_id, chunk_id, title, note, tags, derived_text, content)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			doc.ProjectID, doc.ID, chunkID, ftsTitle, ftsNote, ftsTags, ftsDerived, ftsContent)
		return err
	}

	if len(chunks) == 0 {
		// A single empty chunk keeps the document searchable on its metadata.
		if err := insertChunk(0, "", ""); err != nil {
			return model.DocumentRecord{}, err
		}
	} else {
		for index, c := range chunks {
			if err := insertChunk(index, c, search.BuildSearchText(c)); err != nil {
				return model.DocumentRecord{}, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return model.DocumentRecord{}, err
	}
	return doc, nil
}

// UpdateDocumentCategory sets a document's category, returning (nil, nil) if the
// document does not exist.
func (r *DocumentRepository) UpdateDocumentCategory(documentID, category string) (*model.DocumentRecord, error) {
	existing, err := r.GetDocument(documentID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}
	if _, err := r.db.Exec("UPDATE documents SET category = ?, updated_at = ? WHERE id = ?", category, util.NowISO(), documentID); err != nil {
		return nil, err
	}
	return r.GetDocument(documentID)
}

// DeleteDocument removes a document and its chunks/FTS rows, returning the
// deleted record (or nil if it did not exist).
func (r *DocumentRepository) DeleteDocument(documentID string) (*model.DocumentRecord, error) {
	doc, err := r.GetDocument(documentID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := deleteChunks(tx, documentID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec("DELETE FROM documents WHERE id = ?", documentID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return doc, nil
}

func deleteChunks(q dbtx, documentID string) error {
	if _, err := q.Exec("DELETE FROM document_chunks WHERE document_id = ?", documentID); err != nil {
		return err
	}
	_, err := q.Exec("DELETE FROM document_chunks_fts WHERE document_id = ?", documentID)
	return err
}

// ChunkForEmbedding is a document chunk joined with its document's metadata, used
// to compute embeddings. Mirrors the row shape returned by listChunksForEmbedding.
type ChunkForEmbedding struct {
	ChunkID     string
	DocumentID  string
	ProjectID   string
	ChunkIndex  int
	Content     string
	Title       string
	Note        string
	Tags        []string
	DerivedText string
}

// ListChunksForEmbedding returns chunks (with document metadata) for embedding.
// When documentID is empty, all chunks are returned, ordered by document then
// chunk index. Mirrors listChunksForEmbedding(documentId?).
func (r *DocumentRepository) ListChunksForEmbedding(documentID string) ([]ChunkForEmbedding, error) {
	const cols = `c.id, c.document_id, c.project_id, c.chunk_index, c.content, d.title, d.note, d.tags_json, d.derived_text`
	var (
		rows *sql.Rows
		err  error
	)
	if documentID != "" {
		rows, err = r.db.Query(`
			SELECT `+cols+`
			FROM document_chunks c
			JOIN documents d ON d.id = c.document_id
			WHERE c.document_id = ?
			ORDER BY c.chunk_index ASC`, documentID)
	} else {
		rows, err = r.db.Query(`
			SELECT ` + cols + `
			FROM document_chunks c
			JOIN documents d ON d.id = c.document_id
			ORDER BY d.created_at ASC, c.chunk_index ASC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ChunkForEmbedding{}
	for rows.Next() {
		var (
			ch       ChunkForEmbedding
			tagsJSON string
		)
		if err := rows.Scan(&ch.ChunkID, &ch.DocumentID, &ch.ProjectID, &ch.ChunkIndex, &ch.Content, &ch.Title, &ch.Note, &tagsJSON, &ch.DerivedText); err != nil {
			return nil, err
		}
		ch.Tags = decodeTags(tagsJSON)
		out = append(out, ch)
	}
	return out, rows.Err()
}

// ChunkEmbedding is a chunk embedding to be persisted.
type ChunkEmbedding struct {
	ChunkID    string
	DocumentID string
	ProjectID  string
	Embedding  []float64
	Model      string
}

// UpsertChunkEmbeddings inserts or replaces chunk embeddings. Mirrors
// upsertChunkEmbeddings.
func (r *DocumentRepository) UpsertChunkEmbeddings(rows []ChunkEmbedding) error {
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
			INSERT INTO document_chunk_embeddings (chunk_id, document_id, project_id, embedding_json, model, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(chunk_id) DO UPDATE SET
				document_id = excluded.document_id,
				project_id = excluded.project_id,
				embedding_json = excluded.embedding_json,
				model = excluded.model,
				updated_at = excluded.updated_at`,
			row.ChunkID, row.DocumentID, row.ProjectID, string(embedding), row.Model, updatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RebuildSearchIndex rebuilds document_chunks_fts from scratch, re-tokenizing all
// text with the current tokenizer. Mirrors rebuildSearchIndex.
func (r *DocumentRepository) RebuildSearchIndex() error {
	type rec struct {
		docID, projectID, title, note, tagsJSON, derivedText, chunkID, content string
	}
	rows, err := r.db.Query(`
		SELECT d.id, d.project_id, d.title, d.note, d.tags_json, d.derived_text, c.id, c.content
		FROM documents d
		JOIN document_chunks c ON c.document_id = d.id
		ORDER BY d.created_at ASC, c.chunk_index ASC`)
	if err != nil {
		return err
	}
	var recs []rec
	for rows.Next() {
		var x rec
		if err := rows.Scan(&x.docID, &x.projectID, &x.title, &x.note, &x.tagsJSON, &x.derivedText, &x.chunkID, &x.content); err != nil {
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
	if _, err := tx.Exec("DELETE FROM document_chunks_fts"); err != nil {
		return err
	}
	for _, x := range recs {
		tags := decodeTags(x.tagsJSON)
		if _, err := tx.Exec(`
			INSERT INTO document_chunks_fts (project_id, document_id, chunk_id, title, note, tags, derived_text, content)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			x.projectID, x.docID, x.chunkID,
			search.BuildSearchText(x.title),
			search.BuildSearchText(x.note),
			search.BuildSearchText(strings.Join(tags, " ")),
			search.BuildSearchText(x.derivedText),
			search.BuildSearchText(x.content)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// BackfillInferredCategories re-classifies documents still left at the default
// "misc" category that have never been edited (updated_at == created_at).
// Mirrors backfillInferredCategories.
func (r *DocumentRepository) BackfillInferredCategories() error {
	type rec struct {
		id, title, note, derivedText, contentText string
		filePath                                  sql.NullString
	}
	rows, err := r.db.Query(`
		SELECT id, title, note, derived_text, content_text, file_path
		FROM documents
		WHERE category = 'misc' AND updated_at = created_at`)
	if err != nil {
		return err
	}
	var recs []rec
	for rows.Next() {
		var x rec
		if err := rows.Scan(&x.id, &x.title, &x.note, &x.derivedText, &x.contentText, &x.filePath); err != nil {
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
	if len(recs) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, x := range recs {
		// fileName defaults to the title when file_path is absent, matching the TS.
		fileName := x.filePath.String
		if fileName == "" {
			fileName = x.title
		}
		inferred := doccategory.Infer(doccategory.Input{
			FileName:    fileName,
			Title:       x.title,
			Note:        x.note,
			ContentText: x.contentText,
			DerivedText: x.derivedText,
		})
		if inferred == "misc" {
			continue
		}
		if _, err := tx.Exec("UPDATE documents SET category = ?, updated_at = ? WHERE id = ?", inferred, util.NowISO(), x.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
