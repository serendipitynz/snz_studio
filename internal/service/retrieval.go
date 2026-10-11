package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"snzstudio/internal/model"
	"snzstudio/internal/search"
	"snzstudio/internal/util"
	"snzstudio/internal/vector"
)

// RetrievalService ports retrievalService.ts: hybrid (BM25 + semantic) retrieval
// over document chunks and memories. Cosine similarity is computed in Go from the
// JSON-encoded embeddings (internal/vector), never in SQL — matching the TS
// implementation. It holds the *sql.DB directly, as the TS service held the
// better-sqlite3 handle; every read fully drains and closes its rows before the
// next query, honouring the single-connection discipline.
type RetrievalService struct {
	db         *sql.DB
	embeddings *EmbeddingClient
}

// NewRetrievalService builds a RetrievalService.
func NewRetrievalService(db *sql.DB, embeddings *EmbeddingClient) *RetrievalService {
	return &RetrievalService{db: db, embeddings: embeddings}
}

// MaterialScope selects which of a project's documents and memories a search may
// return. ScopeSharedWithAll is the common project material of a multi-agent turn
// whose speaker does not receive the project material (design §4.4).
type MaterialScope int

const (
	ScopeAll MaterialScope = iota
	ScopeSharedWithAll
)

// sharedWithAllClause narrows a query to the common project material. It goes
// into the SQL rather than filtering the results afterwards: a turn's budget is
// small enough (2 documents, 3 memories) that rows the speaker may not see would
// otherwise eat the whole selection and leave it with nothing.
func (scope MaterialScope) sharedWithAllClause(alias string) string {
	if scope != ScopeSharedWithAll {
		return ""
	}
	return " AND " + alias + ".shared_with_all = 1"
}

func clampScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// semanticToUnitRange maps a cosine similarity in [-1, 1] to [0, 1]. Mirrors
// semanticToUnitRange.
func semanticToUnitRange(value float64) float64 {
	return clampScore((value + 1) / 2)
}

// semanticOnlyFloor is the minimum score an embedding candidate that FTS did not
// find needs to join the candidates (documentSemanticCandidates,
// memorySemanticCandidates). FTS matches need no floor: the shared terms already
// tie them to the query. ruri (ModernBERT-Ja) cosines cluster high — even
// unrelated Japanese text scores ~0.78, mapping to ~0.89 unit — so the default
// floor calibrated for wider-spread embeddings would admit almost everything.
// Use a higher floor for ruri.
//
// PROVISIONAL: the ruri floor is derived from limited spike data (S2). Calibrate it
// with more labeled negatives during GUI E2E before relying on it.
func semanticOnlyFloor(defaultFloor float64, ruri bool) float64 {
	if ruri {
		return 0.9
	}
	return defaultFloor
}

// Retrieval intent detection regexes, ported verbatim from retrievalService.ts.
var (
	reIntentTranslation = regexp.MustCompile(`(?i)(翻訳|訳して|英訳|和訳|訳文|用語統一)`)
	reIntentPlot        = regexp.MustCompile(`(?i)(プロット|構想|展開案|次どう|章構成|起きること|流れ)`)
	reIntentWriting     = regexp.MustCompile(`(?i)(続き|本文|執筆|書いて|書き直|推敲|場面|シーン|会話文|地の文|文体)`)
	reIntentReference   = regexp.MustCompile(`(?i)(設定|世界観|ルール|正史|索引|時系列|人物|キャラ|用語|年表|整合|矛盾|確認)`)
)

// detectRetrievalIntent mirrors detectRetrievalIntent. Order matters: translation,
// then plot, then writing, then reference, else general.
func detectRetrievalIntent(query string) string {
	switch {
	case reIntentTranslation.MatchString(query):
		return "translation"
	case reIntentPlot.MatchString(query):
		return "plot"
	case reIntentWriting.MatchString(query):
		return "writing"
	case reIntentReference.MatchString(query):
		return "reference"
	default:
		return "general"
	}
}

// categoryWeights ports the getCategoryWeight tables. Missing entries default to 1.
var categoryWeights = map[string]map[string]float64{
	"general": {
		"world": 1.05, "character": 1.05, "rule": 1.05, "plot": 1,
		"timeline": 1.05, "index": 1.05, "story": 1, "misc": 1,
	},
	"reference": {
		"world": 1.2, "character": 1.15, "rule": 1.15, "plot": 0.9,
		"timeline": 1.15, "index": 1.25, "story": 0.8, "misc": 1,
	},
	"writing": {
		"world": 1, "character": 1.15, "rule": 1.25, "plot": 1.05,
		"timeline": 1.1, "index": 0.95, "story": 1.2, "misc": 1,
	},
	"plot": {
		"world": 1, "character": 1.1, "rule": 1, "plot": 1.25,
		"timeline": 1.15, "index": 1, "story": 0.95, "misc": 1,
	},
	"translation": {
		"world": 1.05, "character": 1.05, "rule": 1.25, "plot": 0.9,
		"timeline": 0.95, "index": 1.1, "story": 1.2, "misc": 1,
	},
}

func getCategoryWeight(category, intent string) float64 {
	table, ok := categoryWeights[intent]
	if !ok {
		return 1
	}
	if weight, ok := table[category]; ok {
		return weight
	}
	return 1
}

func parseEmbeddingJSON(raw string) []float64 {
	var v []float64
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil
	}
	return v
}

type documentCandidateRow struct {
	sourceID     string
	label        string
	category     string
	chunkContent string
	chunkID      string
	chunkIndex   int
	note         string
	derivedText  string
	score        float64
}

type memoryCandidateRow struct {
	sourceID string
	label    string
	excerpt  string
	score    float64
}

// SearchQuery is what one search looks for. Text is the request, and its FTS
// terms take the slots first. Background is context that should inform the search
// without crowding Text out — a multi-agent turn's scene (design §4.4): its FTS
// terms only fill the slots Text leaves open. The embedding reads both, Text
// first, and so does intent detection.
type SearchQuery struct {
	Text       string
	Background string
}

func (q SearchQuery) fullText() string {
	switch {
	case q.Background == "":
		return q.Text
	case q.Text == "":
		return q.Background
	default:
		return q.Text + "\n" + q.Background
	}
}

// ftsVocab names an FTS table's fts5vocab twin and the plain table with one row
// per FTS row, which counts them without scanning the index.
type ftsVocab struct {
	vocabTable string
	rowsTable  string
}

var (
	documentVocab = ftsVocab{vocabTable: "document_chunks_fts_vocab", rowsTable: "document_chunks"}
	memoryVocab   = ftsVocab{vocabTable: "memories_fts_vocab", rowsTable: "memories"}
)

// buildFtsQuery picks the query's FTS terms by how few rows of the table hold
// them (search.SelectTerms). The counts cover every project, not just the one
// searched: fts5vocab has no project column, and counting per project with a
// MATCH per term took 760 ms for a 218-term paste over 10,600 chunks, against
// 30 ms for the vocabulary. A term's count is the sum over the index terms it
// prefixes — the query prefix-matches — which overcounts a row holding two of
// them; the cap at the row total keeps that from mattering beyond ranking a broad
// prefix (「時」 for 時間, 時代, …) as common, which it is.
func (r *RetrievalService) buildFtsQuery(vocab ftsVocab, query SearchQuery) (string, error) {
	primary := search.QueryTerms(query.Text)
	background := search.QueryTerms(query.Background)
	if len(primary) == 0 && len(background) == 0 {
		return "", nil
	}
	var total int
	if err := r.db.QueryRow(`SELECT count(*) FROM ` + vocab.rowsTable).Scan(&total); err != nil {
		return "", err
	}
	freqs := make(map[string]int, len(primary)+len(background))
	for _, term := range append(append([]string{}, primary...), background...) {
		if _, done := freqs[term]; done {
			continue
		}
		var freq int
		if err := r.db.QueryRow(
			`SELECT coalesce(sum(doc), 0) FROM `+vocab.vocabTable+` WHERE term >= ? AND term < ?`,
			term, term+"\U0010FFFF",
		).Scan(&freq); err != nil {
			return "", err
		}
		freqs[term] = min(freq, total)
	}
	selected := search.SelectTerms(primary, background, func(term string) int { return freqs[term] }, total)
	return search.FtsQuery(selected), nil
}

// SearchDocuments mirrors searchDocuments(projectId, query, limit=4, chunksPerDocument=3),
// narrowed to scope. With embeddings on, the candidates are the FTS matches plus
// the chunks closest to the query that FTS did not find, ranked together; without
// them, the FTS matches alone.
func (r *RetrievalService) SearchDocuments(projectID string, query SearchQuery, limit, chunksPerDocument int, scope MaterialScope) ([]model.RetrievedDocumentReference, error) {
	ftsQuery, err := r.buildFtsQuery(documentVocab, query)
	if err != nil {
		return nil, err
	}
	text := query.fullText()
	intent := detectRetrievalIntent(text)

	var ftsRows []documentCandidateRow
	if ftsQuery != "" {
		ftsRows, err = r.fetchDocumentFtsCandidates(projectID, ftsQuery, limit, chunksPerDocument, scope)
		if err != nil {
			return nil, err
		}
	}

	// The query gets the ruri query prefix (no-op for other models); the FTS query
	// above stays raw.
	queryEmbedding := r.embeddings.CreateEmbedding(r.embeddings.ActivePrefixScheme().Query + text)

	var (
		semanticRows   []documentCandidateRow
		semanticScores map[string]float64
	)
	if queryEmbedding != nil {
		semanticScores, semanticRows, err = r.documentSemanticCandidates(projectID, queryEmbedding, ftsRows, limit*chunksPerDocument, intent, scope)
		if err != nil {
			return nil, err
		}
	}

	ranked := rankDocumentCandidates(ftsRows, semanticRows, semanticScores, chunksPerDocument, intent)
	return r.attachFullDocumentContent(sliceDocRefs(ranked, limit))
}

// attachFullDocumentContent reads the full text of the adopted documents only. The
// candidate queries leave it out because they return one row per chunk: selecting
// it there copied a large document once for every chunk that matched, which for a
// 1MB manuscript meant hundreds of megabytes per search.
func (r *RetrievalService) attachFullDocumentContent(refs []model.RetrievedDocumentReference) ([]model.RetrievedDocumentReference, error) {
	if len(refs) == 0 {
		return refs, nil
	}
	args := make([]any, len(refs))
	for i, ref := range refs {
		args[i] = ref.SourceID
	}
	rows, err := r.db.Query(`SELECT id, content_text FROM documents WHERE id IN (`+placeholders(len(refs))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	contentByID := make(map[string]string, len(refs))
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			return nil, err
		}
		contentByID[id] = content
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range refs {
		refs[i].FullDocumentContent = contentByID[refs[i].SourceID]
	}
	return refs, nil
}

func sliceDocRefs(refs []model.RetrievedDocumentReference, limit int) []model.RetrievedDocumentReference {
	if len(refs) > limit {
		return refs[:limit]
	}
	return refs
}

// SearchChunksInDocument mirrors searchChunksInDocument(documentId, query, limit=5).
func (r *RetrievalService) SearchChunksInDocument(documentID, query string, limit int) ([]model.RetrievedDocumentChunk, error) {
	ftsQuery, err := r.buildFtsQuery(documentVocab, SearchQuery{Text: query})
	if err != nil {
		return nil, err
	}

	if ftsQuery == "" {
		rows, err := r.db.Query(`
			SELECT id AS chunk_id, chunk_index, content
			FROM document_chunks
			WHERE document_id = ?
			ORDER BY chunk_index ASC
			LIMIT ?`, documentID, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []model.RetrievedDocumentChunk{}
		for rows.Next() {
			var c model.RetrievedDocumentChunk
			if err := rows.Scan(&c.ChunkID, &c.ChunkIndex, &c.Content); err != nil {
				return nil, err
			}
			c.Score = 0
			out = append(out, c)
		}
		return out, rows.Err()
	}

	// bm25() weights every FTS5 column by position, UNINDEXED ones included, so the
	// id columns take a 0.0 each before the title/note/tags/derived_text/content weights.
	rows, err := r.db.Query(`
		SELECT
			c.id AS chunk_id,
			c.chunk_index AS chunk_index,
			c.content AS content,
			bm25(document_chunks_fts, 0.0, 0.0, 0.0, 10.0, 2.0, 1.0, 1.0, 4.0) * -1 AS score
		FROM document_chunks_fts
		JOIN document_chunks c ON c.id = document_chunks_fts.chunk_id
		WHERE document_chunks_fts.document_id = ? AND document_chunks_fts MATCH ?
		ORDER BY score DESC, c.chunk_index ASC
		LIMIT ?`, documentID, ftsQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RetrievedDocumentChunk{}
	for rows.Next() {
		var c model.RetrievedDocumentChunk
		if err := rows.Scan(&c.ChunkID, &c.ChunkIndex, &c.Content, &c.Score); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SearchMemories mirrors searchMemories(projectId, query, limit=4), narrowed to
// scope. Candidates are gathered as in SearchDocuments.
func (r *RetrievalService) SearchMemories(projectID string, query SearchQuery, limit int, scope MaterialScope) ([]model.SearchReference, error) {
	ftsQuery, err := r.buildFtsQuery(memoryVocab, query)
	if err != nil {
		return nil, err
	}

	var ftsRows []memoryCandidateRow
	if ftsQuery != "" {
		candidateLimit := limit * 3
		if candidateLimit < 8 {
			candidateLimit = 8
		}
		ftsRows, err = r.fetchMemoryFtsCandidates(projectID, ftsQuery, candidateLimit, scope)
		if err != nil {
			return nil, err
		}
	}

	queryEmbedding := r.embeddings.CreateEmbedding(r.embeddings.ActivePrefixScheme().Query + query.fullText())

	var (
		semanticRows   []memoryCandidateRow
		semanticScores map[string]float64
	)
	if queryEmbedding != nil {
		semanticScores, semanticRows, err = r.memorySemanticCandidates(projectID, queryEmbedding, ftsRows, limit, scope)
		if err != nil {
			return nil, err
		}
	}

	return sliceRefs(rankMemoryCandidates(ftsRows, semanticRows, semanticScores), limit), nil
}

func sliceRefs(refs []model.SearchReference, limit int) []model.SearchReference {
	if len(refs) > limit {
		return refs[:limit]
	}
	return refs
}

func (r *RetrievalService) fetchDocumentFtsCandidates(projectID, ftsQuery string, limit, chunksPerDocument int, scope MaterialScope) ([]documentCandidateRow, error) {
	candidateLimit := limit * chunksPerDocument * 4
	if candidateLimit < 12 {
		candidateLimit = 12
	}
	// bm25() weights every FTS5 column by position, UNINDEXED ones included, so the
	// id columns take a 0.0 each before the title/note/tags/derived_text/content weights.
	rows, err := r.db.Query(`
		SELECT
			d.id AS source_id,
			d.title AS label,
			d.category AS category,
			c.content AS chunk_content,
			c.id AS chunk_id,
			c.chunk_index AS chunk_index,
			d.note AS note,
			d.derived_text AS derived_text,
			bm25(document_chunks_fts, 0.0, 0.0, 0.0, 10.0, 2.0, 1.0, 1.0, 4.0) * -1 AS score
		FROM document_chunks_fts
		JOIN documents d ON d.id = document_chunks_fts.document_id
		JOIN document_chunks c ON c.id = document_chunks_fts.chunk_id
		WHERE document_chunks_fts.project_id = ? AND document_chunks_fts MATCH ?`+
		scope.sharedWithAllClause("d")+`
		ORDER BY score DESC
		LIMIT ?`, projectID, ftsQuery, candidateLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []documentCandidateRow{}
	for rows.Next() {
		var row documentCandidateRow
		if err := rows.Scan(&row.sourceID, &row.label, &row.category, &row.chunkContent, &row.chunkID, &row.chunkIndex, &row.note, &row.derivedText, &row.score); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *RetrievalService) fetchMemoryFtsCandidates(projectID, ftsQuery string, limit int, scope MaterialScope) ([]memoryCandidateRow, error) {
	// bm25() weights every FTS5 column by position, UNINDEXED ones included, so the
	// id columns take a 0.0 each before the kind/title/content weights.
	rows, err := r.db.Query(`
		SELECT
			m.id AS source_id,
			m.title AS label,
			m.content AS excerpt,
			bm25(memories_fts, 0.0, 0.0, 2.0, 6.0, 8.0) * -1 AS score
		FROM memories_fts
		JOIN memories m ON m.id = memories_fts.memory_id
		WHERE memories_fts.project_id = ? AND memories_fts MATCH ?`+
		scope.sharedWithAllClause("m")+`
		ORDER BY score DESC
		LIMIT ?`, projectID, ftsQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []memoryCandidateRow{}
	for rows.Next() {
		var row memoryCandidateRow
		if err := rows.Scan(&row.sourceID, &row.label, &row.excerpt, &row.score); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// docGroup accumulates the chunks of a single document during ranking.
type docGroup struct {
	label        string
	category     string
	fallbackText string
	chunks       []model.RetrievedDocumentChunk
	bestScore    float64
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, 0, n*3-2)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',', ' ')
		}
		b = append(b, '?')
	}
	return string(b)
}

// rankDocumentCandidates scores each chunk and groups the chunks by document. The
// FTS score is the bm25 normalized over the FTS rows; a semantic row, which FTS
// did not match, scores 0 there. semanticScores is nil when embeddings are off,
// and the FTS score is then the whole score.
func rankDocumentCandidates(ftsRows, semanticRows []documentCandidateRow, semanticScores map[string]float64, chunksPerDocument int, intent string) []model.RetrievedDocumentReference {
	rawScores := make([]float64, len(ftsRows))
	for i, row := range ftsRows {
		rawScores[i] = row.score
	}
	ftsScores := append(vector.NormalizeScores(rawScores), make([]float64, len(semanticRows))...)
	rows := append(append([]documentCandidateRow{}, ftsRows...), semanticRows...)

	groups := map[string]*docGroup{}
	order := []string{}

	for i, row := range rows {
		var hybridScore float64
		if semanticScores != nil {
			hybridScore = ftsScores[i]*0.55 + semanticScores[row.chunkID]*0.45
		} else {
			hybridScore = ftsScores[i]
		}
		weightedScore := hybridScore * getCategoryWeight(row.category, intent)

		chunk := model.RetrievedDocumentChunk{
			ChunkID:    row.chunkID,
			ChunkIndex: row.chunkIndex,
			Content:    row.chunkContent,
			Score:      weightedScore,
		}

		existing := groups[row.sourceID]
		if existing == nil {
			fallbackText := row.chunkContent
			if fallbackText == "" {
				fallbackText = row.derivedText
			}
			if fallbackText == "" {
				fallbackText = row.note
			}
			groups[row.sourceID] = &docGroup{
				label:        row.label,
				category:     row.category,
				fallbackText: fallbackText,
				chunks:       []model.RetrievedDocumentChunk{chunk},
				bestScore:    weightedScore,
			}
			order = append(order, row.sourceID)
			continue
		}

		if !containsChunk(existing.chunks, chunk.ChunkID) && len(existing.chunks) < chunksPerDocument {
			existing.chunks = append(existing.chunks, chunk)
		}
		if weightedScore > existing.bestScore {
			existing.bestScore = weightedScore
		}
	}

	sort.SliceStable(order, func(i, j int) bool {
		return groups[order[i]].bestScore > groups[order[j]].bestScore
	})

	results := make([]model.RetrievedDocumentReference, 0, len(order))
	for _, sourceID := range order {
		group := groups[sourceID]
		results = append(results, buildDocumentReference(sourceID, group, chunksPerDocument, "search"))
	}
	return results
}

func containsChunk(chunks []model.RetrievedDocumentChunk, chunkID string) bool {
	for _, c := range chunks {
		if c.ChunkID == chunkID {
			return true
		}
	}
	return false
}

// buildDocumentReference renders a docGroup into a RetrievedDocumentReference,
// reproducing the TS excerpt (top-N chunks by score, displayed in index order) and
// chunks (chunks in index order) shapes.
func buildDocumentReference(sourceID string, group *docGroup, chunksPerDocument int, retrievalMode string) model.RetrievedDocumentReference {
	var excerpt string
	if len(group.chunks) > 0 {
		byScore := append([]model.RetrievedDocumentChunk(nil), group.chunks...)
		sort.SliceStable(byScore, func(i, j int) bool { return byScore[i].Score > byScore[j].Score })
		if len(byScore) > chunksPerDocument {
			byScore = byScore[:chunksPerDocument]
		}
		sort.SliceStable(byScore, func(i, j int) bool { return byScore[i].ChunkIndex < byScore[j].ChunkIndex })
		lines := make([]string, 0, len(byScore))
		for _, c := range byScore {
			lines = append(lines, fmt.Sprintf("[chunk %d] %s", c.ChunkIndex+1, util.Truncate(c.Content, 150)))
		}
		excerpt = joinLines(lines)
	} else {
		excerpt = util.Truncate(group.fallbackText, 220)
	}

	byIndex := append([]model.RetrievedDocumentChunk(nil), group.chunks...)
	sort.SliceStable(byIndex, func(i, j int) bool { return byIndex[i].ChunkIndex < byIndex[j].ChunkIndex })
	if len(byIndex) > chunksPerDocument {
		byIndex = byIndex[:chunksPerDocument]
	}

	return model.RetrievedDocumentReference{
		SearchReference: model.SearchReference{
			SourceType: "document",
			SourceID:   sourceID,
			Label:      group.label,
			Excerpt:    util.Truncate(excerpt, 500),
			Score:      group.bestScore,
		},
		Category:            categoryOrMisc(group.category),
		Chunks:              byIndex,
		IncludeFullDocument: false,
		RetrievalMode:       retrievalMode,
	}
}

func categoryOrMisc(category string) string {
	if category == "" {
		return "misc"
	}
	return category
}

// documentSemanticCandidates scores every embedded chunk in scope against the
// query, returning the unit-range scores by chunk id — which rank the FTS rows too
// — and, as candidate rows, the limit chunks closest to the query that FTS did not
// find and that clear the floor. Without these, a document sharing no term with
// the query could only enter when FTS found fewer documents than the limit, which
// an OR of prefix-matched terms almost never does. The scan reads only ids and
// vectors; chunk text is read for the adopted rows alone.
func (r *RetrievalService) documentSemanticCandidates(projectID string, queryEmbedding []float64, ftsRows []documentCandidateRow, limit int, intent string, scope MaterialScope) (map[string]float64, []documentCandidateRow, error) {
	rows, err := r.db.Query(`
		SELECT e.chunk_id, e.embedding_json, d.category
		FROM document_chunk_embeddings e
		JOIN documents d ON d.id = e.document_id
		WHERE e.project_id = ? AND e.model = ?`+
		scope.sharedWithAllClause("d"), projectID, r.embeddings.GetModel())
	if err != nil {
		return nil, nil, err
	}
	type scoredChunk struct {
		chunkID  string
		weighted float64
	}
	inFts := make(map[string]bool, len(ftsRows))
	for _, row := range ftsRows {
		inFts[row.chunkID] = true
	}
	floor := semanticOnlyFloor(0.55, r.embeddings.ActivePrefixScheme().Active())
	scores := map[string]float64{}
	var outside []scoredChunk
	for rows.Next() {
		var chunkID, raw, category string
		if err := rows.Scan(&chunkID, &raw, &category); err != nil {
			rows.Close()
			return nil, nil, err
		}
		score := semanticToUnitRange(vector.CosineSimilarity(queryEmbedding, parseEmbeddingJSON(raw)))
		scores[chunkID] = score
		if weighted := score * getCategoryWeight(category, intent); !inFts[chunkID] && weighted > floor {
			outside = append(outside, scoredChunk{chunkID, weighted})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	sort.SliceStable(outside, func(i, j int) bool { return outside[i].weighted > outside[j].weighted })
	if len(outside) > limit {
		outside = outside[:limit]
	}
	if len(outside) == 0 {
		return scores, nil, nil
	}

	args := make([]any, len(outside))
	for i, c := range outside {
		args[i] = c.chunkID
	}
	chunkRows, err := r.db.Query(`
		SELECT d.id, d.title, d.category, c.content, c.id, c.chunk_index, d.note, d.derived_text
		FROM document_chunks c
		JOIN documents d ON d.id = c.document_id
		WHERE c.id IN (`+placeholders(len(outside))+`)`, args...)
	if err != nil {
		return nil, nil, err
	}
	defer chunkRows.Close()
	byChunk := make(map[string]documentCandidateRow, len(outside))
	for chunkRows.Next() {
		var row documentCandidateRow
		if err := chunkRows.Scan(&row.sourceID, &row.label, &row.category, &row.chunkContent, &row.chunkID, &row.chunkIndex, &row.note, &row.derivedText); err != nil {
			return nil, nil, err
		}
		byChunk[row.chunkID] = row
	}
	if err := chunkRows.Err(); err != nil {
		return nil, nil, err
	}
	candidates := make([]documentCandidateRow, 0, len(outside))
	for _, c := range outside {
		if row, ok := byChunk[c.chunkID]; ok {
			candidates = append(candidates, row)
		}
	}
	return scores, candidates, nil
}

// rankMemoryCandidates scores memories as rankDocumentCandidates scores chunks.
func rankMemoryCandidates(ftsRows, semanticRows []memoryCandidateRow, semanticScores map[string]float64) []model.SearchReference {
	rawScores := make([]float64, len(ftsRows))
	for i, row := range ftsRows {
		rawScores[i] = row.score
	}
	ftsScores := append(vector.NormalizeScores(rawScores), make([]float64, len(semanticRows))...)
	rows := append(append([]memoryCandidateRow{}, ftsRows...), semanticRows...)

	results := make([]model.SearchReference, 0, len(rows))
	for i, row := range rows {
		var score float64
		if semanticScores != nil {
			score = ftsScores[i]*0.5 + semanticScores[row.sourceID]*0.5
		} else {
			score = ftsScores[i]
		}
		results = append(results, model.SearchReference{
			SourceType: "memory",
			SourceID:   row.sourceID,
			Label:      row.label,
			Excerpt:    util.Truncate(row.excerpt, 220),
			Score:      score,
		})
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results
}

// memorySemanticCandidates is documentSemanticCandidates for memories. Memories
// are short, so their text is read in the same scan.
func (r *RetrievalService) memorySemanticCandidates(projectID string, queryEmbedding []float64, ftsRows []memoryCandidateRow, limit int, scope MaterialScope) (map[string]float64, []memoryCandidateRow, error) {
	rows, err := r.db.Query(`
		SELECT e.memory_id, e.embedding_json, m.title, m.content
		FROM memory_embeddings e
		JOIN memories m ON m.id = e.memory_id
		WHERE e.project_id = ? AND e.model = ?`+
		scope.sharedWithAllClause("m"), projectID, r.embeddings.GetModel())
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	inFts := make(map[string]bool, len(ftsRows))
	for _, row := range ftsRows {
		inFts[row.sourceID] = true
	}
	floor := semanticOnlyFloor(0.58, r.embeddings.ActivePrefixScheme().Active())
	scores := map[string]float64{}
	var outside []memoryCandidateRow
	for rows.Next() {
		var row memoryCandidateRow
		var raw string
		if err := rows.Scan(&row.sourceID, &raw, &row.label, &row.excerpt); err != nil {
			return nil, nil, err
		}
		row.score = semanticToUnitRange(vector.CosineSimilarity(queryEmbedding, parseEmbeddingJSON(raw)))
		scores[row.sourceID] = row.score
		if !inFts[row.sourceID] && row.score > floor {
			outside = append(outside, row)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	sort.SliceStable(outside, func(i, j int) bool { return outside[i].score > outside[j].score })
	if len(outside) > limit {
		outside = outside[:limit]
	}
	return scores, outside, nil
}
