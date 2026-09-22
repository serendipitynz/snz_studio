package service

import (
	"snzstudio/internal/model"
)

// The factors of the weighted rule (design §4.6.1). The names are what the
// speaker event carries; the frontend translates them for display.
const (
	// consecutiveFactor goes to whoever wrote the last message, so the same
	// participant does not speak twice in a row. A human intervention as the last
	// message puts it on no one (§4.6.7): the human took that turn, and keeping it
	// on the previous participant would stop the human from asking that
	// participant to go on — 0.2 × 0.85 × 1.2 loses to any 0.85.
	consecutiveFactor      = "consecutive"
	consecutiveFactorValue = 0.2
	// recentFactor goes to every participant that spoke within the last n
	// participant utterances (n = roster size), the facilitator excepted. In
	// steady state it lands on almost everyone, which leaves a participant silent
	// past the window, and the facilitator, at 1.0.
	recentFactor      = "recent"
	recentFactorValue = 0.85
	// callFactor goes to a participant with an unanswered call inside the window.
	// It has to beat 1.0 from 0.85, so anything above 1/0.85 ≈ 1.176 behaves the
	// same; below it the call never wins (measured in §4.6.3 reading 2). Re-measure
	// with tools/turn-score-sim before moving it.
	callFactor      = "call"
	callFactorValue = 1.2
)

// weightedSpeaker implements the weighted rule: each roster participant's weight
// is the product of the factors that apply to it, and the heaviest speaks. The
// tie-break — longest silence first, a participant that has never spoken counting
// as the longest of all, then sort_order — decides more turns than the weights
// do: the factors take a handful of discrete values, so the top is shared on
// most turns (§4.6.3 reading 4).
//
// facilitator is the roster index of the participant exempt from the
// recent-speaker factor, or -1 for none. Like the other derived rules it reads
// the stored messages alone (§2).
func weightedSpeaker(roster []model.Participant, facilitator int, messages []model.Message) (*model.Participant, []SpeakerWeight) {
	window := len(roster)
	penalized := lastSpeaker(messages)
	weights := make([]SpeakerWeight, len(roster))
	best := -1
	bestSilence := -1
	for i := range roster {
		id := roster[i].ID
		entry := SpeakerWeight{ParticipantID: id, DisplayName: roster[i].DisplayName, Weight: 1, Factors: []WeightFactor{}}
		if id == penalized {
			entry.apply(consecutiveFactor, consecutiveFactorValue)
		}
		if i != facilitator && spokeWithin(messages, id, window) {
			entry.apply(recentFactor, recentFactorValue)
		}
		if outstandingCall(messages, id, window) {
			entry.apply(callFactor, callFactorValue)
		}
		weights[i] = entry

		silence := silenceRank(messages, id)
		if best < 0 || entry.Weight > weights[best].Weight || (entry.Weight == weights[best].Weight && silence > bestSilence) {
			best, bestSilence = i, silence
		}
	}
	return &roster[best], weights
}

func (w *SpeakerWeight) apply(name string, value float64) {
	w.Weight *= value
	w.Factors = append(w.Factors, WeightFactor{Name: name, Value: value})
}

// isParticipantUtterance reports whether a message is a turn some participant
// took. The human's interventions are not, and neither is an assistant message
// left over from before the chat became multi-agent: both carry no
// participant_id, and neither consumes the window, so the window's width does
// not depend on how often the human speaks.
func isParticipantUtterance(m model.Message) bool {
	return m.ParticipantID != nil && *m.ParticipantID != ""
}

// turnsSinceSpoken is how many participant utterances have been stored since the
// participant last spoke, or -1 when it has never spoken.
func turnsSinceSpoken(messages []model.Message, participantID string) int {
	seen := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if !isParticipantUtterance(messages[i]) {
			continue
		}
		if *messages[i].ParticipantID == participantID {
			return seen
		}
		seen++
	}
	return -1
}

func spokeWithin(messages []model.Message, participantID string, window int) bool {
	since := turnsSinceSpoken(messages, participantID)
	return since >= 0 && since < window
}

// silenceRank orders participants by how long they have been silent, one that
// has never spoken ahead of every one that has. turnsSinceSpoken's -1 cannot be
// used as is: it would rank the participant that has waited longest of all as the
// one that spoke last (the bug that moved TASK-28's figures).
func silenceRank(messages []model.Message, participantID string) int {
	if since := turnsSinceSpoken(messages, participantID); since >= 0 {
		return since
	}
	return len(messages) + 1
}

// outstandingCall answers whether a message inside the window called on the
// participant and the participant has not spoken since (§4.6.1). The window is
// the last `window` participant utterances; the human's interventions do not
// consume it, but a call an intervention made is read — whether a message takes
// a turn and whether its body calls on someone are separate questions.
//
// Only the most recent call matters: an older one was answered by the same
// utterance that answered the newer one, or earlier.
func outstandingCall(messages []model.Message, participantID string, window int) bool {
	seen := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if calls(messages[i], participantID) {
			for _, later := range messages[i+1:] {
				if isParticipantUtterance(later) && *later.ParticipantID == participantID {
					return false
				}
			}
			return true
		}
		if isParticipantUtterance(messages[i]) {
			seen++
			if seen >= window {
				return false
			}
		}
	}
	return false
}

func calls(m model.Message, participantID string) bool {
	for _, id := range m.AddressedParticipantIDs {
		if id == participantID {
			return true
		}
	}
	return false
}
