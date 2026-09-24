package service

import (
	"strings"
	"testing"
	"time"

	"snzstudio/internal/model"
)

func exportTestNow() time.Time {
	return time.Date(2026, 9, 20, 14, 5, 0, 0, time.UTC)
}

// exportFixture is a multi-agent chat whose transcript exercises every speaker
// shape at once: a human intervention, a participant still on the roster, and a
// participant that has been removed since it spoke.
func exportFixture() (*model.Project, *model.Chat, []model.Participant, []model.Message) {
	project := &model.Project{ID: "p1", Title: "調査プロジェクト"}
	chat := &model.Chat{
		ID:          "c1",
		ProjectID:   "p1",
		Title:       "第 1 回の検討",
		Kind:        model.ChatKindMultiAgent,
		TurnRule:    model.TurnRuleRoundRobin,
		ScenePrompt: "静かな会議室で今期の方針を詰めている。",
	}
	participants := []model.Participant{
		{ID: "pa1", ChatID: "c1", DisplayName: "田中", ModelName: "qwen3-8b", SortOrder: 0},
		{ID: "pa2", ChatID: "c1", DisplayName: "佐藤", ModelName: "gemma3-4b", SortOrder: 1, DeletedAt: strPtr("2026-09-19 10:00")},
	}
	messages := []model.Message{
		{ID: "m1", ChatID: "c1", Role: "user", Content: "今期の方針を決めたい。"},
		{ID: "m2", ChatID: "c1", Role: "assistant", Content: "まず前期の数字を見ましょう。", ParticipantID: strPtr("pa1"), ModelName: strPtr("qwen3-8b")},
		{ID: "m3", ChatID: "c1", Role: "assistant", Content: "私は別の観点を出します。", ParticipantID: strPtr("pa2"), ModelName: strPtr("gemma3-4b")},
	}
	return project, chat, participants, messages
}

// TestBuildChatMarkdownFacilitatorRule covers TASK-21: the export names the
// facilitator, and reports the fallback the engine actually takes once that
// participant has left the roster rather than the setting the chat still holds.
func TestBuildChatMarkdownFacilitatorRule(t *testing.T) {
	project, chat, participants, messages := exportFixture()
	chat.TurnRule = model.TurnRuleFacilitatorAlternating
	chat.FacilitatorID = "pa1"
	if got := BuildChatMarkdown(project, chat, participants, messages, exportTestNow()); !strings.Contains(got, "facilitator_alternating（進行役「田中」と他の参加者が交互に発言する）") {
		t.Errorf("markdown does not name the facilitator\n--- got ---\n%s", got)
	}

	chat.FacilitatorID = "pa2" // 除籍済み
	if got := BuildChatMarkdown(project, chat, participants, messages, exportTestNow()); !strings.Contains(got, "facilitator_alternating（進行役が編成に居ないため、編成順に回す）") {
		t.Errorf("markdown does not report the fallback\n--- got ---\n%s", got)
	}
}

// TestBuildChatMarkdownWeightedRule: the weighted rule is spelled out, naming the
// facilitator only while it is on the roster.
func TestBuildChatMarkdownWeightedRule(t *testing.T) {
	project, chat, participants, messages := exportFixture()
	chat.TurnRule = model.TurnRuleWeighted
	chat.FacilitatorID = "pa1"
	if got := BuildChatMarkdown(project, chat, participants, messages, exportTestNow()); !strings.Contains(got, "weighted（呼びかけと沈黙の長さで次の話者を決める。進行役「田中」）") {
		t.Errorf("markdown does not describe the weighted rule\n--- got ---\n%s", got)
	}
	chat.FacilitatorID = "pa2" // 除籍済み
	if got := BuildChatMarkdown(project, chat, participants, messages, exportTestNow()); !strings.Contains(got, "weighted（呼びかけと沈黙の長さで次の話者を決める）\n") {
		t.Errorf("markdown names a removed facilitator\n--- got ---\n%s", got)
	}
}

func TestBuildChatMarkdownMultiAgent(t *testing.T) {
	project, chat, participants, messages := exportFixture()
	got := BuildChatMarkdown(project, chat, participants, messages, exportTestNow())

	// AC #2: the scene, the turn rule and the roster with display and model names.
	for _, want := range []string{
		"# 第 1 回の検討",
		"- プロジェクト: 調査プロジェクト",
		"- 出力日時: 2026-09-20 14:05",
		"## 場面設定",
		"静かな会議室で今期の方針を詰めている。",
		"## ターン進行ルール",
		"round_robin（編成順に回す）",
		"## 編成",
		"- 田中 (qwen3-8b)",
		"- 佐藤（除籍済み） (gemma3-4b)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown is missing %q\n--- got ---\n%s", want, got)
		}
	}

	// AC #3 / #4: every utterance is attributed, the removed participant stays
	// identifiable, and the human's turn is told apart from the participants'.
	for _, want := range []string{
		"### ユーザー\n\n今期の方針を決めたい。",
		"### 田中 (qwen3-8b)\n\nまず前期の数字を見ましょう。",
		"### 佐藤（除籍済み） (gemma3-4b)\n\n私は別の観点を出します。",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("transcript is missing %q\n--- got ---\n%s", want, got)
		}
	}

	if strings.Count(got, "## 会話") != 1 {
		t.Errorf("expected exactly one transcript heading\n--- got ---\n%s", got)
	}
}

// TestBuildChatMarkdownStateSection covers the export half of TASK-35 AC #4: the
// current state follows the scene, shared sheet first and then each roster
// participant's, one list item per line so a renderer keeps the lines apart.
// A removed participant's sheet is not part of the table and is left out.
func TestBuildChatMarkdownStateSection(t *testing.T) {
	project, chat, participants, messages := exportFixture()
	chat.StateSheet = "場所: 会議室\n時刻: 14 時"
	participants[0].StateSheet = "持ち時間: 5 分"
	participants[1].StateSheet = "持ち時間: 0 分"
	got := BuildChatMarkdown(project, chat, participants, messages, exportTestNow())

	want := "## 現在の状態\n\n### 共通\n\n- 場所: 会議室\n- 時刻: 14 時\n\n### 田中\n\n- 持ち時間: 5 分\n"
	section := strings.Index(got, want)
	if section < 0 {
		t.Fatalf("markdown lacks the state section %q\n--- got ---\n%s", want, got)
	}
	if scene, rule := strings.Index(got, "## 場面設定"), strings.Index(got, "## ターン進行ルール"); scene > section || rule < section {
		t.Errorf("state section must sit between the scene and the turn rule\n--- got ---\n%s", got)
	}
	if strings.Contains(got, "持ち時間: 0 分") {
		t.Errorf("markdown carries a removed participant's sheet\n--- got ---\n%s", got)
	}

	chat.StateSheet = ""
	participants[0].StateSheet = ""
	if got := BuildChatMarkdown(project, chat, participants, messages, exportTestNow()); strings.Contains(got, "## 現在の状態") {
		t.Errorf("empty sheets must leave the section out\n--- got ---\n%s", got)
	}
}

// A single-assistant chat has no scene, turn rule or roster to report, so those
// sections are absent while the heading and the transcript stay.
func TestBuildChatMarkdownAssistantChatOmitsMultiAgentSections(t *testing.T) {
	project := &model.Project{ID: "p1", Title: "調査プロジェクト"}
	chat := &model.Chat{
		ID:        "c2",
		ProjectID: "p1",
		Title:     "下調べ",
		Kind:      model.ChatKindAssistant,
		TurnRule:  model.TurnRuleRoundRobin,
	}
	messages := []model.Message{
		{ID: "m1", ChatID: "c2", Role: "user", Content: "要点を教えて。"},
		{ID: "m2", ChatID: "c2", Role: "assistant", Content: "3 点あります。", ModelName: strPtr("qwen3-8b")},
	}

	got := BuildChatMarkdown(project, chat, nil, messages, exportTestNow())

	for _, unwanted := range []string{"## 場面設定", "## ターン進行ルール", "## 編成"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("assistant chat should not carry %q\n--- got ---\n%s", unwanted, got)
		}
	}
	for _, want := range []string{
		"# 下調べ",
		"### ユーザー\n\n要点を教えて。",
		"### アシスタント (qwen3-8b)\n\n3 点あります。",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown is missing %q\n--- got ---\n%s", want, got)
		}
	}
}

// The model a turn actually ran against is what the message recorded; the
// participant's current setting may have been changed since.
func TestBuildChatMarkdownPrefersTheModelTheMessageRecorded(t *testing.T) {
	chat := &model.Chat{ID: "c1", Title: "t", Kind: model.ChatKindMultiAgent, TurnRule: model.TurnRuleManual}
	participants := []model.Participant{{ID: "pa1", ChatID: "c1", DisplayName: "田中", ModelName: "new-model"}}
	messages := []model.Message{
		{ID: "m1", ChatID: "c1", Role: "assistant", Content: "古い方のモデルで話した。", ParticipantID: strPtr("pa1"), ModelName: strPtr("old-model")},
		{ID: "m2", ChatID: "c1", Role: "assistant", Content: "記録が無い発言。", ParticipantID: strPtr("pa1")},
	}

	got := BuildChatMarkdown(nil, chat, participants, messages, exportTestNow())

	if !strings.Contains(got, "### 田中 (old-model)") {
		t.Errorf("expected the recorded model on the utterance\n--- got ---\n%s", got)
	}
	// With nothing recorded there is no better answer than the current setting.
	if !strings.Contains(got, "### 田中 (new-model)") {
		t.Errorf("expected the participant's current model as the fallback\n--- got ---\n%s", got)
	}
	if !strings.Contains(got, "- 田中 (new-model)") {
		t.Errorf("the roster reports the participant's current model\n--- got ---\n%s", got)
	}
}

func TestBuildChatMarkdownEmptyChat(t *testing.T) {
	chat := &model.Chat{ID: "c1", Title: "  ", Kind: model.ChatKindMultiAgent}

	got := BuildChatMarkdown(nil, chat, nil, nil, exportTestNow())

	for _, want := range []string{"# 無題のチャット", "## 場面設定\n\n（未設定）", "（未設定）", "（参加者なし）", "（発言なし）"} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown is missing %q\n--- got ---\n%s", want, got)
		}
	}
}

// A display name carrying a newline would otherwise close the heading it sits
// on and let the rest be read as markdown, attributing the message under it to
// a speaker that does not exist.
func TestBuildChatMarkdownFlattensMetadataNewlines(t *testing.T) {
	project := &model.Project{ID: "p1", Title: "調査\nプロジェクト"}
	chat := &model.Chat{ID: "c1", Title: "第 1 回\n### 偽の見出し", Kind: model.ChatKindMultiAgent}
	participants := []model.Participant{
		{ID: "pa1", ChatID: "c1", DisplayName: "Alice\n\n### Bob", ModelName: "qwen\n3-8b"},
	}
	messages := []model.Message{
		{ID: "m1", ChatID: "c1", Role: "assistant", Content: "本文", ParticipantID: strPtr("pa1")},
	}

	got := BuildChatMarkdown(project, chat, participants, messages, exportTestNow())

	for _, want := range []string{
		"# 第 1 回 ### 偽の見出し",
		"- プロジェクト: 調査 プロジェクト",
		"- Alice ### Bob (qwen 3-8b)",
		"### Alice ### Bob (qwen 3-8b)\n\n本文",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown is missing %q\n--- got ---\n%s", want, got)
		}
	}
	headingLines := 0
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "#") {
			headingLines++
		}
	}
	if headingLines != 6 {
		t.Errorf("metadata opened a heading of its own: %d heading lines, want 6\n--- got ---\n%s", headingLines, got)
	}
}
