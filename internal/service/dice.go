package service

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"snzstudio/internal/model"
)

// ErrInvalidRollCommand marks a /roll whose arguments could not be read. Only a
// human's intervention is refused for it (400); a participant's utterance keeps
// the line as text, because failing the turn would stop an auto-advancing
// conversation (design §4.8.3 item 2).
var ErrInvalidRollCommand = errors.New("service: the /roll command could not be read")

const rollKeyword = "/roll"

// The bounds of a /roll expression (design §4.8.3 item 4). The modifier has no
// bound in the design; 999 keeps the total far inside an int and above any
// modifier a table would write.
const (
	rollMaxDice     = 20
	rollMinSides    = 2
	rollMaxSides    = 100
	rollMaxModifier = 999
)

// rollExpression is NdM, NdM+K or NdM-K. The full-width plus and the minus signs
// a Japanese-writing model or IME produces (−, －, ＋) are read as their ASCII
// forms, as the addressee directive reads full-width brackets.
var rollExpression = regexp.MustCompile(`^(\d+)[dD](\d+)(?:([+\-−－＋])(\d+))?$`)

// rollTarget is the target field: 目標N, or targetN for a table that plays in
// English, with an optional colon; 目標なし or 0 asks for no comparison.
var rollTarget = regexp.MustCompile(`^(?i:目標|target)[:：]?(\d*|なし)$`)

// rollTargetField tells a field meant as the target from the action. Any field
// starting with 目標 is the target, so a malformed one is refused rather than
// rolled against the default. "target" is an English word an action can start
// with ("targeted strike"), so it counts only alone or followed by a colon or
// a digit.
var rollTargetField = regexp.MustCompile(`^(?:目標|(?i:target)(?:$|[:：\d０-９]))`)

// rollCommand is a /roll read from an utterance, before the dice are thrown.
type rollCommand struct {
	line       string
	expression string
	count      int
	sides      int
	modifier   int
	target     int  // 0 when the command names none
	noTarget   bool // 目標なし / 目標0: compare nothing, whatever the default
	action     string
}

// splitRollCommand looks for a /roll on the last line of content, where it may
// follow the body without a line break (design §4.8.2 reading 2). It returns the
// content with the command removed and whether one was there; cmd is nil and
// err wraps ErrInvalidRollCommand when it was there but could not be read, and
// the caller decides whether the line stays (design §4.8.3 item 2).
func splitRollCommand(content string) (stripped string, found bool, cmd *rollCommand, err error) {
	trimmed := strings.TrimRight(content, " \t\r\n")
	lineStart := strings.LastIndex(trimmed, "\n") + 1
	at := rollKeywordIndex(trimmed[lineStart:])
	if at < 0 {
		return content, false, nil, nil
	}
	at += lineStart
	stripped = strings.TrimRight(trimmed[:at], " \t\r\n　")
	cmd, err = parseRollCommand(trimmed[at:])
	return stripped, true, cmd, err
}

// diceMarker opens the line the prompt adds for each roll (design §4.8.3 item
// 2). It is the app's notation: a speaker that writes one itself is either
// forging a result or imitating the command, and the app cannot tell its own
// record from such a line once both sit in the transcript.
const diceMarker = "【ダイス】"

// diceMarkerResult is what a 【ダイス】 line carries once a result is written
// into it — the mapping's "→" and the total's "=". A line carrying any of them
// is a forged result and is never rolled, whatever else it holds.
var diceMarkerResult = regexp.MustCompile(`→|->|⇒|=|＝`)

// splitDiceMarkerCommand reads a result-free 【ダイス】 on the last line as a
// /roll the speaker meant to write (TASK-63): a player that copied the mapping's
// shape declared a roll as much as one that wrote the command. The mapping's own
// order, action first and the dice after "—", is read too. It returns what
// splitRollCommand does, and found is false both when there is no such marker
// and when what follows it does not read as a /roll: that line is then removed
// by stripDiceMarkers like any other, rather than kept as text, because it is
// the app's notation either way.
func splitDiceMarkerCommand(content string) (stripped string, found bool, cmd *rollCommand) {
	trimmed := strings.TrimRight(content, " \t\r\n")
	lineStart := strings.LastIndex(trimmed, "\n") + 1
	at := strings.LastIndex(trimmed[lineStart:], diceMarker)
	if at < 0 {
		return content, false, nil
	}
	at += lineStart
	written := strings.TrimSpace(trimmed[at:])
	args := strings.TrimSpace(written[len(diceMarker):])
	if diceMarkerResult.MatchString(args) {
		return content, false, nil
	}
	if action, dice, ok := strings.Cut(args, "—"); ok {
		args = strings.TrimSpace(dice) + " " + strings.TrimSpace(action)
	}
	cmd, err := parseRollCommand(rollKeyword + " " + args)
	if err != nil {
		return content, false, nil
	}
	cmd.line = written
	return strings.TrimRight(trimmed[:at], " \t\r\n　"), true, cmd
}

// stripDiceMarkers removes every 【ダイス】 the text itself carries, from the
// marker to the end of its line, and drops a line left empty. It returns the
// removed parts for the log. Only the app writes the line (TASK-63): left in the
// body, a forged "【ダイス】1d20+3 → 14+3 = 17（目標 12、成功）" would reach every
// later speaker as a result the app never rolled.
func stripDiceMarkers(content string) (string, []string) {
	if !strings.Contains(content, diceMarker) {
		return content, nil
	}
	var removed []string
	lines := strings.Split(content, "\n")
	kept := lines[:0]
	for _, line := range lines {
		at := strings.Index(line, diceMarker)
		if at < 0 {
			kept = append(kept, line)
			continue
		}
		removed = append(removed, strings.TrimSpace(line[at:]))
		if rest := strings.TrimRight(line[:at], " \t\r　"); strings.TrimSpace(rest) != "" {
			kept = append(kept, rest)
		}
	}
	return strings.Trim(strings.Join(kept, "\n"), "\r\n"), removed
}

// rollKeywordIndex finds /roll as a word of its own: at the start of the line or
// after a space or a non-ASCII character (a sentence ending in 。 runs straight
// into it), and followed by a space or the end. A path or URL containing /roll
// is left alone.
func rollKeywordIndex(line string) int {
	offset := 0
	for {
		i := strings.Index(line[offset:], rollKeyword)
		if i < 0 {
			return -1
		}
		i += offset
		after := i + len(rollKeyword)
		if rollBoundaryBefore(line[:i]) && (after == len(line) || startsWithSpace(line[after:])) {
			return i
		}
		offset = after
	}
}

func rollBoundaryBefore(before string) bool {
	if before == "" {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(before)
	return unicode.IsSpace(r) || r > unicode.MaxASCII
}

func startsWithSpace(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsSpace(r)
}

// parseRollCommand reads `/roll <式> [目標<整数>] [行動]`. The arguments come in
// that fixed order and nothing else is understood (design §4.8.3 item 4).
func parseRollCommand(line string) (*rollCommand, error) {
	cmd := &rollCommand{line: strings.TrimSpace(line)}
	expression, rest := nextField(line[len(rollKeyword):])
	if expression == "" {
		return nil, fmt.Errorf("%w: write the dice after /roll, as in /roll 1d20+3", ErrInvalidRollCommand)
	}
	if err := cmd.readExpression(asciiDigits(expression)); err != nil {
		return nil, err
	}

	// A target field that does not read as a whole number is refused rather
	// than taken for the action: "目標12.5" would otherwise roll against the
	// default target, which is not what was written.
	field, afterField := nextField(rest)
	if rollTargetField.MatchString(field) {
		m := rollTarget.FindStringSubmatch(asciiDigits(field))
		if m == nil {
			return nil, errRollTarget()
		}
		value := m[1]
		if value == "" {
			// 目標 12, with the number as the next field.
			value, afterField = nextField(afterField)
			value = asciiDigits(value)
		}
		if value == "なし" || value == "0" {
			cmd.noTarget = true
		} else {
			target, err := strconv.Atoi(value)
			if err != nil || target < 1 || target > model.DiceTargetMax {
				return nil, errRollTarget()
			}
			cmd.target = target
		}
		rest = afterField
	}
	cmd.action = strings.TrimSpace(rest)
	return cmd, nil
}

func errRollTarget() error {
	return fmt.Errorf("%w: 目標 (target) takes a whole number from 1 to %d, or 0 / なし for no comparison", ErrInvalidRollCommand, model.DiceTargetMax)
}

func (c *rollCommand) readExpression(expression string) error {
	m := rollExpression.FindStringSubmatch(expression)
	if m == nil {
		return fmt.Errorf("%w: the dice must look like NdM, NdM+K or NdM-K", ErrInvalidRollCommand)
	}
	c.count, _ = strconv.Atoi(m[1])
	c.sides, _ = strconv.Atoi(m[2])
	if c.count < 1 || c.count > rollMaxDice {
		return fmt.Errorf("%w: roll 1 to %d dice", ErrInvalidRollCommand, rollMaxDice)
	}
	if c.sides < rollMinSides || c.sides > rollMaxSides {
		return fmt.Errorf("%w: a die has %d to %d sides", ErrInvalidRollCommand, rollMinSides, rollMaxSides)
	}
	c.expression = fmt.Sprintf("%dd%d", c.count, c.sides)
	if m[3] != "" {
		modifier, err := strconv.Atoi(m[4])
		if err != nil || modifier > rollMaxModifier {
			return fmt.Errorf("%w: the modifier goes up to %d", ErrInvalidRollCommand, rollMaxModifier)
		}
		sign := "+"
		if m[3] != "+" && m[3] != "＋" {
			sign = "-"
			modifier = -modifier
		}
		c.modifier = modifier
		c.expression += sign + m[4]
	}
	return nil
}

// roll throws the command's dice. A command without a target of its own is
// compared against the chat's default, and with neither only the total is
// recorded (design §4.8.3 item 4). 目標なし records the total alone even when
// the chat has a default: a roll whose dice matter rather than a pass or a fail
// (who goes first, 丁半 read off the total) would otherwise be marked a failure.
func (c *rollCommand) roll(rollDie func(sides int) int, defaultTarget int) model.DiceRoll {
	record := model.DiceRoll{
		Command:    c.line,
		Expression: c.expression,
		Action:     c.action,
		Dice:       make([]int, c.count),
		Modifier:   c.modifier,
		Target:     c.target,
	}
	record.Total = c.modifier
	for i := range record.Dice {
		record.Dice[i] = rollDie(c.sides)
		record.Total += record.Dice[i]
	}
	if record.Target == 0 && !c.noTarget {
		record.Target = defaultTarget
	}
	if record.Target > 0 {
		success := record.Total >= record.Target
		record.Success = &success
	}
	return record
}

// rollDie is the app's own die. Fairness here means the value cannot be chosen
// to suit the story, not that it cannot be predicted, so math/rand/v2 serves and
// no seed is kept (design §4.8.3 item 1).
func rollDie(sides int) int {
	return rand.IntN(sides) + 1
}

// diceRollLine is one roll as the transcript reads it:
// "岩棚を渡る — 1d20+3 → 4+3 = 7（目標 12、失敗）". The prompt prefixes it with
// 【ダイス】 (design §4.8.3 item 2) and the markdown export with the chip's die.
func diceRollLine(r model.DiceRoll) string {
	var b strings.Builder
	if r.Action != "" {
		b.WriteString(r.Action + " — ")
	}
	b.WriteString(r.Expression + " → ")
	parts := make([]string, len(r.Dice))
	for i, die := range r.Dice {
		parts[i] = strconv.Itoa(die)
	}
	b.WriteString(strings.Join(parts, "+"))
	if r.Modifier > 0 {
		fmt.Fprintf(&b, "+%d", r.Modifier)
	} else if r.Modifier < 0 {
		fmt.Fprintf(&b, "-%d", -r.Modifier)
	}
	if len(r.Dice) > 1 || r.Modifier != 0 {
		fmt.Fprintf(&b, " = %d", r.Total)
	}
	if r.Success != nil {
		outcome := "失敗"
		if *r.Success {
			outcome = "成功"
		}
		fmt.Fprintf(&b, "（目標 %d、%s）", r.Target, outcome)
	}
	return b.String()
}

// nextField splits off the first whitespace-separated field (full-width spaces
// included).
func nextField(s string) (string, string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	end := strings.IndexFunc(s, unicode.IsSpace)
	if end < 0 {
		return s, ""
	}
	return s[:end], s[end:]
}

// asciiDigits reads full-width digits as ASCII ones, which an IME left in
// full-width mode types.
func asciiDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '０' && r <= '９' {
			return '0' + (r - '０')
		}
		return r
	}, s)
}
