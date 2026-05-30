// Package chunk ports backend/src/services/documentChunker.ts: it splits a
// document's combined search body into overlapping fixed-size windows that are
// stored as document_chunks rows and embedded individually by the retrieval
// layer.
package chunk

import "strings"

const (
	chunkSize    = 1000
	chunkOverlap = 150
)

// Document splits text into trimmed, non-empty windows of chunkSize runes that
// overlap by chunkOverlap. Mirrors chunkDocumentText(text).
//
// The TS source slices and measures length by UTF-16 code unit; this port uses
// runes. For the Japanese (BMP) prose this app handles, runes and UTF-16 code
// units are 1:1, so the window boundaries match exactly. Only astral-plane
// characters (e.g. emoji), which do not appear in source documents, would
// differ. The TS source also wraps each window in truncate(window, chunkSize);
// that is a no-op here because a window is already <= chunkSize after slicing
// and trimming, so it is intentionally omitted.
func Document(text string) []string {
	normalized := strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if normalized == "" {
		return []string{}
	}

	runes := []rune(normalized)
	total := len(runes)
	chunks := []string{}
	for offset := 0; offset < total; offset += chunkSize - chunkOverlap {
		end := offset + chunkSize
		if end > total {
			end = total
		}
		if window := strings.TrimSpace(string(runes[offset:end])); window != "" {
			chunks = append(chunks, window)
		}
	}
	return chunks
}
