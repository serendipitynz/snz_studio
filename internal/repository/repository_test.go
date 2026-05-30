package repository

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"snzstudio/internal/db"
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
