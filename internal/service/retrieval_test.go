package service

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/db"
	"snzstudio/internal/model"
	"snzstudio/internal/repository"
)

// newServiceTestDB opens a fresh migrated SQLite database for service tests.
func newServiceTestDB(t testing.TB) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "service.sqlite")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// disabledEmbeddingClient returns an embedding client with no model configured, so
// retrieval runs FTS-only without contacting any endpoint.
func disabledEmbeddingClient() *EmbeddingClient {
	return NewEmbeddingClient(testConfig(config.Settings{}))
}

func TestDetectRetrievalIntent(t *testing.T) {
	cases := map[string]string{
		"この文章を英訳してください":       "translation",
		"次のプロットを考えて":          "plot",
		"続きの本文を書いて":           "writing",
		"世界観の設定を確認したい":        "reference",
		"hello there general": "general",
	}
	for query, want := range cases {
		if got := detectRetrievalIntent(query); got != want {
			t.Fatalf("detectRetrievalIntent(%q) = %q, want %q", query, got, want)
		}
	}
}

func TestDetectRetrievalIntentOrdering(t *testing.T) {
	// A query that matches multiple buckets resolves to the first in priority order
	// (translation > plot > writing > reference).
	if got := detectRetrievalIntent("翻訳のプロットと設定"); got != "translation" {
		t.Fatalf("priority order broken: got %q, want translation", got)
	}
}

func TestGetCategoryWeight(t *testing.T) {
	if w := getCategoryWeight("index", "reference"); w != 1.25 {
		t.Fatalf("reference/index weight = %v, want 1.25", w)
	}
	if w := getCategoryWeight("story", "reference"); w != 0.8 {
		t.Fatalf("reference/story weight = %v, want 0.8", w)
	}
	// Unknown intent or category defaults to 1.
	if w := getCategoryWeight("misc", "nonexistent-intent"); w != 1 {
		t.Fatalf("unknown intent weight = %v, want 1", w)
	}
}

func TestSemanticToUnitRange(t *testing.T) {
	if v := semanticToUnitRange(1); v != 1 {
		t.Fatalf("cos 1 => %v, want 1", v)
	}
	if v := semanticToUnitRange(-1); v != 0 {
		t.Fatalf("cos -1 => %v, want 0", v)
	}
	if v := semanticToUnitRange(0); v != 0.5 {
		t.Fatalf("cos 0 => %v, want 0.5", v)
	}
	// Out-of-range inputs clamp to [0, 1].
	if v := semanticToUnitRange(5); v != 1 {
		t.Fatalf("cos 5 => %v, want clamp to 1", v)
	}
}

func TestSearchDocumentsFTSOnly(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	retrieval := NewRetrievalService(d, disabledEmbeddingClient())

	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	dragonDoc, err := documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID:   project.ID,
		Type:        "text",
		Category:    "world",
		Title:       "竜の世界観",
		ContentText: "竜が支配する大陸の世界観と歴史を記した設定資料。",
	})
	if err != nil {
		t.Fatalf("CreateDocument dragon: %v", err)
	}
	if _, err := documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID:   project.ID,
		Type:        "text",
		Category:    "character",
		Title:       "登場人物",
		ContentText: "主人公の少女は魔法学校に通っている。",
	}); err != nil {
		t.Fatalf("CreateDocument character: %v", err)
	}

	refs, err := retrieval.SearchDocuments(project.ID, SearchQuery{Text: "竜の世界観について教えて"}, 4, 3, ScopeAll)
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}
	if len(refs) == 0 {
		t.Fatal("expected at least one document reference for a matching term")
	}
	if refs[0].SourceID != dragonDoc.ID {
		t.Fatalf("top document = %q, want the dragon document %q", refs[0].SourceID, dragonDoc.ID)
	}
	if refs[0].SourceType != "document" || refs[0].RetrievalMode != "search" {
		t.Fatalf("unexpected reference shape: %+v", refs[0].SearchReference)
	}
	if len(refs[0].Chunks) == 0 {
		t.Fatal("expected matched chunks on the top reference")
	}
}

func TestSearchMemoriesFTSOnly(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	memories := repository.NewMemoryRepository(d)
	retrieval := NewRetrievalService(d, disabledEmbeddingClient())

	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	created, err := memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID: project.ID,
		Kind:      "semantic",
		Title:     "舞台設定",
		Content:   "物語の舞台は浮遊大陸である。",
	})
	if err != nil {
		t.Fatalf("CreateMemory: %v", err)
	}

	refs, err := retrieval.SearchMemories(project.ID, SearchQuery{Text: "舞台はどこ"}, 4, ScopeAll)
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(refs) == 0 || refs[0].SourceID != created.ID {
		t.Fatalf("expected memory %q in results, got %+v", created.ID, refs)
	}
	if refs[0].SourceType != "memory" {
		t.Fatalf("sourceType = %q, want memory", refs[0].SourceType)
	}
}

// bm25() takes one weight per FTS5 column, UNINDEXED ones included, so a weight
// list that forgets the id columns silently lands on the wrong text columns. The
// two records below swap the same strings between title and content, which makes
// them tie under equal per-column weights; only the configured title/content
// weights can break the tie.
func TestSearchDocumentsTitleMatchOutranksContentMatch(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	retrieval := NewRetrievalService(d, disabledEmbeddingClient())

	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	titleMatch, err := documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID: project.ID, Type: "text", Category: "world", Title: "飛竜の生態", ContentText: "港町の地図",
	})
	if err != nil {
		t.Fatalf("CreateDocument title match: %v", err)
	}
	contentMatch, err := documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID: project.ID, Type: "text", Category: "world", Title: "港町の地図", ContentText: "飛竜の生態",
	})
	if err != nil {
		t.Fatalf("CreateDocument content match: %v", err)
	}

	refs, err := retrieval.SearchDocuments(project.ID, SearchQuery{Text: "飛竜"}, 4, 3, ScopeAll)
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d references, want both documents: %+v", len(refs), refs)
	}
	if refs[0].SourceID != titleMatch.ID || refs[1].SourceID != contentMatch.ID {
		t.Fatalf("order = [%s, %s], want the title match before the content match", refs[0].Label, refs[1].Label)
	}
	if !(refs[0].Score > refs[1].Score) {
		t.Fatalf("title match score %v is not above content match score %v", refs[0].Score, refs[1].Score)
	}
}

// Without embeddings the lowest FTS row normalizes to 0. It used to fall back to
// its raw bm25 score, which in a corpus large enough for a real IDF exceeds the
// top row's normalized 1 and sorted the weakest match first.
func TestSearchDocumentsFTSOnlyKeepsWeakestMatchLast(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	retrieval := NewRetrievalService(d, disabledEmbeddingClient())

	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	create := func(title, content string) string {
		t.Helper()
		doc, err := documents.CreateDocument(repository.CreateDocumentInput{
			ProjectID: project.ID, Type: "text", Category: "world", Title: title, ContentText: content,
		})
		if err != nil {
			t.Fatalf("CreateDocument %s: %v", title, err)
		}
		return doc.ID
	}
	strong := create("飛竜の生態", "山脈の東側に巣を作る。")
	weak := create("旅の記録", "道中で一度だけ飛竜を見かけた。"+strings.Repeat("街道は長く続いた。", 20))
	for i := 0; i < 20; i++ {
		create(fmt.Sprintf("雑記%d", i), "市場の値段の控え。")
	}

	refs, err := retrieval.SearchDocuments(project.ID, SearchQuery{Text: "飛竜"}, 4, 3, ScopeAll)
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d references, want the two matching documents: %+v", len(refs), refs)
	}
	if refs[0].SourceID != strong || refs[1].SourceID != weak {
		t.Fatalf("order = [%s %v, %s %v], want the title match first", refs[0].Label, refs[0].Score, refs[1].Label, refs[1].Score)
	}
}

// Memories weight content above title, so the swapped pair ranks the other way.
func TestSearchMemoriesContentMatchOutranksTitleMatch(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	memories := repository.NewMemoryRepository(d)
	retrieval := NewRetrievalService(d, disabledEmbeddingClient())

	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	titleMatch, err := memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID: project.ID, Kind: "semantic", Title: "飛竜の生態", Content: "港町の地図",
	})
	if err != nil {
		t.Fatalf("CreateMemory title match: %v", err)
	}
	contentMatch, err := memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID: project.ID, Kind: "semantic", Title: "港町の地図", Content: "飛竜の生態",
	})
	if err != nil {
		t.Fatalf("CreateMemory content match: %v", err)
	}

	refs, err := retrieval.SearchMemories(project.ID, SearchQuery{Text: "飛竜"}, 4, ScopeAll)
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d references, want both memories: %+v", len(refs), refs)
	}
	if refs[0].SourceID != contentMatch.ID || refs[1].SourceID != titleMatch.ID {
		t.Fatalf("order = [%s, %s], want the content match before the title match", refs[0].Label, refs[1].Label)
	}
	if !(refs[0].Score > refs[1].Score) {
		t.Fatalf("content match score %v is not above title match score %v", refs[0].Score, refs[1].Score)
	}
}

// TestSearchDocumentsResultsAcrossBothPaths pins what SearchDocuments returns when
// FTS matches fewer documents than the limit and embedding candidates make up the
// rest. The snapshot was first captured before TASK-74 stopped reading every
// document's full text per candidate chunk, guarding that the adopted documents,
// their chunks, excerpts, scores and full text stayed the same. TASK-101 changed
// only the ranking: the two documents FTS does not match used to follow every FTS
// match with their semantic score alone, and now rank with them at an FTS score
// of 0, so both pass the weaker FTS match.
func TestSearchDocumentsResultsAcrossBothPaths(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	srv, _ := fakeEmbeddingServer(t)
	retrieval := NewRetrievalService(d, enabledEmbeddingClient(srv.URL, "m"))

	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	// The fake endpoint embeds the query as (1, 0, 0); each document's chunks get
	// the vector listed here.
	corpus := []struct {
		title, category, content string
		embedding                []float64
	}{
		{"竜の年代記", "story", strings.Repeat("飛竜の群れが北の砦を越えた。王都では鐘が鳴った。", 60), []float64{0.6, 0.8, 0}},
		{"飛竜の生態", "world", "飛竜は山脈の東側に巣を作り、冬は谷へ下りる。", []float64{0.2, 1, 0}},
		{"港町の地図", "world", "港町には三つの埠頭と灯台がある。", []float64{0.9, 0.1, 0}},
		{"灯台守の手記", "character", "灯台守は毎晩、沖の船の数を記録している。", []float64{0.3, 0.9, 0}},
		{"雑記", "misc", "市場の値段の控え。", []float64{-1, 0, 0}},
	}
	contentByTitle := map[string]string{}
	var embeddings []repository.ChunkEmbedding
	for _, c := range corpus {
		doc, err := documents.CreateDocument(repository.CreateDocumentInput{
			ProjectID: project.ID, Type: "text", Category: c.category, Title: c.title, ContentText: c.content,
		})
		if err != nil {
			t.Fatalf("CreateDocument %s: %v", c.title, err)
		}
		contentByTitle[c.title] = doc.ContentText
		chunks, err := documents.ListChunksForEmbedding(doc.ID)
		if err != nil {
			t.Fatalf("ListChunksForEmbedding: %v", err)
		}
		for _, ch := range chunks {
			embeddings = append(embeddings, repository.ChunkEmbedding{ChunkID: ch.ChunkID, DocumentID: doc.ID, ProjectID: project.ID, Embedding: c.embedding, Model: "m"})
		}
	}
	if err := documents.UpsertChunkEmbeddings(embeddings); err != nil {
		t.Fatalf("UpsertChunkEmbeddings: %v", err)
	}

	refs, err := retrieval.SearchDocuments(project.ID, SearchQuery{Text: "飛竜について"}, 4, 3, ScopeAll)
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}

	var snapshot strings.Builder
	for _, ref := range refs {
		if ref.FullDocumentContent != contentByTitle[ref.Label] {
			t.Errorf("%s: FullDocumentContent is not the document's full text (%d runes)", ref.Label, len([]rune(ref.FullDocumentContent)))
		}
		fmt.Fprintf(&snapshot, "%s category=%s mode=%s score=%.6f full=%v\n", ref.Label, ref.Category, ref.RetrievalMode, ref.Score, ref.IncludeFullDocument)
		for _, ch := range ref.Chunks {
			fmt.Fprintf(&snapshot, "  chunk %d score=%.6f runes=%d\n", ch.ChunkIndex, ch.Score, len([]rune(ch.Content)))
		}
		fmt.Fprintf(&snapshot, "  excerpt: %s\n", ref.Excerpt)
	}
	const want = `竜の年代記 category=story mode=search score=0.910000 full=false
  chunk 0 score=0.910000 runes=1000
  chunk 1 score=0.762864 runes=590
  excerpt: [chunk 1] 飛竜の群れが北の砦を越えた。王都では鐘が鳴った。飛竜の群れが北の砦を越えた。王都では鐘が鳴った。飛竜の群れが北の砦を越えた。王都では鐘が鳴った。飛竜の群れが北の砦を越えた。王都では鐘が鳴った。飛竜の群れが北の砦を越えた。王都では鐘が鳴った。飛竜の群れが北の砦を越えた。王都では鐘が鳴った。飛竜の群れ…
[chunk 2] 越えた。王都では鐘が鳴った。飛竜の群れが北の砦を越えた。王都では鐘が鳴った。飛竜の群れが北の砦を越えた。王都では鐘が鳴った。飛竜の群れが北の砦を越えた。王都では鐘が鳴った。飛竜の群れが北の砦を越えた。王都では鐘が鳴った。飛竜の群れが北の砦を越えた。王都では鐘が鳴った。飛竜の群れが北の砦を越えた。王…
港町の地図 category=world mode=search score=0.471055 full=false
  chunk 0 score=0.471055 runes=16
  excerpt: [chunk 1] 港町には三つの埠頭と灯台がある。
灯台守の手記 category=character mode=search score=0.310959 full=false
  chunk 0 score=0.310959 runes=20
  excerpt: [chunk 1] 灯台守は毎晩、沖の船の数を記録している。
飛竜の生態 category=world mode=search score=0.282582 full=false
  chunk 0 score=0.282582 runes=22
  excerpt: [chunk 1] 飛竜は山脈の東側に巣を作り、冬は谷へ下りる。
`
	if got := snapshot.String(); got != want {
		t.Fatalf("SearchDocuments results changed.\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

// seedSemanticNeighbour seeds four documents and three memories that share the
// query term 「飛竜」 but sit far from the query vector, plus one document and one
// memory that share no term with it but sit on the query vector — the fake
// endpoint embeds every input as (1, 0, 0). FTS alone finds more candidates than
// either limit, which used to leave no room for the neighbours.
func seedSemanticNeighbour(t *testing.T) (d *sql.DB, projectID, neighbourDocID, neighbourMemoryID string) {
	t.Helper()
	d = newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	memories := repository.NewMemoryRepository(d)

	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	far, near := []float64{0, 1, 0}, []float64{1, 0, 0}
	var chunkEmbeddings []repository.ChunkEmbedding
	addDocument := func(title, content string, embedding []float64) string {
		doc, err := documents.CreateDocument(repository.CreateDocumentInput{
			ProjectID: project.ID, Type: "text", Category: "world", Title: title, ContentText: content,
		})
		if err != nil {
			t.Fatalf("CreateDocument %s: %v", title, err)
		}
		chunks, err := documents.ListChunksForEmbedding(doc.ID)
		if err != nil {
			t.Fatalf("ListChunksForEmbedding: %v", err)
		}
		for _, ch := range chunks {
			chunkEmbeddings = append(chunkEmbeddings, repository.ChunkEmbedding{ChunkID: ch.ChunkID, DocumentID: doc.ID, ProjectID: project.ID, Embedding: embedding, Model: "m"})
		}
		return doc.ID
	}
	// Different lengths give the FTS matches different bm25 scores, so their
	// normalized FTS scores spread over [0, 1] instead of all being 1.
	for i := 1; i <= 4; i++ {
		addDocument(fmt.Sprintf("記録%d", i), strings.Repeat("飛竜が谷を越えた。", i)+strings.Repeat("王都では鐘が鳴った。", i*4), far)
	}
	neighbourDocID = addDocument("港町の灯台", "港町の灯台は沖の船を見張る。", near)
	if err := documents.UpsertChunkEmbeddings(chunkEmbeddings); err != nil {
		t.Fatalf("UpsertChunkEmbeddings: %v", err)
	}

	var memoryEmbeddings []repository.MemoryEmbedding
	addMemory := func(title, content string, embedding []float64) string {
		memory, err := memories.CreateMemory(repository.CreateMemoryInput{ProjectID: project.ID, Kind: "semantic", Title: title, Content: content})
		if err != nil {
			t.Fatalf("CreateMemory %s: %v", title, err)
		}
		memoryEmbeddings = append(memoryEmbeddings, repository.MemoryEmbedding{MemoryID: memory.ID, ProjectID: project.ID, Kind: "semantic", Embedding: embedding, Model: "m"})
		return memory.ID
	}
	for i := 1; i <= 3; i++ {
		addMemory(fmt.Sprintf("記録%d", i), strings.Repeat("飛竜が谷を越えた。", i)+strings.Repeat("鐘が鳴った。", i*4), far)
	}
	neighbourMemoryID = addMemory("灯台", "港町の灯台は沖の船を見張る。", near)
	if err := memories.UpsertMemoryEmbeddings(memoryEmbeddings); err != nil {
		t.Fatalf("UpsertMemoryEmbeddings: %v", err)
	}
	return d, project.ID, neighbourDocID, neighbourMemoryID
}

func TestSearchAdmitsSemanticNeighboursBesideFullFtsCandidates(t *testing.T) {
	d, projectID, neighbourDocID, neighbourMemoryID := seedSemanticNeighbour(t)
	srv, _ := fakeEmbeddingServer(t)
	retrieval := NewRetrievalService(d, enabledEmbeddingClient(srv.URL, "m"))

	docs, err := retrieval.SearchDocuments(projectID, SearchQuery{Text: "飛竜"}, 4, 3, ScopeAll)
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}
	if !slices.ContainsFunc(docs, func(ref model.RetrievedDocumentReference) bool { return ref.SourceID == neighbourDocID }) {
		t.Fatalf("the document sharing no term but closest in meaning is not among the top 4: %v", docLabels(docs))
	}

	mems, err := retrieval.SearchMemories(projectID, SearchQuery{Text: "飛竜"}, 3, ScopeAll)
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if !slices.ContainsFunc(mems, func(ref model.SearchReference) bool { return ref.SourceID == neighbourMemoryID }) {
		t.Fatalf("the memory sharing no term but closest in meaning is not among the top 3: %+v", mems)
	}
}

func TestSearchWithoutEmbeddingsUsesFtsMatchesOnly(t *testing.T) {
	d, projectID, neighbourDocID, neighbourMemoryID := seedSemanticNeighbour(t)
	retrieval := NewRetrievalService(d, disabledEmbeddingClient())

	docs, err := retrieval.SearchDocuments(projectID, SearchQuery{Text: "飛竜"}, 5, 3, ScopeAll)
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}
	if len(docs) != 4 || slices.ContainsFunc(docs, func(ref model.RetrievedDocumentReference) bool { return ref.SourceID == neighbourDocID }) {
		t.Fatalf("want the 4 FTS matches only, got %v", docLabels(docs))
	}
	for i := 1; i < len(docs); i++ {
		if docs[i].Score > docs[i-1].Score {
			t.Fatalf("documents are not in FTS score order: %v", docLabels(docs))
		}
	}

	mems, err := retrieval.SearchMemories(projectID, SearchQuery{Text: "飛竜"}, 5, ScopeAll)
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(mems) != 3 || slices.ContainsFunc(mems, func(ref model.SearchReference) bool { return ref.SourceID == neighbourMemoryID }) {
		t.Fatalf("want the 3 FTS matches only, got %+v", mems)
	}
}

func docLabels(refs []model.RetrievedDocumentReference) []string {
	labels := make([]string, len(refs))
	for i, ref := range refs {
		labels[i] = fmt.Sprintf("%s(%.3f)", ref.Label, ref.Score)
	}
	return labels
}

// seedEmbeddedDocument creates a document and gives each of its chunks the vector
// embeddingFor returns for that chunk's index.
func seedEmbeddedDocument(t *testing.T, documents *repository.DocumentRepository, projectID, title, content string, embeddingFor func(chunkIndex int) []float64) (string, int) {
	t.Helper()
	doc, err := documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID: projectID, Type: "text", Category: "world", Title: title, ContentText: content,
	})
	if err != nil {
		t.Fatalf("CreateDocument %s: %v", title, err)
	}
	chunks, err := documents.ListChunksForEmbedding(doc.ID)
	if err != nil {
		t.Fatalf("ListChunksForEmbedding: %v", err)
	}
	rows := make([]repository.ChunkEmbedding, len(chunks))
	for i, ch := range chunks {
		rows[i] = repository.ChunkEmbedding{ChunkID: ch.ChunkID, DocumentID: doc.ID, ProjectID: projectID, Embedding: embeddingFor(ch.ChunkIndex), Model: "m"}
	}
	if err := documents.UpsertChunkEmbeddings(rows); err != nil {
		t.Fatalf("UpsertChunkEmbeddings: %v", err)
	}
	return doc.ID, len(chunks)
}

// A long document whose every chunk sits on the query vector must not take every
// semantic slot: the closest chunks of the other documents still get in.
func TestSearchKeepsDistinctDocumentsAmongSemanticCandidates(t *testing.T) {
	d := newServiceTestDB(t)
	project, err := repository.NewProjectRepository(d).CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	documents := repository.NewDocumentRepository(d)
	srv, _ := fakeEmbeddingServer(t)
	retrieval := NewRetrievalService(d, enabledEmbeddingClient(srv.URL, "m"))

	on := func(int) []float64 { return []float64{1, 0, 0} }
	near := func(int) []float64 { return []float64{0.9, 0.1, 0} }
	_, chunks := seedEmbeddedDocument(t, documents, project.ID, "長い原稿", strings.Repeat("港町の灯台は沖の船を見張る。", 900), on)
	if chunks <= 12 {
		t.Fatalf("the long document has %d chunks, want more than the 12 a 4 × 3 budget holds", chunks)
	}
	want := []string{}
	for _, title := range []string{"灯台守", "埠頭", "市場"} {
		id, _ := seedEmbeddedDocument(t, documents, project.ID, title, title+"の記録。", near)
		want = append(want, id)
	}

	refs, err := retrieval.SearchDocuments(project.ID, SearchQuery{Text: "飛竜"}, 4, 3, ScopeAll)
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}
	for _, id := range want {
		if !slices.ContainsFunc(refs, func(ref model.RetrievedDocumentReference) bool { return ref.SourceID == id }) {
			t.Fatalf("a close short document was crowded out by the long one: %v", docLabels(refs))
		}
	}
}

// A document's chunk slots go to its best chunks: a chunk only the embeddings
// found outranks the weak FTS chunks of the same document and must be kept.
func TestSearchKeepsADocumentsBestChunksAcrossCandidateSets(t *testing.T) {
	d := newServiceTestDB(t)
	project, err := repository.NewProjectRepository(d).CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	documents := repository.NewDocumentRepository(d)
	srv, _ := fakeEmbeddingServer(t)
	retrieval := NewRetrievalService(d, enabledEmbeddingClient(srv.URL, "m"))

	far := func(int) []float64 { return []float64{0, 1, 0} }
	// Chunks start every 850 runes; the term opens chunks 0-2 and is absent from
	// chunk 3, which alone sits on the query vector.
	body := []rune(strings.Repeat("、", 3400))
	for _, at := range []int{0, 850, 1700} {
		copy(body[at:], []rune("。飛竜。"))
	}
	copy(body[3000:], []rune("港町の灯台。"))
	mixedID, chunks := seedEmbeddedDocument(t, documents, project.ID, "記録", string(body), func(index int) []float64 {
		if index == 3 {
			return []float64{1, 0, 0}
		}
		return []float64{0, 1, 0}
	})
	if chunks != 4 {
		t.Fatalf("the mixed document has %d chunks, want 4", chunks)
	}
	// A strong title match tops the FTS scores, pushing the mixed document's FTS
	// chunks to the bottom of the normalized range.
	seedEmbeddedDocument(t, documents, project.ID, "飛竜", "飛竜。飛竜。飛竜。", far)

	refs, err := retrieval.SearchDocuments(project.ID, SearchQuery{Text: "飛竜"}, 4, 3, ScopeAll)
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}
	i := slices.IndexFunc(refs, func(ref model.RetrievedDocumentReference) bool { return ref.SourceID == mixedID })
	if i < 0 {
		t.Fatalf("the mixed document is missing: %v", docLabels(refs))
	}
	if !slices.ContainsFunc(refs[i].Chunks, func(c model.RetrievedDocumentChunk) bool { return c.ChunkIndex == 3 }) {
		t.Fatalf("chunk 3 (best by hybrid score) was left out of the document's chunks: %+v", refs[i].Chunks)
	}
}
