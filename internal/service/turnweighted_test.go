package service

import (
	"math"
	"reflect"
	"testing"

	"snzstudio/internal/model"
)

func weightedRoster(names ...string) []model.Participant {
	roster := make([]model.Participant, len(names))
	for i, name := range names {
		roster[i] = model.Participant{ID: name, DisplayName: name, SortOrder: i}
	}
	return roster
}

func said(speaker string, addressees ...string) model.Message {
	return model.Message{Role: "assistant", ParticipantID: &speaker, AddressedParticipantIDs: append([]string{}, addressees...)}
}

func humanSaid(addressees ...string) model.Message {
	return model.Message{Role: "user", AddressedParticipantIDs: append([]string{}, addressees...)}
}

func weightOf(t *testing.T, weights []SpeakerWeight, id string) SpeakerWeight {
	t.Helper()
	for _, w := range weights {
		if w.ParticipantID == id {
			return w
		}
	}
	t.Fatalf("no weight for %s in %+v", id, weights)
	return SpeakerWeight{}
}

func factorNames(w SpeakerWeight) []string {
	names := []string{}
	for _, f := range w.Factors {
		names = append(names, f.Name)
	}
	return names
}

// TestWeightedMatchesHandCalculation covers TASK-34 AC #7: the weight is the
// product of the three factors of §4.6.1 and the heaviest speaks. It is the same
// history as the simulator's hand-calculation test, so the engine and the tool
// that measured the rule are held to one answer.
func TestWeightedMatchesHandCalculation(t *testing.T) {
	roster := weightedRoster("GM", "A", "B", "C")
	messages := []model.Message{said("GM"), said("A", "C"), said("GM")}

	speaker, weights := weightedSpeaker(roster, 0, messages)

	want := map[string]struct {
		weight  float64
		factors []string
	}{
		// GM wrote the last message and is exempt from the recent factor.
		"GM": {0.2, []string{consecutiveFactor}},
		"A":  {0.85, []string{recentFactor}},
		"B":  {1.0, []string{}},
		// C holds A's call, unanswered and inside the window.
		"C": {1.2, []string{callFactor}},
	}
	for id, w := range want {
		got := weightOf(t, weights, id)
		if math.Abs(got.Weight-w.weight) > 1e-9 || !reflect.DeepEqual(factorNames(got), w.factors) {
			t.Errorf("%s = %v %v, want %v %v", id, got.Weight, factorNames(got), w.weight, w.factors)
		}
		if got.DisplayName != id {
			t.Errorf("%s display name = %q", id, got.DisplayName)
		}
	}
	if speaker.ID != "C" {
		t.Fatalf("speaker = %s, want C", speaker.ID)
	}
	if len(weights) != len(roster) {
		t.Fatalf("weights = %d entries, want one per roster participant", len(weights))
	}
}

// TestWeightedHasNoRosterOrderFactor covers the last sentence of AC #7: nothing
// favours the participant after the last speaker in sort_order. With no call and
// no facilitator, the two never-spoken participants tie, and the one earlier in
// the roster wins by the tie-break alone rather than by a factor.
func TestWeightedHasNoRosterOrderFactor(t *testing.T) {
	roster := weightedRoster("A", "B", "C", "D")
	speaker, weights := weightedSpeaker(roster, -1, []model.Message{said("B")})

	for _, id := range []string{"A", "C", "D"} {
		if w := weightOf(t, weights, id); w.Weight != 1 || len(w.Factors) != 0 {
			t.Errorf("%s = %v %v, want 1 with no factor", id, w.Weight, factorNames(w))
		}
	}
	if speaker.ID != "A" {
		t.Fatalf("speaker = %s, want A: C follows B in sort_order but has no factor for it", speaker.ID)
	}
}

// TestWeightedTieBreak covers AC #2: a tie goes to the longest silent, and a
// participant that has never spoken counts as the longest silent of all. A spoke
// six utterances ago, past the window of five, so A and B both weigh 1.0: reading
// "never spoken" as the shortest silence picks A, and so does a roster-order
// tie-break, so only the rule as designed reaches B.
func TestWeightedTieBreak(t *testing.T) {
	roster := weightedRoster("A", "B", "C", "D", "E")
	messages := []model.Message{said("A"), said("C"), said("D"), said("E"), said("C"), said("D"), said("E")}

	speaker, weights := weightedSpeaker(roster, -1, messages)

	if weightOf(t, weights, "A").Weight != 1 || weightOf(t, weights, "B").Weight != 1 {
		t.Fatalf("weights = %+v, want A and B tied at 1", weights)
	}
	if speaker.ID != "B" {
		t.Fatalf("speaker = %s, want B, which has never spoken", speaker.ID)
	}

	// Equal silence falls back to sort_order: neither C nor D has spoken.
	speaker, _ = weightedSpeaker(weightedRoster("A", "B", "C", "D"), -1, []model.Message{said("A"), said("B")})
	if speaker.ID != "C" {
		t.Fatalf("speaker = %s, want C, first in sort_order of the equally silent", speaker.ID)
	}
}

// TestWeightedHumanLastPenalizesNoOne covers the first half of AC #5: when the
// human's intervention is the last message, the consecutive factor falls on no
// participant (§4.6.7).
func TestWeightedHumanLastPenalizesNoOne(t *testing.T) {
	roster := weightedRoster("GM", "A", "B", "C")
	_, weights := weightedSpeaker(roster, 0, []model.Message{said("GM"), said("A"), humanSaid()})
	for _, w := range weights {
		for _, f := range w.Factors {
			if f.Name == consecutiveFactor {
				t.Fatalf("%s carries the consecutive factor after an intervention: %+v", w.ParticipantID, weights)
			}
		}
	}
}

// playWeighted runs the rule for n turns from the given messages, each speaker
// calling on no one, and returns who spoke in order.
func playWeighted(roster []model.Participant, facilitator int, messages []model.Message, n int) []string {
	spoken := []string{}
	for i := 0; i < n; i++ {
		speaker, _ := weightedSpeaker(roster, facilitator, messages)
		spoken = append(spoken, speaker.ID)
		messages = append(messages, said(speaker.ID))
	}
	return spoken
}

func contains(ids []string, id string) bool {
	for _, got := range ids {
		if got == id {
			return true
		}
	}
	return false
}

// TestWeightedHumanCallsTheLastSpeaker covers the second half of AC #5: the human
// asking the participant that just spoke to go on reaches it within the call
// window (n = roster size), not necessarily on the very next turn.
func TestWeightedHumanCallsTheLastSpeaker(t *testing.T) {
	roster := weightedRoster("GM", "A", "B", "C")
	for _, last := range []string{"A", "B", "C", "GM"} {
		history := []model.Message{said("GM"), said("A"), said("GM"), said("B"), said("GM"), said("C"), said("GM"), said(last), humanSaid(last)}
		if spoken := playWeighted(roster, 0, history, len(roster)); !contains(spoken, last) {
			t.Errorf("human called %s after it spoke; the next %d turns went to %v", last, len(roster), spoken)
		}
	}
}

// TestWeightedTwoAddresseesBothSpeak covers the multi-call case of the task's
// verification: an utterance calling on two participants gets both of them to
// speak within the window, the call held by the second after the first answers.
func TestWeightedTwoAddresseesBothSpeak(t *testing.T) {
	roster := weightedRoster("GM", "A", "B", "C")
	history := []model.Message{said("GM"), said("A"), said("GM"), said("B"), said("GM"), said("C"), said("GM", "B", "C")}
	spoken := playWeighted(roster, 0, history, len(roster))
	if !contains(spoken, "B") || !contains(spoken, "C") {
		t.Fatalf("GM called on B and C; the next %d turns went to %v", len(roster), spoken)
	}
}

// TestWeightedCallExpires covers the window of AC #7: the boost holds while the
// call is unanswered inside the last n participant utterances, and drops once
// the participant answers or the window passes. The human's interventions do not
// consume the window, but a call one made is read.
func TestWeightedCallExpires(t *testing.T) {
	cases := []struct {
		name     string
		messages []model.Message
		want     bool
	}{
		{"unanswered", []model.Message{said("X", "P"), said("Y")}, true},
		{"answered", []model.Message{said("X", "P"), said("P"), said("Y")}, false},
		{"out of the window", []model.Message{said("X", "P"), said("Y"), said("Z"), said("W")}, false},
		{"the window's oldest utterance", []model.Message{said("X", "P"), said("Y"), said("Z")}, true},
		{"interventions do not consume the window", []model.Message{said("X", "P"), humanSaid(), humanSaid(), said("Y"), said("Z")}, true},
		{"an intervention's call is read", []model.Message{said("Y"), humanSaid("P")}, true},
		{"a newer call after the answer", []model.Message{said("X", "P"), said("P"), said("Y", "P")}, true},
	}
	for _, tc := range cases {
		if got := outstandingCall(tc.messages, "P", 3); got != tc.want {
			t.Errorf("%s: outstandingCall = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestWeightedWithoutFacilitatorExemptsNoOne: an unset facilitator (-1) leaves
// everyone subject to the recent factor.
func TestWeightedWithoutFacilitatorExemptsNoOne(t *testing.T) {
	roster := weightedRoster("GM", "A", "B")
	_, weights := weightedSpeaker(roster, -1, []model.Message{said("GM"), said("A")})
	if got := factorNames(weightOf(t, weights, "GM")); !reflect.DeepEqual(got, []string{recentFactor}) {
		t.Fatalf("GM factors = %v, want the recent factor with no facilitator set", got)
	}
}
