package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestManagerIntegrationRealSidecar exercises the full pipeline (download-skip →
// llama-server start → /health → 256-dim probe → ready callback) against a REAL
// llama-server and ruri GGUF. It is skipped unless both SNZ_LLAMA_SERVER_BIN and
// SNZ_TEST_RURI_GGUF point at real files, so CI and a plain `go test` skip it. Run
// locally with the spike artifacts, e.g.:
//
//	SNZ_LLAMA_SERVER_BIN=/tmp/sidecar-arm64/llama-server \
//	SNZ_TEST_RURI_GGUF=/tmp/ruri-v3-30m-q8_0.gguf \
//	go test ./internal/embed/ -run Integration -v
//
// The GGUF must be the exact q8_0 build pinned in RuriV3_30m (size+sha verified), so
// downloadModel skips the network and the placeholder URL is never used.
func TestManagerIntegrationRealSidecar(t *testing.T) {
	bin := os.Getenv("SNZ_LLAMA_SERVER_BIN")
	gguf := os.Getenv("SNZ_TEST_RURI_GGUF")
	if bin == "" || gguf == "" {
		t.Skip("set SNZ_LLAMA_SERVER_BIN and SNZ_TEST_RURI_GGUF to run the sidecar integration test")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("SNZ_LLAMA_SERVER_BIN missing: %v", err)
	}

	modelsDir := t.TempDir()
	data, err := os.ReadFile(gguf)
	if err != nil {
		t.Fatalf("read GGUF: %v", err)
	}
	// Pre-place the GGUF where downloadModel expects it so verifyFile (size+sha256)
	// passes and the download is skipped.
	if err := os.WriteFile(filepath.Join(modelsDir, RuriV3_30m.FileName), data, 0o644); err != nil {
		t.Fatalf("stage GGUF: %v", err)
	}

	m := NewManager(modelsDir)
	if m.binPath == "" {
		t.Fatal("manager did not resolve the binary from SNZ_LLAMA_SERVER_BIN")
	}
	readyCh := make(chan string, 1)
	m.SetCallbacks(
		func(baseURL, modelID string) {
			select {
			case readyCh <- baseURL:
			default:
			}
		},
		func() {},
	)
	defer m.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	m.EnsureInternalReady(ctx)

	var baseURL string
	select {
	case baseURL = <-readyCh:
	case <-ctx.Done():
		t.Fatalf("sidecar not ready within timeout; status=%+v", m.Status())
	}

	if s := m.Status(); s.State != StateReady {
		t.Fatalf("status=%s, want ready", s.State)
	}
	if m.BaseURL() == "" || baseURL == "" {
		t.Fatal("BaseURL empty after ready")
	}

	// Confirm the live endpoint returns a 256-dim embedding end-to-end.
	body, _ := json.Marshal(map[string]any{"input": []string{"検索クエリ: テスト"}})
	resp, err := http.Post(baseURL+"/v1/embeddings", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("embeddings request: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Data) != 1 || len(out.Data[0].Embedding) != RuriV3_30m.Dim {
		got := 0
		if len(out.Data) > 0 {
			got = len(out.Data[0].Embedding)
		}
		t.Fatalf("embedding dim %d, want %d", got, RuriV3_30m.Dim)
	}
	t.Logf("sidecar ready at %s, %d-dim embeddings via the manager pipeline", baseURL, RuriV3_30m.Dim)
}
