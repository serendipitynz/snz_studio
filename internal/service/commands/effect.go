package commands

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"snzstudio/internal/model"
)

// ErrInvalidEffectCommand marks an effect command whose arguments could not be
// read, its owner included. It is treated as an unreadable /roll is: a human's
// intervention is refused (400), a participant's utterance keeps the line as
// text (design §4.8.8).
var ErrInvalidEffectCommand = errors.New("commands: the effect command could not be read")

// EffectMarker opens the line the prompt adds for each effect (design §4.8.8),
// as DiceMarker does for each roll. Unlike 【ダイス】 it is not removed when a
// model writes it: no imitation of it has been observed, and §4.8.7 leaves
// such a list to be drawn from what is seen.
const EffectMarker = "【効果】"

// SharedOwner is how an effect command names the chat's shared state sheet as
// its owner (design §4.8.4).
const SharedOwner = "共通"

const (
	addKeyword = "/add"
	useKeyword = "/use"
	setKeyword = "/set"
)

// effectMaxAmount bounds a whole-number /add, as the modifier of a /roll is
// bounded: it catches a slip, not a rule of any game.
const effectMaxAmount = 9999

// ownerAnnotation is the parenthesised note the bundled presets add to a
// display name ("レン (斥候)"), which an owner is written without, as a call is
// (design §4.6.5).
var ownerAnnotation = regexp.MustCompile(`\s*[(（][^()（）]*[)）]\s*$`)

var wholeNumber = regexp.MustCompile(`^\d+$`)

// Effect is an effect command read from an utterance, its owner resolved on the
// roster, before it is applied to the owner's state sheet.
type Effect struct {
	line          string
	kind          string // add, use or set
	owner         string // SharedOwner or the participant's display name
	participantID string // empty for the shared sheet
	item          string
	amount        int   // add's whole number, use's -1
	dice          *Roll // add's dice, when it wrote an expression
	negative      bool  // the sign in front of add's dice
	value         string
}

// ParticipantID is the participant whose sheet the effect goes to, empty for
// the chat's shared sheet.
func (e *Effect) ParticipantID() string {
	return e.participantID
}

// parseEffect reads `/add <持ち主> <項目名> <±整数 または 式>`, `/use <持ち主>
// <項目名>` or `/set <持ち主> <項目名> <値>` (design §4.8.4, §4.8.8).
func parseEffect(line, keyword string, roster []model.Participant) (*Effect, error) {
	effect := &Effect{line: strings.TrimSpace(line), kind: strings.TrimPrefix(keyword, "/")}
	owner, participantID, rest, ok := matchOwner(strings.TrimSpace(line[len(keyword):]), roster)
	if !ok {
		return nil, fmt.Errorf("%w: begin with the owner, %s or a participant's name, as in %s レン HP -3", ErrInvalidEffectCommand, SharedOwner, addKeyword)
	}
	effect.owner, effect.participantID = owner, participantID

	// "/set 共通 場所: 第二坑道" and "場所:第二坑道" both name the item 場所: the
	// colon is how the sheet itself writes it.
	item, rest := nextField(rest)
	if cut := strings.IndexAny(item, ":："); cut >= 0 {
		_, size := utf8.DecodeRuneInString(item[cut:])
		item, rest = item[:cut], item[cut+size:]+" "+rest
	}
	if item == "" {
		return nil, fmt.Errorf("%w: write the item name after the owner", ErrInvalidEffectCommand)
	}
	effect.item = item

	switch keyword {
	case addKeyword:
		amount, extra := nextField(rest)
		if strings.TrimSpace(extra) != "" || amount == "" {
			return nil, fmt.Errorf("%w: /add takes one amount after the item, such as -3 or -1d6", ErrInvalidEffectCommand)
		}
		if err := effect.readAmount(amount); err != nil {
			return nil, err
		}
	case useKeyword:
		if strings.TrimSpace(rest) != "" {
			return nil, fmt.Errorf("%w: /use takes nothing after the item", ErrInvalidEffectCommand)
		}
		effect.amount = -1
	default:
		effect.value = strings.TrimSpace(rest)
		if effect.value == "" {
			return nil, fmt.Errorf("%w: write the value after the item", ErrInvalidEffectCommand)
		}
	}
	return effect, nil
}

// readAmount reads ±N or [±]NdM[±K]. A sign in front of dice applies to the
// whole expression, so -1d6+1 takes 1d6+1 away, the way damage is written.
func (e *Effect) readAmount(field string) error {
	field = asciiDigits(field)
	negative := false
	switch r, size := utf8.DecodeRuneInString(field); r {
	case '+', '＋':
		field = field[size:]
	case '-', '−', '－':
		negative = true
		field = field[size:]
	}
	if wholeNumber.MatchString(field) {
		n, err := strconv.Atoi(field)
		if err != nil || n > effectMaxAmount {
			return fmt.Errorf("%w: the amount goes up to %d", ErrInvalidEffectCommand, effectMaxAmount)
		}
		if negative {
			n = -n
		}
		e.amount = n
		return nil
	}
	dice := &Roll{}
	if err := dice.readExpression(field); err != nil {
		return fmt.Errorf("%w: the amount must be a whole number such as -3 or dice such as -1d6 (%v)", ErrInvalidEffectCommand, err)
	}
	e.dice, e.negative = dice, negative
	return nil
}

// matchOwner reads the owner at the start of args: 共通, or a roster
// participant by display name or by the name before its note, case folded. The
// longest match wins, so a display name with a space in it can be written in
// full. A name more than one participant has is nobody's, as for a call
// (design §4.6.5), and 共通 always means the shared sheet.
func matchOwner(args string, roster []model.Participant) (owner, participantID, rest string, ok bool) {
	type candidate struct {
		owner, id string
		shared    int // how many participants have the name
	}
	candidates := map[string]*candidate{}
	for _, p := range roster {
		full := strings.TrimSpace(p.DisplayName)
		for _, name := range []string{full, strings.TrimSpace(ownerAnnotation.ReplaceAllString(full, ""))} {
			key := strings.ToLower(name)
			if name == "" || key == strings.ToLower(SharedOwner) {
				continue
			}
			if c, seen := candidates[key]; seen {
				if c.id != p.ID {
					c.shared++
				}
				continue
			}
			candidates[key] = &candidate{owner: full, id: p.ID, shared: 1}
		}
	}
	candidates[SharedOwner] = &candidate{owner: SharedOwner, shared: 1}

	matched := ""
	for name, c := range candidates {
		if c.shared > 1 || len(name) <= len(matched) {
			continue
		}
		prefix, after := splitRunes(args, utf8.RuneCountInString(name))
		if strings.EqualFold(prefix, name) && (after == "" || startsWithSpace(after)) {
			matched, owner, participantID, rest = name, c.owner, c.id, after
		}
	}
	return owner, participantID, rest, matched != ""
}

func splitRunes(s string, n int) (string, string) {
	i := 0
	for ; n > 0 && i < len(s); n-- {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i], s[i:]
}

// Apply applies the effect to the owner's sheet as it stands now and returns
// the sheet to store with the record of what happened (design §4.8.8). The
// preconditions are read here, off the sheet, because a prompt cannot keep a
// model from using what it does not have (§4.7.2 reading 3). An effect that is
// not applied returns the sheet unchanged. add's dice are thrown only once
// its preconditions hold.
func (e *Effect) Apply(sheet string, rollDie func(sides int) int) (string, model.StateEffect) {
	record := model.StateEffect{
		Command:       e.line,
		Kind:          e.kind,
		Owner:         e.owner,
		ParticipantID: e.participantID,
		Item:          e.item,
		Delta:         e.amount,
		Value:         e.value,
	}
	if e.dice != nil {
		record.Expression = e.signedExpression()
	}
	var lines []string
	if strings.TrimSpace(sheet) != "" {
		lines = strings.Split(strings.TrimSpace(sheet), "\n")
	}
	index, valueStart := findItem(lines, e.item)
	if index >= 0 {
		before := strings.TrimSpace(lines[index][valueStart:])
		record.Before = &before
	}
	notApplied := func(reason string) (string, model.StateEffect) {
		record.Reason = reason
		return sheet, record
	}

	var after string
	switch {
	case e.kind == "set":
		after = e.value
		if index < 0 {
			lines = append(lines, e.item+": "+after)
		}
	case index < 0:
		return notApplied(model.EffectMissingItem)
	default:
		number, remainder, ok := leadingInteger(*record.Before)
		if !ok {
			return notApplied(model.EffectNotInteger)
		}
		if e.kind == "use" && number < 1 {
			return notApplied(model.EffectNotPositive)
		}
		if e.dice != nil {
			record.Dice, record.Modifier, record.Delta = e.throwDice(rollDie)
		}
		after = strconv.Itoa(number+record.Delta) + remainder
	}
	if index >= 0 {
		lines[index] = lines[index][:valueStart] + after
	}

	updated := strings.Join(lines, "\n")
	if utf8.RuneCountInString(updated) > e.limit() {
		record.Dice, record.Modifier, record.Delta = nil, 0, e.amount
		return notApplied(model.EffectOverLimit)
	}
	record.After = after
	record.Applied = true
	return updated, record
}

func (e *Effect) limit() int {
	if e.participantID == "" {
		return model.ChatStateSheetMaxRunes
	}
	return model.ParticipantStateSheetMaxRunes
}

func (e *Effect) signedExpression() string {
	if e.negative {
		return "-" + e.dice.expression
	}
	return "+" + e.dice.expression
}

func (e *Effect) throwDice(rollDie func(sides int) int) (dice []int, modifier, delta int) {
	thrown := e.dice.Throw(rollDie, 0)
	if e.negative {
		return thrown.Dice, thrown.Modifier, -thrown.Total
	}
	return thrown.Dice, thrown.Modifier, thrown.Total
}

// findItem finds the item's line: the first whose text before its first colon
// is the item name, case folded. valueStart is where the value begins, after
// the colon and the spaces that follow it; a line with nothing after its colon
// gets a space there, so the value written into it is set apart.
func findItem(lines []string, item string) (index, valueStart int) {
	for i, line := range lines {
		cut := strings.IndexAny(line, ":：")
		if cut < 0 || !strings.EqualFold(strings.TrimSpace(line[:cut]), item) {
			continue
		}
		_, size := utf8.DecodeRuneInString(line[cut:])
		start := cut + size
		value := strings.TrimLeftFunc(line[start:], unicode.IsSpace)
		if value == "" {
			lines[i] = line[:start] + " "
			return i, start + 1
		}
		return i, len(line) - len(value)
	}
	return -1, 0
}

// leadingInteger splits a value into its leading whole number and the rest:
// "7/10" is 7 and "/10", "-2 本" is -2 and " 本". Full-width digits and minus
// signs read as a human's IME types them; the number is written back in ASCII.
func leadingInteger(value string) (int, string, bool) {
	var number strings.Builder
	rest := value
	if r, size := utf8.DecodeRuneInString(rest); strings.ContainsRune("+-−－＋", r) {
		if r == '-' || r == '−' || r == '－' {
			number.WriteByte('-')
		}
		rest = rest[size:]
	}
	digits := 0
	for rest != "" {
		r, size := utf8.DecodeRuneInString(rest)
		switch {
		case r >= '0' && r <= '9':
			number.WriteRune(r)
		case r >= '０' && r <= '９':
			number.WriteRune('0' + (r - '０'))
		default:
			return finishInteger(number.String(), rest, digits)
		}
		digits++
		rest = rest[size:]
	}
	return finishInteger(number.String(), rest, digits)
}

func finishInteger(number, rest string, digits int) (int, string, bool) {
	if digits == 0 {
		return 0, "", false
	}
	n, err := strconv.Atoi(number)
	if err != nil {
		return 0, "", false
	}
	return n, rest, true
}

// EffectLine is one effect as the transcript reads it: "レン HP -3: 7/10 →
// 4/10", or "レン たいまつ -1 — 適用されず（たいまつが 1 未満）" for one that was
// not applied. The prompt prefixes it with 【効果】 and the markdown export and
// the memory draft with 📝 (design §4.8.8), as they do 【ダイス】 and 🎲.
func EffectLine(r model.StateEffect) string {
	subject := r.Owner + " " + r.Item
	if r.Kind == "set" {
		if !r.Applied {
			return fmt.Sprintf("%s → %s — 適用されず（%s）", subject, r.Value, effectReason(r))
		}
		before := "（無し）"
		if r.Before != nil {
			before = *r.Before
		}
		return fmt.Sprintf("%s: %s → %s", subject, before, r.After)
	}
	change := effectAmount(r)
	if !r.Applied {
		return fmt.Sprintf("%s %s — 適用されず（%s）", subject, change, effectReason(r))
	}
	return fmt.Sprintf("%s %s: %s → %s", subject, change, *r.Before, r.After)
}

// effectAmount is "-3" for a whole number, and "-1d6+1（3+1 = 4）" for dice,
// with the breakdown only once they were thrown.
func effectAmount(r model.StateEffect) string {
	if r.Expression == "" {
		return fmt.Sprintf("%+d", r.Delta)
	}
	if len(r.Dice) == 0 {
		return r.Expression
	}
	parts := make([]string, len(r.Dice))
	for i, die := range r.Dice {
		parts[i] = strconv.Itoa(die)
	}
	breakdown := strings.Join(parts, "+")
	if r.Modifier != 0 {
		breakdown += fmt.Sprintf("%+d", r.Modifier)
	}
	if len(r.Dice) > 1 || r.Modifier != 0 {
		total := r.Delta
		if total < 0 {
			total = -total
		}
		breakdown += fmt.Sprintf(" = %d", total)
	}
	return r.Expression + "（" + breakdown + "）"
}

func effectReason(r model.StateEffect) string {
	switch r.Reason {
	case model.EffectMissingItem:
		return r.Item + "の行が無い"
	case model.EffectNotInteger:
		return r.Item + "の値が整数で始まらない"
	case model.EffectNotPositive:
		return r.Item + "が 1 未満"
	case model.EffectOverLimit:
		limit := model.ChatStateSheetMaxRunes
		if r.ParticipantID != "" {
			limit = model.ParticipantStateSheetMaxRunes
		}
		return fmt.Sprintf("状態が %d 字を超える", limit)
	}
	return r.Reason
}
