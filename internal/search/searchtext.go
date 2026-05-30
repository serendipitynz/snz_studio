package search

import (
	"regexp"
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

// TokenizeSearchTerms returns up to limit unique tokens from input.
// Mirrors tokenizeSearchTerms(input, limit=12).
func TokenizeSearchTerms(input string, limit int) []string {
	if limit <= 0 {
		limit = 12
	}
	terms := make([]string, 0, limit)
	seen := make(map[string]bool)

	for _, candidate := range extractTokenCandidates(input) {
		token, ok := cleanToken(candidate)
		if !ok || seen[token] {
			continue
		}
		seen[token] = true
		terms = append(terms, token)
		if len(terms) >= limit {
			break
		}
	}
	return terms
}

func escapeFtsToken(token string) string {
	return `"` + strings.ReplaceAll(token, `"`, `""`) + `"`
}

// ToFtsQuery converts input into an FTS5 MATCH query. Mirrors toFtsQuery(input).
func ToFtsQuery(input string) string {
	tokens := TokenizeSearchTerms(input, 12)
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		parts = append(parts, escapeFtsToken(token)+"*")
	}
	return strings.Join(parts, " OR ")
}
