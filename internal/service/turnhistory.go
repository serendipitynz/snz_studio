package service

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"snzstudio/internal/lang"
	"snzstudio/internal/model"
	"snzstudio/internal/search"
)

// TurnRecentRounds is R of design §4.9.1: the recent section holds roster size × R
// participant utterances. It is exported so the spectator view can state it
// instead of keeping a copy of the number.
const TurnRecentRounds = 2

// The values measured in design §4.9.7. The conversion factor sits below what
// either measured model showed (1.10 and 1.35), so the window is under-filled
// rather than overrun; the reserve only shrinks the prompt and does not cap the
// generation.
const (
	turnBudgetPerParticipantChars = 2000
	turnGenerationReserveTokens   = 2048
	turnCharsPerToken             = 1.0
	turnDefaultContextTokens      = 8192
)

// The utterance score of design §4.9.3. Length earns nothing: it is what the
// budget charges for, not a sign of value.
const (
	turnScoreHuman           = 3
	turnScoreEchoPerTerm     = 3
	turnScoreEchoTotal       = 5
	turnScoreRelevance       = 3
	turnScoreSpeakerInvolved = 2
	turnScoreOpening         = 2

	turnRareTermPercent = 10
	turnRareTermMinimum = 2
)

// errTurnHistoryOverWindow means the assembly broke its own invariant. Sending the
// prompt anyway would only trade this for the endpoint's context-size error.
var errTurnHistoryOverWindow = errors.New("service: assembled history exceeds the model's window")

// turnWindowChars is the room left for the history once the system prompt and the
// hand-off line are paid for (design §4.9.2 step 1). The hand-off is charged even
// on a turn that ends up not using it: whether it does depends on the history
// this number is about to shape.
func turnWindowChars(contextTokens int, systemPrompt, cue string) int {
	generated := float64(contextTokens-turnGenerationReserveTokens) * turnCharsPerToken
	return int(generated) - runeLen(systemPrompt) - runeLen(cue)
}

// turnHistoryEntry is one message that reaches the prompt, next to the message it
// came from: the budget counts the mapped text, while the score reads the source.
type turnHistoryEntry struct {
	source model.Message
	mapped model.Message
	chars  int
}

type turnHistoryRequest struct {
	speaker      *model.Participant
	participants []model.Participant
	scene        string
	windowChars  int
	language     lang.Language
}

// assembleTurnHistory picks what the speaker reads, in transcript order (design
// §4.9.2). It runs per speaker because models and windows differ by participant
// and the score has a per-speaker part. The caller refuses a window of 0 or less.
//
// Extension point: TASK-105 adds the pin to scoreTurnHistory, and TASK-106 stores
// the selected and shortened utterances as references.
func assembleTurnHistory(messages []model.Message, req turnHistoryRequest) ([]model.Message, error) {
	entries := mapHistoryForSpeaker(messages, req.speaker, req.participants, req.language)
	if len(entries) == 0 {
		return nil, nil
	}
	text := promptTextFor(req.language)
	rosterSize := len(onRoster(req.participants))
	budget := min(turnBudgetPerParticipantChars*rosterSize, req.windowChars)

	last := len(entries) - 1
	// The line is charged at its longest count: its real count is only known once
	// the selection is done, and fewer digits can only leave room to spare.
	omissionChars := 0
	if last > 0 {
		omissionChars = runeLen(fmt.Sprintf(text.historyOmitted, last))
	}
	if req.windowChars <= omissionChars {
		// Nothing but a shortened last utterance can fit, and no line is left to
		// say what was dropped.
		entries[last] = shortenTurnEntry(entries[last], req.windowChars, text)
		return checkTurnHistoryFits(arrangeTurnHistory(entries[last:], []bool{true}, text), req.windowChars)
	}
	entries[last] = shortenTurnEntry(entries[last], req.windowChars-omissionChars, text)

	included := make([]bool, len(entries))
	included[last] = true
	recentStart := turnRecentSectionStart(entries, rosterSize*TurnRecentRounds)
	used := entries[last].chars
	recentFits := true
	for i := last - 1; i >= recentStart; i-- {
		needed := entries[i].chars
		if i > 0 {
			needed += omissionChars
		}
		if used+needed > budget {
			recentFits = false
			break
		}
		used += entries[i].chars
		included[i] = true
	}
	if recentFits && recentStart > 0 {
		selectTurnEntries(entries, included, recentStart, budget, req, text)
	}
	return checkTurnHistoryFits(arrangeTurnHistory(entries, included, text), req.windowChars)
}

// mapHistoryForSpeaker rewrites the transcript into the speaker's own point of
// view, which is what an OpenAI-compatible API can express: its own past turns
// become assistant messages, and everyone else's (participants and the human
// alike) become user messages prefixed with the speaker's name (§4.3).
func mapHistoryForSpeaker(messages []model.Message, speaker *model.Participant, participants []model.Participant, language lang.Language) []turnHistoryEntry {
	labels := speakerLabels(participants)
	entries := make([]turnHistoryEntry, 0, len(messages))
	for _, m := range messages {
		content := contentWithRecords(m, language)
		if content == "" {
			continue
		}
		mapped := model.Message{Role: "assistant", Content: content}
		if m.ParticipantID == nil || *m.ParticipantID != speaker.ID {
			mapped = model.Message{Role: "user", Content: speakerLabel(m, labels, language) + ": " + content}
		}
		entries = append(entries, turnHistoryEntry{source: m, mapped: mapped, chars: runeLen(mapped.Content)})
	}
	return entries
}

// shortenTurnEntry keeps the head and the tail of an over-long utterance, where
// the premise and the question tend to sit. The turn is not failed instead: with
// no way to edit or delete a message, every later turn would fail the same way
// (§4.9.2).
func shortenTurnEntry(entry turnHistoryEntry, limit int, text *promptText) turnHistoryEntry {
	if entry.chars <= limit {
		return entry
	}
	runes := []rune(entry.mapped.Content)
	// The longest possible count sizes the marker, as with the omission line.
	keep := limit - runeLen(fmt.Sprintf(text.historyElided, len(runes)))
	if keep < 2 {
		entry.mapped.Content = string(runes[:limit])
	} else {
		head, tail := keep-keep/2, keep/2
		entry.mapped.Content = string(runes[:head]) + fmt.Sprintf(text.historyElided, len(runes)-keep) + string(runes[len(runes)-tail:])
	}
	entry.chars = runeLen(entry.mapped.Content)
	return entry
}

// turnRecentSectionStart walks back until count participant utterances are in;
// human interventions between them belong to the section without counting.
func turnRecentSectionStart(entries []turnHistoryEntry, count int) int {
	spoken := 0
	for i := len(entries) - 1; i >= 0; i-- {
		if isTurnHuman(entries[i].source) {
			continue
		}
		spoken++
		if spoken == count {
			return i
		}
	}
	return 0
}

// selectTurnEntries adds utterances from before the recent section by score, each
// with the one it answers: an answer read without its question misleads more than
// it informs. A pair that does not fit is skipped, not a reason to stop, because a
// shorter one further down the order may still fit.
func selectTurnEntries(entries []turnHistoryEntry, included []bool, recentStart, budget int, req turnHistoryRequest, text *promptText) {
	scores := scoreTurnHistory(entries, req)
	candidates := make([]int, recentStart)
	for i := range candidates {
		candidates[i] = recentStart - 1 - i
	}
	sort.SliceStable(candidates, func(a, b int) bool {
		return scores[candidates[a]] > scores[candidates[b]]
	})

	tentative := make([]bool, len(included))
	for _, candidate := range candidates {
		if included[candidate] {
			continue
		}
		copy(tentative, included)
		tentative[candidate] = true
		if candidate > 0 {
			tentative[candidate-1] = true
		}
		if turnLayoutChars(entries, tentative, text) <= budget {
			copy(included, tentative)
		}
	}
}

// turnLayoutChars is the history's mapped length with the given entries in. It is
// recounted for every candidate because a pair can add an omission line, close a
// gap and remove one, or shorten the count in one.
func turnLayoutChars(entries []turnHistoryEntry, included []bool, text *promptText) int {
	total, omitted := 0, 0
	for i, entry := range entries {
		if !included[i] {
			omitted++
			continue
		}
		if omitted > 0 {
			total += runeLen(fmt.Sprintf(text.historyOmitted, omitted))
			omitted = 0
		}
		total += entry.chars
	}
	return total
}

// arrangeTurnHistory lays the entries out in transcript order. The omission line
// is its own user message rather than part of a speaker's turn, or the speaker
// would take it for its own style and copy it (§4.9.2 step 6).
func arrangeTurnHistory(entries []turnHistoryEntry, included []bool, text *promptText) []model.Message {
	history := make([]model.Message, 0, len(entries))
	omitted := 0
	for i, entry := range entries {
		if !included[i] {
			omitted++
			continue
		}
		if omitted > 0 {
			history = append(history, model.Message{Role: "user", Content: fmt.Sprintf(text.historyOmitted, omitted)})
			omitted = 0
		}
		history = append(history, entry.mapped)
	}
	return history
}

func checkTurnHistoryFits(history []model.Message, windowChars int) ([]model.Message, error) {
	total := 0
	for _, m := range history {
		total += runeLen(m.Content)
	}
	if total > windowChars {
		return nil, fmt.Errorf("%w: %d characters in a window of %d", errTurnHistoryOverWindow, total, windowChars)
	}
	return history, nil
}

// scoreTurnHistory estimates how much later turns would miss each utterance
// (design §4.9.3). Display names and scene words are not counted: names recur in
// every call and the scene is always in the system prompt, so they say nothing
// about the utterance that happened to mention them first.
func scoreTurnHistory(entries []turnHistoryEntry, req turnHistoryRequest) []int {
	ignored := map[string]bool{}
	for _, term := range search.QueryTerms(req.scene) {
		ignored[term] = true
	}
	for _, p := range req.participants {
		for _, term := range search.QueryTerms(p.DisplayName) {
			ignored[term] = true
		}
	}

	terms := make([][]string, len(entries))
	utterancesWith := map[string]int{}
	for i, entry := range entries {
		for _, term := range search.QueryTerms(entry.source.Content) {
			if !ignored[term] {
				terms[i] = append(terms[i], term)
				utterancesWith[term]++
			}
		}
	}
	recent := latestTurnUtterances(entries)
	rareLimit := max(turnRareTermMinimum, len(entries)*turnRareTermPercent/100)
	rosterSize := len(onRoster(req.participants))

	scores := make([]int, len(entries))
	seen := map[string]bool{}
	for i, entry := range entries {
		score := 0
		if isTurnHuman(entry.source) {
			score += turnScoreHuman
		}
		if isTurnSpeakerInvolved(entry.source, req.speaker.ID) {
			score += turnScoreSpeakerInvolved
		}
		if i < rosterSize {
			score += turnScoreOpening
		}

		echo, relevance := 0, 0
		for _, term := range terms[i] {
			if !seen[term] {
				seen[term] = true
				echo += min(utterancesWith[term]-1, turnScoreEchoPerTerm)
			}
			if utterancesWith[term] <= rareLimit && inOtherTurnUtterance(recent, i, term, terms) {
				relevance++
			}
		}
		scores[i] = score + min(echo, turnScoreEchoTotal) + min(relevance, turnScoreRelevance)
	}
	return scores
}

// latestTurnUtterances is the range buildTurnMaterialQuery searches with, by the
// same reading: the last few with a body, however many command-only messages lie
// between them.
func latestTurnUtterances(entries []turnHistoryEntry) []int {
	latest := make([]int, 0, turnMaterialRecentUtterances)
	for i := len(entries) - 1; i >= 0 && len(latest) < turnMaterialRecentUtterances; i-- {
		if strings.TrimSpace(entries[i].source.Content) != "" {
			latest = append(latest, i)
		}
	}
	return latest
}

// inOtherTurnUtterance leaves the utterance itself out: on a small roster the
// recent section can be shorter than the query range, and an utterance would
// otherwise be relevant to itself.
func inOtherTurnUtterance(utterances []int, self int, term string, terms [][]string) bool {
	for _, i := range utterances {
		if i != self && slices.Contains(terms[i], term) {
			return true
		}
	}
	return false
}

// isTurnHuman reads the same field for a human and for an assistant message from
// before the chat became multi-agent, as the score's data source does (§4.9.3).
func isTurnHuman(m model.Message) bool {
	return m.ParticipantID == nil || *m.ParticipantID == ""
}

func isTurnSpeakerInvolved(m model.Message, speakerID string) bool {
	if m.ParticipantID != nil && *m.ParticipantID == speakerID {
		return true
	}
	return slices.Contains(m.AddressedParticipantIDs, speakerID)
}

// turnSettingsTooLong names what the human can shorten in the composition panel
// when the system prompt alone fills the window (§4.9.2 step 1).
func turnSettingsTooLong(chat *model.Chat, speaker *model.Participant, participants []model.Participant, systemPrompt string, contextTokens int, language lang.Language) error {
	settings := []struct {
		name  string
		chars int
	}{
		{"scene", runeLen(chat.ScenePrompt)},
		{"role prompt", runeLen(speaker.RolePrompt)},
		{"state sheets", runeLen(buildStateSection(chat, participants, promptTextFor(language)))},
	}
	longest := settings[0]
	for _, setting := range settings[1:] {
		if setting.chars > longest.chars {
			longest = setting
		}
	}
	return fmt.Errorf("%w: %s: the system prompt takes %d characters and leaves no room in a context of %d tokens; the longest setting is the %s (%d characters)",
		ErrTurnSettingsTooLong, speaker.DisplayName, runeLen(systemPrompt), contextTokens, longest.name, longest.chars)
}
