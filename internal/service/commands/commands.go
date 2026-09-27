// Package commands reads the slash commands a multi-agent utterance carries on
// its last line (docs/multi-agent-chat-design.md §4.8): /roll, and the
// 【ダイス】 lines that imitate its record. A chat enables the commands it uses
// (§4.8.7); what it has not enabled is text somebody wrote and is left as such.
//
// The package only reads and rolls. Where the result goes — the message's
// columns, the name match of §4.6.5 — is internal/service's business.
package commands

import "snzstudio/internal/model"

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
	// RemovedDice is each 【ダイス】 line taken out of the body because the app
	// did not write it (TASK-63), for the log.
	RemovedDice []string
}

// Extract runs the enabled commands over an utterance body whose addressee
// directive has already been removed. Only the last line is searched for a
// command, and only for the enabled ones (§4.8.7): an effect command (TASK-36)
// joins as another keyword searched there, still one command per utterance.
//
// A line opening with 【ダイス】 is /roll's notation, so it is handled only
// where /roll is enabled (TASK-63): a result-free one ending the text is rolled
// in place of a missing /roll, and every other one shaped like a record — a
// forged result above all — is removed.
func Extract(body string, enabled model.ChatCommands) Result {
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
