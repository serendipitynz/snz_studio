package repository

import (
	"testing"

	"snzstudio/internal/model"
)

// A multi-agent turn stores its utterance and its references together: a
// reference that cannot be stored must take the utterance with it, or the turn
// would count as spoken (round_robin reads the transcript) while being reported
// as failed.
func TestAddMessageWithReferencesIsAtomic(t *testing.T) {
	d := newTestDB(t)
	projects := NewProjectRepository(d)
	chats := NewChatRepository(d)
	project, err := projects.CreateProject(CreateProjectInput{Title: "P"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	chat, err := chats.CreateChat(CreateChatInput{ProjectID: project.ID, Kind: model.ChatKindMultiAgent})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	stored, err := chats.AddMessageWithReferences(AddMessageInput{ChatID: chat.ID, Role: "assistant", Content: "発言"}, []ReferenceInput{
		{SourceType: "document", SourceID: "doc_1", Label: "灯台", Excerpt: "霧笛", Score: 0.9},
		{SourceType: "memory", SourceID: "mem_1", Label: "記憶", Score: 0.5},
	})
	if err != nil {
		t.Fatalf("AddMessageWithReferences: %v", err)
	}
	messages, err := chats.GetMessagesWithReferences(chat.ID)
	if err != nil {
		t.Fatalf("GetMessagesWithReferences: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != stored.ID || len(messages[0].References) != 2 {
		t.Fatalf("stored %d messages with %v references, want 1 with 2", len(messages), refCounts(messages))
	}
	if messages[0].References[0].SourceID != "doc_1" {
		t.Fatalf("references not ordered by score: %+v", messages[0].References)
	}

	// source_type is CHECK-constrained, so an unknown type fails the insert.
	_, err = chats.AddMessageWithReferences(AddMessageInput{ChatID: chat.ID, Role: "assistant", Content: "二度目"}, []ReferenceInput{
		{SourceType: "bogus", SourceID: "x", Label: "x"},
	})
	if err == nil {
		t.Fatal("expected the constrained reference to fail the insert")
	}
	remaining, err := chats.ListMessages(chat.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("a failed reference left %d messages, want the first one only", len(remaining))
	}
}

func refCounts(messages []model.MessageWithReferences) []int {
	out := make([]int, 0, len(messages))
	for _, m := range messages {
		out = append(out, len(m.References))
	}
	return out
}
