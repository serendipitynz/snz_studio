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

// semanticFallbackFloor is the minimum score a candidate needs to qualify in the
// semantic-ONLY fallback (used when FTS returned too few candidates). ruri
// (ModernBERT-Ja) cosines cluster high — even unrelated Japanese text scores ~0.78,
// mapping to ~0.89 unit — so the default floor calibrated for wider-spread
// embeddings would admit almost everything. Use a higher floor for ruri.
//
// PROVISIONAL: the ruri floor is derived from limited spike data (S2). Calibrate it
// with more labeled negatives during GUI E2E before relying on the fallback path.
func semanticFallbackFloor(defaultFloor float64, ruri bool) float64 {
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
	fullContent  string
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

// SearchDocuments mirrors searchDocuments(projectId, query, limit=4, chunksPerDocument=3),
// narrowed to scope.
func (r *RetrievalService) SearchDocuments(projectID, query string, limit, chunksPerDocument int, scope MaterialScope) ([]model.RetrievedDocumentReference, error) {
	ftsQuery := search.ToFtsQuery(query)
	intent := detectRetrievalIntent(query)

	var candidateRows []documentCandidateRow
	if ftsQuery != "" {
		rows, err := r.fetchDocumentFtsCandidates(projectID, ftsQuery, limit, chunksPerDocument, scope)
		if err != nil {
			return nil, err
		}
		candidateRows = rows
	}

	// The query gets the ruri query prefix (no-op for other models); the FTS query
	// above stays raw.
	queryEmbedding := r.embeddings.CreateEmbedding(r.embeddings.ActivePrefixScheme().Query + query)

	ranked, err := r.rankDocumentCandidates(candidateRows, queryEmbedding, chunksPerDocument, intent)
	if err != nil {
		return nil, err
	}
	if len(ranked) >= limit || queryEmbedding == nil {
		return sliceDocRefs(ranked, limit), nil
	}

	seen := make(map[string]bool, len(ranked))
	for _, item := range ranked {
		seen[item.SourceID] = true
	}
	fallback, err := r.searchDocumentsBySemantic(projectID, queryEmbedding, limit, chunksPerDocument, seen, intent, scope)
	if err != nil {
		return nil, err
	}
	return sliceDocRefs(append(ranked, fallback...), limit), nil
}

func sliceDocRefs(refs []model.RetrievedDocumentReference, limit int) []model.RetrievedDocumentReference {
	if len(refs) > limit {
		return refs[:limit]
	}
	return refs
}

// SearchChunksInDocument mirrors searchChunksInDocument(documentId, query, limit=5).
func (r *RetrievalService) SearchChunksInDocument(documentID, query string, limit int) ([]model.RetrievedDocumentChunk, error) {
	ftsQuery := search.ToFtsQuery(query)

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

	rows, err := r.db.Query(`
		SELECT
			c.id AS chunk_id,
			c.chunk_index AS chunk_index,
			c.content AS content,
			bm25(document_chunks_fts, 10.0, 2.0, 1.0, 1.0, 4.0) * -1 AS score
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
// scope.
func (r *RetrievalService) SearchMemories(projectID, query string, limit int, scope MaterialScope) ([]model.SearchReference, error) {
	ftsQuery := search.ToFtsQuery(query)

	var candidateRows []memoryCandidateRow
	if ftsQuery != "" {
		candidateLimit := limit * 3
		if candidateLimit < 8 {
			candidateLimit = 8
		}
		rows, err := r.fetchMemoryFtsCandidates(projectID, ftsQuery, candidateLimit, scope)
		if err != nil {
			return nil, err
		}
		candidateRows = rows
	}

	queryEmbedding := r.embeddings.CreateEmbedding(r.embeddings.ActivePrefixScheme().Query + query)

	ranked, err := r.rankMemoryCandidates(candidateRows, queryEmbedding)
	if err != nil {
		return nil, err
	}
	if len(ranked) >= limit || queryEmbedding == nil {
		return sliceRefs(ranked, limit), nil
	}

	seen := make(map[string]bool, len(ranked))
	for _, item := range ranked {
		seen[item.SourceID] = true
	}
	fallback, err := r.searchMemoriesBySemantic(projectID, queryEmbedding, limit, seen, scope)
	if err != nil {
		return nil, err
	}
	return sliceRefs(append(ranked, fallback...), limit), nil
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
	rows, err := r.db.Query(`
		SELECT
			d.id AS source_id,
			d.title AS label,
			d.category AS category,
			d.content_text AS full_content,
			c.content AS chunk_content,
			c.id AS chunk_id,
			c.chunk_index AS chunk_index,
			d.note AS note,
			d.derived_text AS derived_text,
			bm25(document_chunks_fts, 10.0, 2.0, 1.0, 1.0, 4.0) * -1 AS score
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
		if err := rows.Scan(&row.sourceID, &row.label, &row.category, &row.fullContent, &row.chunkContent, &row.chunkID, &row.chunkIndex, &row.note, &row.derivedText, &row.score); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *RetrievalService) fetchMemoryFtsCandidates(projectID, ftsQuery string, limit int, scope MaterialScope) ([]memoryCandidateRow, error) {
	rows, err := r.db.Query(`
		SELECT
			m.id AS source_id,
			m.title AS label,
			m.content AS excerpt,
			bm25(memories_fts, 2.0, 6.0, 8.0) * -1 AS score
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
	fullContent  string
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

func (r *RetrievalService) rankDocumentCandidates(rows []documentCandidateRow, queryEmbedding []float64, chunksPerDocument int, intent string) ([]model.RetrievedDocumentReference, error) {
	if len(rows) == 0 {
		return []model.RetrievedDocumentReference{}, nil
	}

	rawScores := make([]float64, len(rows))
	for i, row := range rows {
		rawScores[i] = row.score
	}
	ftsScores := vector.NormalizeScores(rawScores)

	semanticScores := map[string]float64{}
	if queryEmbedding != nil {
		chunkIDs := make([]string, len(rows))
		for i, row := range rows {
			chunkIDs[i] = row.chunkID
		}
		scores, err := r.chunkSemanticScores(queryEmbedding, chunkIDs)
		if err != nil {
			return nil, err
		}
		semanticScores = scores
	}

	groups := map[string]*docGroup{}
	order := []string{}

	for i, row := range rows {
		var hybridScore float64
		if queryEmbedding != nil {
			hybridScore = ftsScores[i]*0.55 + semanticScores[row.chunkID]*0.45
		} else if ftsScores[i] != 0 {
			hybridScore = ftsScores[i]
		} else {
			hybridScore = row.score
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
				fullContent:  row.fullContent,
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
	return results, nil
}

// chunkSemanticScores computes the unit-range semantic score for each chunk id that
// has a stored embedding for the active model.
func (r *RetrievalService) chunkSemanticScores(queryEmbedding []float64, chunkIDs []string) (map[string]float64, error) {
	scores := map[string]float64{}
	if len(chunkIDs) == 0 {
		return scores, nil
	}
	args := make([]any, 0, len(chunkIDs)+1)
	args = append(args, r.embeddings.GetModel())
	for _, id := range chunkIDs {
		args = append(args, id)
	}
	rows, err := r.db.Query(`SELECT chunk_id, embedding_json FROM document_chunk_embeddings WHERE model = ? AND chunk_id IN (`+placeholders(len(chunkIDs))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var chunkID, raw string
		if err := rows.Scan(&chunkID, &raw); err != nil {
			return nil, err
		}
		scores[chunkID] = semanticToUnitRange(vector.CosineSimilarity(queryEmbedding, parseEmbeddingJSON(raw)))
	}
	return scores, rows.Err()
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
		FullDocumentContent: group.fullContent,
		RetrievalMode:       retrievalMode,
	}
}

func categoryOrMisc(category string) string {
	if category == "" {
		return "misc"
	}
	return category
}

func (r *RetrievalService) searchDocumentsBySemantic(projectID string, queryEmbedding []float64, limit, chunksPerDocument int, seen map[string]bool, intent string, scope MaterialScope) ([]model.RetrievedDocumentReference, error) {
	rows, err := r.db.Query(`
		SELECT
			e.document_id,
			e.chunk_id,
			e.embedding_json,
			d.title,
			d.category,
			d.content_text AS full_content,
			d.note,
			d.derived_text,
			c.content AS chunk_content,
			c.chunk_index
		FROM document_chunk_embeddings e
		JOIN documents d ON d.id = e.document_id
		JOIN document_chunks c ON c.id = e.chunk_id
		WHERE e.project_id = ? AND e.model = ?`+
		scope.sharedWithAllClause("d"), projectID, r.embeddings.GetModel())
	if err != nil {
		return nil, err
	}
	type semRow struct {
		documentID   string
		chunkID      string
		title        string
		category     string
		fullContent  string
		note         string
		derivedText  string
		chunkContent string
		chunkIndex   int
		score        float64
	}
	floor := semanticFallbackFloor(0.55, r.embeddings.ActivePrefixScheme().Active())
	var scored []semRow
	for rows.Next() {
		var sr semRow
		var raw string
		if err := rows.Scan(&sr.documentID, &sr.chunkID, &raw, &sr.title, &sr.category, &sr.fullContent, &sr.note, &sr.derivedText, &sr.chunkContent, &sr.chunkIndex); err != nil {
			rows.Close()
			return nil, err
		}
		sr.score = semanticToUnitRange(vector.CosineSimilarity(queryEmbedding, parseEmbeddingJSON(raw))) * getCategoryWeight(sr.category, intent)
		if sr.score > floor {
			scored = append(scored, sr)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })

	groups := map[string]*docGroup{}
	order := []string{}
	for _, row := range scored {
		if seen[row.documentID] {
			continue
		}
		chunk := model.RetrievedDocumentChunk{
			ChunkID:    row.chunkID,
			ChunkIndex: row.chunkIndex,
			Content:    row.chunkContent,
			Score:      row.score,
		}
		existing := groups[row.documentID]
		if existing == nil {
			fallbackText := row.chunkContent
			if fallbackText == "" {
				fallbackText = row.derivedText
			}
			if fallbackText == "" {
				fallbackText = row.note
			}
			groups[row.documentID] = &docGroup{
				label:        row.title,
				category:     row.category,
				fullContent:  row.fullContent,
				fallbackText: fallbackText,
				chunks:       []model.RetrievedDocumentChunk{chunk},
				bestScore:    row.score,
			}
			order = append(order, row.documentID)
		} else if !containsChunk(existing.chunks, chunk.ChunkID) && len(existing.chunks) < chunksPerDocument {
			existing.chunks = append(existing.chunks, chunk)
		}
		if len(groups) >= limit {
			break
		}
	}

	results := make([]model.RetrievedDocumentReference, 0, len(order))
	for _, documentID := range order {
		group := groups[documentID]
		results = append(results, buildSemanticDocumentReference(documentID, group, chunksPerDocument))
	}
	return results, nil
}

// buildSemanticDocumentReference renders a semantic-fallback docGroup. Its excerpt
// uses the chunks in index order (no score re-sort), mirroring the TS fallback.
func buildSemanticDocumentReference(documentID string, group *docGroup, chunksPerDocument int) model.RetrievedDocumentReference {
	byIndex := append([]model.RetrievedDocumentChunk(nil), group.chunks...)
	sort.SliceStable(byIndex, func(i, j int) bool { return byIndex[i].ChunkIndex < byIndex[j].ChunkIndex })

	lines := make([]string, 0, len(byIndex))
	for _, c := range byIndex {
		lines = append(lines, fmt.Sprintf("[chunk %d] %s", c.ChunkIndex+1, util.Truncate(c.Content, 150)))
	}
	excerpt := joinLines(lines)
	if excerpt == "" {
		excerpt = group.fallbackText
	}

	chunks := byIndex
	if len(chunks) > chunksPerDocument {
		chunks = chunks[:chunksPerDocument]
	}

	return model.RetrievedDocumentReference{
		SearchReference: model.SearchReference{
			SourceType: "document",
			SourceID:   documentID,
			Label:      group.label,
			Excerpt:    util.Truncate(excerpt, 500),
			Score:      group.bestScore,
		},
		Category:            categoryOrMisc(group.category),
		Chunks:              chunks,
		IncludeFullDocument: false,
		FullDocumentContent: group.fullContent,
		RetrievalMode:       "search",
	}
}

func (r *RetrievalService) rankMemoryCandidates(rows []memoryCandidateRow, queryEmbedding []float64) ([]model.SearchReference, error) {
	if len(rows) == 0 {
		return []model.SearchReference{}, nil
	}

	rawScores := make([]float64, len(rows))
	for i, row := range rows {
		rawScores[i] = row.score
	}
	ftsScores := vector.NormalizeScores(rawScores)

	semanticScores := map[string]float64{}
	if queryEmbedding != nil {
		memoryIDs := make([]string, len(rows))
		for i, row := range rows {
			memoryIDs[i] = row.sourceID
		}
		scores, err := r.memorySemanticScores(queryEmbedding, memoryIDs)
		if err != nil {
			return nil, err
		}
		semanticScores = scores
	}

	results := make([]model.SearchReference, 0, len(rows))
	for i, row := range rows {
		var score float64
		if queryEmbedding != nil {
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
	return results, nil
}

func (r *RetrievalService) memorySemanticScores(queryEmbedding []float64, memoryIDs []string) (map[string]float64, error) {
	scores := map[string]float64{}
	if len(memoryIDs) == 0 {
		return scores, nil
	}
	args := make([]any, 0, len(memoryIDs)+1)
	args = append(args, r.embeddings.GetModel())
	for _, id := range memoryIDs {
		args = append(args, id)
	}
	rows, err := r.db.Query(`SELECT memory_id, embedding_json FROM memory_embeddings WHERE model = ? AND memory_id IN (`+placeholders(len(memoryIDs))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var memoryID, raw string
		if err := rows.Scan(&memoryID, &raw); err != nil {
			return nil, err
		}
		scores[memoryID] = semanticToUnitRange(vector.CosineSimilarity(queryEmbedding, parseEmbeddingJSON(raw)))
	}
	return scores, rows.Err()
}

func (r *RetrievalService) searchMemoriesBySemantic(projectID string, queryEmbedding []float64, limit int, seen map[string]bool, scope MaterialScope) ([]model.SearchReference, error) {
	rows, err := r.db.Query(`
		SELECT
			e.memory_id,
			e.embedding_json,
			m.title,
			m.content
		FROM memory_embeddings e
		JOIN memories m ON m.id = e.memory_id
		WHERE e.project_id = ? AND e.model = ?`+
		scope.sharedWithAllClause("m"), projectID, r.embeddings.GetModel())
	if err != nil {
		return nil, err
	}
	type scoredMemory struct {
		memoryID string
		title    string
		content  string
		score    float64
	}
	floor := semanticFallbackFloor(0.58, r.embeddings.ActivePrefixScheme().Active())
	var scored []scoredMemory
	for rows.Next() {
		var sm scoredMemory
		var raw string
		if err := rows.Scan(&sm.memoryID, &raw, &sm.title, &sm.content); err != nil {
			rows.Close()
			return nil, err
		}
		if seen[sm.memoryID] {
			continue
		}
		sm.score = semanticToUnitRange(vector.CosineSimilarity(queryEmbedding, parseEmbeddingJSON(raw)))
		if sm.score > floor {
			scored = append(scored, sm)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > limit {
		scored = scored[:limit]
	}

	results := make([]model.SearchReference, 0, len(scored))
	for _, sm := range scored {
		results = append(results, model.SearchReference{
			SourceType: "memory",
			SourceID:   sm.memoryID,
			Label:      sm.title,
			Excerpt:    util.Truncate(sm.content, 220),
			Score:      sm.score,
		})
	}
	return results, nil
}
