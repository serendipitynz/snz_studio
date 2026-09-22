package service

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"snzstudio/internal/model"
)

// addresseeDirective is the trailing line a participant is asked to write under
// the weighted rule when it wants someone in particular to speak next (design
// §4.6.5 (a)): "[次: 表示名]", several names separated by 読点. Full-width
// brackets and colon are accepted because a Japanese-writing model switches
// between the two forms freely.
var addresseeDirective = regexp.MustCompile(`^[\[［]\s*次\s*[:：]\s*(.*?)\s*[\]］]$`)

// addresseeDirectiveSeparators splits the names inside a directive. The reminder
// asks for 読点, but a model that writes a comma instead still means a list.
var addresseeDirectiveSeparators = regexp.MustCompile(`[、,，]`)

// sentenceBoundary ends a sentence for the name match of §4.6.5 (b).
var sentenceBoundary = regexp.MustCompile(`[。．.！!？?\n]`)

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

// splitAddresseeDirective removes the directive when it is the last line, and
// only then: a directive-shaped line anywhere else is content the model wrote,
// and stripping it would be the one edit this function can get wrong.
func splitAddresseeDirective(content string) (string, []string, bool) {
	trimmed := strings.TrimRight(content, " \t\r\n")
	lastLine := trimmed
	body := ""
	if i := strings.LastIndex(trimmed, "\n"); i >= 0 {
		lastLine, body = trimmed[i+1:], trimmed[:i]
	}
	match := addresseeDirective.FindStringSubmatch(strings.TrimSpace(lastLine))
	if match == nil {
		return content, nil, false
	}
	return strings.TrimRight(body, " \t\r\n"), addresseeDirectiveSeparators.Split(match[1], -1), true
}

func resolveDirectiveNames(names []string, speakerID string, roster []model.Participant) []string {
	addressees := []string{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		for _, p := range roster {
			if p.ID != speakerID && namesOf(p, 1)[strings.ToLower(name)] {
				addressees = appendUnique(addressees, p.ID)
				break
			}
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
	for _, p := range roster {
		if p.ID == speakerID {
			continue
		}
		for name := range namesOf(p, minMatchedNameLength) {
			if strings.Contains(sentence, name) {
				addressees = appendUnique(addressees, p.ID)
				break
			}
		}
	}
	return addressees
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
