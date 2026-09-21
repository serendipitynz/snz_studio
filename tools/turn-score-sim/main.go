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

const noAddressee = -1

type participant struct {
	name        string
	facilitator bool
}

// utterance is one stored message: who spoke, and whom that message called on.
// The addressee is fixed when the message is stored and never re-derived, which
// is what keeps the next speaker the same across a restart (design §2).
type utterance struct {
	speaker   int
	addressee int
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
	seed           int64
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
	}

	for _, sc := range scenarios {
		fmt.Printf("\n=== %s (%d ターン, 呼びかけ率 %.0f%%, 介入 %s) ===\n",
			sc.name, sc.turns, sc.addressRate*100, interventionLabel(sc.humanEvery))
		report(sc, rules())
	}
}

func rules() []rule {
	proposed := coefficients{lastSpeaker: 0.2, recentSpeaker: 0.85, notAddressee: 0.5, notRosterNext: 0.95, facilitatorExemptRecent: true}

	return []rule{
		{name: "原案", pick: weighted(proposed, penalizeLastParticipant, byRosterOrder)},
		{name: "原案+沈黙同点", pick: weighted(proposed, penalizeLastParticipant, byLongestSilence)},
		{name: "原案+人間=話者", pick: weighted(proposed, humanIsCurrentSpeaker, byRosterOrder)},
		{name: "原案+介入は進行役", pick: weighted(proposed, facilitatorAnswersHuman, byRosterOrder)},
		{name: "呼びかけ強め(0.2)", pick: weighted(withAddressee(proposed, 0.2), humanIsCurrentSpeaker, byRosterOrder)},
		{name: "直近抑制なし", pick: weighted(withRecent(proposed, 1.0), humanIsCurrentSpeaker, byRosterOrder)},
		{name: "直近抑制0.5", pick: weighted(withRecent(proposed, 0.5), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "進行役の免除なし", pick: weighted(withoutExemption(proposed), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "推奨案", pick: weighted(proposed, facilitatorAnswersPlainHuman, byLongestSilence)},
		{name: "決定木", pick: decisionTree},
		{name: "round_robin", pick: roundRobin},
		{name: "facilitator交互", pick: facilitatorAlternating},
	}
}

func withAddressee(c coefficients, v float64) coefficients { c.notAddressee = v; return c }
func withRecent(c coefficients, v float64) coefficients    { c.recentSpeaker = v; return c }
func withoutExemption(c coefficients) coefficients         { c.facilitatorExemptRecent = false; return c }

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
			plain := history[len(history)-1].addressee == noAddressee
			if human == facilitatorAnswersHuman || (human == facilitatorAnswersPlainHuman && plain) {
				return gm, nil
			}
		}

		lastParticipant := lastParticipantSpeaker(history)
		penalized := lastParticipant
		if human != penalizeLastParticipant && lastIsHuman(history) {
			penalized = -1
		}

		addressee := noAddressee
		if n := len(history); n > 0 {
			addressee = history[n-1].addressee
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
			if spokeWithin(history, i, window) && !(c.facilitatorExemptRecent && r[i].facilitator) {
				w *= c.recentSpeaker
			}
			if addressee != noAddressee && i != addressee {
				w *= c.notAddressee
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
		if s := turnsSinceSpoken(history, i); s > silence {
			silence, best = s, i
		}
	}
	return best
}

// decisionTree is the alternative TASK-28 leaves open: the same intentions as
// ordered rules rather than as a product. To make the comparison fair it is the
// rules the engine already ships, with the addressee overriding them — not a
// weaker rotation invented for the contrast.
func decisionTree(r roster, history []utterance) (int, []float64) {
	if len(r) == 0 {
		return -1, nil
	}
	// No guard against the addressee having just spoken: a participant calling on
	// itself is discarded when the utterance is stored, so an addressee that spoke
	// last can only come from the human asking that speaker to go on.
	if n := len(history); n > 0 && history[n-1].addressee != noAddressee {
		return history[n-1].addressee, nil
	}
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
	ties          int     // 同点が起きたターン数
	order         string
}

func run(sc scenario, rl rule) measurement {
	rng := rand.New(rand.NewSource(sc.seed))
	history := make([]utterance, 0, sc.turns)
	ties := 0

	for turn := 1; turn <= sc.turns; turn++ {
		if sc.humanEvery > 0 && turn%sc.humanEvery == 0 {
			called := noAddressee
			if sc.humanCallsLast {
				called = lastParticipantSpeaker(history)
			}
			history = append(history, utterance{speaker: speakerHuman, addressee: called})
		}
		speaker, weights := rl.pick(sc.roster, history)
		if speaker < 0 {
			break
		}
		if tiedAtTop(weights) {
			ties++
		}
		history = append(history, utterance{speaker: speaker, addressee: drawAddressee(rng, sc, speaker)})
	}
	return measure(sc.roster, history, ties)
}

// drawAddressee picks whom the utterance calls on. A speaker never calls on
// itself, which is what the detection of §4.6 would discard anyway.
func drawAddressee(rng *rand.Rand, sc scenario, speaker int) int {
	if sc.addressRate <= 0 || rng.Float64() >= sc.addressRate {
		return noAddressee
	}
	for {
		if candidate := rng.Intn(len(sc.roster)); candidate != speaker {
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

func followRate(history []utterance) string {
	called, followed := 0, 0
	for i, u := range history {
		if u.addressee == noAddressee {
			continue
		}
		called++
		for j := i + 1; j < len(history); j++ {
			if history[j].speaker == speakerHuman {
				continue
			}
			if history[j].speaker == u.addressee {
				followed++
			}
			break
		}
	}
	if called == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d", followed, called)
}

func report(sc scenario, rules []rule) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "規則\t連続発言率\t一巡ターン数\t進行役の間隔\t呼びかけ追従\t同点\t発言順 (先頭24)")
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
		fmt.Fprintf(w, "%s\t%.0f%%\t%s\t%s\t%s\t%d\t%s\n",
			rl.name, m.repeatRate, pass, gap, m.addressFollow, m.ties, m.order)
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
