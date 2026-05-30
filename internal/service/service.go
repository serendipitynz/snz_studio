// Package service ports backend/src/services/*.ts to Go. It is the orchestration
// layer that sits above internal/repository: it talks to the local OpenAI-compatible
// LLM / embedding endpoints, performs hybrid retrieval, assembles chat context, and
// drives the streaming chat turn.
//
// Dependency direction: service -> {repository, model, util, search, vector, chunk,
// doccategory, config, llmresponse}. Nothing below service imports it.
//
// Concurrency note inherited from Phase 4: db.Open pins the pool to a single
// connection. RetrievalService issues raw read queries directly against *sql.DB
// (mirroring the TS service that held the better-sqlite3 handle); every such read
// is fully drained and its rows closed before the next query runs, so it never
// self-deadlocks against the single connection. The other services only ever call
// repository methods.
//
// Logging: the TS services emitted verbose debug traces gated behind
// config.debugChatFlow / config.debugRetrieval. Those traces are observability
// only (never behaviour), so they are omitted here; the few unconditional warnings
// (LLM completion failure, embedding self-disable) are kept via the standard log
// package.
package service

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
)

// reLineSplit splits an SSE chunk into lines on CRLF or LF, mirroring JS /\r?\n/.
var reLineSplit = regexp.MustCompile(`\r?\n`)

// stripTrailingSlash removes a single trailing slash, mirroring `.replace(/\/$/, "")`.
func stripTrailingSlash(s string) string {
	if strings.HasSuffix(s, "/") {
		return s[:len(s)-1]
	}
	return s
}

var reLmStudioV1Suffix = regexp.MustCompile(`(?i)(/api)?/v1$`)

// getLmStudioAPIRoot mirrors getLmStudioApiRoot: strip a trailing slash then a
// trailing "/v1" or "/api/v1" segment, leaving the LM Studio root.
func getLmStudioAPIRoot(baseURL string) string {
	return reLmStudioV1Suffix.ReplaceAllString(stripTrailingSlash(baseURL), "")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// round2 rounds to two decimal places, mirroring Number(x.toFixed(2)).
func round2(x float64) float64 {
	return math.Round(x*100) / 100
}

func float64Ptr(v float64) *float64 { return &v }
func int64Ptr(v int64) *int64       { return &v }

// sliceFromRune returns s with its first offset runes removed (or "" when offset
// exceeds s's length). It mirrors JS String.prototype.slice(offset) using rune
// counts in place of UTF-16 code units — equal for every BMP character.
func sliceFromRune(s string, offset int) string {
	if offset <= 0 {
		return s
	}
	r := []rune(s)
	if offset >= len(r) {
		return ""
	}
	return string(r[offset:])
}

// sortedStrings returns a lexicographically sorted copy, mirroring Array.sort()
// for the ASCII model identifiers these helpers deal with.
func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// joinLines joins lines with "\n". Mirrors Array.prototype.join("\n").
func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

// reWhitespaceRun matches a run of whitespace, widened to Unicode separators to
// match JS `\s` under the `u` flag (Go's `\s` is ASCII only).
var reWhitespaceRun = regexp.MustCompile(`[\s\p{Z}]+`)

// collapseWhitespace replaces every whitespace run with a single space and trims
// the ends, mirroring `s.replace(/\s+/g, " ").trim()`.
func collapseWhitespace(s string) string {
	return strings.TrimSpace(reWhitespaceRun.ReplaceAllString(s, " "))
}

// lastN returns the final n elements of items (or all of them when fewer),
// mirroring Array.prototype.slice(-n).
func lastN[T any](items []T, n int) []T {
	if n <= 0 {
		return nil
	}
	if len(items) <= n {
		return items
	}
	return items[len(items)-n:]
}

// reJSONFence matches a ```json ... ``` fenced block (case-insensitive, dotall).
var reJSONFence = regexp.MustCompile("(?is)```json\\s*(.*?)```")

// extractJSONObject pulls a JSON object out of an LLM response: a fenced ```json
// block if present, otherwise the span between the first "{" and last "}". Mirrors
// the extractJsonObject helper shared by memoryService and memoryOrganizerService.
func extractJSONObject(input string) string {
	if m := reJSONFence.FindStringSubmatch(input); m != nil {
		return strings.TrimSpace(m[1])
	}
	first := strings.Index(input, "{")
	last := strings.LastIndex(input, "}")
	if first >= 0 && last > first {
		return input[first : last+1]
	}
	return ""
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// marshalJSONIndentNoEscape JSON-encodes v with 2-space indentation and without
// HTML escaping, matching JSON.stringify(v, null, 2) which leaves <, >, & intact.
// Used to build LLM prompts that embed memory / summary payloads.
func marshalJSONIndentNoEscape(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	// Encoder.Encode appends a trailing newline; JSON.stringify does not.
	return strings.TrimRight(buf.String(), "\n"), nil
}
