package service

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"snzstudio/internal/model"
	"snzstudio/internal/repository"
	"snzstudio/internal/service/commands"
)

// enableEffects turns on /add, /use and /set for the chat and fixes the dice the
// engine throws.
func (g *turnGraph) enableEffects(t *testing.T, chatID string, dice ...int) {
	t.Helper()
	enabled := model.ChatCommands{Add: &model.EffectSettings{}, Use: &model.EffectSettings{}, Set: &model.EffectSettings{}}
	if chat, err := g.chats.UpdateMultiAgentSettings(chatID, repository.MultiAgentSettings{Commands: &enabled}); err != nil || chat == nil {
		t.Fatalf("UpdateMultiAgentSettings(commands) = %v, %v", chat, err)
	}
	g.engine.rollDie = func(int) int {
		v := dice[0]
		dice = dice[1:]
		return v
	}
}

func (g *turnGraph) setSheet(t *testing.T, participantID, sheet string) {
	t.Helper()
	if p, err := g.participants.UpdateParticipant(repository.UpdateParticipantInput{ParticipantID: participantID, StateSheet: &sheet}); err != nil || p == nil {
		t.Fatalf("UpdateParticipant(stateSheet) = %v, %v", p, err)
	}
}

func (g *turnGraph) sheetOf(t *testing.T, participantID string) string {
	t.Helper()
	p, err := g.participants.GetParticipant(participantID)
	if err != nil || p == nil {
		t.Fatalf("GetParticipant = %v, %v", p, err)
	}
	return p.StateSheet
}

// TestTurnEngineAppliesEffect covers TASK-36 AC #1 and #2 on a participant's
// utterance: the /add on its last line reaches the owner's sheet, the record of
// it is stored with the message that wrote it, and every later speaker reads it
// as a 【効果】 line after that message's body and the new value in its
// 【現在の状態】 section.
func TestTurnEngineAppliesEffect(t *testing.T) {
	srv := newTurnLLMServer(t, "崩れた岩がレンを打つ。\n/add レン HP -1d6", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "", srv.URL, "GM", "レン (斥候)")
	g.enableEffects(t, chat.ID, 3, 2)
	g.setSheet(t, roster[1].ID, "HP: 7/10\nたいまつ: 2")

	message, err := g.engine.RunTurn(chat.ID, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "崩れた岩がレンを打つ。" || len(message.StateEffects) != 1 {
		t.Fatalf("stored = (%q, %+v), want the body without the command and one effect", message.Content, message.StateEffects)
	}
	effect := message.StateEffects[0]
	if !effect.Applied || effect.Delta != -3 || effect.ParticipantID != roster[1].ID || effect.After != "4/10" {
		t.Fatalf("effect = %+v, want 7/10 - 3 applied to レン", effect)
	}
	if sheet := g.sheetOf(t, roster[1].ID); sheet != "HP: 4/10\nたいまつ: 2" {
		t.Fatalf("sheet = %q, want the HP line changed and the rest kept", sheet)
	}
	stored, err := g.chats.ListMessages(chat.ID)
	if err != nil || len(stored) != 1 || !reflect.DeepEqual(stored[0].StateEffects, message.StateEffects) {
		t.Fatalf("read back %+v (%v), want the effect stored with the message", stored, err)
	}

	g.runTurns(t, chat.ID, []model.Participant{roster[1]})
	request := srv.captured()[1]
	if last := request.Messages[len(request.Messages)-1]; last.Content != "GM: 崩れた岩がレンを打つ。\n【効果】レン (斥候) HP -1d6（3）: 7/10 → 4/10" {
		t.Fatalf("レン read %q, want the body then the 【効果】 line", last.Content)
	}
	if !strings.Contains(request.Messages[0].Content, "HP: 4/10") {
		t.Fatalf("system prompt = %q, want the sheet as the effect left it", request.Messages[0].Content)
	}
}

// TestTurnEngineEffectReadsSheetWhenStored covers design §4.8.4's "when the
// effect lands": a human's edit made while the turn was generating is what the
// effect is judged against, not the sheet the turn began with.
func TestTurnEngineEffectReadsSheetWhenStored(t *testing.T) {
	g := newTurnGraph(t)
	var renID string
	srv := newTurnLLMServer(t, "たいまつを灯す。\n/use レン たいまつ", func() {
		g.setSheet(t, renID, "たいまつ: 0")
	})
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "", srv.URL, "レン", "ミラ")
	renID = roster[0].ID
	g.enableEffects(t, chat.ID)
	g.setSheet(t, renID, "たいまつ: 1")

	message, err := g.engine.RunTurn(chat.ID, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if effect := message.StateEffects[0]; effect.Applied || effect.Reason != model.EffectNotPositive {
		t.Fatalf("effect = %+v, want the torch the human took away not used", effect)
	}
	if sheet := g.sheetOf(t, renID); sheet != "たいまつ: 0" {
		t.Fatalf("sheet = %q, want the human's edit kept", sheet)
	}
}

// TestStoreHumanMessageEffects covers TASK-36 AC #3 and #5 on the human's
// intervention, and the store-time order of §4.8.3 item 2: an effect that
// fails its precondition or the limit is recorded and leaves the sheet alone,
// an effect-only message is stored, a name in the command is no call while the
// call written before it stays, and a command that cannot be read — its owner
// not on the roster included — is refused with nothing stored.
func TestStoreHumanMessageEffects(t *testing.T) {
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleWeighted, "", "http://unused.invalid/v1", "レン (斥候)", "ミラ (神官戦士)")
	ren := roster[0]
	g.enableEffects(t, chat.ID)
	const sheet = "HP: 7/10\nたいまつ: 0\n所持品: ロープ"
	g.setSheet(t, ren.ID, sheet)

	for _, tc := range []struct {
		content string
		reason  string
	}{
		{"/use レン たいまつ", model.EffectNotPositive},
		{"/use レン 聖水", model.EffectMissingItem},
		{"/add レン 所持品 -1", model.EffectNotInteger},
		{"/set レン 所持品 " + strings.Repeat("縄", 200), model.EffectOverLimit},
	} {
		message, err := g.engine.StoreHumanMessage(chat.ID, tc.content)
		if err != nil {
			t.Fatalf("StoreHumanMessage(%q) = %v", tc.content, err)
		}
		if message.Content != "" || len(message.StateEffects) != 1 || message.StateEffects[0].Applied || message.StateEffects[0].Reason != tc.reason {
			t.Errorf("%q stored (%q, %+v), want an effect-only message not applied for %s", tc.content, message.Content, message.StateEffects, tc.reason)
		}
		if got := g.sheetOf(t, ren.ID); got != sheet {
			t.Errorf("%q changed the sheet to %q", tc.content, got)
		}
		if len(message.AddressedParticipantIDs) != 0 {
			t.Errorf("%q called on %v, want the owner's name no call", tc.content, message.AddressedParticipantIDs)
		}
	}

	message, err := g.engine.StoreHumanMessage(chat.ID, "ミラ、手当てを。\n/set 共通 場所 第二坑道")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(message.AddressedParticipantIDs, []string{roster[1].ID}) || !message.StateEffects[0].Applied {
		t.Fatalf("stored (%v, %+v), want the call on ミラ and the place set", message.AddressedParticipantIDs, message.StateEffects)
	}
	if updated, _ := g.chats.GetChat(chat.ID); updated.StateSheet != "場所: 第二坑道" {
		t.Fatalf("shared sheet = %q, want the place added", updated.StateSheet)
	}

	before, _ := g.chats.ListMessages(chat.ID)
	for _, content := range []string{"/add ガルド HP -1", "/add レン HP", "/use レン"} {
		if _, err := g.engine.StoreHumanMessage(chat.ID, content); !errors.Is(err, commands.ErrInvalidEffectCommand) {
			t.Errorf("StoreHumanMessage(%q) = %v, want ErrInvalidEffectCommand", content, err)
		}
	}
	if after, _ := g.chats.ListMessages(chat.ID); len(after) != len(before) {
		t.Fatalf("stored %d messages, want the refusals to store nothing", len(after)-len(before))
	}
}

// TestTurnEngineEffectKeptAsText: an effect command a participant wrote that
// cannot be read, or that the chat has not enabled, stays in the body, and the
// turn is stored with no effect rather than failed.
func TestTurnEngineEffectKeptAsText(t *testing.T) {
	unreadable := newTurnLLMServer(t, "ガルドが倒れる。\n/add ガルド HP -5", nil)
	g := newTurnGraph(t)
	chat, _ := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "", unreadable.URL, "GM", "レン")
	g.enableEffects(t, chat.ID)
	message, err := g.engine.RunTurn(chat.ID, "", nil, nil)
	if err != nil {
		t.Fatalf("RunTurn = %v, want the unreadable command kept as text", err)
	}
	if message.Content != "ガルドが倒れる。\n/add ガルド HP -5" || len(message.StateEffects) != 0 {
		t.Fatalf("stored = (%q, %+v), want the line kept and no effect", message.Content, message.StateEffects)
	}

	disabled := newTurnLLMServer(t, "たいまつを灯す。\n/use レン たいまつ", nil)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "", disabled.URL, "レン", "ミラ")
	g.setSheet(t, roster[0].ID, "たいまつ: 2")
	message, err = g.engine.RunTurn(chat.ID, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "たいまつを灯す。\n/use レン たいまつ" || len(message.StateEffects) != 0 || g.sheetOf(t, roster[0].ID) != "たいまつ: 2" {
		t.Fatalf("stored = (%q, %+v), want /use as text where the chat has not enabled it", message.Content, message.StateEffects)
	}
}

// TestTurnEngineEffectLineAbove covers the game master of the TASK-36
// measurement (design §4.8.8): the /add written on a line of its own where the
// damage is narrated, with narration after it, reaches the sheet, and the call
// in the last sentence still stands.
func TestTurnEngineEffectLineAbove(t *testing.T) {
	srv := newTurnLLMServer(t, "岩がレンの肩を打つ。\n/add レン HP -2\n\n奥の物音が近づく。ミラ、どうする？", nil)
	g := newTurnGraph(t)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "", srv.URL, "GM", "レン (斥候)", "ミラ (神官戦士)")
	g.enableEffects(t, chat.ID)
	g.setSheet(t, roster[1].ID, "HP: 7/10")

	message, err := g.engine.RunTurn(chat.ID, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "岩がレンの肩を打つ。\n\n奥の物音が近づく。ミラ、どうする？" || len(message.StateEffects) != 1 || !message.StateEffects[0].Applied {
		t.Fatalf("stored = (%q, %+v), want the command line taken out and applied", message.Content, message.StateEffects)
	}
	if sheet := g.sheetOf(t, roster[1].ID); sheet != "HP: 5/10" {
		t.Fatalf("sheet = %q, want 7 - 2", sheet)
	}
	if !reflect.DeepEqual(message.AddressedParticipantIDs, []string{roster[2].ID}) {
		t.Fatalf("called on %v, want ミラ from the last sentence", message.AddressedParticipantIDs)
	}
}
