package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"snzstudio/internal/config"
	"snzstudio/internal/lang"
	"snzstudio/internal/model"
	"snzstudio/internal/repository"
)

// promptCapture stands in for the LLM endpoint and keeps every prompt it is sent.
type promptCapture struct {
	*httptest.Server
	mu       sync.Mutex
	prompts  []string
	response string
}

func newPromptCapture(t *testing.T, response string) *promptCapture {
	t.Helper()
	c := &promptCapture{response: response}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		var body capturedRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		var b strings.Builder
		for _, m := range body.Messages {
			b.WriteString("[" + m.Role + "]\n" + m.Content + "\n")
		}
		c.mu.Lock()
		c.prompts = append(c.prompts, b.String())
		c.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": c.response}}},
		})
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *promptCapture) last(t *testing.T) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.prompts) == 0 {
		t.Fatal("the endpoint received no prompt")
	}
	return c.prompts[len(c.prompts)-1]
}

var rememberRequest = map[lang.Language]string{lang.Japanese: "覚えて", lang.English: "Please remember this"}

var promptIDField = regexp.MustCompile(`"(id|chatId)": "[^"]+"`)

// promptFixtures runs every path that sends the LLM an instruction the app wrote
// and returns what was sent, keyed by the golden file it is compared with.
func promptFixtures(t *testing.T, language lang.Language) map[string]string {
	t.Helper()
	capture := newPromptCapture(t, "{}")
	d := newServiceTestDB(t)
	cfg := testConfig(config.Settings{
		Editable:           config.Editable{LLMBaseURL: capture.URL, LLMModel: "test-model"},
		LLMTimeoutMs:       2000,
		ProcessingLanguage: language,
	})
	text := promptTextFor(language)
	projects := repository.NewProjectRepository(d)
	documents := repository.NewDocumentRepository(d)
	memories := repository.NewMemoryRepository(d)
	chats := repository.NewChatRepository(d)
	llm := NewLLMClient(cfg)
	retrieval := NewRetrievalService(d, NewEmbeddingClient(cfg))
	embeddingSync := NewEmbeddingSyncService(documents, memories, NewEmbeddingClient(cfg))
	contextService := NewContextService(projects, chats, documents, memories, retrieval)
	summary := NewSummaryService(llm, cfg)
	memoryService := NewMemoryService(memories, llm, cfg)
	organizer := NewMemoryOrganizerService(memories, chats, llm, embeddingSync, cfg)
	review := NewReviewService(chats, contextService, llm, cfg)

	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "Saga", Description: "Project description"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	chat, err := chats.CreateChat(repository.CreateChatInput{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if _, err := memories.CreateMemory(repository.CreateMemoryInput{ProjectID: project.ID, Kind: "semantic", Title: "Fact", Content: "A fact", Source: "manual"}); err != nil {
		t.Fatalf("CreateMemory: %v", err)
	}
	draft, err := chats.AddMessage(repository.AddMessageInput{ChatID: chat.ID, Role: "assistant", Content: "A draft"})
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	recent := []model.Message{{Role: "user", Content: "Hello"}, {Role: "assistant", Content: "Hi"}, {Role: "system", Content: "Note"}}
	out := map[string]string{}

	summary.UpdateSummary("", recent)
	out["summary-first"] = capture.last(t)
	summary.UpdateSummary("Earlier", recent)
	out["summary-update"] = capture.last(t)
	out["summary-fallback"] = buildFallbackSummary("Earlier", recent, text)
	summary.GenerateChatTitle(recent)
	out["chat-title"] = capture.last(t)

	if _, err := memoryService.MaybeStoreFromExplicitRequest(project.ID, chat.ID, rememberRequest[language], recent); err != nil {
		t.Fatalf("MaybeStoreFromExplicitRequest: %v", err)
	}
	out["memory-extraction"] = capture.last(t)

	if _, err := organizer.AnalyzeProject(project.ID); err != nil {
		t.Fatalf("AnalyzeProject: %v", err)
	}
	out["memory-organizer"] = promptIDField.ReplaceAllString(capture.last(t), `"$1": "ID"`)
	empty, err := projects.CreateProject(repository.CreateProjectInput{Title: "Empty"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	nothing, err := organizer.AnalyzeProject(empty.ID)
	if err != nil {
		t.Fatalf("AnalyzeProject: %v", err)
	}
	duplicates := []model.Memory{{ID: "m1", Kind: "semantic", Title: "T", Content: "C"}, {ID: "m2", Kind: "semantic", Title: "T", Content: "C"}}
	deduped := buildFallbackPlan(duplicates, nil, text)
	synthesized := buildFallbackPlan(nil, []model.ChatSummary{{Summary: "Progress"}}, text)
	none := buildFallbackPlan(nil, nil, text)
	out["memory-organizer-fallback"] = strings.Join([]string{
		nothing.Summary, deduped.Summary, deduped.Changes[0].Reason, synthesized.Changes[0].Reason, none.Summary,
	}, "\n") + "\n"

	if _, err := review.ReviewMessage(draft.ID); err != nil {
		t.Fatalf("ReviewMessage: %v", err)
	}
	out["creative-review"] = capture.last(t)

	maID := "p1"
	otherID := "p2"
	participants := []model.Participant{
		{ID: maID, DisplayName: "Ren", RolePrompt: "Role of Ren", StateSheet: "HP: 10/10"},
		{ID: otherID, DisplayName: "Mira", RolePrompt: ""},
	}
	success, failure := true, false
	before := "7/10"
	transcript := []model.Message{
		{Role: "user", Content: "Begin"},
		{Role: "assistant", ParticipantID: &maID, Content: "I climb", DiceRolls: []model.DiceRoll{
			{Action: "Climb", Expression: "1d20+3", Dice: []int{4}, Modifier: 3, Total: 7, Target: 12, Success: &failure},
			{Action: "Leap", Expression: "1d20", Dice: []int{15}, Total: 15, Target: 12, Success: &success},
		}},
		{Role: "assistant", ParticipantID: &otherID, Content: "I watch", StateEffects: []model.StateEffect{
			{Kind: "add", Owner: "Ren", ParticipantID: maID, Item: "HP", Delta: -3, Applied: true, Before: &before, After: "4/10"},
			{Kind: "use", Owner: "Ren", ParticipantID: maID, Item: "Torch", Delta: -1, Applied: false, Reason: model.EffectMissingItem},
			{Kind: "use", Owner: "Ren", ParticipantID: maID, Item: "Rope", Delta: -1, Applied: false, Reason: model.EffectNotInteger},
			{Kind: "use", Owner: "Ren", ParticipantID: maID, Item: "Oil", Delta: -1, Applied: false, Reason: model.EffectNotPositive},
			{Kind: "set", Owner: "共通", Item: "Place", Value: "Shaft", Applied: false, Reason: model.EffectOverLimit},
			{Kind: "set", Owner: "共通", Item: "Time", Applied: true, After: "Noon"},
		}},
		{Role: "assistant", Content: "Left over"},
	}
	if _, err := summary.DraftConclusion(transcript, participants); err != nil {
		t.Fatalf("DraftConclusion: %v", err)
	}
	out["conclusion"] = capture.last(t)

	turnChat := &model.Chat{ScenePrompt: "Scene", StateSheet: "Place: Gate", TurnRule: model.TurnRuleRoundRobin}
	weightedChat := &model.Chat{ScenePrompt: "Scene", TurnRule: model.TurnRuleWeighted}
	out["turn-system"] = buildTurnSystemPrompt("Material", turnChat, &participants[0], participants, language)
	out["turn-system-weighted"] = buildTurnSystemPrompt("", weightedChat, &participants[0], participants, language)
	mapped := mapHistoryForSpeaker(transcript, &participants[0], participants, language)
	lines := make([]string, len(mapped))
	for i, m := range mapped {
		lines[i] = "[" + m.Role + "] " + m.Content
	}
	out["turn-history"] = strings.Join(lines, "\n")
	_, out["turn-cue"] = buildTurnPrompt(nil, &participants[0], language)

	material := buildTurnMaterial(
		&model.Project{ID: "proj", Title: "Saga", Description: "Description"},
		[]model.RetrievedDocumentReference{{SearchReference: model.SearchReference{Label: "Doc", Excerpt: "Excerpt"}}},
		[]model.SearchReference{{Label: "Memory", Excerpt: "Excerpt"}},
		text,
	)
	out["turn-material"] = material.Prompt

	out["image-description"] = text.imageDescription

	exported := &model.Chat{Kind: model.ChatKindMultiAgent, TurnRule: model.TurnRuleWeighted, FacilitatorID: maID, StateSheet: "Place: Gate"}
	removedAt := "2026-01-01"
	exportRoster := []model.Participant{participants[0], participants[1], {ID: "p3", DisplayName: "Gone", DeletedAt: &removedAt}}
	out["export"] = BuildChatMarkdown(&project, exported, exportRoster, append(transcript, model.Message{Role: "assistant", Content: " "}), time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC), language)
	out["export-empty"] = BuildChatMarkdown(nil, &model.Chat{Kind: model.ChatKindMultiAgent}, nil, nil, time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC), language)
	return out
}

// comparePromptGoldens holds what is sent to the files under testdata/prompts/<dir>.
// UPDATE_PROMPT_GOLDENS rewrites them. The Japanese files were written from the code
// as it stood before the processing language existed, so they must not be
// regenerated to make a change pass: they are what "unchanged" means.
func comparePromptGoldens(t *testing.T, dir string, got map[string]string) {
	t.Helper()
	root := filepath.Join("testdata", "prompts", dir)
	if os.Getenv("UPDATE_PROMPT_GOLDENS") == dir {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, text := range got {
			if err := os.WriteFile(filepath.Join(root, name+".txt"), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, text := range got {
		want, err := os.ReadFile(filepath.Join(root, name+".txt"))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if string(want) != text {
			t.Errorf("%s differs from its golden.\n--- got\n%s\n--- want\n%s", name, text, want)
		}
	}
}

// TestJapanesePromptsAreUnchanged is TASK-109 AC #5. The one sentence added on
// purpose is the role reminder's language line, which is taken off before comparing.
func TestJapanesePromptsAreUnchanged(t *testing.T) {
	got := promptFixtures(t, lang.Japanese)
	for _, name := range []string{"turn-system", "turn-system-weighted"} {
		trimmed, found := strings.CutSuffix(got[name], "\n"+japanesePromptText.reminderLanguage)
		if !found {
			t.Errorf("%s does not end with the language line: %q", name, got[name])
		}
		got[name] = trimmed
	}
	comparePromptGoldens(t, "ja", got)
}

// TestEnglishPromptsAreEnglish is TASK-109 AC #4: the golden files pin the wording,
// and this also fails on any Japanese left in what the app writes itself. Its
// fixtures are all ASCII, so a Japanese character can only have come from the app.
func TestEnglishPromptsAreEnglish(t *testing.T) {
	got := promptFixtures(t, lang.English)
	for name, text := range got {
		if found := reJapanese.FindString(text); found != "" {
			t.Errorf("%s contains Japanese (%q):\n%s", name, found, text)
		}
	}
	comparePromptGoldens(t, "en", got)
}

// TestPromptTextsAreComplete fails when a field added to promptText was left empty
// in one language, which would send that language an empty instruction.
func TestPromptTextsAreComplete(t *testing.T) {
	for language, text := range map[lang.Language]promptText{lang.Japanese: japanesePromptText, lang.English: englishPromptText} {
		value := reflect.ValueOf(text)
		for i := 0; i < value.NumField(); i++ {
			if value.Field(i).IsZero() {
				t.Errorf("%s: %s is empty", language, value.Type().Field(i).Name)
			}
		}
	}
}
