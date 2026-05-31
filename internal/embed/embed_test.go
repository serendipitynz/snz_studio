package embed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func testSpec(url string, content []byte) ModelSpec {
	return ModelSpec{
		ModelID:   "test-model",
		FileName:  "test.gguf",
		URL:       url,
		SHA256:    sha256Hex(content),
		SizeBytes: int64(len(content)),
		Dim:       256,
	}
}

func TestVerifyFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello ruri embedding")
	path := filepath.Join(dir, "f.gguf")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	good := ModelSpec{SHA256: sha256Hex(content), SizeBytes: int64(len(content))}
	if !verifyFile(path, good) {
		t.Fatal("verifyFile should pass for matching size+sha")
	}
	if verifyFile(path, ModelSpec{SHA256: sha256Hex(content), SizeBytes: 9999}) {
		t.Fatal("size mismatch must fail")
	}
	if verifyFile(path, ModelSpec{SHA256: "deadbeef", SizeBytes: int64(len(content))}) {
		t.Fatal("sha mismatch must fail")
	}
	if verifyFile(filepath.Join(dir, "missing"), good) {
		t.Fatal("missing file must fail")
	}
}

func TestDownloadModelFullAndIdempotent(t *testing.T) {
	content := bytes.Repeat([]byte("ruri-v3-30m"), 5000) // ~55KB
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.ServeContent(w, r, "test.gguf", time.Time{}, bytes.NewReader(content))
	}))
	defer srv.Close()

	dir := t.TempDir()
	spec := testSpec(srv.URL, content)

	path, err := downloadModel(context.Background(), srv.Client(), spec, dir, nil)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if !verifyFile(path, spec) {
		t.Fatal("downloaded file does not verify")
	}
	if _, err := os.Stat(path + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part should be gone after success")
	}

	// Idempotent: the verified file is reused, no second fetch.
	hitsAfterFirst := hits
	if _, err := downloadModel(context.Background(), srv.Client(), spec, dir, nil); err != nil {
		t.Fatalf("second download: %v", err)
	}
	if hits != hitsAfterFirst {
		t.Fatalf("expected no re-download, got %d extra hits", hits-hitsAfterFirst)
	}
}

func TestDownloadModelResumesPartial(t *testing.T) {
	content := bytes.Repeat([]byte("ABCDEFGH"), 4000) // 32KB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// http.ServeContent honours Range, returning 206 for resume requests.
		http.ServeContent(w, r, "test.gguf", time.Time{}, bytes.NewReader(content))
	}))
	defer srv.Close()

	dir := t.TempDir()
	spec := testSpec(srv.URL, content)
	// Seed a partial .part with the first 10KB.
	if err := os.WriteFile(filepath.Join(dir, spec.FileName+".part"), content[:10000], 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := downloadModel(context.Background(), srv.Client(), spec, dir, nil)
	if err != nil {
		t.Fatalf("resume download: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("resumed file mismatch: got %d bytes, want %d", len(got), len(content))
	}
}

func TestDownloadModelChecksumMismatch(t *testing.T) {
	content := []byte("real content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "test.gguf", time.Time{}, bytes.NewReader([]byte("tampered content!!")))
	}))
	defer srv.Close()

	dir := t.TempDir()
	spec := testSpec(srv.URL, content) // sha/size of the *real* content, server serves tampered
	if _, err := downloadModel(context.Background(), srv.Client(), spec, dir, nil); err == nil {
		t.Fatal("expected checksum/size mismatch error")
	}
	if _, err := os.Stat(filepath.Join(dir, spec.FileName+".part")); !os.IsNotExist(err) {
		t.Fatal(".part should be removed on verification failure")
	}
}

func TestResolveServerBinaryEnvOverride(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "llama-server")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SNZ_LLAMA_SERVER_BIN", bin)
	if got := resolveServerBinary(); got != bin {
		t.Fatalf("resolveServerBinary = %q, want %q", got, bin)
	}
	t.Setenv("SNZ_LLAMA_SERVER_BIN", filepath.Join(dir, "nope"))
	if got := resolveServerBinary(); got != "" {
		t.Fatalf("missing override should resolve to empty, got %q", got)
	}
}

func TestManagerNoBinaryDegrades(t *testing.T) {
	t.Setenv("SNZ_LLAMA_SERVER_BIN", filepath.Join(t.TempDir(), "absent"))
	m := NewManager(t.TempDir())
	if s := m.Status(); s.State != StateDisabled {
		t.Fatalf("initial state = %q, want disabled", s.State)
	}
	m.EnsureInternalReady(context.Background())
	if s := m.Status(); s.State != StateError {
		t.Fatalf("no-binary EnsureInternalReady state = %q, want error", s.State)
	}
	if m.BaseURL() != "" {
		t.Fatal("BaseURL must be empty when not ready")
	}
}
