package repository

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"snzstudio/internal/db"
	"snzstudio/internal/model"
	"snzstudio/internal/search"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// tick advances wall-clock past the millisecond resolution of NowISO so that
// rows created in sequence get strictly increasing timestamps for ORDER BY.
func tick() { time.Sleep(2 * time.Millisecond) }

// ftsDocIDs returns the distinct document IDs matching term in the document FTS.
func ftsDocIDs(t *testing.T, d *sql.DB, term string) map[string]bool {
	t.Helper()
	query := search.ToFtsQuery(term)
	if query == "" {
		t.Fatalf("ToFtsQuery(%q) returned empty", term)
	}
	rows, err := d.Query("SELECT DISTINCT document_id FROM document_chunks_fts WHERE document_chunks_fts MATCH ?", query)
	if err != nil {
		t.Fatalf("fts query: %v", err)
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[id] = true
	}
	return ids
}

func ftsMemoryIDs(t *testing.T, d *sql.DB, term string) map[string]bool {
	t.Helper()
	query := search.ToFtsQuery(term)
	if query == "" {
		t.Fatalf("ToFtsQuery(%q) returned empty", term)
	}
	rows, err := d.Query("SELECT memory_id FROM memories_fts WHERE memories_fts MATCH ?", query)
	if err != nil {
		t.Fatalf("fts query: %v", err)
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[id] = true
	}
	return ids
}

func TestProjectRepository(t *testing.T) {
	d := newTestDB(t)
	repo := NewProjectRepository(d)

	p1, err := repo.CreateProject(CreateProjectInput{Title: "  First  ", Description: " desc ", SystemPrompt: " sp "})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p1.Title != "First" || p1.Description != "desc" || p1.SystemPrompt != "sp" {
		t.Errorf("create did not trim: %+v", p1)
	}
	if p1.SortOrder != 0 || p1.ChatCount != 0 {
		t.Errorf("p1 sortOrder=%d chatCount=%d, want 0/0", p1.SortOrder, p1.ChatCount)
	}
	tick()
	p2, err := repo.CreateProject(CreateProjectInput{Title: "Second"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p2.SortOrder != 1 {
		t.Errorf("p2 sortOrder=%d, want 1", p2.SortOrder)
	}

	list, err := repo.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(list) != 2 || list[0].ID != p1.ID || list[1].ID != p2.ID {
		t.Errorf("ListProjects order wrong: %+v", list)
	}

	// chat_count is derived from joined chats.
	chatRepo := NewChatRepository(d)
	if _, err := chatRepo.CreateChat(CreateChatInput{ProjectID: p1.ID, Title: "c1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := chatRepo.CreateChat(CreateChatInput{ProjectID: p1.ID, Title: "c2"}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetProject(p1.ID)
	if err != nil || got == nil {
		t.Fatalf("GetProject: %v, %v", got, err)
	}
	if got.ChatCount != 2 {
		t.Errorf("chatCount=%d, want 2", got.ChatCount)
	}

	if missing, err := repo.GetProject("nope"); err != nil || missing != nil {
		t.Errorf("GetProject(missing) = %v, %v; want nil, nil", missing, err)
	}

	updated, err := repo.UpdateProjectTitle(p2.ID, "  Renamed ")
	if err != nil || updated == nil || updated.Title != "Renamed" {
		t.Errorf("UpdateProjectTitle = %v, %v", updated, err)
	}
	if nilp, err := repo.UpdateProjectSystemPrompt("nope", "x"); err != nil || nilp != nil {
		t.Errorf("UpdateProjectSystemPrompt(missing) = %v, %v; want nil, nil", nilp, err)
	}

	// Reorder: reverse, then verify sort order, and that a bad set is rejected.
	reordered, err := repo.ReorderProjects([]string{p2.ID, p1.ID})
	if err != nil {
		t.Fatalf("ReorderProjects: %v", err)
	}
	if reordered[0].ID != p2.ID || reordered[0].SortOrder != 0 || reordered[1].ID != p1.ID || reordered[1].SortOrder != 1 {
		t.Errorf("reorder result wrong: %+v", reordered)
	}
	if _, err := repo.ReorderProjects([]string{p1.ID, "ghost"}); err != ErrProjectReorderMismatch {
		t.Errorf("ReorderProjects(bad) err = %v, want ErrProjectReorderMismatch", err)
	}

	// Delete cascades chats.
	ok, err := repo.DeleteProject(p1.ID)
	if err != nil || !ok {
		t.Fatalf("DeleteProject = %v, %v", ok, err)
	}
	if again, err := repo.DeleteProject(p1.ID); err != nil || again {
		t.Errorf("DeleteProject(again) = %v, %v; want false, nil", again, err)
	}
	chats, err := chatRepo.ListByProject(p1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 0 {
		t.Errorf("chats not cascaded: %d remain", len(chats))
	}
}

func TestDocumentRepositoryCRUDAndFTS(t *testing.T) {
	d := newTestDB(t)
	projects := NewProjectRepository(d)
	docs := NewDocumentRepository(d)

	proj, err := projects.CreateProject(CreateProjectInput{Title: "P"})
	if err != nil {
		t.Fatal(err)
	}

	doc, err := docs.CreateDocument(CreateDocumentInput{
		ProjectID:   proj.ID,
		Type:        "text",
		Title:       "東京案内",
		Note:        "観光メモ",
		Tags:        []string{"旅行", "東京"},
		ContentText: "東京タワーは美しい建物です。観光客で賑わいます。",
	})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if doc.Category == "" {
		t.Error("category not inferred/set")
	}
	if len(doc.Tags) != 2 {
		t.Errorf("tags = %v", doc.Tags)
	}

	// The document is findable via the pre-tokenized FTS index.
	if ids := ftsDocIDs(t, d, "東京"); !ids[doc.ID] {
		t.Errorf("FTS search for 東京 did not find doc %s (got %v)", doc.ID, ids)
	}
	if ids := ftsDocIDs(t, d, "大阪"); ids[doc.ID] {
		t.Errorf("FTS search for 大阪 unexpectedly found doc %s", doc.ID)
	}

	// Empty-body document still indexed on its title metadata.
	tick()
	empty, err := docs.CreateDocument(CreateDocumentInput{ProjectID: proj.ID, Type: "text", Title: "空の資料"})
	if err != nil {
		t.Fatal(err)
	}
	var chunkCount int
	if err := d.QueryRow("SELECT COUNT(*) FROM document_chunks WHERE document_id = ?", empty.ID).Scan(&chunkCount); err != nil {
		t.Fatal(err)
	}
	if chunkCount != 1 {
		t.Errorf("empty doc chunk count = %d, want 1", chunkCount)
	}
	if ids := ftsDocIDs(t, d, "資料"); !ids[empty.ID] {
		t.Errorf("empty doc not findable by title term")
	}

	// List newest first.
	list, err := docs.ListByProject(proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != empty.ID || list[1].ID != doc.ID {
		t.Errorf("ListByProject order wrong: %+v", list)
	}

	if missing, err := docs.GetDocument("nope"); err != nil || missing != nil {
		t.Errorf("GetDocument(missing) = %v, %v", missing, err)
	}

	// Category update.
	cat, err := docs.UpdateDocumentCategory(doc.ID, "world")
	if err != nil || cat == nil || cat.Category != "world" {
		t.Errorf("UpdateDocumentCategory = %v, %v", cat, err)
	}

	// chunks for embedding + embedding upsert round-trip.
	chunks, err := docs.ListChunksForEmbedding(doc.ID)
	if err != nil || len(chunks) == 0 {
		t.Fatalf("ListChunksForEmbedding = %d chunks, %v", len(chunks), err)
	}
	if err := docs.UpsertChunkEmbeddings([]ChunkEmbedding{{
		ChunkID: chunks[0].ChunkID, DocumentID: doc.ID, ProjectID: proj.ID, Embedding: []float64{0.1, 0.2}, Model: "test",
	}}); err != nil {
		t.Fatalf("UpsertChunkEmbeddings: %v", err)
	}
	var embJSON string
	if err := d.QueryRow("SELECT embedding_json FROM document_chunk_embeddings WHERE chunk_id = ?", chunks[0].ChunkID).Scan(&embJSON); err != nil {
		t.Fatalf("embedding not stored: %v", err)
	}
	if embJSON != "[0.1,0.2]" {
		t.Errorf("embedding_json = %q", embJSON)
	}

	// Rebuild index keeps the document searchable.
	if err := docs.RebuildSearchIndex(); err != nil {
		t.Fatalf("RebuildSearchIndex: %v", err)
	}
	if ids := ftsDocIDs(t, d, "東京"); !ids[doc.ID] {
		t.Error("doc not findable after RebuildSearchIndex")
	}

	// Delete removes chunks + fts rows.
	deleted, err := docs.DeleteDocument(doc.ID)
	if err != nil || deleted == nil {
		t.Fatalf("DeleteDocument = %v, %v", deleted, err)
	}
	if err := d.QueryRow("SELECT COUNT(*) FROM document_chunks WHERE document_id = ?", doc.ID).Scan(&chunkCount); err != nil {
		t.Fatal(err)
	}
	if chunkCount != 0 {
		t.Errorf("chunks remain after delete: %d", chunkCount)
	}
	var ftsCount int
	if err := d.QueryRow("SELECT COUNT(*) FROM document_chunks_fts WHERE document_id = ?", doc.ID).Scan(&ftsCount); err != nil {
		t.Fatal(err)
	}
	if ftsCount != 0 {
		t.Errorf("fts rows remain after delete: %d", ftsCount)
	}
	if nilDoc, err := docs.DeleteDocument(doc.ID); err != nil || nilDoc != nil {
		t.Errorf("DeleteDocument(again) = %v, %v", nilDoc, err)
	}
}

func TestDocumentBackfillInferredCategories(t *testing.T) {
	d := newTestDB(t)
	projects := NewProjectRepository(d)
	docs := NewDocumentRepository(d)
	proj, err := projects.CreateProject(CreateProjectInput{Title: "P"})
	if err != nil {
		t.Fatal(err)
	}

	// Insert a raw "misc" document (updated_at == created_at) whose body should
	// classify as world, bypassing CreateDocument's inference.
	const ts = "2026-01-01T00:00:00.000Z"
	if _, err := d.Exec(`
		INSERT INTO documents (id, project_id, type, category, title, note, derived_text, content_text, created_at, updated_at)
		VALUES ('docX', ?, 'text', 'misc', '設定資料', '', '', '世界設定と魔法体系の詳細', ?, ?)`,
		proj.ID, ts, ts); err != nil {
		t.Fatal(err)
	}

	if err := docs.BackfillInferredCategories(); err != nil {
		t.Fatalf("BackfillInferredCategories: %v", err)
	}
	got, err := docs.GetDocument("docX")
	if err != nil || got == nil {
		t.Fatalf("GetDocument: %v, %v", got, err)
	}
	if got.Category != "world" {
		t.Errorf("category = %q, want world", got.Category)
	}
}

func TestMemoryRepository(t *testing.T) {
	d := newTestDB(t)
	projects := NewProjectRepository(d)
	memories := NewMemoryRepository(d)
	proj, err := projects.CreateProject(CreateProjectInput{Title: "P"})
	if err != nil {
		t.Fatal(err)
	}

	m1, err := memories.CreateMemory(CreateMemoryInput{
		ProjectID: proj.ID, Kind: "semantic", Title: "主人公の名前", Content: "主人公はサクラと呼ばれている",
	})
	if err != nil {
		t.Fatalf("CreateMemory: %v", err)
	}
	if m1.Source != "manual" || m1.Locked {
		t.Errorf("defaults wrong: source=%q locked=%v", m1.Source, m1.Locked)
	}
	if ids := ftsMemoryIDs(t, d, "サクラ"); !ids[m1.ID] {
		t.Errorf("memory not findable by content term")
	}

	tick()
	m2, err := memories.CreateMemory(CreateMemoryInput{
		ProjectID: proj.ID, Kind: "procedural", Title: "文体ルール", Content: "一人称で書くこと", Locked: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Locked memory sorts first.
	list, err := memories.ListByProject(proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != m2.ID {
		t.Errorf("ListByProject locked-first wrong: %+v", list)
	}

	kinded, err := memories.ListByProjectAndKind(proj.ID, "semantic")
	if err != nil {
		t.Fatal(err)
	}
	if len(kinded) != 1 || kinded[0].ID != m1.ID {
		t.Errorf("ListByProjectAndKind wrong: %+v", kinded)
	}

	// Update without Locked keeps existing locked=true (COALESCE) and refreshes FTS.
	upd, err := memories.UpdateMemory(UpdateMemoryInput{MemoryID: m2.ID, Kind: "procedural", Title: "文体ルール", Content: "三人称で書くこと"})
	if err != nil || upd == nil {
		t.Fatalf("UpdateMemory: %v, %v", upd, err)
	}
	if !upd.Locked {
		t.Error("UpdateMemory without Locked cleared locked flag")
	}
	if ids := ftsMemoryIDs(t, d, "三人称"); !ids[m2.ID] {
		t.Error("updated content not searchable")
	}
	if ids := ftsMemoryIDs(t, d, "一人称"); ids[m2.ID] {
		t.Error("stale content still searchable after update")
	}

	// A rewrite takes the memory out of the common project material, whatever it
	// was before: the organizer folds other memories in, and the content is no
	// longer the content the human agreed to share (design §4.4).
	m3, err := memories.CreateMemory(CreateMemoryInput{
		ProjectID: proj.ID, Kind: "semantic", Title: "港の掟", Content: "霧笛が三度鳴ったら船を舫う",
	})
	if err != nil {
		t.Fatal(err)
	}
	shared, err := memories.SetMemorySharedWithAll(m3.ID, true)
	if err != nil || shared == nil || !shared.SharedWithAll {
		t.Fatalf("SetMemorySharedWithAll: %+v, %v", shared, err)
	}
	rewritten, err := memories.UpdateMemory(UpdateMemoryInput{
		MemoryID: m3.ID, Kind: "semantic", Title: "港の掟", Content: "霧笛が三度鳴ったら船を舫う。合図を決めたのは灯台守である。",
	})
	if err != nil || rewritten == nil {
		t.Fatalf("UpdateMemory rewrite: %v, %v", rewritten, err)
	}
	if rewritten.SharedWithAll {
		t.Error("a rewritten memory must leave the common project material until it is shared again")
	}

	// Explicit unlock.
	unlocked, err := memories.SetMemoryLocked(m2.ID, false)
	if err != nil || unlocked == nil || unlocked.Locked {
		t.Errorf("SetMemoryLocked = %v, %v", unlocked, err)
	}
	if nilm, err := memories.SetMemoryLocked("nope", true); err != nil || nilm != nil {
		t.Errorf("SetMemoryLocked(missing) = %v, %v", nilm, err)
	}

	// hasSimilarMemory: ASCII case-insensitive + Japanese exact.
	if _, err := memories.CreateMemory(CreateMemoryInput{ProjectID: proj.ID, Kind: "semantic", Title: "Hello", Content: "World"}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := memories.HasSimilarMemory(proj.ID, "hello", "world"); !ok {
		t.Error("HasSimilarMemory ASCII case-insensitive failed")
	}
	if ok, _ := memories.HasSimilarMemory(proj.ID, "主人公の名前", "主人公はサクラと呼ばれている"); !ok {
		t.Error("HasSimilarMemory Japanese exact failed")
	}
	if ok, _ := memories.HasSimilarMemory(proj.ID, "違う", "ない"); ok {
		t.Error("HasSimilarMemory false positive")
	}

	// Embeddings round-trip.
	forEmb, err := memories.ListForEmbedding(nil)
	if err != nil || len(forEmb) == 0 {
		t.Fatalf("ListForEmbedding(all) = %d, %v", len(forEmb), err)
	}
	subset, err := memories.ListForEmbedding([]string{m1.ID})
	if err != nil || len(subset) != 1 || subset[0].ID != m1.ID {
		t.Errorf("ListForEmbedding(subset) = %+v, %v", subset, err)
	}
	if err := memories.UpsertMemoryEmbeddings([]MemoryEmbedding{{MemoryID: m1.ID, ProjectID: proj.ID, Kind: "semantic", Embedding: []float64{1}, Model: "test"}}); err != nil {
		t.Fatalf("UpsertMemoryEmbeddings: %v", err)
	}

	// Rebuild keeps search working.
	if err := memories.RebuildSearchIndex(); err != nil {
		t.Fatalf("RebuildSearchIndex: %v", err)
	}
	if ids := ftsMemoryIDs(t, d, "サクラ"); !ids[m1.ID] {
		t.Error("memory not searchable after rebuild")
	}

	// Delete removes FTS row.
	if del, err := memories.DeleteMemory(m1.ID); err != nil || del == nil {
		t.Fatalf("DeleteMemory = %v, %v", del, err)
	}
	if ids := ftsMemoryIDs(t, d, "サクラ"); ids[m1.ID] {
		t.Error("memory still in FTS after delete")
	}
	if nilm, err := memories.DeleteMemory(m1.ID); err != nil || nilm != nil {
		t.Errorf("DeleteMemory(again) = %v, %v", nilm, err)
	}
}

func TestChatRepository(t *testing.T) {
	d := newTestDB(t)
	projects := NewProjectRepository(d)
	chats := NewChatRepository(d)
	proj, err := projects.CreateProject(CreateProjectInput{Title: "P"})
	if err != nil {
		t.Fatal(err)
	}

	chat, err := chats.CreateChat(CreateChatInput{ProjectID: proj.ID, Title: " My Chat "})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if chat.Title != "My Chat" || chat.IsTemporary {
		t.Errorf("create chat = %+v", chat)
	}
	// An empty summary row is seeded.
	sum, err := chats.GetSummary(chat.ID)
	if err != nil || sum == nil || sum.Summary != "" {
		t.Errorf("seeded summary = %v, %v", sum, err)
	}

	// Streaming: empty assistant message -> content update -> finalize metrics.
	asst, err := chats.AddMessage(AddMessageInput{ChatID: chat.ID, Role: "assistant", Content: ""})
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	if _, err := chats.UpdateMessageContent(asst.ID, "partial"); err != nil {
		t.Fatal(err)
	}
	respMs := int64(1200)
	outTok := int64(42)
	tps := 35.0
	model := "gpt-test"
	fin, err := chats.FinalizeMessage(FinalizeMessageInput{
		MessageID: asst.ID, Content: "final answer", ResponseMs: &respMs, OutputTokens: &outTok, TokensPerSecond: &tps, ModelName: &model,
	})
	if err != nil || fin == nil {
		t.Fatalf("FinalizeMessage: %v, %v", fin, err)
	}
	if fin.Content != "final answer" || fin.ResponseMs == nil || *fin.ResponseMs != 1200 || fin.ModelName == nil || *fin.ModelName != "gpt-test" {
		t.Errorf("finalize result = %+v", fin)
	}

	// COALESCE: finalize again with nil ModelName keeps the existing model_name.
	fin2, err := chats.FinalizeMessage(FinalizeMessageInput{MessageID: asst.ID, Content: "v2"})
	if err != nil || fin2 == nil {
		t.Fatalf("FinalizeMessage(2): %v, %v", fin2, err)
	}
	if fin2.ModelName == nil || *fin2.ModelName != "gpt-test" {
		t.Errorf("model_name not preserved by COALESCE: %+v", fin2.ModelName)
	}
	if fin2.ResponseMs != nil {
		t.Errorf("responseMs should be reset to NULL, got %v", *fin2.ResponseMs)
	}

	if missing, err := chats.GetMessage("nope"); err != nil || missing != nil {
		t.Errorf("GetMessage(missing) = %v, %v", missing, err)
	}

	// listRecentMessages returns chronological order with a limit.
	tick()
	if _, err := chats.AddMessage(AddMessageInput{ChatID: chat.ID, Role: "user", Content: "u1"}); err != nil {
		t.Fatal(err)
	}
	tick()
	last, err := chats.AddMessage(AddMessageInput{ChatID: chat.ID, Role: "user", Content: "u2"})
	if err != nil {
		t.Fatal(err)
	}
	recent, err := chats.ListRecentMessages(chat.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[1].ID != last.ID {
		t.Errorf("ListRecentMessages chronological tail wrong: %+v", recent)
	}

	all, err := chats.ListMessages(chat.ID)
	if err != nil || len(all) != 3 {
		t.Fatalf("ListMessages = %d, %v", len(all), err)
	}
	if all[0].ID != asst.ID {
		t.Errorf("ListMessages not chronological: %+v", all)
	}

	// References attach to the assistant message, ordered by score desc.
	if err := chats.ReplaceAssistantReferences(asst.ID, []ReferenceInput{
		{SourceType: "document", SourceID: "d1", Label: "low", Excerpt: "e", Score: 0.2},
		{SourceType: "memory", SourceID: "m1", Label: "high", Excerpt: "e", Score: 0.9},
	}); err != nil {
		t.Fatalf("ReplaceAssistantReferences: %v", err)
	}
	withRefs, err := chats.GetMessagesWithReferences(chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range withRefs {
		if m.ID == asst.ID {
			if len(m.References) != 2 || m.References[0].Label != "high" {
				t.Errorf("references order/grouping wrong: %+v", m.References)
			}
		} else if m.References == nil {
			t.Errorf("non-assistant message should have empty (non-nil) references")
		}
	}
	// Replacing references clears the old set.
	if err := chats.ReplaceAssistantReferences(asst.ID, nil); err != nil {
		t.Fatal(err)
	}
	var refCount int
	if err := d.QueryRow("SELECT COUNT(*) FROM assistant_message_references WHERE assistant_message_id = ?", asst.ID).Scan(&refCount); err != nil {
		t.Fatal(err)
	}
	if refCount != 0 {
		t.Errorf("references not cleared: %d", refCount)
	}

	// Title / temporary updates and the summary temporary filter.
	if _, err := chats.UpdateChatTitle(chat.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	tempChat, err := chats.CreateChat(CreateChatInput{ProjectID: proj.ID, Title: "temp", IsTemporary: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated, err := chats.SetTemporary(chat.ID, false); err != nil || updated == nil || updated.IsTemporary {
		t.Errorf("SetTemporary = %v, %v", updated, err)
	}
	if err := chats.UpsertSummary(chat.ID, "s1"); err != nil {
		t.Fatal(err)
	}
	if err := chats.UpsertSummary(tempChat.ID, "s2"); err != nil {
		t.Fatal(err)
	}
	withTemp, err := chats.ListSummariesByProject(proj.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(withTemp) != 2 {
		t.Errorf("ListSummariesByProject(include) = %d, want 2", len(withTemp))
	}
	withoutTemp, err := chats.ListSummariesByProject(proj.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(withoutTemp) != 1 || withoutTemp[0].ChatID != chat.ID {
		t.Errorf("ListSummariesByProject(exclude) = %+v, want only non-temporary chat", withoutTemp)
	}

	// Delete cascades messages.
	if del, err := chats.DeleteChat(chat.ID); err != nil || del == nil {
		t.Fatalf("DeleteChat = %v, %v", del, err)
	}
	msgs, err := chats.ListMessages(chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Errorf("messages not cascaded: %d", len(msgs))
	}
	if nilc, err := chats.DeleteChat(chat.ID); err != nil || nilc != nil {
		t.Errorf("DeleteChat(again) = %v, %v", nilc, err)
	}
}

func TestParticipantRepository(t *testing.T) {
	d := newTestDB(t)
	projects := NewProjectRepository(d)
	chats := NewChatRepository(d)
	participants := NewParticipantRepository(d)

	proj, err := projects.CreateProject(CreateProjectInput{Title: "P"})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := chats.CreateChat(CreateChatInput{
		ProjectID: proj.ID, Title: "Debate", Kind: model.ChatKindMultiAgent, ScenePrompt: "論題: AI",
	})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if chat.Kind != model.ChatKindMultiAgent || chat.TurnRule != model.TurnRuleRoundRobin || chat.ScenePrompt != "論題: AI" {
		t.Errorf("create multi-agent chat = %+v", chat)
	}
	// A chat created without the multi-agent fields stays the single-assistant kind.
	plain, err := chats.CreateChat(CreateChatInput{ProjectID: proj.ID, Title: "Plain"})
	if err != nil {
		t.Fatal(err)
	}
	if plain.Kind != model.ChatKindAssistant || plain.TurnRule != model.TurnRuleRoundRobin {
		t.Errorf("default chat kind/turn rule = %q/%q", plain.Kind, plain.TurnRule)
	}

	pro, err := participants.CreateParticipant(CreateParticipantInput{
		ChatID: chat.ID, DisplayName: " 賛成派 ", RolePrompt: " you argue for ", BaseURL: " http://localhost:1234/v1 ", ModelName: " model-a ",
	})
	if err != nil {
		t.Fatalf("CreateParticipant: %v", err)
	}
	if pro.DisplayName != "賛成派" || pro.RolePrompt != "you argue for" || pro.BaseURL != "http://localhost:1234/v1" || pro.ModelName != "model-a" {
		t.Errorf("create trims fields: %+v", pro)
	}
	if pro.SortOrder != 0 || pro.DeletedAt != nil {
		t.Errorf("first participant = sort %d, deletedAt %v", pro.SortOrder, pro.DeletedAt)
	}
	// TASK-20 AC #1: a participant created without saying anything about the
	// project material reads it.
	if !pro.ReceivesProjectMaterial {
		t.Error("a participant created without receivesProjectMaterial must receive the project material")
	}
	con, err := participants.CreateParticipant(CreateParticipantInput{
		ChatID: chat.ID, DisplayName: "反対派", RolePrompt: "you argue against", BaseURL: "http://localhost:1235/v1", ModelName: "model-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if con.SortOrder != 1 {
		t.Errorf("second participant sort_order = %d, want 1", con.SortOrder)
	}

	roster, err := participants.ListRoster(chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 2 || roster[0].ID != pro.ID || roster[1].ID != con.ID {
		t.Errorf("roster order = %+v", roster)
	}

	// A participant of another chat must be distinguishable by chat_id alone, so
	// that the service layer can reject one named under the wrong chat.
	other, err := participants.CreateParticipant(CreateParticipantInput{
		ChatID: plain.ID, DisplayName: "余所者", RolePrompt: "r", BaseURL: "http://localhost:1236/v1", ModelName: "model-c",
	})
	if err != nil {
		t.Fatal(err)
	}
	fetched, err := participants.GetParticipant(other.ID)
	if err != nil || fetched == nil {
		t.Fatalf("GetParticipant = %v, %v", fetched, err)
	}
	if fetched.ChatID != plain.ID {
		t.Errorf("GetParticipant chat_id = %q, want %q", fetched.ChatID, plain.ID)
	}
	if inRoster, err := participants.ListRoster(chat.ID); err != nil {
		t.Fatal(err)
	} else if len(inRoster) != 2 {
		t.Errorf("another chat's participant leaked into the roster: %+v", inRoster)
	}

	// Partial update: the named fields change (trimmed), the rest is kept.
	name := "  賛成派 (改)  "
	order := 5
	updated, err := participants.UpdateParticipant(UpdateParticipantInput{
		ParticipantID: pro.ID, DisplayName: &name, SortOrder: &order,
	})
	if err != nil || updated == nil {
		t.Fatalf("UpdateParticipant = %v, %v", updated, err)
	}
	if updated.DisplayName != "賛成派 (改)" || updated.SortOrder != 5 {
		t.Errorf("update applied wrongly: %+v", updated)
	}
	if updated.RolePrompt != pro.RolePrompt || updated.ModelName != pro.ModelName || updated.ChatID != chat.ID {
		t.Errorf("update clobbered untouched fields: %+v", updated)
	}
	if !updated.ReceivesProjectMaterial {
		t.Error("an update that does not name receivesProjectMaterial must leave it alone")
	}

	// TASK-20 AC #1: the flag is settable both ways, and creating with it false
	// is what a preset needs.
	receives := false
	if updated, err = participants.UpdateParticipant(UpdateParticipantInput{
		ParticipantID: pro.ID, ReceivesProjectMaterial: &receives,
	}); err != nil || updated == nil {
		t.Fatalf("UpdateParticipant(receivesProjectMaterial): %v, %v", updated, err)
	}
	if updated.ReceivesProjectMaterial || updated.DisplayName != "賛成派 (改)" {
		t.Errorf("receivesProjectMaterial not cleared, or the update clobbered a neighbour: %+v", updated)
	}
	if reread, err := participants.GetParticipant(pro.ID); err != nil || reread == nil {
		t.Fatalf("GetParticipant: %v, %v", reread, err)
	} else if reread.ReceivesProjectMaterial {
		t.Error("receivesProjectMaterial false did not survive a re-read")
	}
	receives = true
	if updated, err = participants.UpdateParticipant(UpdateParticipantInput{
		ParticipantID: pro.ID, ReceivesProjectMaterial: &receives,
	}); err != nil || updated == nil || !updated.ReceivesProjectMaterial {
		t.Fatalf("receivesProjectMaterial not restored: %v, %v", updated, err)
	}
	// Created in the other chat, so the roster of this one keeps the shape the
	// removal assertions below count on.
	quiet := false
	silent, err := participants.CreateParticipant(CreateParticipantInput{
		ChatID: plain.ID, DisplayName: "耳役", ReceivesProjectMaterial: &quiet,
	})
	if err != nil {
		t.Fatal(err)
	}
	if silent.ReceivesProjectMaterial {
		t.Errorf("create ignored receivesProjectMaterial: %+v", silent)
	}
	// sort_order now decides the cycle order.
	if roster, err := participants.ListRoster(chat.ID); err != nil {
		t.Fatal(err)
	} else if roster[0].ID != con.ID {
		t.Errorf("roster not reordered by sort_order: %+v", roster)
	}

	// A participant's turn is stored with its participant_id.
	turn, err := chats.AddMessage(AddMessageInput{
		ChatID: chat.ID, Role: "assistant", Content: "賛成です", ModelName: &pro.ModelName, ParticipantID: &pro.ID,
	})
	if err != nil {
		t.Fatalf("AddMessage(participant turn): %v", err)
	}
	if turn.ParticipantID == nil || *turn.ParticipantID != pro.ID {
		t.Errorf("participant_id not stored: %+v", turn.ParticipantID)
	}
	human, err := chats.AddMessage(AddMessageInput{ChatID: chat.ID, Role: "user", Content: "野次"})
	if err != nil {
		t.Fatal(err)
	}
	if human.ParticipantID != nil {
		t.Errorf("human message should have a nil participant_id, got %v", *human.ParticipantID)
	}

	// Removal from the roster is logical: the row and its past turn survive.
	removed, err := participants.RemoveParticipant(pro.ID)
	if err != nil || removed == nil {
		t.Fatalf("RemoveParticipant = %v, %v", removed, err)
	}
	if removed.DeletedAt == nil {
		t.Fatal("RemoveParticipant left deleted_at NULL")
	}
	roster, err = participants.ListRoster(chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 1 || roster[0].ID != con.ID {
		t.Errorf("removed participant still on the roster: %+v", roster)
	}
	all, err := participants.ListAll(chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("ListAll = %d rows, want both participants", len(all))
	}
	// The past turn still resolves to a display name and model name, and the
	// roster still offers a starting point for the next round_robin turn.
	byID := map[string]model.Participant{}
	for _, p := range all {
		byID[p.ID] = p
	}
	speaker, ok := byID[*turn.ParticipantID]
	if !ok || speaker.DisplayName != "賛成派 (改)" || speaker.ModelName != "model-a" {
		t.Errorf("removed speaker unresolvable from ListAll: %+v", speaker)
	}
	if len(roster) == 0 {
		t.Error("round_robin has no roster left to advance from")
	}
	// Removing twice keeps the original timestamp (idempotent).
	again, err := participants.RemoveParticipant(pro.ID)
	if err != nil || again == nil {
		t.Fatalf("RemoveParticipant(again) = %v, %v", again, err)
	}
	if *again.DeletedAt != *removed.DeletedAt {
		t.Errorf("deleted_at rewritten on repeat: %q -> %q", *removed.DeletedAt, *again.DeletedAt)
	}
	// A removed participant is still reachable by id, with its chat_id.
	got, err := participants.GetParticipant(pro.ID)
	if err != nil || got == nil || got.ChatID != chat.ID || got.DeletedAt == nil {
		t.Errorf("GetParticipant(removed) = %+v, %v", got, err)
	}
	// ...and still updatable, so the service layer decides the policy, not this layer.
	newModel := "model-a2"
	if upd, err := participants.UpdateParticipant(UpdateParticipantInput{ParticipantID: pro.ID, ModelName: &newModel}); err != nil || upd == nil || upd.ModelName != "model-a2" {
		t.Errorf("UpdateParticipant(removed) = %+v, %v", upd, err)
	}

	// Missing ids report absence rather than an error.
	if missing, err := participants.GetParticipant("nope"); err != nil || missing != nil {
		t.Errorf("GetParticipant(missing) = %v, %v", missing, err)
	}
	if upd, err := participants.UpdateParticipant(UpdateParticipantInput{ParticipantID: "nope", ModelName: &newModel}); err != nil || upd != nil {
		t.Errorf("UpdateParticipant(missing) = %v, %v", upd, err)
	}
	if rm, err := participants.RemoveParticipant("nope"); err != nil || rm != nil {
		t.Errorf("RemoveParticipant(missing) = %v, %v", rm, err)
	}

	// Turn rule and scene prompt update independently.
	manual := model.TurnRuleManual
	settings, err := chats.UpdateMultiAgentSettings(chat.ID, MultiAgentSettings{TurnRule: &manual})
	if err != nil || settings == nil {
		t.Fatalf("UpdateMultiAgentSettings = %v, %v", settings, err)
	}
	if settings.TurnRule != model.TurnRuleManual || settings.ScenePrompt != "論題: AI" {
		t.Errorf("turn rule update clobbered the scene prompt: %+v", settings)
	}
	scene := "論題: 猫"
	settings, err = chats.UpdateMultiAgentSettings(chat.ID, MultiAgentSettings{ScenePrompt: &scene})
	if err != nil || settings == nil {
		t.Fatalf("UpdateMultiAgentSettings(scene) = %v, %v", settings, err)
	}
	if settings.TurnRule != model.TurnRuleManual || settings.ScenePrompt != scene {
		t.Errorf("scene prompt update = %+v", settings)
	}
	if nilc, err := chats.UpdateMultiAgentSettings("nope", MultiAgentSettings{TurnRule: &manual}); err != nil || nilc != nil {
		t.Errorf("UpdateMultiAgentSettings(missing) = %v, %v", nilc, err)
	}

	// Deleting the chat takes its participants with it.
	if _, err := chats.DeleteChat(chat.ID); err != nil {
		t.Fatal(err)
	}
	if rows, err := participants.ListAll(chat.ID); err != nil || len(rows) != 0 {
		t.Errorf("participants not cascaded: %d rows, %v", len(rows), err)
	}
}

// documentIndex returns a document's chunk contents and FTS rows in chunk order,
// without the per-row ids, so two documents' indexes can be compared.
func documentIndex(t *testing.T, d *sql.DB, documentID string) []string {
	t.Helper()
	rows, err := d.Query(`
		SELECT c.chunk_index, c.content, f.title, f.note, f.tags, f.derived_text, f.content
		FROM document_chunks c
		JOIN document_chunks_fts f ON f.chunk_id = c.id
		WHERE c.document_id = ?
		ORDER BY c.chunk_index`, documentID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var (
			index                                        int
			content, title, note, tags, derived, ftsBody string
		)
		if err := rows.Scan(&index, &content, &title, &note, &tags, &derived, &ftsBody); err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.Join([]string{strconv.Itoa(index), content, title, note, tags, derived, ftsBody}, "|"))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDocumentUpdateContentRebuildsIndex(t *testing.T) {
	d := newTestDB(t)
	projects := NewProjectRepository(d)
	docs := NewDocumentRepository(d)
	proj, err := projects.CreateProject(CreateProjectInput{Title: "P"})
	if err != nil {
		t.Fatal(err)
	}
	filePath := "/files/a.png"
	doc, err := docs.CreateDocument(CreateDocumentInput{
		ProjectID: proj.ID, Type: "image", Title: "写真", Note: "旧メモ", Tags: []string{"旧タグ"},
		DerivedText: "灯台の写真", FilePath: &filePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := docs.ListChunksForEmbedding(doc.ID)
	if err != nil || len(chunks) == 0 {
		t.Fatalf("ListChunksForEmbedding = %d, %v", len(chunks), err)
	}
	if err := docs.UpsertChunkEmbeddings([]ChunkEmbedding{{
		ChunkID: chunks[0].ChunkID, DocumentID: doc.ID, ProjectID: proj.ID, Embedding: []float64{0.1}, Model: "test",
	}}); err != nil {
		t.Fatal(err)
	}

	tick()
	updated, err := docs.UpdateDocumentContent(doc.ID, UpdateDocumentContentInput{
		Note: "  新メモ  ", Tags: []string{"港"}, DerivedText: "\n夕暮れの桟橋\n",
	})
	if err != nil || updated == nil {
		t.Fatalf("UpdateDocumentContent = %v, %v", updated, err)
	}
	if updated.Note != "新メモ" || updated.DerivedText != "夕暮れの桟橋" || len(updated.Tags) != 1 || updated.Tags[0] != "港" {
		t.Fatalf("updated fields = %+v", updated)
	}
	if updated.UpdatedAt == updated.CreatedAt {
		t.Error("updated_at was not advanced")
	}
	stored, err := docs.GetDocument(doc.ID)
	if err != nil || stored == nil || stored.DerivedText != "夕暮れの桟橋" || stored.Title != "写真" || stored.Type != "image" {
		t.Fatalf("stored = %+v, %v", stored, err)
	}

	for _, term := range []string{"桟橋", "新メモ", "港"} {
		if ids := ftsDocIDs(t, d, term); !ids[doc.ID] {
			t.Errorf("FTS for %q did not find the updated document", term)
		}
	}
	for _, term := range []string{"灯台", "旧メモ", "旧タグ"} {
		if ids := ftsDocIDs(t, d, term); ids[doc.ID] {
			t.Errorf("FTS for %q still finds the old content", term)
		}
	}

	// The rebuilt chunks are new rows, so the old vectors went with the old ones.
	var embeddings int
	if err := d.QueryRow("SELECT COUNT(*) FROM document_chunk_embeddings WHERE document_id = ?", doc.ID).Scan(&embeddings); err != nil {
		t.Fatal(err)
	}
	if embeddings != 0 {
		t.Errorf("embeddings left after update = %d, want 0", embeddings)
	}

	// Same content through create and through update gives the same index.
	twin, err := docs.CreateDocument(CreateDocumentInput{
		ProjectID: proj.ID, Type: "image", Title: "写真", Note: "新メモ", Tags: []string{"港"},
		DerivedText: "夕暮れの桟橋", FilePath: &filePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, want := documentIndex(t, d, doc.ID), documentIndex(t, d, twin.ID)
	if len(got) == 0 || strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("updated index differs from created one:\nupdated %q\ncreated %q", got, want)
	}

	if missing, err := docs.UpdateDocumentContent("nope", UpdateDocumentContentInput{}); err != nil || missing != nil {
		t.Errorf("UpdateDocumentContent(missing) = %v, %v", missing, err)
	}
}

func TestDocumentUpdateContentReinfersOnlyMisc(t *testing.T) {
	d := newTestDB(t)
	projects := NewProjectRepository(d)
	docs := NewDocumentRepository(d)
	proj, err := projects.CreateProject(CreateProjectInput{Title: "P"})
	if err != nil {
		t.Fatal(err)
	}
	filePath := "/files/b.png"
	// An image with no description: nothing to classify, so it lands in misc.
	blank, err := docs.CreateDocument(CreateDocumentInput{ProjectID: proj.ID, Type: "image", Title: "b.png", FilePath: &filePath})
	if err != nil {
		t.Fatal(err)
	}
	if blank.Category != "misc" {
		t.Fatalf("precondition: category = %q, want misc", blank.Category)
	}
	const worldText = "世界設定と魔法体系の詳細"
	updated, err := docs.UpdateDocumentContent(blank.ID, UpdateDocumentContentInput{DerivedText: worldText})
	if err != nil || updated == nil {
		t.Fatalf("UpdateDocumentContent = %v, %v", updated, err)
	}
	if updated.Category != "world" {
		t.Errorf("misc document category after update = %q, want world", updated.Category)
	}
	if stored, _ := docs.GetDocument(blank.ID); stored == nil || stored.Category != "world" {
		t.Errorf("stored category = %+v, want world", stored)
	}

	// A category other than misc stays, whatever the new content suggests.
	chosen, err := docs.CreateDocument(CreateDocumentInput{ProjectID: proj.ID, Type: "image", Category: "character", Title: "c.png", FilePath: &filePath})
	if err != nil {
		t.Fatal(err)
	}
	updated, err = docs.UpdateDocumentContent(chosen.ID, UpdateDocumentContentInput{DerivedText: worldText})
	if err != nil || updated == nil || updated.Category != "character" {
		t.Errorf("non-misc category after update = %+v, %v, want character", updated, err)
	}
}
