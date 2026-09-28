// Package commands reads the slash commands a multi-agent utterance carries on
// its last line (docs/multi-agent-chat-design.md §4.8): /roll and the
// 【ダイス】 lines that imitate its record, and the effect commands /add, /use
// and /set (§4.8.8). A chat enables the commands it uses (§4.8.7); what it has
// not enabled is text somebody wrote and is left as such.
//
// The package reads, rolls and works out what an effect does to a state sheet
// it is handed. Where the result goes — the message's columns, the sheet's
// row, the name match of §4.6.5 — is internal/service's business.
package commands

import (
	"strings"

	"snzstudio/internal/model"
)

// Result is what the command pass makes of an utterance body (design §4.8.3
// item 2, steps (2) and the 【ダイス】 handling between (2) and (3)).
type Result struct {
	// Content is the body to store.
	Content string
	// CallText is what the name match of §4.6.5 (b) reads: the body without
	// any command line, readable or not, so an action's names are no call.
	CallText string
	// Roll is the /roll to throw, nil when there is none that could be read.
	Roll *Roll
	// RollErr is set when the last line held a /roll that could not be read.
	// The line then stays in Content; whether that is acceptable is the
	// caller's call.
	RollErr error
	// Effect is the effect command to apply, nil when there is none that could
	// be read.
	Effect *Effect
	// EffectErr is set, as RollErr is, when the last line held an effect
	// command that could not be read, an owner not on the roster included.
	EffectErr error
	// RemovedDice is each 【ダイス】 line taken out of the body because the app
	// did not write it (TASK-63), for the log.
	RemovedDice []string
}

// Extract runs the enabled commands over an utterance body whose addressee
// directive has already been removed. The last line is searched for a command,
// and only for the enabled ones (§4.8.7); the first one on it is the
// utterance's one command, the rest of the line its arguments. Only when the
// last line has none is the rest of the body searched, and then only for an
// effect command that can be read (§4.8.8): a game master writes the damage
// where it narrates it and goes on narrating. roster is who an effect command
// can name as the owner.
//
// A line opening with 【ダイス】 is /roll's notation, so it is handled only
// where /roll is enabled (TASK-63): a result-free one ending the text is rolled
// in place of a missing command, and every other one shaped like a record — a
// forged result above all — is removed.
func Extract(body string, enabled model.ChatCommands, roster []model.Participant) Result {
	keyword, at := lastLineCommand(body, enabled)
	if keyword != "" && keyword != rollKeyword {
		return extractEffect(body, keyword, at, len(strings.TrimRight(body, " \t\r\n")), enabled, roster)
	}
	if keyword == "" && !endsInDiceMarkerCommand(body, enabled) {
		if keyword, at, end := effectAbove(body, enabled, roster); keyword != "" {
			return extractEffect(body, keyword, at, end, enabled, roster)
		}
	}
	if enabled.Roll == nil {
		return Result{Content: body, CallText: body}
	}
	withoutCommand, found, roll, rollErr := splitRollCommand(body)
	if !found {
		withoutCommand, found, roll = splitDiceMarkerCommand(body)
	}
	withoutCommand, removed := stripDiceMarkers(withoutCommand)
	result := Result{
		Content:     withoutCommand,
		CallText:    withoutTrailingDiceMarkerLine(withoutCommand),
		Roll:        roll,
		RollErr:     rollErr,
		RemovedDice: removed,
	}
	if found && rollErr != nil {
		result.Content, _ = stripDiceMarkers(body)
	}
	return result
}

// extractEffect takes the effect command from `at` to `end`, the end of its
// line, out of the body. An unreadable one stays in Content, as an unreadable
// /roll does, and is left out of CallText either way so the names in it are no
// call. Where /roll is enabled, the forged 【ダイス】 lines around the command
// are still removed.
func extractEffect(body, keyword string, at, end int, enabled model.ChatCommands, roster []model.Participant) Result {
	withoutCommand := strings.TrimRight(body[:at], " \t\r\n　")
	after := strings.TrimLeft(strings.TrimRight(body[end:], " \t\r\n"), " \t\r　")
	switch {
	case withoutCommand == "":
		withoutCommand = strings.TrimLeft(after, "\r\n")
	case after != "":
		withoutCommand += after
	}
	effect, err := parseEffect(body[at:end], keyword, roster)
	result := Result{Content: withoutCommand, CallText: withoutCommand, Effect: effect, EffectErr: err}
	if err != nil {
		result.Content = body
	}
	if enabled.Roll != nil {
		result.Content, result.RemovedDice = stripDiceMarkers(result.Content)
		result.CallText, _ = stripDiceMarkers(result.CallText)
		result.CallText = withoutTrailingDiceMarkerLine(result.CallText)
	}
	return result
}

// lastLineCommand finds the first enabled command keyword on the last line of
// content, and its index in content; "" and -1 when there is none.
func lastLineCommand(content string, enabled model.ChatCommands) (string, int) {
	trimmed := strings.TrimRight(content, " \t\r\n")
	lineStart := strings.LastIndex(trimmed, "\n") + 1
	keyword, at := "", -1
	for _, candidate := range enabledKeywords(enabled) {
		if i := keywordIndex(trimmed[lineStart:], candidate); i >= 0 && (at < 0 || i < at) {
			keyword, at = candidate, i
		}
	}
	if at < 0 {
		return "", -1
	}
	return keyword, lineStart + at
}

// effectAbove finds the first effect command above the last line that can be
// read, found as on the last line (a word of its own, running to the end of
// its line), and returns its keyword, where it starts and where its line ends;
// "" when there is none. One that cannot be read is prose here, not a refused
// command: text that explains a command ("/add は …のように書く") sits in a body
// as naturally as a command does, and only the last line is where a command is
// asked to be.
func effectAbove(body string, enabled model.ChatCommands, roster []model.Participant) (keyword string, at, end int) {
	trimmed := strings.TrimRight(body, " \t\r\n")
	lastLine := strings.LastIndex(trimmed, "\n") + 1
	for start := 0; start < lastLine; {
		lineEnd := start + strings.Index(trimmed[start:], "\n")
		line := strings.TrimRight(trimmed[start:lineEnd], " \t\r")
		// The leftmost command on the line is the one read, as on the last
		// line; another one right of it is part of its arguments.
		keyword, at := "", -1
		for _, candidate := range enabledKeywords(enabled) {
			if i := keywordIndex(line, candidate); candidate != rollKeyword && i >= 0 && (at < 0 || i < at) {
				keyword, at = candidate, i
			}
		}
		if at >= 0 {
			if _, err := parseEffect(line[at:], keyword, roster); err == nil {
				return keyword, start + at, start + len(line)
			}
		}
		start = lineEnd + 1
	}
	return "", -1, -1
}

// endsInDiceMarkerCommand reports a result-free 【ダイス】 line ending the body
// where /roll is enabled: it is the utterance's command (TASK-63), so no effect
// command above it is read.
func endsInDiceMarkerCommand(body string, enabled model.ChatCommands) bool {
	if enabled.Roll == nil {
		return false
	}
	_, found, _ := splitDiceMarkerCommand(body)
	return found
}

func enabledKeywords(enabled model.ChatCommands) []string {
	keywords := make([]string, 0, 4)
	for _, c := range []struct {
		on      bool
		keyword string
	}{
		{enabled.Roll != nil, rollKeyword},
		{enabled.Add != nil, addKeyword},
		{enabled.Use != nil, useKeyword},
		{enabled.Set != nil, setKeyword},
	} {
		if c.on {
			keywords = append(keywords, c.keyword)
		}
	}
	return keywords
}
