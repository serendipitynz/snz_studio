package service

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"snzstudio/internal/model"
)

// addresseeDirective is what a participant is asked to write on the last line
// under the weighted rule when it wants someone in particular to speak next
// (design §4.6.5 (a)): "[次: 表示名]", several names separated by 読点. It is
// matched at the very end of the utterance, so one a model appends to its last
// sentence without a line break is stripped too. Full-width brackets and colon
// are accepted because a Japanese-writing model switches between the two forms
// freely.
var addresseeDirective = regexp.MustCompile(`[\[［]\s*次\s*[:：]\s*([^\]］\n]*?)\s*[\]］]$`)

// addresseeDirectiveSeparators splits the names inside a directive. The reminder
// asks for 読点, but a model that writes a comma instead still means a list.
var addresseeDirectiveSeparators = regexp.MustCompile(`[、,，]`)

// sentenceBoundary ends a sentence for the name match of §4.6.5 (b). An ASCII
// period counts only before whitespace or the end, so a decimal ("3.5") does not
// cut the sentence and hide a name written before it.
var sentenceBoundary = regexp.MustCompile(`[。．！!？?\n]|\.(?:\s|$)`)

// nameAnnotation is a parenthesised note at the end of a display name, the way
// the bundled presets label a role: "レン (斥候)". A speaker calling on that
// participant writes "レン", never the note, so the name before it matches too.
var nameAnnotation = regexp.MustCompile(`\s*[(（][^()（）]*[)）]\s*$`)

// minMatchedNameLength keeps the name match off very short display names, which a
// substring match would otherwise find inside ordinary words ("リン" in "リンゴ").
// The match has to be a substring one: Japanese has no word boundaries to match
// on, which is why SillyTavern's \b\w+\b never finds a Japanese name (§4.6.5).
const minMatchedNameLength = 2

// detectAddressees fixes, at store time, whom a message calls on, and returns the
// content with the directive line removed (design §4.6.5). speakerID is the
// participant that wrote the message, empty for the human; a call on oneself is
// dropped.
//
// A directive on the last line decides the call on its own: its names that are
// on the roster are the addressees, the others are dropped one by one, and the
// line is removed even when none of them match, so no control syntax is left in
// the transcript. Only without a directive does the name match run, over the last
// sentence alone. Every roster name found there counts, the whole roster included
// — a call on everyone still lets the ones who have not answered keep their
// boost after the others have.
func detectAddressees(content, speakerID string, roster []model.Participant) (string, []string) {
	body, names, found := splitAddresseeDirective(content)
	if found {
		return body, resolveDirectiveNames(names, speakerID, roster)
	}
	return content, matchNamesInLastSentence(content, speakerID, roster)
}

// DetectHumanAddressees is detectAddressees for the human's intervention: the
// name match only, since the human is never asked for a directive, and the body
// is stored as typed (§4.6.5). roster is the chat's current roster.
func DetectHumanAddressees(content string, roster []model.Participant) []string {
	return matchNamesInLastSentence(content, "", roster)
}

// splitAddresseeDirective removes the directive when it ends the utterance, and
// only then: a directive-shaped text anywhere else is content the model wrote,
// and stripping it would be the one edit this function can get wrong.
func splitAddresseeDirective(content string) (string, []string, bool) {
	trimmed := strings.TrimRight(content, " \t\r\n")
	loc := addresseeDirective.FindStringSubmatchIndex(trimmed)
	if loc == nil {
		return content, nil, false
	}
	names := addresseeDirectiveSeparators.Split(trimmed[loc[2]:loc[3]], -1)
	return strings.TrimRight(trimmed[:loc[0]], " \t\r\n"), names, true
}

func resolveDirectiveNames(names []string, speakerID string, roster []model.Participant) []string {
	callable := callableNames(roster, 1)
	addressees := []string{}
	for _, name := range names {
		if id, ok := callable[strings.ToLower(strings.TrimSpace(name))]; ok && id != speakerID {
			addressees = appendUnique(addressees, id)
		}
	}
	return addressees
}

func matchNamesInLastSentence(content, speakerID string, roster []model.Participant) []string {
	sentence := strings.ToLower(lastSentence(content))
	addressees := []string{}
	if sentence == "" {
		return addressees
	}
	callable := callableNames(roster, minMatchedNameLength)
	for _, p := range roster {
		if p.ID == speakerID {
			continue
		}
		for name := range namesOf(p, minMatchedNameLength) {
			if callable[name] == p.ID && strings.Contains(sentence, name) {
				addressees = appendUnique(addressees, p.ID)
				break
			}
		}
	}
	return addressees
}

// callableNames maps each name form to the one participant it calls. A form
// more than one participant has — "買い手" for both "買い手 (情シス担当)" and
// "買い手 (部長)" in the bundled negotiation preset — is left out: matching it
// would call on both, or on whichever came first, rather than on the one meant.
// Such participants stay callable by their full display names.
func callableNames(roster []model.Participant, minLength int) map[string]string {
	owners := map[string][]string{}
	for _, p := range roster {
		for name := range namesOf(p, minLength) {
			owners[name] = append(owners[name], p.ID)
		}
	}
	callable := make(map[string]string, len(owners))
	for name, ids := range owners {
		if len(ids) == 1 {
			callable[name] = ids[0]
		}
	}
	return callable
}

// namesOf is the lower-cased forms a participant can be called by: the display
// name, and the name before a trailing parenthesised note. A form shorter than
// minLength characters is left out.
func namesOf(p model.Participant, minLength int) map[string]bool {
	names := map[string]bool{}
	full := strings.ToLower(strings.TrimSpace(p.DisplayName))
	for _, name := range []string{full, strings.TrimSpace(nameAnnotation.ReplaceAllString(full, ""))} {
		if utf8.RuneCountInString(name) >= minLength {
			names[name] = true
		}
	}
	return names
}

// lastSentence is the text after the last sentence boundary, once the trailing
// boundaries themselves are cut: "…でした。A さん、どう思う？" ends in "A さん、どう思う".
func lastSentence(content string) string {
	text := strings.TrimSpace(content)
	for text != "" {
		r, size := utf8.DecodeLastRuneInString(text)
		if !sentenceBoundary.MatchString(string(r)) && !strings.ContainsRune(" \t\r」』）)\"'", r) {
			break
		}
		text = text[:len(text)-size]
	}
	if loc := sentenceBoundary.FindAllStringIndex(text, -1); len(loc) > 0 {
		text = text[loc[len(loc)-1][1]:]
	}
	return strings.TrimSpace(text)
}

func appendUnique(ids []string, id string) []string {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}
