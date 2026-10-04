package service

import (
	"database/sql"
	"encoding/json"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/embed"
	"snzstudio/internal/repository"
	"snzstudio/internal/search"
)

// The corpus reproduces the condition TASK-74 is about: one ~1MB manuscript split
// into a few hundred chunks beside a handful of small notes, every chunk embedded
// with ruri-sized vectors. The query term appears only in the manuscript, so FTS
// fills its whole candidate budget from one document, SearchDocuments ranks fewer
// documents than its limit, and it falls back to the semantic scan of every chunk
// in the project. A few note chunks sit close to the query vector so that scan
// adopts documents rather than returning nothing.

const (
	benchManuscriptBytes = 1 << 20
	benchSmallDocuments  = 20
	benchSmallDocBytes   = 6 << 10
	benchCloseNotes      = 3
	benchQuery           = "飛竜騎士団"
)

var (
	benchManuscriptWords = []string{"飛竜騎士団", "王都", "北の砦", "見張り塔", "古い盟約", "第三王子", "灰色の森", "銀の鐘"}
	benchNoteWords       = []string{"港町", "商人組合", "塩の街道", "南の島", "灯台守", "漁師", "交易船", "市場"}
	benchPredicates      = []string{"を目指して進んだ。", "について語り合った。", "の噂が広まっていた。", "を守るために集まった。", "は夜明け前に静まり返っていた。"}
)

type retrievalBenchCorpus struct {
	db             *sql.DB
	projectID      string
	queryEmbedding []float64
	chunks         int
}

func benchProse(rng *rand.Rand, words []string, bytes int) string {
	var b strings.Builder
	for b.Len() < bytes {
		b.WriteString(words[rng.IntN(len(words))])
		b.WriteString(benchPredicates[rng.IntN(len(benchPredicates))])
		if rng.IntN(6) == 0 {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

// benchVector returns base plus Gaussian noise of the given scale, normalized, with
// each component passed through float32 text form — the way llama-server's
// /embeddings answer reaches the JSON column — so embedding_json has the length the
// real column has. A nil base gives an unrelated random vector.
func benchVector(rng *rand.Rand, base []float64, noise float64) []float64 {
	v := make([]float64, embed.RuriV3_30m.Dim)
	var norm float64
	for i := range v {
		v[i] = rng.NormFloat64() * noise
		if base != nil {
			v[i] += base[i]
		}
		norm += v[i] * v[i]
	}
	norm = math.Sqrt(norm)
	for i := range v {
		v[i], _ = strconv.ParseFloat(strconv.FormatFloat(v[i]/norm, 'g', -1, 32), 64)
	}
	return v
}

func seedRetrievalBenchCorpus(b *testing.B) retrievalBenchCorpus {
	b.Helper()
	rng := rand.New(rand.NewPCG(74, 74))
	d := newServiceTestDB(b)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)

	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Bench"})
	if err != nil {
		b.Fatalf("CreateProject: %v", err)
	}
	if _, err := documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID: project.ID, Type: "markdown", Category: "story",
		Title: "原稿", ContentText: benchProse(rng, benchManuscriptWords, benchManuscriptBytes),
	}); err != nil {
		b.Fatalf("CreateDocument manuscript: %v", err)
	}
	closeNotes := map[string]bool{}
	for i := 0; i < benchSmallDocuments; i++ {
		note, err := documents.CreateDocument(repository.CreateDocumentInput{
			ProjectID: project.ID, Type: "markdown", Category: "world",
			Title: "メモ " + strconv.Itoa(i), ContentText: benchProse(rng, benchNoteWords, benchSmallDocBytes),
		})
		if err != nil {
			b.Fatalf("CreateDocument note: %v", err)
		}
		if i < benchCloseNotes {
			closeNotes[note.ID] = true
		}
	}

	query := benchVector(rng, nil, 1)
	chunks, err := documents.ListChunksForEmbedding("")
	if err != nil {
		b.Fatalf("ListChunksForEmbedding: %v", err)
	}
	rows := make([]repository.ChunkEmbedding, len(chunks))
	for i, c := range chunks {
		embedding := benchVector(rng, nil, 1)
		if closeNotes[c.DocumentID] && c.ChunkIndex == 0 {
			embedding = benchVector(rng, query, 0.02)
		}
		rows[i] = repository.ChunkEmbedding{ChunkID: c.ChunkID, DocumentID: c.DocumentID, ProjectID: c.ProjectID, Embedding: embedding, Model: embed.RuriV3_30m.ModelID}
	}
	if err := documents.UpsertChunkEmbeddings(rows); err != nil {
		b.Fatalf("UpsertChunkEmbeddings: %v", err)
	}
	return retrievalBenchCorpus{db: d, projectID: project.ID, queryEmbedding: query, chunks: len(chunks)}
}

// benchRetrieval wires a RetrievalService to an embedding endpoint that answers the
// corpus' query vector for every input, under the ruri model id so the ruri
// prefixes and fallback floor apply as they do in the app.
func benchRetrieval(b *testing.B, corpus retrievalBenchCorpus) *RetrievalService {
	b.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data := make([]map[string]any, len(body.Input))
		for i := range body.Input {
			data[i] = map[string]any{"embedding": corpus.queryEmbedding}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	b.Cleanup(srv.Close)
	settings := config.Settings{EmbeddingTimeoutMs: 5000}
	settings.EmbeddingBaseURL = srv.URL
	settings.EmbeddingModel = embed.RuriV3_30m.ModelID
	return NewRetrievalService(corpus.db, NewEmbeddingClient(testConfig(settings)))
}

func BenchmarkRetrievalLargeDocument(b *testing.B) {
	corpus := seedRetrievalBenchCorpus(b)
	retrieval := benchRetrieval(b, corpus)
	b.Logf("corpus: %d chunks in project", corpus.chunks)

	b.Run("FtsCandidates", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			rows, err := retrieval.fetchDocumentFtsCandidates(corpus.projectID, search.ToFtsQuery(benchQuery), 4, 3, ScopeAll)
			if err != nil || len(rows) == 0 {
				b.Fatalf("fetchDocumentFtsCandidates: %d rows, %v", len(rows), err)
			}
		}
	})

	b.Run("SemanticFallback", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			refs, err := retrieval.searchDocumentsBySemantic(corpus.projectID, corpus.queryEmbedding, 4, 3, map[string]bool{}, "general", ScopeAll)
			if err != nil || len(refs) == 0 {
				b.Fatalf("searchDocumentsBySemantic: %d refs, %v", len(refs), err)
			}
		}
	})

	b.Run("SearchDocuments", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			refs, err := retrieval.SearchDocuments(corpus.projectID, benchQuery, 4, 3, ScopeAll)
			if err != nil || len(refs) != 4 {
				b.Fatalf("SearchDocuments: %d refs, %v", len(refs), err)
			}
		}
	})
}
