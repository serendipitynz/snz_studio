// Simulates the candidate turn rules of TASK-28 over a synthetic transcript, so
// that docs/multi-agent-chat-design.md §4.6 can quote measurements rather than
// intuitions. Run from repo root:  go run ./tools/turn-score-sim
//
// It is not part of the app: nothing under internal/ imports it, and it holds its
// own miniature of the roster and the transcript. The engine derives the speaker
// from stored messages alone (design §2), which is exactly what makes the rule
// simulable without a database or a model — the same derivation runs here over a
// slice of utterances.
//
// Determinism: the only randomness is which participant a speaker calls on, drawn
// from a fixed seed, so a re-run reproduces the numbers in the design doc.
package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"text/tabwriter"
)

// speakerHuman marks the human's own intervention, which carries no
// participant_id in the real transcript (design §3).
const speakerHuman = -1

type participant struct {
	name        string
	facilitator bool
}

// utterance is one stored message: who spoke, and whom that message called on.
// The addressees are fixed when the message is stored and never re-derived, which
// is what keeps the next speaker the same across a restart (design §2).
//
// There can be more than one — "A, B, what do you think?" calls on two. Every
// participant called on escapes the notAddressee coefficient, and the remaining
// coefficients decide which of them speaks first. An ordered rule can read the
// set too (addressedLongestSilent does), so this is not something only the score
// can express; and neither form carries a call that went unanswered past the next
// utterance, since the addressees are read from the last message alone.
type utterance struct {
	speaker    int
	addressees []int
}

func (u utterance) calls(participant int) bool {
	for _, a := range u.addressees {
		if a == participant {
			return true
		}
	}
	return false
}

type roster []participant

func (r roster) facilitator() int {
	for i := range r {
		if r[i].facilitator {
			return i
		}
	}
	return -1
}

// humanPolicy is the open question 5 of TASK-28: who speaks after an
// intervention that calls on no one.
type humanPolicy int

const (
	// penalizeLastParticipant keeps the ×0.2 on whoever spoke last among the
	// participants, whether or not the human has spoken since.
	penalizeLastParticipant humanPolicy = iota
	// humanIsCurrentSpeaker treats the intervention itself as the current turn's
	// utterance, so no participant carries the ×0.2.
	humanIsCurrentSpeaker
	// facilitatorAnswersHuman forces the facilitator after an intervention, which
	// is what facilitator_alternating already does (design §4.5 step 2).
	facilitatorAnswersHuman
	// facilitatorAnswersPlainHuman forces the facilitator only when the
	// intervention called on no one, so §4.5's choice is kept for the case it was
	// made for while an intervention that names someone still reaches them.
	facilitatorAnswersPlainHuman
)

// tieBreak decides between participants that share the highest weight.
type tieBreak int

const (
	// byRosterOrder takes the earliest sort_order, the tie-break every existing
	// rule already implies.
	byRosterOrder tieBreak = iota
	// byLongestSilence takes the participant that has gone longest without
	// speaking, falling back to roster order when even that ties.
	byLongestSilence
)

type coefficients struct {
	lastSpeaker   float64 // the participant that spoke the current last utterance
	recentSpeaker float64 // a participant that spoke within the silence window
	notAddressee  float64 // everyone but the participant the last utterance called on
	notRosterNext float64 // everyone but the participant after the last one in sort_order

	// addresseeBoost is the other way to express the call: raise the participant
	// that was called on instead of lowering everyone else. The two are the same
	// ranking (notAddressee=x ranks exactly as addresseeBoost=1/x), so a variant
	// sets one and leaves the other at 1; having both lets the boost be tuned down
	// to where the call competes with the other coefficients instead of settling
	// the turn on its own.
	addresseeBoost float64

	// silenceGain replaces the fixed recentSpeaker coefficient with a term that
	// grows with how long a participant has been silent (×(1+gain×turns)). It is
	// the only continuous term any variant here has: with the fixed coefficients
	// the weights take a handful of discrete values, so a call can only win always
	// or lose always. A term that grows lets a long-silent participant outweigh a
	// call by degrees, which is what "the call influences the score" would mean.
	silenceGain float64

	// facilitatorExemptRecent leaves the facilitator out of recentSpeaker, which
	// is what lets it come back every other turn.
	facilitatorExemptRecent bool
}

type rule struct {
	name string
	// pick answers with the speaker's roster index and, when the rule computes
	// them, the weights behind that choice.
	pick func(r roster, history []utterance) (int, []float64)
}

type scenario struct {
	name        string
	roster      roster
	turns       int
	addressRate float64 // probability that a generated utterance calls on someone
	humanEvery  int     // insert an intervention every N turns; 0 disables it
	// humanCallsLast makes each intervention call on the participant that just
	// spoke, which is the one case where the addressee also carries the ×0.2.
	humanCallsLast bool
	// multiCallRate is the share of calls that name two participants rather than
	// one ("A, B, what do you think?").
	multiCallRate float64
	seed          int64
}

func main() {
	scenarios := []scenario{
		{name: "3人・進行役なし", roster: plainRoster("A", "B", "C"), turns: 60, seed: 1},
		{name: "4人・進行役なし", roster: plainRoster("A", "B", "C", "D"), turns: 60, seed: 2},
		{name: "4人・進行役あり", roster: gmRoster("GM", "A", "B", "C"), turns: 60, seed: 3},
		{name: "4人・進行役あり・呼びかけ3割", roster: gmRoster("GM", "A", "B", "C"), turns: 60, addressRate: 0.3, seed: 4},
		{name: "4人・進行役あり・呼びかけ3割+介入", roster: gmRoster("GM", "A", "B", "C"), turns: 60, addressRate: 0.3, humanEvery: 7, seed: 5},
		{name: "3人・進行役なし・呼びかけ3割+介入", roster: plainRoster("A", "B", "C"), turns: 60, addressRate: 0.3, humanEvery: 7, seed: 6},
		{name: "4人・進行役あり・介入が直前の話者を呼ぶ", roster: gmRoster("GM", "A", "B", "C"), turns: 60, humanEvery: 5, humanCallsLast: true, seed: 7},
		{name: "4人・進行役あり・呼びかけ4割 (半分は2人呼び)", roster: gmRoster("GM", "A", "B", "C"), turns: 60, addressRate: 0.4, multiCallRate: 0.5, seed: 8},
	}

	for _, sc := range scenarios {
		fmt.Printf("\n=== %s (%d ターン, 呼びかけ率 %.0f%%, うち 2 人呼び %.0f%%, 介入 %s) ===\n",
			sc.name, sc.turns, sc.addressRate*100, sc.multiCallRate*100, interventionLabel(sc.humanEvery))
		report(sc, rules())
	}
}

func rules() []rule {
	// The ticket's coefficients. notAddressee 0.5 is its "the call influences the
	// weight" rule, written as a suppression of everyone else.
	proposed := coefficients{lastSpeaker: 0.2, recentSpeaker: 0.85, notAddressee: 0.5, notRosterNext: 0.95, addresseeBoost: 1.0, facilitatorExemptRecent: true}
	// The same rule written as a boost, which is what can be tuned: the ticket's
	// 0.5 is boost 2.0, and the threshold where a call stops being able to lose is
	// 1/(0.85×0.95) ≈ 1.238, so 1.3 sits just above it and 1.2 just below.
	tuned := func(boost float64) coefficients {
		c := proposed
		c.notAddressee, c.addresseeBoost = 1.0, boost
		return c
	}

	return []rule{
		{name: "原案", pick: weighted(proposed, penalizeLastParticipant, byRosterOrder)},
		{name: "原案+沈黙同点", pick: weighted(proposed, penalizeLastParticipant, byLongestSilence)},
		{name: "原案+人間=話者", pick: weighted(proposed, humanIsCurrentSpeaker, byRosterOrder)},
		{name: "原案+介入は進行役", pick: weighted(proposed, facilitatorAnswersHuman, byRosterOrder)},
		{name: "呼びかけ×2.0(原案と等価)", pick: weighted(tuned(2.0), facilitatorAnswersPlainHuman, byLongestSilence)},
		{name: "呼びかけ×1.3", pick: weighted(tuned(1.3), facilitatorAnswersPlainHuman, byLongestSilence)},
		{name: "呼びかけ×1.2", pick: weighted(tuned(1.2), facilitatorAnswersPlainHuman, byLongestSilence)},
		{name: "呼びかけ×1.1", pick: weighted(tuned(1.1), facilitatorAnswersPlainHuman, byLongestSilence)},
		{name: "呼びかけ係数なし", pick: weighted(tuned(1.0), facilitatorAnswersPlainHuman, byLongestSilence)},
		{name: "沈黙を連続量+呼びかけ×1.3", pick: weighted(continuousSilence(tuned(1.3), 0.15), facilitatorAnswersPlainHuman, byLongestSilence)},
		{name: "沈黙を連続量+呼びかけ×2.0", pick: weighted(continuousSilence(tuned(2.0), 0.15), facilitatorAnswersPlainHuman, byLongestSilence)},
		{name: "直近抑制なし", pick: weighted(withRecent(proposed, 1.0), humanIsCurrentSpeaker, byRosterOrder)},
		{name: "進行役の免除なし", pick: weighted(withoutExemption(proposed), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "採用(仕様どおり)", pick: weighted(proposed, humanIsCurrentSpeaker, byLongestSilence)},
		{name: "採用+名指し無し介入は進行役", pick: weighted(proposed, facilitatorAnswersPlainHuman, byLongestSilence)},
		{name: "呼びかけ最優先(先頭)", pick: addressedFirst},
		{name: "呼びかけ最優先(沈黙が長い側)", pick: addressedLongestSilent},
		{name: "round_robin", pick: roundRobin},
		{name: "facilitator交互", pick: facilitatorAlternating},
	}
}

func withRecent(c coefficients, v float64) coefficients { c.recentSpeaker = v; return c }
func withoutExemption(c coefficients) coefficients      { c.facilitatorExemptRecent = false; return c }

func continuousSilence(c coefficients, gain float64) coefficients {
	c.silenceGain = gain
	return c
}

// weighted is the score rule: every participant's weight is the product of the
// coefficients that apply to it, and the highest weight speaks. The product is
// used rather than the draft's "redistribute what was taken away" because the
// product does not depend on the order the rules are applied in.
func weighted(c coefficients, human humanPolicy, tie tieBreak) func(roster, []utterance) (int, []float64) {
	return func(r roster, history []utterance) (int, []float64) {
		if len(r) == 0 {
			return -1, nil
		}
		gm := r.facilitator()
		if gm >= 0 && lastIsHuman(history) {
			plain := len(history[len(history)-1].addressees) == 0
			if human == facilitatorAnswersHuman || (human == facilitatorAnswersPlainHuman && plain) {
				return gm, nil
			}
		}

		lastParticipant := lastParticipantSpeaker(history)
		penalized := lastParticipant
		if human != penalizeLastParticipant && lastIsHuman(history) {
			penalized = -1
		}

		var called utterance
		if n := len(history); n > 0 {
			called = history[n-1]
		}

		next := 0
		if lastParticipant >= 0 {
			next = (lastParticipant + 1) % len(r)
		}

		// The silence window is the roster size (TASK-28): in steady state every
		// participant has spoken inside it, so the coefficient cancels out and only
		// a genuinely silent participant is left un-penalised.
		window := len(r)
		weights := make([]float64, len(r))
		for i := range r {
			w := 1.0
			if i == penalized {
				w *= c.lastSpeaker
			}
			if c.silenceGain > 0 {
				// Capped at the roster size so a participant that has never spoken does
				// not outweigh everything else for the rest of the conversation.
				silent := silenceRank(history, i)
				if silent > len(r) {
					silent = len(r)
				}
				w *= 1 + c.silenceGain*float64(silent)
			} else if spokeWithin(history, i, window) && !(c.facilitatorExemptRecent && r[i].facilitator) {
				w *= c.recentSpeaker
			}
			if len(called.addressees) > 0 {
				if called.calls(i) {
					w *= c.addresseeBoost
				} else {
					w *= c.notAddressee
				}
			}
			if i != next {
				w *= c.notRosterNext
			}
			weights[i] = w
		}
		return argmax(weights, history, tie), weights
	}
}

func argmax(weights []float64, history []utterance, tie tieBreak) int {
	best := 0
	for i := 1; i < len(weights); i++ {
		if weights[i] > weights[best] {
			best = i
		}
	}
	if tie != byLongestSilence {
		return best
	}
	silence := -1
	for i := range weights {
		if weights[i] != weights[best] {
			continue
		}
		if s := silenceRank(history, i); s > silence {
			silence, best = s, i
		}
	}
	return best
}

// silenceRank orders participants by how long they have been silent, with one
// that has never spoken ranked ahead of every participant that has. Using
// turnsSinceSpoken directly would rank it last instead: its -1 loses to any real
// count, so on a fresh roster the tie-break would pass over the participant that
// has been waiting longest of all. The scan runs from the back and stops at the
// first match, so ties resolve to the earliest sort_order among equals.
func silenceRank(history []utterance, participant int) int {
	if since := turnsSinceSpoken(history, participant); since >= 0 {
		return since
	}
	return len(history) + 1
}

// addressedFirst is the alternative TASK-28 leaves open: the same intentions as
// ordered rules rather than as a product, with the call as the top rule instead
// of a coefficient. Underneath it is the rules the engine already ships, not a
// weaker rotation invented for the contrast.
//
// Taking addressees[0] is this variant's own choice and not a limit of ordered
// rules — addressedLongestSilent picks out of the whole set without coefficients,
// which is what the comparison has to be against.
func addressedFirst(r roster, history []utterance) (int, []float64) {
	if len(r) == 0 {
		return -1, nil
	}
	// No guard against an addressee having just spoken: a participant calling on
	// itself is discarded when the utterance is stored, so an addressee that spoke
	// last can only come from the human asking that speaker to go on.
	//
	// Taking the first of several addressees is this variant's choice, not a limit
	// of ordered rules — addressedLongestSilent orders the same set by silence. The
	// two are measured separately because that choice, and not the ordered form,
	// is what makes the rest wait out the rotation.
	if n := len(history); n > 0 && len(history[n-1].addressees) > 0 {
		return history[n-1].addressees[0], nil
	}
	return addressedFallback(r, history)
}

// addressedLongestSilent is the ordered rule with the set read properly: among the
// participants the last utterance called on, the one that has waited longest
// speaks. It needs no coefficients to do that, so it is the baseline the score has
// to beat on a call that names several.
func addressedLongestSilent(r roster, history []utterance) (int, []float64) {
	if len(r) == 0 {
		return -1, nil
	}
	if n := len(history); n > 0 && len(history[n-1].addressees) > 0 {
		best, rank := -1, -1
		for _, called := range history[n-1].addressees {
			if s := silenceRank(history, called); s > rank {
				rank, best = s, called
			}
		}
		return best, nil
	}
	return addressedFallback(r, history)
}

func addressedFallback(r roster, history []utterance) (int, []float64) {
	if r.facilitator() >= 0 {
		return facilitatorAlternating(r, history)
	}
	return roundRobin(r, history)
}

func roundRobin(r roster, history []utterance) (int, []float64) {
	last := lastParticipantSpeaker(history)
	if last < 0 {
		return 0, nil
	}
	return (last + 1) % len(r), nil
}

func facilitatorAlternating(r roster, history []utterance) (int, []float64) {
	gm := r.facilitator()
	if gm < 0 {
		return roundRobin(r, history)
	}
	if n := len(history); n == 0 || history[n-1].speaker != gm {
		return gm, nil
	}
	previous := lastParticipantSpeakerExcept(history, gm)
	for i := 1; i <= len(r); i++ {
		candidate := (previous + i) % len(r)
		if candidate != gm {
			return candidate, nil
		}
	}
	return gm, nil
}

func lastIsHuman(history []utterance) bool {
	n := len(history)
	return n > 0 && history[n-1].speaker == speakerHuman
}

func lastParticipantSpeaker(history []utterance) int {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].speaker != speakerHuman {
			return history[i].speaker
		}
	}
	return -1
}

func lastParticipantSpeakerExcept(history []utterance, excluded int) int {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].speaker != speakerHuman && history[i].speaker != excluded {
			return history[i].speaker
		}
	}
	return -1
}

// spokeWithin counts the window in participant utterances, skipping the human's
// interventions: an intervention is not a turn anyone took, so letting it push
// participants out of the window would make the coefficient depend on how often
// the human speaks.
func spokeWithin(history []utterance, participant, window int) bool {
	since := turnsSinceSpoken(history, participant)
	return since >= 0 && since < window
}

// turnsSinceSpoken is how many participant utterances have been stored since this
// participant last spoke, or -1 when it has never spoken.
func turnsSinceSpoken(history []utterance, participant int) int {
	seen := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].speaker == speakerHuman {
			continue
		}
		if history[i].speaker == participant {
			return seen
		}
		seen++
	}
	return -1
}

type measurement struct {
	repeatRate    float64 // 連続発言率
	passTurns     int     // 一巡ターン数 (最大)
	gmGapMin      int     // 進行役の間隔 (最小)
	gmGapMax      int     // 進行役の間隔 (最大)
	addressFollow string  // 呼びかけ追従率
	secondWait    string  // 複数呼びかけで後回しになった側が話すまでのターン数 (最悪)
	ties          int     // 同点が起きたターン数
	order         string
}

func run(sc scenario, rl rule) measurement {
	rng := rand.New(rand.NewSource(sc.seed))
	history := make([]utterance, 0, sc.turns)
	ties := 0

	for turn := 1; turn <= sc.turns; turn++ {
		if sc.humanEvery > 0 && turn%sc.humanEvery == 0 {
			var called []int
			if last := lastParticipantSpeaker(history); sc.humanCallsLast && last >= 0 {
				called = []int{last}
			}
			history = append(history, utterance{speaker: speakerHuman, addressees: called})
		}
		speaker, weights := rl.pick(sc.roster, history)
		if speaker < 0 {
			break
		}
		if tiedAtTop(weights) {
			ties++
		}
		history = append(history, utterance{speaker: speaker, addressees: drawAddressees(rng, sc, speaker)})
	}
	return measure(sc.roster, history, ties)
}

// drawAddressees picks whom the utterance calls on. A speaker never calls on
// itself, which is what the detection of §4.6 would discard anyway. With
// multiCallRate set, some calls name two participants instead of one.
func drawAddressees(rng *rand.Rand, sc scenario, speaker int) []int {
	if sc.addressRate <= 0 || rng.Float64() >= sc.addressRate {
		return nil
	}
	first := drawOther(rng, len(sc.roster), speaker, -1)
	if sc.multiCallRate <= 0 || len(sc.roster) < 3 || rng.Float64() >= sc.multiCallRate {
		return []int{first}
	}
	return []int{first, drawOther(rng, len(sc.roster), speaker, first)}
}

func drawOther(rng *rand.Rand, size, speaker, taken int) int {
	for {
		if candidate := rng.Intn(size); candidate != speaker && candidate != taken {
			return candidate
		}
	}
}

func tiedAtTop(weights []float64) bool {
	if len(weights) == 0 {
		return false
	}
	top, count := weights[0], 0
	for _, w := range weights {
		if w > top {
			top, count = w, 0
		}
		if w == top {
			count++
		}
	}
	return count > 1
}

func measure(r roster, history []utterance, ties int) measurement {
	spoken := make([]int, 0, len(history))
	order := make([]string, 0, len(history))
	for _, u := range history {
		if u.speaker == speakerHuman {
			order = append(order, "人")
			continue
		}
		spoken = append(spoken, u.speaker)
		order = append(order, r[u.speaker].name)
	}

	// Counted over adjacent stored messages rather than over participant
	// utterances alone: what a watcher sees as the same person speaking twice is
	// two consecutive messages, and an intervention in between breaks that.
	repeats, pairs := 0, 0
	for i := 1; i < len(history); i++ {
		if history[i].speaker == speakerHuman || history[i-1].speaker == speakerHuman {
			continue
		}
		pairs++
		if history[i].speaker == history[i-1].speaker {
			repeats++
		}
	}
	repeatRate := 0.0
	if pairs > 0 {
		repeatRate = float64(repeats) / float64(pairs) * 100
	}

	m := measurement{
		repeatRate:    repeatRate,
		passTurns:     longestPass(spoken, len(r)),
		addressFollow: followRate(history),
		secondWait:    secondCallWait(history),
		ties:          ties,
		order:         strings.Join(order[:min(len(order), 24)], " "),
	}
	m.gmGapMin, m.gmGapMax = facilitatorGaps(r, spoken)
	return m
}

// longestPass is the worst case of "how many turns until everyone has spoken",
// measured from every starting turn rather than only from the first. Windows that
// run off the end of the transcript are not counted, since their tail is missing
// rather than starved; -1 means no window completed at all, which is starvation.
func longestPass(spoken []int, size int) int {
	worst := -1
	for start := range spoken {
		seen := map[int]bool{}
		for i := start; i < len(spoken); i++ {
			seen[spoken[i]] = true
			if len(seen) == size {
				if n := i - start + 1; n > worst {
					worst = n
				}
				break
			}
		}
	}
	return worst
}

func facilitatorGaps(r roster, spoken []int) (int, int) {
	gm := r.facilitator()
	if gm < 0 {
		return -1, -1
	}
	low, high, gap, started := -1, -1, 0, false
	for _, s := range spoken {
		if s != gm {
			gap++
			continue
		}
		if started {
			if low < 0 || gap < low {
				low = gap
			}
			if gap > high {
				high = gap
			}
		}
		started, gap = true, 0
	}
	return low, high
}

// followRate counts only the calls whose answer is actually in the transcript. A
// call in the last utterance has not been answered yet rather than answered
// wrongly, and counting it as a miss would charge every rule for where the run
// happened to stop. A call on several participants is followed when any one of
// them answers: the call asks the group, and which of them goes first is what the
// remaining coefficients are for.
func followRate(history []utterance) string {
	observed, followed := 0, 0
	for i, u := range history {
		if len(u.addressees) == 0 {
			continue
		}
		for j := i + 1; j < len(history); j++ {
			if history[j].speaker == speakerHuman {
				continue
			}
			observed++
			if u.calls(history[j].speaker) {
				followed++
			}
			break
		}
	}
	if observed == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d", followed, observed)
}

// secondCallWait answers what happens to the other participants a call named: for
// every call on two or more, how many turns pass before each of the ones that did
// not go first has spoken. It is reported as the worst case, since the question is
// whether anyone is left hanging, not what the average is.
func secondCallWait(history []utterance) string {
	worst, calls, unanswered := 0, 0, 0
	for i, u := range history {
		if len(u.addressees) < 2 {
			continue
		}
		for _, called := range u.addressees {
			calls++
			wait, answered := 0, false
			for j := i + 1; j < len(history); j++ {
				if history[j].speaker == speakerHuman {
					continue
				}
				wait++
				if history[j].speaker == called {
					answered = true
					break
				}
			}
			// An unanswered call is counted rather than skipped. Taking the maximum
			// over the answered ones alone reported a short wait for a transcript in
			// which someone was never answered at all — the starvation the number
			// exists to expose.
			if !answered {
				unanswered++
				continue
			}
			if wait > worst {
				worst = wait
			}
		}
	}
	if calls == 0 {
		return "-"
	}
	if unanswered > 0 {
		return fmt.Sprintf("%d (未応答 %d/%d)", worst, unanswered, calls)
	}
	return fmt.Sprintf("%d", worst)
}

func report(sc scenario, rules []rule) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "規則\t連続発言率\t一巡ターン数\t進行役の間隔\t呼びかけ追従\t複数呼びかけ待ち\t同点\t発言順 (先頭24)")
	for _, rl := range rules {
		m := run(sc, rl)
		gap := "-"
		if m.gmGapMax >= 0 {
			gap = fmt.Sprintf("%d〜%d", m.gmGapMin, m.gmGapMax)
		}
		pass := fmt.Sprintf("%d", m.passTurns)
		if m.passTurns < 0 {
			pass = "一巡せず"
		}
		fmt.Fprintf(w, "%s\t%.0f%%\t%s\t%s\t%s\t%s\t%d\t%s\n",
			rl.name, m.repeatRate, pass, gap, m.addressFollow, m.secondWait, m.ties, m.order)
	}
	w.Flush()
}

func plainRoster(names ...string) roster {
	r := make(roster, len(names))
	for i, n := range names {
		r[i] = participant{name: n}
	}
	return r
}

func gmRoster(names ...string) roster {
	r := plainRoster(names...)
	r[0].facilitator = true
	return r
}

func interventionLabel(every int) string {
	if every <= 0 {
		return "なし"
	}
	return fmt.Sprintf("%d ターンごと", every)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
