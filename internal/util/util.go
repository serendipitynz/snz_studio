// Package util ports the small shared helpers in backend/src/lib/utils.ts that
// the repository and service layers rely on: ISO-8601 timestamps, prefixed
// identifiers, and comma-separated tag parsing.
package util

import (
	"strings"
	"time"

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
