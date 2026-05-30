package httpapi

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// readSSEEvent reads one "event:/data:" frame (terminated by a blank line).
func readSSEEvent(r *bufio.Reader) (event, data string, err error) {
	for {
		line, e := r.ReadString('\n')
		if e != nil {
			return event, data, e
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if event != "" || data != "" {
				return event, data, nil
			}
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

// TestSSEIncrementalDelivery proves the first event reaches the client while the
// handler is still blocked, i.e. http.Flusher delivers incrementally rather than
// buffering until the handler returns. This is the HTTP-level guarantee the
// streaming chat UX depends on. (WebView consumption is verified manually in the
// running Wails app.)
func TestSSEIncrementalDelivery(t *testing.T) {
	release := make(chan struct{})
	firstSent := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse, err := NewSSEWriter(w)
		if err != nil {
			t.Errorf("NewSSEWriter: %v", err)
			return
		}
		if err := sse.Event("delta", map[string]string{"content": "first"}); err != nil {
			t.Errorf("first event: %v", err)
			return
		}
		close(firstSent)
		<-release // block: a buffering transport would withhold "first" until now
		_ = sse.Event("delta", map[string]string{"content": "second"})
		_ = sse.Event("done", map[string]bool{"ok": true})
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	event, data, err := readSSEEvent(reader)
	if err != nil {
		t.Fatalf("read first event: %v", err)
	}
	if event != "delta" || !strings.Contains(data, "first") {
		t.Fatalf("first frame = event:%q data:%q", event, data)
	}

	// We received the first frame; confirm the handler was still blocked.
	select {
	case <-firstSent:
	default:
		t.Fatal("handler had not sent first event yet")
	}
	select {
	case <-release:
		t.Fatal("release channel already closed; delivery was not incremental")
	default:
	}

	close(release) // let the handler finish

	event, data, err = readSSEEvent(reader)
	if err != nil {
		t.Fatalf("read second event: %v", err)
	}
	if event != "delta" || !strings.Contains(data, "second") {
		t.Errorf("second frame = event:%q data:%q", event, data)
	}

	event, _, err = readSSEEvent(reader)
	if err != nil {
		t.Fatalf("read done event: %v", err)
	}
	if event != "done" {
		t.Errorf("third frame event = %q, want done", event)
	}
}
