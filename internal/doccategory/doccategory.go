// Package doccategory ports backend/src/lib/documentCategory.ts: a lightweight,
// rule-based classifier that guesses a document's category from its filename,
// title, note and body when one is not supplied explicitly.
package doccategory

import (
	"regexp"
	"strings"
)

// validCategories mirrors the CATEGORIES list.
var validCategories = map[string]bool{
	"world": true, "character": true, "rule": true, "plot": true,
	"timeline": true, "index": true, "story": true, "misc": true,
}

// IsValid reports whether value is one of the known categories. Mirrors
// isDocumentCategory.
func IsValid(value string) bool {
	return validCategories[value]
}

// ws is the whitespace fragment used in place of the TS regexes' `\s`. Go's RE2
// `\s` is ASCII-only, whereas JavaScript's `\s` (with the /u flag) also matches
// Unicode separators such as the full-width space U+3000 that appear in Japanese
// headings. Combining `\s` with `\p{Z}` restores that behaviour.
const ws = `[\s\p{Z}]`

type hint struct {
	category string
	patterns []*regexp.Regexp
}

func compileAll(patterns ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		out[i] = regexp.MustCompile(p)
	}
	return out
}

// hints mirrors CATEGORY_HINTS, preserving each category's pattern set.
var hints = []hint{
	{"story", compileAll(
		`(?i)note[_-]?chapter`,
		`(?i)chapter`+ws+`*\d+`,
		`第`+ws+`*\d+`+ws+`*(?:話|章)`,
		`(?m)^#`+ws+`*第`+ws+`*\d+`+ws+`*(?:話|章)`,
		`\*\*第[一二三四五六七八九十]+部`,
	)},
	{"plot", compileAll(`プロット`, `物語構想`, `各章内容設計`, `物語的機能`, `章末の状態`, `起きること`)},
	{"timeline", compileAll(`時系列`, `\bD\+?\d+\b`, `全体の時間幅`, `この時点での情報保有`, `前後関係`)},
	{"index", compileAll(`索引`, `インデックス`, `正史確定`, `暫定確定`, `保留`, `用語索引`, `地理索引`)},
	{"character", compileAll(`主要人物`, `人物メモ`, `キャラクター`, `主人公候補`, `年齢`, `性格`, `口調`, `家族構成`)},
	{"rule", compileAll(`基礎ルール`, `本文運用メモ`, `運用メモ`, `恒常原則`, `文体制限`, `方針`, `優先`, `不要`)},
	{"world", compileAll(`世界設定`, `世界観`, `創世神話`, `神々`, `国家`, `魔法体系`, `生活世界設定`, `種族`)},
}

// categoryOrder mirrors CATEGORY_ORDER: it defines both the iteration order for
// initialising scores and the tie-break order (earlier wins) when selecting the
// best category.
var categoryOrder = []string{"story", "plot", "timeline", "index", "character", "rule", "world", "misc"}

var (
	storyHeadingPattern = regexp.MustCompile(`(?m)^#` + ws + `*第` + ws + `*\d+` + ws + `*(?:話|章)`)
	plotStrongPattern   = regexp.MustCompile(`\*\*視点\*\*|字数目安|物語的機能|章末の状態`)
)

// Input carries the candidate text fields used for inference. Empty fields are
// ignored, mirroring the TS filter(Boolean).
type Input struct {
	FileName    string
	Title       string
	Note        string
	ContentText string
	DerivedText string
}

// Infer guesses a document category from the input fields, returning "misc" when
// nothing matches. Mirrors inferDocumentCategory.
func Infer(in Input) string {
	parts := make([]string, 0, 5)
	for _, s := range []string{in.FileName, in.Title, in.Note, in.ContentText, in.DerivedText} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	sample := sliceRunes(strings.Join(parts, "\n"), 5000)
	if strings.TrimSpace(sample) == "" {
		return "misc"
	}

	scores := make(map[string]int, len(categoryOrder))
	for _, c := range categoryOrder {
		scores[c] = 0
	}

	for _, h := range hints {
		for _, p := range h.patterns {
			if p.MatchString(sample) {
				scores[h.category]++
			}
		}
	}

	if storyHeadingPattern.MatchString(sample) {
		scores["story"] += 3
	}
	if plotStrongPattern.MatchString(sample) {
		scores["plot"] += 3
	}

	best := "misc"
	bestScore := 0
	for _, c := range categoryOrder {
		if scores[c] > bestScore {
			best = c
			bestScore = scores[c]
		}
	}
	if bestScore > 0 {
		return best
	}
	return "misc"
}

// sliceRunes returns the first max runes of s, mirroring the TS `.slice(0, max)`
// (rune-based; see the note in package chunk on UTF-16 vs rune parity).
func sliceRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
