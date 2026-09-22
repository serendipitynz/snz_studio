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
	// talkativeness is SillyTavern's per-character "how likely to speak" setting
	// (_sandbox/sillytavern-natural-order.md) expressed the only way this engine
	// can: a constant factor on the weight, not a probability. 0 means unset and is
	// read as 1.0. The question it is here to answer is whether a constant factor
	// starves a quiet participant outright, which a lottery never does.
	talkativeness float64
}

func (p participant) factor() float64 {
	if p.talkativeness == 0 {
		return 1
	}
	return p.talkativeness
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

	// callWindow widens the call from the last message alone to the last N
	// participant utterances, so a call that has not been answered yet still
	// counts. 0 keeps the last-message-only form, and -1 means the roster size,
	// which is what the design specifies — spelling a number here instead let a
	// variant run a window of 4 against a roster of 3. It is what lets the second
	// participant of "A, B, what do you think?" keep its advantage after A speaks;
	// without it the call leaves the transcript's view the moment anyone answers.
	callWindow int
	// callBoost is what a participant with an outstanding call is multiplied by.
	callBoost float64
	// callExpiresOnAnswer drops the boost once the participant has spoken since the
	// call. Without it a participant that has already answered keeps the advantage
	// and sits on the turn.
	callExpiresOnAnswer bool

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
	// humanCallsAll makes each intervention call on the whole roster ("everyone,
	// what do you think?"), which is the case the design's "discard a call that
	// names everyone" filter was written for.
	humanCallsAll bool
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
		// talkativeness を定数係数として入れたときに、値の低い参加者が締め出されないかを見る。
		{name: "talkativeness: 全員 1.0 (基準)", roster: gmRoster("GM", "A", "B", "C"), turns: 60, addressRate: 0.3, seed: 9},
		{name: "talkativeness: C=0.8 (控えめ)", roster: withTalkativeness(gmRoster("GM", "A", "B", "C"), 1, 1, 1, 0.8), turns: 60, addressRate: 0.3, seed: 9},
		{name: "talkativeness: C=0.5 (かなり控えめ)", roster: withTalkativeness(gmRoster("GM", "A", "B", "C"), 1, 1, 1, 0.5), turns: 60, addressRate: 0.3, seed: 10},
		{name: "talkativeness: C=0.2 (ほぼ黙る)", roster: withTalkativeness(gmRoster("GM", "A", "B", "C"), 1, 1, 1, 0.2), turns: 60, addressRate: 0.3, seed: 11},
		// 進行役の免除を talkativeness で置き換えられるかを見る (免除なし + GM を上げる)。
		{name: "進行役の免除なし + GM=1.15", roster: withTalkativeness(gmRoster("GM", "A", "B", "C"), 1.15, 1, 1, 1), turns: 60, seed: 12},
		// 「編成の全員を呼ぶ」介入を捨てるか残すかで挙動が変わるかを見る。
		{name: "介入が編成の全員を呼ぶ", roster: gmRoster("GM", "A", "B", "C"), turns: 60, humanEvery: 6, humanCallsAll: true, seed: 13},
	}

	for _, sc := range scenarios {
		fmt.Printf("\n=== %s (%d ターン, 呼びかけ率 %.0f%%, うち 2 人呼び %.0f%%, 介入 %s) ===\n",
			sc.name, sc.turns, sc.addressRate*100, sc.multiCallRate*100, interventionLabel(sc.humanEvery))
		report(sc, rules())
	}
}

func rules() []rule {
	// The ticket's original coefficients: the call as a suppression of everyone but
	// the addressees, read from the last message alone.
	proposed := coefficients{lastSpeaker: 0.2, recentSpeaker: 0.85, notAddressee: 0.5, notRosterNext: 0.95, addresseeBoost: 1.0, facilitatorExemptRecent: true}
	// The same last-message-only form written as a raise on the addressee, which is
	// ranking-equivalent (notAddressee=x ranks as addresseeBoost=1/x) but tunable.
	lastMessageOnly := func(boost float64) coefficients {
		c := proposed
		c.notAddressee, c.addresseeBoost = 1.0, boost
		return c
	}
	// The adopted form: the call is a raise held over a window of the last
	// len(roster) participant utterances while it is unanswered. Every variant of
	// it below changes exactly one thing, so that a difference in the tables is
	// attributable — the earlier list varied the expiry policy and the coefficient
	// together and the comparison meant nothing.
	adopted := windowedCall(proposed, -1, 1.2, true)

	return []rule{
		{name: "採用", pick: weighted(adopted, humanIsCurrentSpeaker, byLongestSilence)},
		{name: "採用・加点×1.1", pick: weighted(windowedCall(proposed, -1, 1.1, true), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "採用・加点×1.3", pick: weighted(windowedCall(proposed, -1, 1.3, true), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "採用・加点×2.0", pick: weighted(windowedCall(proposed, -1, 2.0, true), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "採用・加点なし", pick: weighted(windowedCall(proposed, -1, 1.0, true), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "採用・応答でも残る", pick: weighted(windowedCall(proposed, -1, 1.2, false), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "採用・窓2固定", pick: weighted(windowedCall(proposed, 2, 1.2, true), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "採用・窓8固定", pick: weighted(windowedCall(proposed, 8, 1.2, true), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "採用・同点=名簿順", pick: weighted(adopted, humanIsCurrentSpeaker, byRosterOrder)},
		{name: "採用・介入時も×0.2", pick: weighted(adopted, penalizeLastParticipant, byLongestSilence)},
		{name: "採用・介入後は進行役", pick: weighted(adopted, facilitatorAnswersHuman, byLongestSilence)},
		{name: "採用・名指し無し介入は進行役", pick: weighted(adopted, facilitatorAnswersPlainHuman, byLongestSilence)},
		{name: "採用・直近抑制なし", pick: weighted(withRecent(adopted, 1.0), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "採用・進行役の免除なし", pick: weighted(withoutExemption(adopted), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "採用・沈黙を連続量", pick: weighted(continuousSilence(adopted, 0.15), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "起票時の原案(末尾のみ・他を×0.5)", pick: weighted(proposed, humanIsCurrentSpeaker, byLongestSilence)},
		{name: "起票時の原案(同点=名簿順・介入時も×0.2)", pick: weighted(proposed, penalizeLastParticipant, byRosterOrder)},
		{name: "末尾のみ・加点×1.2", pick: weighted(lastMessageOnly(1.2), humanIsCurrentSpeaker, byLongestSilence)},
		{name: "呼びかけ最優先(先頭)", pick: addressedFirst},
		{name: "呼びかけ最優先(沈黙が長い側)", pick: addressedLongestSilent},
		{name: "呼びかけ最優先(窓・未応答・沈黙が長い側)", pick: addressedOutstandingLongestSilent},
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

// windowedCall replaces the last-message-only call with one that stays in effect
// for the last `window` participant utterances.
func windowedCall(c coefficients, window int, boost float64, expires bool) coefficients {
	c.notAddressee, c.addresseeBoost = 1.0, 1.0
	c.callWindow, c.callBoost, c.callExpiresOnAnswer = window, boost, expires
	return c
}

// outstandingCall answers whether this participant was called on inside the
// window and, when expires is set, has not spoken since that call.
//
// The window is counted in participant utterances and the human's interventions
// do not consume it, the same way the silence window is counted: an interjection
// is not a turn anyone took, so letting it push a call out of the window would
// make the coefficient depend on how often the human speaks. A call the human
// made is still read — it is the message's content, not whose turn it was.
func outstandingCall(history []utterance, participant, window int, expires bool) bool {
	seen := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].calls(participant) {
			if !expires {
				return true
			}
			// Only the most recent call matters: if the participant has spoken since
			// it, every older call is staler still and was answered by the same
			// utterance or an earlier one.
			for j := i + 1; j < len(history); j++ {
				if history[j].speaker == participant {
					return false
				}
			}
			return true
		}
		if history[i].speaker != speakerHuman {
			seen++
			if seen >= window {
				return false
			}
		}
	}
	return false
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
			if c.callWindow != 0 {
				callWindow := c.callWindow
				if callWindow < 0 {
					callWindow = len(r)
				}
				if outstandingCall(history, i, callWindow, c.callExpiresOnAnswer) {
					w *= c.callBoost
				}
			} else if len(called.addressees) > 0 {
				if called.calls(i) {
					w *= c.addresseeBoost
				} else {
					w *= c.notAddressee
				}
			}
			if i != next {
				w *= c.notRosterNext
			}
			w *= r[i].factor()
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

// addressedOutstandingLongestSilent is the ordered rule given the same window the
// score gets: among the participants with a call that is still unanswered inside
// the window, the one that has waited longest speaks. Without this variant the
// comparison would credit the score for the window itself rather than for
// blending the call with the other pressures.
func addressedOutstandingLongestSilent(r roster, history []utterance) (int, []float64) {
	if len(r) == 0 {
		return -1, nil
	}
	best, rank := -1, -1
	for i := range r {
		if !outstandingCall(history, i, len(r), true) {
			continue
		}
		if s := silenceRank(history, i); s > rank {
			rank, best = s, i
		}
	}
	if best >= 0 {
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
	topShare      string  // 最も多く話した参加者の発言シェア (均等なら 1/人数)
	addressFollow string  // 呼びかけ追従率
	callWait      string  // 呼びかけから呼ばれた側が話すまでのターン数 (最悪) と未応答件数
	secondWait    string  // 複数呼びかけで後回しになった側が話すまでのターン数 (最悪)
	ties          int     // 同点が起きたターン数
	order         string
}

func run(sc scenario, rl rule) measurement {
	schedule := buildCallSchedule(sc)
	history := make([]utterance, 0, sc.turns)
	ties := 0

	for turn := 1; turn <= sc.turns; turn++ {
		if sc.humanEvery > 0 && turn%sc.humanEvery == 0 {
			var called []int
			if last := lastParticipantSpeaker(history); sc.humanCallsLast && last >= 0 {
				called = []int{last}
			}
			if sc.humanCallsAll {
				called = make([]int, len(sc.roster))
				for i := range called {
					called[i] = i
				}
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
		history = append(history, utterance{speaker: speaker, addressees: addresseesFor(schedule[turn], len(sc.roster), speaker)})
	}
	return measure(sc.roster, history, ties)
}

// buildCallSchedule decides, once per scenario and before any rule runs, which
// turns carry a call and how many participants each names. It stores offsets from
// whoever ends up speaking rather than participant ids, so the schedule is the
// same for every rule while a speaker still never calls on itself.
//
// Why not draw at the point of use: the draw then happens after the rule has
// picked its speaker, and rejection-sampling "anyone but the speaker" consumes a
// number of draws that depends on who that was. The call schedule diverged per
// rule, so a row with 22 calls was being compared against a row with 17 — not the
// same test. Offsets in [1, len(roster)-1] need no rejection at all.
func buildCallSchedule(sc scenario) [][]int {
	rng := rand.New(rand.NewSource(sc.seed))
	size := len(sc.roster)
	schedule := make([][]int, sc.turns+1)
	for turn := range schedule {
		if sc.addressRate <= 0 || rng.Float64() >= sc.addressRate {
			continue
		}
		first := 1 + rng.Intn(size-1)
		if sc.multiCallRate <= 0 || size < 3 || rng.Float64() >= sc.multiCallRate {
			schedule[turn] = []int{first}
			continue
		}
		second := 1 + rng.Intn(size-2)
		if second >= first {
			second++
		}
		schedule[turn] = []int{first, second}
	}
	return schedule
}

// addresseesFor resolves a turn's scheduled offsets against the participant that
// actually spoke.
func addresseesFor(offsets []int, size, speaker int) []int {
	if len(offsets) == 0 {
		return nil
	}
	called := make([]int, len(offsets))
	for i, off := range offsets {
		called[i] = (speaker + off) % size
	}
	return called
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
		topShare:      topSpeakerShare(r, spoken),
		addressFollow: followRate(history),
		callWait:      callAnswerWait(history, 1, len(r)),
		secondWait:    secondCallWait(history, len(r)),
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

// topSpeakerShare is the share of participant utterances taken by whoever spoke
// most, against the even share for the roster size. It is what answers "does this
// coefficient let someone sit on the conversation" — the follow rate and the pass
// length can both look healthy while one participant takes a third of the turns.
func topSpeakerShare(r roster, spoken []int) string {
	if len(spoken) == 0 {
		return "-"
	}
	counts := make([]int, len(r))
	for _, s := range spoken {
		counts[s]++
	}
	top, who := 0, 0
	for i, n := range counts {
		if n > top {
			top, who = n, i
		}
	}
	return fmt.Sprintf("%.0f%% (%s, 均等 %.0f%%)",
		float64(top)/float64(len(spoken))*100, r[who].name, 100/float64(len(r)))
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

// callAnswerWait is how long a call waits to be answered, over every call naming
// at least minNamed participants. The follow rate only asks whether the addressee
// spoke on the very next turn, which a windowed call deliberately allows it not
// to — so without this, widening the window reads as a regression when what
// actually happened is that the call was honoured a turn or two later.
func callAnswerWait(history []utterance, minNamed, window int) string {
	worst, calls, unanswered := 0, 0, 0
	for i, u := range history {
		if len(u.addressees) < minNamed {
			continue
		}
		// A call with fewer than `window` participant utterances after it has not
		// been left unanswered — the run stopped before the window it was promised
		// (4.6.1) could elapse. followRate was corrected for the same artifact in
		// the first review round; these two were not, and the ×1.2-over-×1.3 choice
		// was being made on the residual queue at turn 60.
		if remainingTurns(history, i) < window {
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
	return fmt.Sprintf("%d (%d件)", worst, calls)
}

// remainingTurns counts the participant utterances stored after index i.
func remainingTurns(history []utterance, i int) int {
	n := 0
	for j := i + 1; j < len(history); j++ {
		if history[j].speaker != speakerHuman {
			n++
		}
	}
	return n
}

// secondCallWait answers what happens to every participant a call on two or more
// named: how many turns pass before each of them has spoken, reported as the worst
// case, since the question is whether anyone is left hanging rather than what the
// average is. The participant that answered first cannot hold that maximum, so it
// is left in rather than special-cased.
func secondCallWait(history []utterance, window int) string {
	worst, calls, unanswered := 0, 0, 0
	for i, u := range history {
		if len(u.addressees) < 2 {
			continue
		}
		if remainingTurns(history, i) < window {
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
	fmt.Fprintln(w, "規則\t連続発言率\t最大シェア\t一巡ターン数\t進行役の間隔\t即時追従\t呼びかけ応答待ち\t複数呼びかけ待ち\t同点\t発言順 (先頭24)")
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
		fmt.Fprintf(w, "%s\t%.0f%%\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
			rl.name, m.repeatRate, m.topShare, pass, gap, m.addressFollow, m.callWait, m.secondWait, m.ties, m.order)
	}
	w.Flush()
}

// withTalkativeness sets the per-participant factor by roster position.
func withTalkativeness(r roster, factors ...float64) roster {
	out := make(roster, len(r))
	copy(out, r)
	for i := range out {
		if i < len(factors) {
			out[i].talkativeness = factors[i]
		}
	}
	return out
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
