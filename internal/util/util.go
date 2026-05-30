// Package util ports the small shared helpers in backend/src/lib/utils.ts that
// the repository and service layers rely on: ISO-8601 timestamps, prefixed
// identifiers, and comma-separated tag parsing.
package util

import (
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// NowISO returns the current time as an ISO-8601 / RFC-3339 string in UTC with
// millisecond precision and a trailing "Z", matching JavaScript's
// `new Date().toISOString()`.
func NowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

// NewID returns a prefixed identifier, mirroring createId(prefix) =
// `${prefix}_${crypto.randomUUID()}`. uuid.NewString returns a random (v4) UUID,
// the same flavour crypto.randomUUID produces.
func NewID(prefix string) string {
	return prefix + "_" + uuid.NewString()
}

// ParseTags splits a comma-separated tag string into trimmed, non-empty tags.
// Mirrors parseTags() for the string input form; the array form is handled by
// callers, which pass the slice through unchanged.
func ParseTags(raw string) []string {
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// Truncate clips text to maxLength, appending a single "…" (U+2026) when it had
// to cut. Mirrors truncate() from utils.ts:
//
//	if (text.length <= maxLength) return text;
//	return `${text.slice(0, Math.max(0, maxLength - 1)).trimEnd()}…`;
//
// Lengths are measured in runes rather than UTF-16 code units; the two agree for
// every BMP character (i.e. all Japanese text), differing only for astral
// characters — the same parity caveat noted across the rest of the port.
func Truncate(text string, maxLength int) string {
	r := []rune(text)
	if len(r) <= maxLength {
		return text
	}
	cut := maxLength - 1
	if cut < 0 {
		cut = 0
	}
	return strings.TrimRightFunc(string(r[:cut]), unicode.IsSpace) + "…"
}
