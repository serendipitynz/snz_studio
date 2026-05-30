package service

import (
	"reflect"
	"testing"

	"snzstudio/internal/repository"
)

func TestSplitSentences(t *testing.T) {
	// Terminators stay attached; newlines are consumed. The empty fragment between
	// a terminator and an immediately-following newline is preserved here and
	// filtered later by the >= 10 rune check, matching the TS pipeline order.
	got := splitSentences("First sentence.\nSecond?Third")
	want := []string{"First sentence.", "", "Second?", "Third"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitSentences = %#v, want %#v", got, want)
	}
}

func TestInferKindFromText(t *testing.T) {
	cases := map[string]string{
		"日本語で簡潔に答えてください":    "procedural",
		"このプロジェクトはGoで構成します": "semantic",
		"前回のリリースで対応した":      "episodic",
		"無関係な文章":            "episodic", // default
	}
	for input, want := range cases {
		if got := inferKindFromText(input); got != want {
			t.Fatalf("inferKindFromText(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestShouldSkipAutoMemorySentence(t *testing.T) {
	// A transient one-off request is skipped for procedural...
	if !shouldSkipAutoMemorySentence("この文章を要約してください", "procedural") {
		t.Fatal("transient procedural request should be skipped")
	}
	// ...unless it also carries a durable procedural cue.
	if shouldSkipAutoMemorySentence("常に日本語で要約してください", "procedural") {
		t.Fatal("durable procedural cue should override the transient skip")
	}
	// Non-procedural kinds are never skipped here.
	if shouldSkipAutoMemorySentence("要約してください", "semantic") {
		t.Fatal("non-procedural sentence should not be skipped")
	}
}

func TestGenerateMemoryTitle(t *testing.T) {
	// Heading markers are stripped and the first sentence (up to a terminator)
	// becomes the title. Note newlines are collapsed to spaces before the sentence
	// split, so a terminator (。) is what delimits the title — matching the TS order.
	if got := generateMemoryTitle("# 重要な決定。本文が続く", "episodic"); got != "重要な決定" {
		t.Fatalf("title = %q, want %q", got, "重要な決定")
	}
	// Empty content falls back to the kind-specific default.
	if got := generateMemoryTitle("   ", "procedural"); got != "Working preference" {
		t.Fatalf("empty procedural title = %q, want Working preference", got)
	}
}

func TestMaybeStoreFromUserMessageRuleExtraction(t *testing.T) {
	d := newServiceTestDB(t)
	projects := repository.NewProjectRepository(d)
	memories := repository.NewMemoryRepository(d)
	// The LLM client is never reached on this rule-based path.
	svc := NewMemoryService(memories, NewLLMClient(testConfig(streamSettings("http://127.0.0.1:0", "standard"))))

	chats := repository.NewChatRepository(d)
	project, err := projects.CreateProject(repository.CreateProjectInput{Title: "P"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	chatRecord, err := chats.CreateChat(repository.CreateChatInput{ProjectID: project.ID, Title: "c"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	chat := chatRecord.ID

	// A durable procedural preference should be captured.
	created, err := svc.MaybeStoreFromUserMessage(project.ID, chat, "今後は必ず日本語で回答してください。")
	if err != nil {
		t.Fatalf("MaybeStoreFromUserMessage: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(created))
	}
	if created[0].Kind != "procedural" || created[0].Title != "Working preference" {
		t.Fatalf("unexpected memory: kind=%q title=%q", created[0].Kind, created[0].Title)
	}

	// A question carrying the same cue must be ignored (QUESTION_CUES).
	none, err := svc.MaybeStoreFromUserMessage(project.ID, chat, "今後は必ず日本語にしますか？")
	if err != nil {
		t.Fatalf("MaybeStoreFromUserMessage question: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("question should not produce memories, got %d", len(none))
	}

	// A sentence with no durable cue is ignored entirely.
	none, err = svc.MaybeStoreFromUserMessage(project.ID, chat, "空はとても青くて気持ちがよかったです。")
	if err != nil {
		t.Fatalf("MaybeStoreFromUserMessage plain: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("cue-free chit-chat should not create memories, got %d", len(none))
	}
}
