package main

import (
	"math"
	"testing"
)

// These are the regression cases for the measurement bugs that moved the
// conclusion of TASK-28 three times. The tool had no tests at all then, and the
// weighted rule of TASK-34 is verified against its figures, so each bug that was
// actually hit is pinned here.

func adoptedRule() func(roster, []utterance) (int, []float64) {
	for _, rl := range rules() {
		if rl.name == "採用" {
			return rl.pick
		}
	}
	panic("the adopted rule is missing from rules()")
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestAdoptedRuleMatchesHandCalculation(t *testing.T) {
	r := gmRoster("GM", "A", "B", "C")
	const gm, a, b, c = 0, 1, 2, 3
	history := []utterance{{speaker: gm}, {speaker: a, addressees: []int{c}}, {speaker: gm}}

	speaker, weights := adoptedRule()(r, history)

	// GM spoke last (×0.2) and is exempt from the recent-speaker factor. A spoke
	// inside the window (×0.85). B has never spoken (1.0). C has never spoken and
	// holds A's unanswered call (×1.2).
	want := []float64{0.2, 0.85, 1.0, 1.2}
	for i := range want {
		if !approxEqual(weights[i], want[i]) {
			t.Fatalf("weights = %v, want %v", weights, want)
		}
	}
	if speaker != c {
		t.Fatalf("speaker = %d, want C (%d)", speaker, c)
	}
}

func TestTieBreakRanksNeverSpokenAsLongestSilent(t *testing.T) {
	r := plainRoster("A", "B", "C", "D", "E")
	const a, b, c, d, e = 0, 1, 2, 3, 4
	// A spoke six participant utterances ago, outside the window of 5, so A and B
	// both sit at 1.0. B has never spoken and must win: reading
	// turnsSinceSpoken's -1 as the shortest silence picks A, and so does a
	// roster-order tie-break, so B is only reached by the rule as designed.
	history := []utterance{{speaker: a}, {speaker: c}, {speaker: d}, {speaker: e}, {speaker: c}, {speaker: d}, {speaker: e}}

	speaker, weights := adoptedRule()(r, history)

	if !approxEqual(weights[a], 1.0) || !approxEqual(weights[b], 1.0) {
		t.Fatalf("weights = %v, want A and B tied at 1.0", weights)
	}
	if speaker != b {
		t.Fatalf("speaker = %d, want B (%d), the participant that has never spoken", speaker, b)
	}
}

func TestMetricsSkipCallsTheRunCutShort(t *testing.T) {
	const a, b, c = 0, 1, 2
	// The first call has a full window after it and was answered on the next
	// turn. The last one was made on the final turn: it is not unanswered, the run
	// just stopped before its window could elapse.
	history := []utterance{
		{speaker: a, addressees: []int{b}},
		{speaker: b},
		{speaker: c},
		{speaker: a},
		{speaker: b, addressees: []int{a, c}},
	}

	if got := callAnswerWait(history, 1, 3); got != "1 (1件)" {
		t.Fatalf("callAnswerWait = %q, want the last call left out", got)
	}
	if got := secondCallWait(history, 3); got != "-" {
		t.Fatalf("secondCallWait = %q, want the cut-short two-person call left out", got)
	}
	if got := followRate(history); got != "1/1" {
		t.Fatalf("followRate = %q, want the call in the last utterance left out", got)
	}
}

func TestCallScheduleDoesNotDependOnTheRule(t *testing.T) {
	for _, sc := range scenarios() {
		if sc.addressRate <= 0 {
			continue
		}
		want := -1
		for _, rl := range rules() {
			history, _ := simulate(sc, rl)
			calls := 0
			for _, u := range history {
				if u.speaker != speakerHuman && len(u.addressees) > 0 {
					calls++
				}
			}
			if want < 0 {
				want = calls
				continue
			}
			if calls != want {
				t.Fatalf("%s: rule %q made %d calls, the first rule made %d", sc.name, rl.name, calls, want)
			}
		}
	}
}

func TestOutstandingCallWindow(t *testing.T) {
	const x, y, z, w, p = 0, 1, 2, 3, 4
	human := speakerHuman
	cases := []struct {
		name    string
		history []utterance
		window  int
		expires bool
		want    bool
	}{
		{"unanswered inside the window", []utterance{{speaker: x, addressees: []int{p}}, {speaker: y}}, 3, true, true},
		{"answered", []utterance{{speaker: x, addressees: []int{p}}, {speaker: p}, {speaker: y}}, 3, true, false},
		{"answered but kept when it does not expire", []utterance{{speaker: x, addressees: []int{p}}, {speaker: p}, {speaker: y}}, 3, false, true},
		{"left the window", []utterance{{speaker: x, addressees: []int{p}}, {speaker: y}, {speaker: z}, {speaker: w}}, 3, true, false},
		{"the calling utterance is the window's last", []utterance{{speaker: x, addressees: []int{p}}, {speaker: y}, {speaker: z}}, 3, true, true},
		{"interventions do not consume the window", []utterance{{speaker: x, addressees: []int{p}}, {speaker: human}, {speaker: human}, {speaker: y}, {speaker: z}}, 3, true, true},
		{"an intervention's own call is read", []utterance{{speaker: y}, {speaker: human, addressees: []int{p}}}, 3, true, true},
	}
	for _, tc := range cases {
		if got := outstandingCall(tc.history, p, tc.window, tc.expires); got != tc.want {
			t.Errorf("%s: outstandingCall = %v, want %v", tc.name, got, tc.want)
		}
	}
}
