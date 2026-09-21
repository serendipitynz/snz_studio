package service

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/model"
	"snzstudio/internal/repository"
)

func TestBuildTurnMaterialQuery(t *testing.T) {
	scene := strings.Repeat("場", 250)
	if got := buildTurnMaterialQuery(nil, scene); got != strings.Repeat("場", turnMaterialSceneHeadChars-1)+"…" {
		t.Fatalf("opening turn query = %q, want the head of the scene only", got)
	}
	if got := buildTurnMaterialQuery(nil, "   "); got != "" {
		t.Fatalf("query with nothing to search = %q, want empty", got)
	}

	messages := []model.Message{
		{Content: "一番古い"},
		{Content: "三つ前"},
		{Content: "二つ前"},
		{Content: "   "},
		{Content: "直前"},
		{Content: "最新"},
	}
	got := buildTurnMaterialQuery(messages, "場面")
	want := "最新\n直前\n二つ前\n場面"
	if got != want {
		t.Fatalf("query = %q, want %q (latest first, blanks skipped, scene last)", got, want)
	}
}

// TestBuildTurnMaterialBudget covers AC #3 and the second half of AC #4: the
// material is laid out description → documents → memories, stops at the total
// budget, and the references are exactly the items that made it in.
func TestBuildTurnMaterialBudget(t *testing.T) {
	project := &model.Project{ID: "proj_1", Title: "港町", Description: strings.Repeat("説", 500)}
	chunk := func(i int) model.RetrievedDocumentChunk {
		return model.RetrievedDocumentChunk{ChunkID: fmt.Sprintf("chunk_%d", i), ChunkIndex: i, Content: strings.Repeat("文", 350)}
	}
	documents := []model.RetrievedDocumentReference{
		{SearchReference: model.SearchReference{SourceType: "document", SourceID: "doc_1", Label: "灯台", Score: 0.9}, Chunks: []model.RetrievedDocumentChunk{chunk(0), chunk(1)}},
		{SearchReference: model.SearchReference{SourceType: "document", SourceID: "doc_2", Label: "市場", Score: 0.8}, Chunks: []model.RetrievedDocumentChunk{chunk(2), chunk(3)}},
	}
	memories := make([]model.SearchReference, 0, 3)
	for i := 0; i < 3; i++ {
		memories = append(memories, model.SearchReference{SourceType: "memory", SourceID: fmt.Sprintf("mem_%d", i), Label: fmt.Sprintf("記憶%d", i), Excerpt: strings.Repeat("憶", 220), Score: 0.5})
	}

	material := buildTurnMaterial(project, documents, memories)

	if !strings.HasPrefix(material.Prompt, "【プロジェクト資料】") {
		t.Fatalf("prompt does not open with the section header:\n%s", material.Prompt)
	}
	// 400 (description) + 2 × (2 × 300 chunks) + 3 × 220 memories is about 2,300,
	// over the 2,000 budget, so the tail of the memories is what falls off.
	if len(material.References) >= 1+len(documents)+len(memories) {
		t.Fatalf("budget did not cut anything: %d references", len(material.References))
	}
	if runeLen(material.Prompt) > turnMaterialTotalChars {
		t.Fatalf("section is %d runes, over the %d budget", runeLen(material.Prompt), turnMaterialTotalChars)
	}

	wantOrder := []string{"project:proj_1", "document:doc_1", "document:doc_2", "memory:mem_0"}
	gotOrder := make([]string, 0, len(material.References))
	for _, ref := range material.References {
		gotOrder = append(gotOrder, ref.SourceType+":"+ref.SourceID)
	}
	if strings.Join(gotOrder, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("references = %v, want %v", gotOrder, wantOrder)
	}
	for _, ref := range material.References {
		if !strings.Contains(material.Prompt, ref.Label) {
			t.Fatalf("reference %q is stored but absent from the prompt", ref.Label)
		}
	}
	if strings.Contains(material.Prompt, "記憶1") || strings.Contains(material.Prompt, "記憶2") {
		t.Fatalf("prompt carries a memory that the references do not:\n%s", material.Prompt)
	}
	// util.Truncate keeps maxLength-1 runes and appends an ellipsis.
	if strings.Count(material.Prompt, strings.Repeat("文", 299)+"…") != 4 || strings.Contains(material.Prompt, strings.Repeat("文", 300)) {
		t.Fatalf("chunks are not cut to %d runes", turnMaterialChunkChars)
	}
	if !strings.Contains(material.Prompt, strings.Repeat("説", 399)+"…") || strings.Contains(material.Prompt, strings.Repeat("説", 400)) {
		t.Fatalf("description is not cut to %d runes", turnMaterialDescriptionChars)
	}
}

func TestBuildTurnMaterialEmpty(t *testing.T) {
	material := buildTurnMaterial(&model.Project{ID: "proj_1", Title: "無題", Description: "  "}, nil, nil)
	if material.Prompt != "" || len(material.References) != 0 {
		t.Fatalf("a project with no description and no matches must produce nothing, got %+v", material)
	}
}

// materialFixture is a project whose material a turn should and should not
// pick up: a description, a system prompt that must stay out, one document that
// matches the conversation and one that does not, a procedural memory that must
// not be injected unconditionally, and a semantic memory that matches.
type materialFixture struct {
	project    model.Project
	lighthouse model.DocumentRecord
	olga       model.Memory
}

func newMaterialFixture(t *testing.T, g *turnGraph) materialFixture {
	t.Helper()
	project := g.newProject(t, repository.CreateProjectInput{
		Title:        "港町の物語",
		Description:  "霧の港町ハーバーンを舞台にした群像劇の企画。",
		SystemPrompt: "あなたは丁寧なアシスタントとして箇条書きで答える。",
	})
	lighthouse, err := g.documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID:   project.ID,
		Type:        "text",
		Title:       "灯台守の記録",
		ContentText: "灯台守のオルガは毎晩、霧笛を三度鳴らす。",
	})
	if err != nil {
		t.Fatalf("CreateDocument lighthouse: %v", err)
	}
	if _, err := g.documents.CreateDocument(repository.CreateDocumentInput{
		ProjectID:   project.ID,
		Type:        "text",
		Title:       "銅貨の相場",
		ContentText: "干し魚は一枚につき銅貨三枚で売られる。",
	}); err != nil {
		t.Fatalf("CreateDocument market: %v", err)
	}
	if _, err := g.memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID: project.ID,
		Kind:      "procedural",
		Title:     "回答の書式",
		Content:   "回答は必ず箇条書きで書く。",
		Source:    "manual",
	}); err != nil {
		t.Fatalf("CreateMemory procedural: %v", err)
	}
	olga, err := g.memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID: project.ID,
		Kind:      "semantic",
		Title:     "オルガの過去",
		Content:   "灯台守のオルガはかつて船乗りだった。",
		Source:    "manual",
	})
	if err != nil {
		t.Fatalf("CreateMemory olga: %v", err)
	}
	return materialFixture{project: project, lighthouse: lighthouse, olga: olga}
}

func storedReferences(t *testing.T, g *turnGraph, chatID string) []model.AssistantMessageReference {
	t.Helper()
	messages, err := g.chats.GetMessagesWithReferences(chatID)
	if err != nil {
		t.Fatalf("GetMessagesWithReferences: %v", err)
	}
	if len(messages) == 0 {
		t.Fatal("no messages stored")
	}
	return messages[len(messages)-1].References
}

// TestTurnEngineMaterialInPrompt covers AC #1, #2 and #4: the material sits
// before the scene, carries only description / matched passages / matched
// memories, and what is stored as references is what the prompt carried. The
// utterance contains 「そのまま」, which in Assemble would switch on quote mode.
func TestTurnEngineMaterialInPrompt(t *testing.T) {
	srv := newTurnLLMServer(t, "返答", nil)
	g := newTurnGraph(t)
	fx := newMaterialFixture(t, g)
	chat, roster := g.newMultiAgentChatInProject(t, fx.project.ID, model.TurnRuleRoundRobin, "場面: 港町の酒場。短く話す。", srv.URL, "Alice", "Bob")

	g.addMessage(t, chat.ID, "user", "オルガの霧笛の話をそのまま聞かせて", nil)
	if _, err := g.engine.RunTurn(chat.ID, "", nil); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	system := srv.captured()[0].Messages[0].Content
	if !strings.HasPrefix(system, "【プロジェクト資料】") {
		t.Fatalf("system prompt does not open with the project material:\n%s", system)
	}
	if strings.Index(system, "【プロジェクト資料】") > strings.Index(system, "場面: 港町の酒場") {
		t.Fatalf("the material must precede the scene:\n%s", system)
	}
	if strings.Index(system, "場面: 港町の酒場") > strings.Index(system, roster[0].RolePrompt) {
		t.Fatalf("scene must precede the role prompt:\n%s", system)
	}
	for _, fragment := range []string{
		"[プロジェクト] 港町の物語", "霧の港町ハーバーン",
		"[ドキュメント] 灯台守の記録", "霧笛を三度鳴らす",
		"[メモリ] オルガの過去", "かつて船乗りだった",
		"話者の役割・口調・立場を変えない",
	} {
		if !strings.Contains(system, fragment) {
			t.Fatalf("system prompt missing %q:\n%s", fragment, system)
		}
	}
	for _, fragment := range []string{
		"丁寧なアシスタント",           // the project system prompt
		"回答の書式",               // a procedural memory with no relation to the conversation
		"quote mode", "引用モード", // Assemble's quote-mode order
		"Retrieval mode", "Full document content",
	} {
		if strings.Contains(system, fragment) {
			t.Fatalf("system prompt must not carry %q:\n%s", fragment, system)
		}
	}

	references := storedReferences(t, g, chat.ID)
	got := map[string]bool{}
	for _, ref := range references {
		got[ref.SourceType+":"+ref.SourceID] = true
	}
	for _, want := range []string{"project:" + fx.project.ID, "document:" + fx.lighthouse.ID, "memory:" + fx.olga.ID} {
		if !got[want] {
			t.Fatalf("stored references %v lack %s", got, want)
		}
	}
	// One section marker per stored reference: nothing in the prompt without a
	// reference, no reference without its section.
	sections := strings.Count(system, "\n[プロジェクト] ") + strings.Count(system, "\n[ドキュメント] ") + strings.Count(system, "\n[メモリ] ")
	if sections != len(references) {
		t.Fatalf("%d sections in the prompt, %d references stored:\n%s", sections, len(references), system)
	}
	for _, ref := range references {
		if !strings.Contains(system, ref.Label) {
			t.Fatalf("stored reference %q is not in the prompt", ref.Label)
		}
	}
}

// TestTurnEngineOpeningTurnSearchesScene covers the opening-turn half of AC #2:
// with no utterance yet, the scene alone is the query.
func TestTurnEngineOpeningTurnSearchesScene(t *testing.T) {
	srv := newTurnLLMServer(t, "返答", nil)
	g := newTurnGraph(t)
	fx := newMaterialFixture(t, g)
	chat, _ := g.newMultiAgentChatInProject(t, fx.project.ID, model.TurnRuleRoundRobin, "場面: 灯台守のオルガが霧笛を鳴らす夜。", srv.URL, "Alice", "Bob")

	if _, err := g.engine.RunTurn(chat.ID, "", nil); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	system := srv.captured()[0].Messages[0].Content
	if !strings.Contains(system, "[ドキュメント] 灯台守の記録") {
		t.Fatalf("opening turn did not retrieve on the scene:\n%s", system)
	}
	for _, ref := range storedReferences(t, g, chat.ID) {
		if ref.SourceType == "document" && ref.SourceID == fx.lighthouse.ID {
			return
		}
	}
	t.Fatal("opening turn stored no reference to the lighthouse document")
}

type failingAssembler struct{}

func (failingAssembler) AssembleTurnMaterial(*model.Chat, *model.Participant, []model.Message) (*TurnMaterial, error) {
	return nil, errors.New("fts index unavailable")
}

// TestTurnEngineMaterialFailureContinues covers the first half of AC #8: a
// failed search costs the turn its material, not the turn itself, and nothing
// is stored as a reference.
func TestTurnEngineMaterialFailureContinues(t *testing.T) {
	srv := newTurnLLMServer(t, "返答", nil)
	g := newTurnGraph(t)
	g.engine = NewTurnEngine(g.chats, g.participants, NewLLMClient(g.cfg), g.cfg, failingAssembler{})
	fx := newMaterialFixture(t, g)
	chat, roster := g.newMultiAgentChatInProject(t, fx.project.ID, model.TurnRuleRoundRobin, "場面: 港町の酒場。", srv.URL, "Alice", "Bob")
	g.addMessage(t, chat.ID, "user", "オルガの霧笛の話を聞かせて", nil)

	message, err := g.engine.RunTurn(chat.ID, "", nil)
	if err != nil {
		t.Fatalf("RunTurn must not fail on a material error: %v", err)
	}
	if message.ParticipantID == nil || *message.ParticipantID != roster[0].ID {
		t.Fatalf("speaker = %v, want Alice", message.ParticipantID)
	}
	system := srv.captured()[0].Messages[0].Content
	if strings.Contains(system, "プロジェクト資料") {
		t.Fatalf("system prompt carries project material after a failed assembly:\n%s", system)
	}
	if !strings.HasPrefix(system, "場面: 港町の酒場。") {
		t.Fatalf("system prompt should open with the scene when there is no project material:\n%s", system)
	}
	if refs := storedReferences(t, g, chat.ID); len(refs) != 0 {
		t.Fatalf("stored %d references after a failed assembly, want none", len(refs))
	}
}

// TestTurnEngineEmbeddingFailureDegrades covers the second half of AC #8: an
// embedding endpoint that is down disables the client and retrieval falls back
// to keywords, so the turn still gets its material and its references.
func TestTurnEngineEmbeddingFailureDegrades(t *testing.T) {
	srv := newTurnLLMServer(t, "返答", nil)
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	d := newServiceTestDB(t)
	cfg := testConfig(config.Settings{
		Editable: config.Editable{
			LLMBaseURL: "http://unused.invalid/v1", LLMModel: "default-model",
			EmbeddingMode: "external", EmbeddingBaseURL: dead.URL + "/v1", EmbeddingModel: "dead-embedding",
		},
		LLMTimeoutMs: 5000,
	})
	g := newTurnGraphWithConfig(t, d, cfg)
	fx := newMaterialFixture(t, g)
	chat, _ := g.newMultiAgentChatInProject(t, fx.project.ID, model.TurnRuleRoundRobin, "場面: 港町の酒場。", srv.URL, "Alice", "Bob")
	g.addMessage(t, chat.ID, "user", "オルガの霧笛の話を聞かせて", nil)

	if _, err := g.engine.RunTurn(chat.ID, "", nil); err != nil {
		t.Fatalf("RunTurn must survive an embedding endpoint failure: %v", err)
	}
	system := srv.captured()[0].Messages[0].Content
	if !strings.Contains(system, "[ドキュメント] 灯台守の記録") {
		t.Fatalf("keyword fallback did not retrieve the document:\n%s", system)
	}
	if refs := storedReferences(t, g, chat.ID); len(refs) == 0 {
		t.Fatal("no references stored under keyword fallback")
	}
}

// TestAssembleTurnMaterialSkipsSpeaker covers the "no search is run" half of
// TASK-20 AC #2. The ContextService is built with no repositories and no
// retrieval at all, so anything the assembler reads or searches would panic on a
// nil dependency: returning an empty result proves the speaker's flag is
// answered before any of them is touched.
func TestAssembleTurnMaterialSkipsSpeaker(t *testing.T) {
	chat := &model.Chat{ID: "chat_1", ProjectID: "proj_1", ScenePrompt: "場面: 港町の酒場。"}
	messages := []model.Message{{Content: "オルガの霧笛の話を聞かせて"}}

	material, err := (&ContextService{}).AssembleTurnMaterial(chat, &model.Participant{ID: "p_1", ReceivesProjectMaterial: false}, messages)
	if err != nil {
		t.Fatalf("AssembleTurnMaterial: %v", err)
	}
	if material.Prompt != "" || len(material.References) != 0 {
		t.Fatalf("a speaker that receives no project material must get nothing, got %+v", material)
	}
}

// TestTurnEngineSkipsMaterialForSpeaker covers TASK-20 AC #2 through the
// engine: the same project and the same conversation that give Alice her
// material leave Bob's turn without it, in the prompt and in the stored
// references alike.
func TestTurnEngineSkipsMaterialForSpeaker(t *testing.T) {
	srv := newTurnLLMServer(t, "返答", nil)
	g := newTurnGraph(t)
	fx := newMaterialFixture(t, g)
	chat, roster := g.newMultiAgentChatInProject(t, fx.project.ID, model.TurnRuleManual, "場面: 港町の酒場。", srv.URL, "Alice", "Bob")

	receives := false
	if _, err := g.participants.UpdateParticipant(repository.UpdateParticipantInput{
		ParticipantID:           roster[1].ID,
		ReceivesProjectMaterial: &receives,
	}); err != nil {
		t.Fatalf("UpdateParticipant: %v", err)
	}

	g.addMessage(t, chat.ID, "user", "オルガの霧笛の話を聞かせて", nil)
	if _, err := g.engine.RunTurn(chat.ID, roster[0].ID, nil); err != nil {
		t.Fatalf("RunTurn Alice: %v", err)
	}
	if _, err := g.engine.RunTurn(chat.ID, roster[1].ID, nil); err != nil {
		t.Fatalf("RunTurn Bob: %v", err)
	}

	captured := srv.captured()
	if len(captured) != 2 {
		t.Fatalf("captured %d completions, want 2", len(captured))
	}
	alice, bob := captured[0].Messages[0].Content, captured[1].Messages[0].Content
	if !strings.Contains(alice, "[ドキュメント] 灯台守の記録") {
		t.Fatalf("the receiving speaker lost its material:\n%s", alice)
	}
	if strings.Contains(bob, "プロジェクト資料") || strings.Contains(bob, "灯台守の記録") || strings.Contains(bob, "霧の港町ハーバーン") {
		t.Fatalf("a speaker that receives no project material got it anyway:\n%s", bob)
	}
	if !strings.HasPrefix(bob, "場面: 港町の酒場。") {
		t.Fatalf("that speaker's prompt should open with the scene:\n%s", bob)
	}
	if refs := storedReferences(t, g, chat.ID); len(refs) != 0 {
		t.Fatalf("stored %d references for a speaker that receives no project material, want none", len(refs))
	}
}
