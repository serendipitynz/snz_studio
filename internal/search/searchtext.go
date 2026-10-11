package search

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Port of backend/src/lib/searchText.ts. The application pre-tokenizes text in
// this layer before inserting into / querying the FTS5 tables, so the Go output
// must match the JS output exactly (verified by searchtext_test.go).

// JS: /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}ー]+|[\p{L}\p{N}]+(?:[_-][\p{L}\p{N}]+)*/gu
var tokenPattern = regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}ー]+|[\p{L}\p{N}]+(?:[_-][\p{L}\p{N}]+)*`)

// JS: /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]/u
var japaneseTokenPattern = regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}]`)

// JS: /^[ぁ-ゖァ-ヺー]$/u
var singleKanaPattern = regexp.MustCompile(`^[ぁ-ゖァ-ヺー]$`)

// JS: .replace(/\s+/g, " ")
var whitespacePattern = regexp.MustCompile(`\s+`)

var englishStopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "for": true,
	"from": true, "how": true, "into": true, "our": true, "that": true,
	"the": true, "this": true, "use": true, "with": true, "your": true,
}

var japaneseStopwords = map[string]bool{
	"これ": true, "それ": true, "あれ": true, "こと": true, "もの": true,
	"ため": true, "よう": true, "です": true, "ます": true, "した": true,
	"して": true, "する": true, "ある": true, "いる": true, "この": true,
	"その": true, "ですか": true, "ますか": true, "ください": true, "お願いします": true,
}

func extractTokenCandidates(input string) []string {
	normalized := norm.NFKC.String(input)
	normalized = whitespacePattern.ReplaceAllString(normalized, " ")
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return nil
	}

	var tokens []string
	for _, segment := range Segment(normalized) {
		matches := tokenPattern.FindAllString(segment, -1)
		tokens = append(tokens, matches...)
	}
	return tokens
}

// cleanToken returns the cleaned token and whether it should be kept.
func cleanToken(token string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(norm.NFKC.String(token)))
	if normalized == "" {
		return "", false
	}

	if japaneseTokenPattern.MatchString(normalized) {
		if singleKanaPattern.MatchString(normalized) || japaneseStopwords[normalized] {
			return "", false
		}
		return normalized, true
	}

	if utf8.RuneCountInString(normalized) < 2 || englishStopwords[normalized] {
		return "", false
	}
	return normalized, true
}

// BuildSearchText tokenizes the given parts into a space-joined string that is
// stored in the FTS5 tables. Mirrors buildSearchText(...parts).
func BuildSearchText(parts ...string) string {
	var tokens []string
	for _, part := range parts {
		if part == "" {
			continue
		}
		for _, candidate := range extractTokenCandidates(part) {
			if token, ok := cleanToken(candidate); ok {
				tokens = append(tokens, token)
			}
		}
	}
	return strings.Join(tokens, " ")
}

// QueryTerms returns the unique tokens of input in order of first appearance,
// cleaned as BuildSearchText cleans them, minus the fragments of stopwords the
// segmenter split apart (see trimSplitStopwords). The index side keeps such
// fragments (BuildSearchText is unchanged), which is harmless because no query
// asks for them.
func QueryTerms(input string) []string {
	terms := []string{}
	seen := make(map[string]bool)
	for _, candidate := range trimSplitStopwords(extractTokenCandidates(input)) {
		token, ok := cleanToken(candidate)
		if !ok || seen[token] {
			continue
		}
		seen[token] = true
		terms = append(terms, token)
	}
	return terms
}

// splitStopwords are the stopwords long enough for TinySegmenter to cut them
// across tokens: 「ください」 comes out as 「く / ださい」 or 「てく / ださい」, and
// the stopword check, which sees one token at a time, keeps 「ださい」. The
// two-character stopwords are left out because, matched across a boundary, they
// cut real words (「うそ / の」 holds 「その」).
var splitStopwords = []string{"ですか", "ますか", "ください", "お願いします"}

// trimSplitStopwords removes, from each candidate, the part covered by a
// splitStopwords occurrence that crosses a candidate boundary. A candidate
// wholly inside one disappears; one partly inside keeps its remainder, which is
// then cleaned as usual (「てく」 becomes 「て」, a single kana, and is dropped).
func trimSplitStopwords(candidates []string) []string {
	joined := strings.Join(candidates, "")
	boundaries := make([]int, 0, len(candidates)+1)
	offset := 0
	for _, candidate := range candidates {
		boundaries = append(boundaries, offset)
		offset += len(candidate)
	}
	boundaries = append(boundaries, offset)

	covered := make([]bool, len(joined))
	for _, stopword := range splitStopwords {
		for from := 0; ; {
			at := strings.Index(joined[from:], stopword)
			if at < 0 {
				break
			}
			start, end := from+at, from+at+len(stopword)
			if crossesBoundary(boundaries, start, end) {
				for i := start; i < end; i++ {
					covered[i] = true
				}
			}
			from = end
		}
	}

	out := make([]string, 0, len(candidates))
	for i := range candidates {
		var rest strings.Builder
		for j := boundaries[i]; j < boundaries[i+1]; j++ {
			if !covered[j] {
				rest.WriteByte(joined[j])
			}
		}
		if rest.Len() > 0 {
			out = append(out, rest.String())
		}
	}
	return out
}

func crossesBoundary(boundaries []int, start, end int) bool {
	for _, b := range boundaries {
		if start < b && b < end {
			return true
		}
	}
	return false
}

// Term selection limits. maxQueryTerms caps the terms that can match: with 12
// terms OR-ed and prefix-matched, a 212-chunk corpus matched 180 chunks, which is
// why the terms are chosen by rarity rather than position. On that corpus
// (2026-10-11) a short request rarely reaches the cap — three of four had 5 to 7
// terms left — and a long one (a pasted passage, a multi-agent turn) gets its
// rarest terms, with the embedding candidates covering the rest of its meaning.
//
// A term counts as too common once it is in more than commonTermRatio of the
// rows: FTS5's bm25 gives such a term an IDF of log((N-n+0.5)/(n+0.5)) ≤ 0,
// clamped to 1e-6, so it pulls rows in without ranking them. Below
// commonTermFloor rows the table is small enough that pulling most of it in
// costs nothing, so no term is dropped for being common there.
const (
	maxQueryTerms   = 12
	commonTermRatio = 0.5
	commonTermFloor = 10
)

// SelectTerms picks the FTS terms of a query by document frequency — the number
// of rows a term (as a prefix) occurs in, from docFreq — instead of by position,
// so the same words in any order give the same query. primary terms take the
// slots first, rarest first; background terms only fill the slots primary leaves
// open (a multi-agent turn's scene, which would otherwise pin the same rare names
// into every turn's query).
//
// A term with frequency 0 cannot pull a row in, so it is kept outside the limit
// rather than dropped: the frequency comes from the index vocabulary, which the
// FTS tokenizer may have folded differently (diacritics, a hyphenated token split
// into a phrase), and dropping it would lose a match the old query had. A term
// more common than the cut-off is dropped, unless every matching term is, in
// which case the cut-off is lifted rather than leaving the query empty.
func SelectTerms(primary, background []string, docFreq func(string) int, total int) []string {
	type scoredTerm struct {
		term string
		freq int
		tier int
	}
	seen := make(map[string]bool)
	var absent, present []scoredTerm
	for tier, terms := range [][]string{primary, background} {
		for _, term := range terms {
			if seen[term] {
				continue
			}
			seen[term] = true
			freq := docFreq(term)
			if freq == 0 {
				absent = append(absent, scoredTerm{term, 0, tier})
			} else {
				present = append(present, scoredTerm{term, freq, tier})
			}
		}
	}

	cutoff := commonTermRatio * float64(total)
	if cutoff < commonTermFloor {
		cutoff = commonTermFloor
	}
	var kept []scoredTerm
	for _, t := range present {
		if float64(t.freq) <= cutoff {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		kept = present
	}

	byRarity := func(terms []scoredTerm) {
		sort.Slice(terms, func(i, j int) bool {
			if terms[i].tier != terms[j].tier {
				return terms[i].tier < terms[j].tier
			}
			if terms[i].freq != terms[j].freq {
				return terms[i].freq < terms[j].freq
			}
			return terms[i].term < terms[j].term
		})
	}
	byRarity(absent)
	byRarity(kept)
	if len(kept) > maxQueryTerms {
		kept = kept[:maxQueryTerms]
	}

	selected := make([]string, 0, len(absent)+len(kept))
	for _, t := range absent {
		selected = append(selected, t.term)
	}
	for _, t := range kept {
		selected = append(selected, t.term)
	}
	return selected
}

func escapeFtsToken(token string) string {
	return `"` + strings.ReplaceAll(token, `"`, `""`) + `"`
}

// FtsQuery turns terms into an FTS5 MATCH query: each term prefix-matched, OR-ed.
func FtsQuery(terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	parts := make([]string, 0, len(terms))
	for _, token := range terms {
		parts = append(parts, escapeFtsToken(token)+"*")
	}
	return strings.Join(parts, " OR ")
}
