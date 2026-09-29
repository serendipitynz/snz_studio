package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testAuthToken = "test-token-0123456789abcdef"

// newAuthedTestServer is newTestServer with the per-launch token installed, so
// requests pass through withAuth the way they do in the running app. It also
// drops one upload into uploadDir and returns its name.
func newAuthedTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	srv := newTestServer(t)
	srv.SetAuthToken(testAuthToken)
	if err := os.MkdirAll(srv.uploadDir, 0o755); err != nil {
		t.Fatalf("mkdir uploads: %v", err)
	}
	name := "upload.txt"
	if err := os.WriteFile(filepath.Join(srv.uploadDir, name), []byte("upload body"), 0o644); err != nil {
		t.Fatalf("write upload: %v", err)
	}
	return srv, name
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthTokenGatesAPIAndFiles(t *testing.T) {
	srv, upload := newAuthedTestServer(t)
	h := srv.Handler()

	apiRequest := func(token string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
		if token != "" {
			req.Header.Set("X-SNZ-Studio-Token", token)
		}
		return req
	}
	fileRequest := func(token string) *http.Request {
		target := "/files/" + upload
		if token != "" {
			target += "?t=" + token
		}
		return httptest.NewRequest(http.MethodGet, target, nil)
	}

	cases := []struct {
		name  string
		req   *http.Request
		wantS int
	}{
		{"api without token", apiRequest(""), http.StatusUnauthorized},
		{"api with wrong token", apiRequest("wrong-token"), http.StatusUnauthorized},
		{"api with token prefix", apiRequest(testAuthToken[:len(testAuthToken)-1]), http.StatusUnauthorized},
		{"api with correct token", apiRequest(testAuthToken), http.StatusOK},
		{"files without token", fileRequest(""), http.StatusUnauthorized},
		{"files with wrong token", fileRequest("wrong-token"), http.StatusUnauthorized},
		{"files with correct token", fileRequest(testAuthToken), http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(h, tc.req)
			if rec.Code != tc.wantS {
				t.Fatalf("status = %d, want %d (body=%s)", rec.Code, tc.wantS, rec.Body.String())
			}
			if tc.wantS == http.StatusUnauthorized && strings.Contains(rec.Body.String(), "upload body") {
				t.Fatalf("unauthorized response leaked the file: %s", rec.Body.String())
			}
		})
	}
}

// The token travels in a different place per route (header for /api, query for
// /files, since <img> cannot set headers). Each route must accept only its own
// carrier, so a token leaked into one channel does not open the other.
func TestAuthTokenCarrierIsPerRoute(t *testing.T) {
	srv, upload := newAuthedTestServer(t)
	h := srv.Handler()

	apiWithQuery := httptest.NewRequest(http.MethodGet, "/api/projects?t="+testAuthToken, nil)
	wantStatus(t, serve(h, apiWithQuery), http.StatusUnauthorized)

	fileWithHeader := httptest.NewRequest(http.MethodGet, "/files/"+upload, nil)
	fileWithHeader.Header.Set("X-SNZ-Studio-Token", testAuthToken)
	wantStatus(t, serve(h, fileWithHeader), http.StatusUnauthorized)
}

func TestAuthTokenLetsOnlyPreflightThrough(t *testing.T) {
	srv, upload := newAuthedTestServer(t)
	h := srv.Handler()

	for _, target := range []string{"/api/projects", "/files/" + upload} {
		req := httptest.NewRequest(http.MethodOptions, target, nil)
		req.Header.Set("Origin", "http://wails.localhost")
		req.Header.Set("Access-Control-Request-Method", http.MethodGet)
		req.Header.Set("Access-Control-Request-Headers", "X-SNZ-Studio-Token")
		rec := serve(h, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("OPTIONS %s status = %d, want %d", target, rec.Code, http.StatusNoContent)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("OPTIONS %s returned a body: %s", target, rec.Body.String())
		}
	}

	// Every other method without a token is refused, including the ones a
	// browser can send cross-origin without a preflight.
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := serve(h, httptest.NewRequest(method, "/api/projects", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s /api/projects without token status = %d, want %d", method, rec.Code, http.StatusUnauthorized)
		}
	}
}

// fileHandler must keep /files inside uploadDir. The test server's DB lives at
// <tmp>/app.sqlite, one level above <tmp>/uploads, so escaping by one segment
// would reach it.
func TestFilesRejectsPathTraversal(t *testing.T) {
	srv, _ := newAuthedTestServer(t)
	h := srv.Handler()

	secret := filepath.Join(filepath.Dir(srv.uploadDir), "app.sqlite")
	if _, err := os.Stat(secret); err != nil {
		t.Fatalf("expected the DB beside uploads at %s: %v", secret, err)
	}
	// A file named with a backslash would be reachable on Windows through the
	// separator; the guard refuses it on every OS.
	outside := filepath.Join(filepath.Dir(srv.uploadDir), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside body"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	targets := []string{
		"/files/..%2fapp.sqlite",
		"/files/..%2Fapp.sqlite",
		"/files/%2e%2e",
		"/files/%2e%2e%2fapp.sqlite",
		"/files/..%5coutside.txt",
		"/files/..%5Capp.sqlite",
		"/files/%2e",
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target+"?t="+testAuthToken, nil)
			rec := serve(h, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d (Location=%q body=%q)", rec.Code, http.StatusNotFound, rec.Header().Get("Location"), rec.Body.String())
			}
			body := rec.Body.String()
			if strings.Contains(body, "SQLite format") || strings.Contains(body, "outside body") {
				t.Fatalf("response leaked a file outside uploadDir: %q", body)
			}
		})
	}
}
