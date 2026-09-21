package service

import (
	"database/sql"
	"path/filepath"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/db"
	"snzstudio/internal/repository"
)

// newServiceTestDB opens a fresh migrated SQLite database for service tests.
func newServiceTestDB(t *testing.T) *sql.DB {
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

	refs, err := retrieval.SearchDocuments(project.ID, "竜の世界観について教えて", 4, 3, ScopeAll)
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

	refs, err := retrieval.SearchMemories(project.ID, "舞台はどこ", 4, ScopeAll)
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
