// Package httpapi will host the Go port of the Express HTTP API. sse.go provides
// the Server-Sent Events writer used by the streaming chat/review endpoints.
//
// The wire format matches the Express implementation in backend/src/index.ts and
// the frontend parser in ChatPage.tsx (frames are "event: <name>\ndata: <json>\n\n"),
// so the existing frontend streaming code consumes it unchanged. Each event is
// flushed immediately via http.Flusher, which delivers incrementally over a real
// net/http connection (the reason the desktop app serves the API from a local
// loopback server rather than through the Wails AssetServer bridge).
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// SSEWriter writes Server-Sent Events and flushes after each one.
type SSEWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

// NewSSEWriter sets streaming headers and returns a writer, or an error if the
// underlying ResponseWriter does not support flushing.
func NewSSEWriter(w http.ResponseWriter) (*SSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming unsupported: ResponseWriter is not an http.Flusher")
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	return &SSEWriter{w: w, f: flusher}, nil
}

// Event marshals data to JSON and writes a single named SSE frame, flushing it.
func (s *SSEWriter) Event(event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}
