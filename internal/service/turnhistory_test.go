package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/lang"
	"snzstudio/internal/model"
	"snzstudio/internal/repository"
)

func historyRoster(names ...string) []model.Participant {
	roster := make([]model.Participant, len(names))
	for i, name := range names {
		roster[i] = model.Participant{ID: strings.ToLower(name), DisplayName: name}
	}
	return roster
}

func spoken(id, content string) model.Message {
	return model.Message{Role: "assistant", Content: content, ParticipantID: strPtr(id)}
}

func historyRequest(roster []model.Participant, window int) turnHistoryRequest {
	return turnHistoryRequest{speaker: &roster[0], participants: roster, windowChars: window, language: lang.Japanese}
}

func assembled(t *testing.T, messages []model.Message, req turnHistoryRequest) []model.Message {
	t.Helper()
	history, err := assembleTurnHistory(messages, req)
	if err != nil {
		t.Fatalf("assembleTurnHistory: %v", err)
	}
	return history
}

func historyChars(history []model.Message) int {
	total := 0
	for _, m := range history {
		total += runeLen(m.Content)
	}
	return total
}

func historyContains(history []model.Message, fragment string) bool {
	for _, m := range history {
		if strings.Contains(m.Content, fragment) {
			return true
		}
	}
	return false
}

func isOmissionLine(m model.Message) bool {
	return m.Role == "user" && strings.HasPrefix(m.Content, "（ここで ")
}

// TestAssembleTurnHistorySelectsAndMarksGaps covers AC #1: an utterance from before
// the recent section comes in with the one it answers, in transcript order, with
// an omission line of its own wherever the transcript is not contiguous.
func TestAssembleTurnHistorySelectsAndMarksGaps(t *testing.T) {
	roster := historyRoster("Alice", "Bob")
	filler := strings.Repeat("あ", 400)
	messages := []model.Message{spoken("alice", filler+"序"), spoken("bob", filler+"破")}
	for i := 0; i < 8; i++ {
		messages = append(messages, spoken("bob", fmt.Sprintf("%s途中%d", filler, i)))
	}
	messages = append(messages, model.Message{Role: "user", Content: filler + "問い"})
	messages = append(messages, spoken("bob", filler+"近1"), spoken("alice", filler+"近2"), spoken("bob", filler+"近3"), spoken("alice", filler+"近4"))

	history := assembled(t, messages, historyRequest(roster, 100000))

	if !historyContains(history, "序") || !historyContains(history, "破") {
		t.Fatalf("the opening pair is missing: %+v", history)
	}
	for _, recent := range []string{"問い", "近1", "近2", "近3", "近4"} {
		if !historyContains(history, recent) {
			t.Fatalf("recent utterance %s is missing", recent)
		}
	}
	omissions := 0
	for _, m := range history {
		if isOmissionLine(m) {
			omissions++
		}
	}
	if omissions == 0 {
		t.Fatalf("no omission line between the selected and the recent: %+v", history)
	}
	if isOmissionLine(history[len(history)-1]) || !strings.HasSuffix(history[len(history)-1].Content, "近4") {
		t.Fatalf("the history does not end with the last utterance: %+v", history[len(history)-1])
	}
	if chars := historyChars(history); chars > 4000 {
		t.Fatalf("history is %d characters, over the budget of 4000", chars)
	}
}

// TestAssembleTurnHistoryCountsMappedText covers AC #2: the budget pays for what
// the model reads, so the name prefix, the record lines and the omission line all
// count, and a command-only message is as long as its record line.
func TestAssembleTurnHistoryCountsMappedText(t *testing.T) {
	roster := historyRoster("Alice", "Bob")
	withRoll := model.Message{Role: "user", DiceRolls: []model.DiceRoll{{Command: "/roll 1d6", Expression: "1d6", Dice: []int{4}, Total: 4}}}
	entries := mapHistoryForSpeaker([]model.Message{withRoll, spoken("bob", "本文"), spoken("alice", "自分")}, &roster[0], roster, lang.Japanese)
	if len(entries) != 3 {
		t.Fatalf("%d entries, want the command-only message kept", len(entries))
	}
	if entries[0].chars != runeLen(entries[0].mapped.Content) || entries[0].chars <= runeLen("ユーザー: ") {
		t.Fatalf("command-only message counted as %d: %q", entries[0].chars, entries[0].mapped.Content)
	}
	if entries[1].chars != runeLen("Bob: 本文") || entries[2].chars != runeLen("自分") {
		t.Fatalf("mapped lengths = %d, %d", entries[1].chars, entries[2].chars)
	}

	// A body that fits the budget alone stops fitting once its record line counts.
	roll := []model.DiceRoll{{Command: "/roll 1d6", Expression: "1d6", Dice: []int{4}, Total: 4}}
	last := spoken("bob", "末尾")
	room := 4000 - runeLen("Bob: 末尾") - runeLen("ユーザー: ")
	bare := model.Message{Role: "user", Content: strings.Repeat("あ", room)}
	if history := assembled(t, []model.Message{bare, last}, historyRequest(roster, 100000)); !historyContains(history, "あああ") {
		t.Fatalf("an utterance exactly at the budget was dropped: %+v", history)
	}
	withRecord := bare
	withRecord.DiceRolls = roll
	if history := assembled(t, []model.Message{withRecord, last}, historyRequest(roster, 100000)); historyContains(history, "あああ") {
		t.Fatalf("the record line was not counted: %+v", history)
	}

	// Command-only messages are not free, however many there are.
	messages := make([]model.Message, 0, 301)
	for i := 0; i < 300; i++ {
		messages = append(messages, model.Message{Role: "user", DiceRolls: roll})
	}
	messages = append(messages, last)
	if chars := historyChars(assembled(t, messages, historyRequest(roster, 100000))); chars > 4000 || chars < 3000 {
		t.Fatalf("history is %d characters, want it filling the budget of 4000", chars)
	}
}

// TestAssembleTurnHistoryFollowsRoster covers AC #3.
func TestAssembleTurnHistoryFollowsRoster(t *testing.T) {
	entriesFor := func(count int) []turnHistoryEntry {
		entries := make([]turnHistoryEntry, 0, count+1)
		for i := 0; i < count; i++ {
			entries = append(entries, turnHistoryEntry{source: spoken("alice", "x")})
		}
		// A human between utterances is carried along without being counted.
		entries = append(entries[:count-1:count-1], turnHistoryEntry{source: model.Message{Role: "user"}}, entries[count-1])
		return entries
	}
	for _, size := range []int{2, 3, 5} {
		entries := entriesFor(30)
		start := turnRecentSectionStart(entries, size*TurnRecentRounds)
		participants := 0
		for _, entry := range entries[start:] {
			if !isTurnHuman(entry.source) {
				participants++
			}
		}
		if participants != size*TurnRecentRounds {
			t.Fatalf("roster of %d: %d participant utterances in the recent section", size, participants)
		}
	}

	// A narrow window caps the budget however large the roster is.
	roster := historyRoster("Alice", "Bob", "Carol", "Dave")
	messages := []model.Message{}
	for i := 0; i < 40; i++ {
		messages = append(messages, spoken(roster[i%4].ID, strings.Repeat("あ", 300)))
	}
	if chars := historyChars(assembled(t, messages, historyRequest(roster, 1500))); chars > 1500 {
		t.Fatalf("history is %d characters, over the window of 1500", chars)
	}
	if chars := historyChars(assembled(t, messages, historyRequest(roster, 100000))); chars > 8000 || chars < 5000 {
		t.Fatalf("history is %d characters, want near the roster budget of 8000", chars)
	}
}

// TestAssembleTurnHistoryRecentOverBudget covers AC #4.
func TestAssembleTurnHistoryRecentOverBudget(t *testing.T) {
	roster := historyRoster("Alice", "Bob")
	long := strings.Repeat("あ", 1500)
	messages := []model.Message{spoken("bob", "選ばれるはずの先頭")}
	messages = append(messages, spoken("alice", long), spoken("bob", long), spoken("alice", long), spoken("bob", long+"末尾"))

	history := assembled(t, messages, historyRequest(roster, 100000))

	if historyContains(history, "選ばれるはずの先頭") {
		t.Fatalf("a selected utterance came in with the recent section over budget: %+v", history)
	}
	if !strings.HasSuffix(history[len(history)-1].Content, "末尾") {
		t.Fatalf("the last utterance is missing: %+v", history[len(history)-1])
	}
	if !isOmissionLine(history[0]) {
		t.Fatalf("no omission line in front of what fit: %+v", history[0])
	}
	if chars := historyChars(history); chars > 4000 {
		t.Fatalf("history is %d characters, over the budget of 4000", chars)
	}
}

// TestAssembleTurnHistoryLastUtteranceOverBudget: the last utterance may pass the
// per-participant budget, but nothing else comes in beside it.
func TestAssembleTurnHistoryLastUtteranceOverBudget(t *testing.T) {
	roster := historyRoster("Alice", "Bob")
	body := strings.Repeat("あ", 5000)
	history := assembled(t, []model.Message{spoken("alice", "前"), spoken("bob", body)}, historyRequest(roster, 100000))
	if len(history) != 2 || !isOmissionLine(history[0]) || history[1].Content != "Bob: "+body {
		t.Fatalf("history = %d messages, want the omission line and the whole utterance", len(history))
	}
}

// TestAssembleTurnHistoryElidesOverWindow covers AC #5's history half.
func TestAssembleTurnHistoryElidesOverWindow(t *testing.T) {
	roster := historyRoster("Alice", "Bob")
	body := "前提" + strings.Repeat("あ", 5000) + "問い"
	history := assembled(t, []model.Message{spoken("bob", body)}, historyRequest(roster, 1000))
	if len(history) != 1 {
		t.Fatalf("%d messages, want the last utterance alone", len(history))
	}
	got := history[0].Content
	if !strings.HasPrefix(got, "Bob: 前提") || !strings.HasSuffix(got, "問い") || !strings.Contains(got, "……（中略 ") {
		t.Fatalf("elided utterance = %q", got)
	}
	if chars := runeLen(got); chars > 1000 || chars < 990 {
		t.Fatalf("elided to %d characters, want just under the window of 1000", chars)
	}
}

// TestAssembleTurnHistoryLastUtteranceAtWindow covers AC #8: with something before
// it, the utterance gives up the room of the omission line instead of pushing the
// prompt past the window.
func TestAssembleTurnHistoryLastUtteranceAtWindow(t *testing.T) {
	roster := historyRoster("Alice", "Bob")
	const window = 1000
	body := strings.Repeat("あ", window-runeLen("Bob: "))
	before := spoken("alice", strings.Repeat("前", 100))
	history := assembled(t, []model.Message{before, spoken("bob", body)}, historyRequest(roster, window))
	if len(history) != 2 || !isOmissionLine(history[0]) {
		t.Fatalf("history = %+v, want the omission line and the shortened utterance", history)
	}
	if chars := historyChars(history); chars > window {
		t.Fatalf("history is %d characters, over the window of %d", chars, window)
	}
}

// TestAssembleTurnHistoryStaysInsideWindow runs AC #8's invariant across window
// sizes down to the ones too narrow for the omission line.
func TestAssembleTurnHistoryStaysInsideWindow(t *testing.T) {
	roster := historyRoster("Alice", "Bob", "Carol")
	messages := []model.Message{}
	for i := 0; i < 25; i++ {
		messages = append(messages, spoken(roster[i%3].ID, fmt.Sprintf("発言%d %s", i, strings.Repeat("あ", i*17%400))))
	}
	for window := 1; window < 7000; window += 13 {
		history := assembled(t, messages, historyRequest(roster, window))
		if chars := historyChars(history); chars > window {
			t.Fatalf("window %d: history is %d characters", window, chars)
		}
	}
}

// TestCheckTurnHistoryFitsRefusesOverWindow is the last check of AC #8: the
// assembly should never trip it, so it is exercised on its own.
func TestCheckTurnHistoryFitsRefusesOverWindow(t *testing.T) {
	history := []model.Message{{Content: "（ここで 3 発言を省略）"}, {Content: strings.Repeat("あ", 10)}}
	total := historyChars(history)
	if _, err := checkTurnHistoryFits(history, total); err != nil {
		t.Fatalf("a history at the window was refused: %v", err)
	}
	if _, err := checkTurnHistoryFits(history, total-1); !errors.Is(err, errTurnHistoryOverWindow) {
		t.Fatalf("err = %v, want errTurnHistoryOverWindow", err)
	}
}

// TestScoreTurnHistory covers AC #6, one bonus at a time against a baseline that
// earns nothing.
func TestScoreTurnHistory(t *testing.T) {
	roster := historyRoster("Alice", "Bob")
	req := historyRequest(roster, 100000)
	// Far enough from the opening, and from any word of the rest.
	padding := func() []model.Message {
		return []model.Message{spoken("bob", "一"), spoken("bob", "二")}
	}
	score := func(messages []model.Message, target int, mutate func(*turnHistoryRequest)) int {
		t.Helper()
		r := req
		if mutate != nil {
			mutate(&r)
		}
		return scoreTurnHistory(mapHistoryForSpeaker(messages, r.speaker, r.participants, lang.Japanese), r)[target]
	}

	t.Run("nothing to earn", func(t *testing.T) {
		if got := score(append(padding(), spoken("bob", "foo")), 2, nil); got != 0 {
			t.Fatalf("score = %d, want 0", got)
		}
	})
	t.Run("human", func(t *testing.T) {
		if got := score(append(padding(), model.Message{Role: "user", Content: "foo"}), 2, nil); got != turnScoreHuman {
			t.Fatalf("score = %d, want %d", got, turnScoreHuman)
		}
	})
	t.Run("speaker's own and addressed", func(t *testing.T) {
		if got := score(append(padding(), spoken("alice", "foo")), 2, nil); got != turnScoreSpeakerInvolved {
			t.Fatalf("own = %d, want %d", got, turnScoreSpeakerInvolved)
		}
		addressed := spoken("bob", "foo")
		addressed.AddressedParticipantIDs = []string{"alice"}
		if got := score(append(padding(), addressed), 2, nil); got != turnScoreSpeakerInvolved {
			t.Fatalf("addressed = %d, want %d", got, turnScoreSpeakerInvolved)
		}
	})
	t.Run("opening", func(t *testing.T) {
		messages := []model.Message{spoken("bob", "foo"), spoken("bob", "bar"), spoken("bob", "baz")}
		if got := score(messages, 0, nil); got != turnScoreOpening {
			t.Fatalf("first = %d, want %d", got, turnScoreOpening)
		}
		if got := score(messages, 2, nil); got != 0 {
			t.Fatalf("past the roster size = %d, want 0", got)
		}
	})
	t.Run("echo counts later uses of a first use, per term and in total", func(t *testing.T) {
		later := func(words string, n int) []model.Message {
			out := make([]model.Message, n)
			for i := range out {
				out[i] = spoken("bob", words)
			}
			return out
		}
		// Later utterances beyond the last three are the only ones the rare-term
		// bonus cannot see, so the padding keeps the two apart.
		base := append(padding(), spoken("bob", "apple"))
		noise := later("noise", 3)
		one := append(append(append([]model.Message{}, base...), later("apple", 1)...), noise...)
		if got := score(one, 2, nil); got != 1 {
			t.Fatalf("one echo = %d, want 1", got)
		}
		capped := append(append(append([]model.Message{}, base...), later("apple", 6)...), noise...)
		if got := score(capped, 2, nil); got != turnScoreEchoPerTerm {
			t.Fatalf("per-term cap = %d, want %d", got, turnScoreEchoPerTerm)
		}
		many := append(padding(), spoken("bob", "apple grape melon"))
		many = append(many, later("apple grape melon", 4)...)
		many = append(many, noise...)
		if got := score(many, 2, nil); got != turnScoreEchoTotal {
			t.Fatalf("total cap = %d, want %d", got, turnScoreEchoTotal)
		}
		// A second use is not a first use.
		if got := score(one, 3, nil); got != 0 {
			t.Fatalf("second use = %d, want 0", got)
		}
	})
	t.Run("echo leaves out names and scene words", func(t *testing.T) {
		messages := append(padding(), spoken("bob", "三"), spoken("bob", "castle Carol"), spoken("bob", "castle Carol"), spoken("bob", "noise"), spoken("bob", "noise"), spoken("bob", "noise"))
		withCarol := func(r *turnHistoryRequest) {
			r.participants = append(historyRoster("Alice", "Bob"), model.Participant{ID: "carol", DisplayName: "Carol"})
			r.scene = "the castle"
		}
		if got := score(messages, 3, withCarol); got != 0 {
			t.Fatalf("score = %d, want 0 for a name and a scene word", got)
		}
	})
	t.Run("relevance counts only rare terms shared with the latest three", func(t *testing.T) {
		messages := []model.Message{}
		for i := 0; i < 30; i++ {
			messages = append(messages, spoken("bob", fmt.Sprintf("topic filler%d", i)))
		}
		messages[5] = spoken("bob", "topic oldfact otherfact")
		messages = append(messages, spoken("bob", "topic oldfact"), spoken("bob", "topic otherfact"), spoken("bob", "topic"))
		// topic is in nearly every utterance and earns nothing here; its echo is
		// paid to the first utterance only.
		echo := min(1+1, turnScoreEchoTotal) // oldfact and otherfact, one later use each
		if got := score(messages, 5, nil); got != 2+echo {
			t.Fatalf("score = %d, want %d", got, 2+echo)
		}
	})
	t.Run("relevance has a cap", func(t *testing.T) {
		messages := []model.Message{}
		for i := 0; i < 30; i++ {
			messages = append(messages, spoken("bob", fmt.Sprintf("filler%d", i)))
		}
		messages[5] = spoken("bob", "kiwi plum pear lime")
		messages = append(messages, spoken("bob", "kiwi plum pear lime"))
		if got := score(messages, 5, nil); got != turnScoreRelevance+4 {
			t.Fatalf("score = %d, want %d", got, turnScoreRelevance+4)
		}
	})
	t.Run("length earns nothing", func(t *testing.T) {
		long := strings.Repeat("foo ", 500)
		if got := score(append(padding(), spoken("bob", long)), 2, nil); got != 0 {
			t.Fatalf("score = %d, want 0", got)
		}
	})
}

// TestSelectTurnEntriesPrefersScoreThenNewer: the selection follows the score, not
// the position, and an equal score goes to the newer utterance.
func TestSelectTurnEntriesPrefersScoreThenNewer(t *testing.T) {
	roster := historyRoster("Alice", "Bob")
	body := func(i int) string { return strings.Repeat(fmt.Sprintf("word%d ", i), 50) }
	messages := make([]model.Message, 30)
	for i := range messages {
		messages[i] = spoken("bob", body(i))
	}
	messages[5] = model.Message{Role: "user", Content: body(5)}

	history := assembled(t, messages, historyRequest(roster, 100000))

	for _, kept := range []int{0, 1, 4, 5, 24, 25, 26, 29} {
		if !historyContains(history, fmt.Sprintf("word%d ", kept)) {
			t.Fatalf("utterance %d is missing", kept)
		}
	}
	for _, dropped := range []int{2, 3, 6, 10, 22, 23} {
		if historyContains(history, fmt.Sprintf("word%d ", dropped)) {
			t.Fatalf("utterance %d should have lost to the rest", dropped)
		}
	}
}

// contextServer answers LM Studio's listing with a fixed context length.
func contextServer(t *testing.T, modelKey string, contextLength int) *turnLLMServer {
	t.Helper()
	inner := newTurnLLMServer(t, "返答", nil)
	listing := fmt.Sprintf(`{"models":[
		{"type":"llm","key":%q,"loaded_instances":[]},
		{"type":"llm","key":%q,"loaded_instances":[{"id":"a","config":{"context_length":%d}}]},
		{"type":"llm","key":"another","loaded_instances":[{"id":"b","config":{"context_length":1}}]}
	]}`, modelKey, modelKey, contextLength)
	handler := inner.Config.Handler
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/models" {
			io.WriteString(w, listing)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	inner.Server = proxy
	return inner
}

// TestLoadedContextLength reads the listing the way LM Link leaves it: the same key
// twice, one of them with nothing loaded.
func TestLoadedContextLength(t *testing.T) {
	cfg := testConfig(config.Settings{LLMTimeoutMs: 5000})
	client := NewLLMClient(cfg)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"models":[
			{"type":"llm","key":"m","loaded_instances":[]},
			{"type":"llm","key":"m","loaded_instances":[{"config":{"context_length":40960}},{"config":{"context_length":8192}}]},
			{"type":"embedding","key":"m","loaded_instances":[{"config":{"context_length":1}}]},
			{"type":"llm","key":"idle","loaded_instances":[]}
		]}`)
	}))
	defer srv.Close()

	if got, ok := client.LoadedContextLength("m", srv.URL+"/v1"); !ok || got != 8192 {
		t.Fatalf("LoadedContextLength = (%d, %v), want the smallest instance", got, ok)
	}
	if _, ok := client.LoadedContextLength("idle", srv.URL+"/v1"); ok {
		t.Fatal("a model with nothing loaded reported a length")
	}
	notLMStudio := httptest.NewServer(http.NotFoundHandler())
	defer notLMStudio.Close()
	if _, ok := client.LoadedContextLength("m", notLMStudio.URL+"/v1"); ok {
		t.Fatal("an endpoint without the listing reported a length")
	}
}

// TestTurnEngineFitsPromptToWindow covers AC #1 and #5 through a real turn: the
// loaded context length decides what is sent, and the whole prompt stays inside it.
func TestTurnEngineFitsPromptToWindow(t *testing.T) {
	g := newTurnGraph(t)
	srv := contextServer(t, "model-alice", 3000)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleManual, "場面", srv.URL, "Alice", "Bob")
	g.addMessage(t, chat.ID, "assistant", "前の発言", &roster[1].ID)
	g.addMessage(t, chat.ID, "assistant", strings.Repeat("あ", 5000)+"問い", &roster[1].ID)

	if _, err := g.engine.RunTurn(chat.ID, roster[0].ID, nil, nil); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	requests := srv.captured()
	if len(requests) != 1 {
		t.Fatalf("%d completions, want 1", len(requests))
	}
	total := 0
	for _, m := range requests[0].Messages {
		total += runeLen(m.Content)
	}
	if allowed := 3000 - turnGenerationReserveTokens; total > allowed {
		t.Fatalf("prompt is %d characters, over the %d the window leaves", total, allowed)
	}
	last := requests[0].Messages[len(requests[0].Messages)-1]
	if !strings.HasSuffix(last.Content, "問い") || !strings.Contains(last.Content, "中略") {
		t.Fatalf("final message = %q", last.Content)
	}
}

// TestTurnEngineRefusesSettingsOverWindow covers AC #5's other half: the turn does
// not start, nothing is announced or generated, and the error names what to shorten.
func TestTurnEngineRefusesSettingsOverWindow(t *testing.T) {
	g := newTurnGraph(t)
	srv := contextServer(t, "model-alice", 2100)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleManual, strings.Repeat("場", 500), srv.URL, "Alice")

	announced := false
	_, err := g.engine.RunTurn(chat.ID, roster[0].ID, func(SpeakerChoice) { announced = true }, nil)
	if !errors.Is(err, ErrTurnSettingsTooLong) {
		t.Fatalf("err = %v, want ErrTurnSettingsTooLong", err)
	}
	for _, fragment := range []string{"Alice", "scene", "500"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not name %q", err, fragment)
		}
	}
	if announced {
		t.Fatal("the speaker was announced for a turn that never started")
	}
	if len(srv.captured()) != 0 {
		t.Fatal("a completion ran for a refused turn")
	}
	messages, listErr := g.chats.ListMessages(chat.ID)
	if listErr != nil || len(messages) != 0 {
		t.Fatalf("messages = %d, err = %v, want nothing stored", len(messages), listErr)
	}
}

// TestTurnEngineDefaultContextForOtherEndpoints: an endpoint without LM Studio's
// listing gets the default rather than a failed turn.
func TestTurnEngineDefaultContextForOtherEndpoints(t *testing.T) {
	g := newTurnGraph(t)
	srv := newTurnLLMServer(t, "返答", nil)
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleManual, "場面", srv.URL, "Alice", "Bob")
	body := strings.Repeat("あ", 20000)
	if _, err := g.chats.AddMessage(repository.AddMessageInput{ChatID: chat.ID, Role: "user", Content: body}); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	if _, err := g.engine.RunTurn(chat.ID, roster[0].ID, nil, nil); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	messages := srv.captured()[0].Messages
	if got := runeLen(messages[len(messages)-1].Content); got > turnDefaultContextTokens-turnGenerationReserveTokens {
		t.Fatalf("final message is %d characters, over the default window", got)
	}
}
