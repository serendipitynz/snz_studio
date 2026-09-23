package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"snzstudio/internal/service"
)

var testPNG = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)

func enableImageDescription(t *testing.T, srv *Server, baseURL string) {
	t.Helper()
	editable := srv.cfg.GetEditable()
	editable.ImageDescriptionBaseURL = baseURL
	editable.ImageDescriptionModel = "vision"
	if _, err := srv.cfg.UpdateEditable(editable); err != nil {
		t.Fatalf("UpdateEditable: %v", err)
	}
}

func TestGetImageDescriptionReportsAvailability(t *testing.T) {
	srv := newTestServer(t)
	h := srv.Handler()

	var body struct {
		Enabled  bool     `json:"enabled"`
		MaxBytes int      `json:"maxBytes"`
		Formats  []string `json:"formats"`
	}
	rec := doJSON(t, h, "GET", "/api/image-description", nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s (%v)", rec.Code, rec.Body.String(), err)
	}
	if body.Enabled || body.MaxBytes != service.MaxImageDescriptionBytes || len(body.Formats) == 0 {
		t.Fatalf("disabled body = %+v", body)
	}

	enableImageDescription(t, srv, "http://127.0.0.1:1/v1")
	rec = doJSON(t, h, "GET", "/api/image-description", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.Enabled {
		t.Fatal("enabled = false after a model was configured")
	}
}

func TestDescribeImageMapsErrorsToStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"model does not support image inputs."}}`))
	}))
	t.Cleanup(upstream.Close)

	srv := newTestServer(t)
	h := srv.Handler()

	if rec := doMultipart(t, h, "/api/image-description", nil, "file", "a.png", testPNG, "image/png"); rec.Code != http.StatusConflict {
		t.Fatalf("disabled: status = %d, want 409 (%s)", rec.Code, rec.Body.String())
	}

	enableImageDescription(t, srv, upstream.URL+"/v1")
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	// The part header claims PNG; the service must go by the bytes.
	if rec := doMultipart(t, h, "/api/image-description", nil, "file", "a.png", svg, "image/png"); rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("svg: status = %d, want 415 (%s)", rec.Code, rec.Body.String())
	}
	if rec := doMultipart(t, h, "/api/image-description", map[string]string{"x": "y"}, "", "", nil, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("no file: status = %d, want 400", rec.Code)
	}
	rec := doMultipart(t, h, "/api/image-description", nil, "file", "a.png", testPNG, "image/png")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "does not support image inputs") {
		t.Fatalf("upstream refusal: status = %d body = %s, want 502 carrying the endpoint message", rec.Code, rec.Body.String())
	}
}

func TestDescribeImageRefusesDeclaredOversizeBeforeReading(t *testing.T) {
	srv := newTestServer(t)
	enableImageDescription(t, srv, "http://127.0.0.1:1/v1")

	req := httptest.NewRequest("POST", "/api/image-description", bytes.NewReader(nil))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	req.ContentLength = service.MaxImageDescriptionBytes + imageDescriptionMultipartSlack + 1
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 (%s)", rec.Code, rec.Body.String())
	}
}

func TestDescribeImageStoresNothing(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"灯台の地図"}}]}`))
	}))
	t.Cleanup(upstream.Close)

	srv := newTestServer(t)
	enableImageDescription(t, srv, upstream.URL+"/v1")
	h := srv.Handler()

	rec := doMultipart(t, h, "/api/image-description", nil, "file", "a.png", testPNG, "image/png")
	var body struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK || body.Description != "灯台の地図" {
		t.Fatalf("describe = %d %s", rec.Code, rec.Body.String())
	}
	if entries, err := os.ReadDir(srv.uploadDir); err == nil && len(entries) > 0 {
		t.Fatalf("upload dir has %d entries after describe, want none", len(entries))
	}
}
