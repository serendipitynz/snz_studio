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

// rollTarget is 目標N written as one field, with an optional colon.
var rollTarget = regexp.MustCompile(`^目標[:：]?(\d*)$`)

// rollCommand is a /roll read from an utterance, before the dice are thrown.
type rollCommand struct {
	line       string
	expression string
	count      int
	sides      int
	modifier   int
	target     int // 0 when the command names none
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

	field, afterField := nextField(rest)
	if m := rollTarget.FindStringSubmatch(asciiDigits(field)); m != nil {
		digits := m[1]
		if digits == "" {
			// 目標 12, with the number as the next field.
			digits, afterField = nextField(afterField)
			digits = asciiDigits(digits)
		}
		target, err := strconv.Atoi(digits)
		if err != nil || target < 1 || target > model.DiceTargetMax {
			return nil, fmt.Errorf("%w: 目標 takes a whole number from 1 to %d", ErrInvalidRollCommand, model.DiceTargetMax)
		}
		cmd.target = target
		rest = afterField
	}
	cmd.action = strings.TrimSpace(rest)
	return cmd, nil
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
// recorded (design §4.8.3 item 4).
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
	if record.Target == 0 {
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
