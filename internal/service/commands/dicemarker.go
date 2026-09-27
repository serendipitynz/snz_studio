package commands

import (
	"regexp"
	"strings"
)

// DiceMarker opens the line the prompt adds for each roll (design §4.8.3 item
// 2). It is the app's notation: a speaker that writes one itself is either
// forging a result or imitating the command, and the app cannot tell its own
// record from such a line once both sit in the transcript.
const DiceMarker = "【ダイス】"

// diceMarkerArrow is the mapping's "→" and the total's "=", which only a
// written result carries.
var diceMarkerArrow = regexp.MustCompile(`→|->|⇒|=|＝`)

// diceMarkerOutcome is an outcome word. gpt-oss-20b forged "【ダイス】15 成功"
// with no arrow or equals sign (TASK-63), but the word is also ordinary in an
// action ("交渉を成功させる") and in a heading, so diceMarkerHasResult reads it
// as a result only beside a number that is not a roll.
var diceMarkerOutcome = regexp.MustCompile(`成功|失敗`)

// diceLikeToken is dice written as a field, readable or not ("1d20", "d20").
var diceLikeToken = regexp.MustCompile(`(?:^|\s)[0-9０-９]*[dDｄＤ][0-9０-９]+`)

// diceMarkerHasResult reports marker arguments that carry a written result: an
// arrow or an equals sign, or an outcome word with a number when the arguments
// do not read as a /roll. A forged line is never rolled; a mistyped roll whose
// action happens to say 成功 still is.
func diceMarkerHasResult(args string) bool {
	if diceMarkerArrow.MatchString(args) {
		return true
	}
	return diceMarkerOutcome.MatchString(args) &&
		strings.ContainsAny(asciiDigits(args), "0123456789") &&
		readDiceMarkerRoll(args) == nil
}

// diceMarkerArgs returns what follows a 【ダイス】 that opens the line. Only the
// start of a line counts: that is where the app writes it, and where every one
// the TASK-63 measurement saw stood (95 of 95). One inside a sentence is
// somebody writing about dice, which a general conversation may well do.
func diceMarkerArgs(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " \t　")
	if !strings.HasPrefix(trimmed, DiceMarker) {
		return "", false
	}
	return strings.TrimSpace(trimmed[len(DiceMarker):]), true
}

// readDiceMarkerRoll reads a marker line's arguments as the /roll they stand
// for. Across a "—" the dice may stand on either side: the mapping's own order
// puts the action first.
func readDiceMarkerRoll(args string) *Roll {
	candidates := []string{args}
	if before, after, ok := strings.Cut(args, "—"); ok {
		before, after = strings.TrimSpace(before), strings.TrimSpace(after)
		candidates = []string{after + " " + before, before + " " + after}
	}
	for _, candidate := range candidates {
		if cmd, err := parseRollCommand(rollKeyword + " " + candidate); err == nil {
			return cmd
		}
	}
	return nil
}

// isDiceRecordLine reports a line shaped like the app's record or a copy of
// it: a 【ダイス】 opening the line with nothing after it, a result, or dice
// that read as a /roll. Anything else after the marker — a heading such as
// "【ダイス】の確率について" — is text somebody wrote and stays (owner's ruling,
// TASK-63): it carries no result a later speaker could take for a roll.
func isDiceRecordLine(line string) bool {
	args, ok := diceMarkerArgs(line)
	if !ok {
		return false
	}
	return args == "" || diceMarkerHasResult(args) || readDiceMarkerRoll(args) != nil
}

// splitDiceMarkerCommand reads a result-free 【ダイス】 line ending the text as
// a /roll the speaker meant to write (TASK-63): a player that copied the
// mapping's shape declared a roll as much as one that wrote the command. It
// returns what splitRollCommand does; found is false when the last line is no
// such line, and then stripDiceMarkers decides whether it stays.
func splitDiceMarkerCommand(content string) (stripped string, found bool, cmd *Roll) {
	trimmed := strings.TrimRight(content, " \t\r\n")
	lineStart := strings.LastIndex(trimmed, "\n") + 1
	args, ok := diceMarkerArgs(trimmed[lineStart:])
	if !ok || diceMarkerHasResult(args) {
		return content, false, nil
	}
	if cmd = readDiceMarkerRoll(args); cmd == nil {
		return content, false, nil
	}
	cmd.line = strings.TrimSpace(trimmed[lineStart:])
	return strings.TrimRight(trimmed[:lineStart], " \t\r\n　"), true, cmd
}

// stripDiceMarkers removes the lines isDiceRecordLine reports and returns them
// for the log. Only the app writes such a line (TASK-63): left in the body, a
// forged "【ダイス】1d20+3 → 14+3 = 17（目標 12、成功）" would reach every later
// speaker as a result the app never rolled.
func stripDiceMarkers(content string) (string, []string) {
	if !strings.Contains(content, DiceMarker) {
		return content, nil
	}
	var removed []string
	lines := strings.Split(content, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if isDiceRecordLine(line) {
			removed = append(removed, strings.TrimSpace(line))
			continue
		}
		kept = append(kept, line)
	}
	if removed == nil {
		return content, nil
	}
	return strings.Trim(strings.Join(kept, "\n"), "\r\n"), removed
}

// withoutTrailingDiceMarkerLine drops a last line that opens with 【ダイス】 and
// writes dice that could not be read. The name match skips it for the reason it
// skips an unreadable /roll: "【ダイス】d20 ミラを庇う" is a botched roll, not a
// call on ミラ. A heading kept as text ("【ダイス】の確率、ミラはどう思う？") has
// no dice and keeps its call.
func withoutTrailingDiceMarkerLine(content string) string {
	trimmed := strings.TrimRight(content, " \t\r\n")
	lineStart := strings.LastIndex(trimmed, "\n") + 1
	if args, ok := diceMarkerArgs(trimmed[lineStart:]); !ok || !diceLikeToken.MatchString(args) {
		return content
	}
	return trimmed[:lineStart]
}
