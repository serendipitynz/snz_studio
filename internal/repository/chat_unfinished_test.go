package repository

import (
	"slices"
	"testing"
)

// TestDeleteUnfinishedAssistantMessages pins which messages the startup cleanup
// removes: only the empty placeholder of a streaming turn that never finished.
// A finished reply that came back empty carries response_ms and stays, as does a
// placeholder that had already received deltas.
func TestDeleteUnfinishedAssistantMessages(t *testing.T) {
	d := newTestDB(t)
	projects := NewProjectRepository(d)
	chats := NewChatRepository(d)
	proj, err := projects.CreateProject(CreateProjectInput{Title: "P"})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := chats.CreateChat(CreateChatInput{ProjectID: proj.ID})
	if err != nil {
		t.Fatal(err)
	}

	add := func(role, content string) string {
		t.Helper()
		m, err := chats.AddMessage(AddMessageInput{ChatID: chat.ID, Role: role, Content: content})
		if err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
		tick()
		return m.ID
	}
	question := add("user", "質問")
	unfinished := add("assistant", "")
	partial := add("assistant", "")
	if _, err := chats.UpdateMessageContent(partial, "途中まで"); err != nil {
		t.Fatal(err)
	}
	emptyReply := add("assistant", "")
	responseMs := int64(800)
	if _, err := chats.FinalizeMessage(FinalizeMessageInput{MessageID: emptyReply, Content: "", ResponseMs: &responseMs}); err != nil {
		t.Fatal(err)
	}

	removed, err := chats.DeleteUnfinishedAssistantMessages()
	if err != nil {
		t.Fatalf("DeleteUnfinishedAssistantMessages: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed %d messages, want 1", removed)
	}
	messages, err := chats.ListMessages(chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range messages {
		ids = append(ids, m.ID)
	}
	want := []string{question, partial, emptyReply}
	if !slices.Equal(ids, want) {
		t.Fatalf("remaining messages = %v, want %v (the unfinished %s removed)", ids, want, unfinished)
	}
}
