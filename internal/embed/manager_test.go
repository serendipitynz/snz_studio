package embed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeSidecarEnv makes this test binary act as llama-server: the Manager launches
// it through the same command path as the real binary, so the start, health,
// probe and kill sequence is the production one.
const fakeSidecarEnv = "SNZ_FAKE_LLAMA_SERVER"

const fakeSidecarDim = 256

func TestMain(m *testing.M) {
	// Checked before m.Run because the llama-server arguments are not test flags.
	if os.Getenv(fakeSidecarEnv) == "1" {
		runFakeLlamaServer(os.Args[1:])
		return
	}
	os.Exit(m.Run())
}

// runFakeLlamaServer serves /health and a fixed-dimension /v1/embeddings on the
// --port it was given, until it is killed.
func runFakeLlamaServer(args []string) {
	port := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--port" {
			port = args[i+1]
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v1/embeddings", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []any{map[string]any{"embedding": make([]float64, fakeSidecarDim)}},
		})
	})
	err := http.ListenAndServe("127.0.0.1:"+port, mux)
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// newFakeSidecarManager returns a Manager whose binary is this test binary in
// fake llama-server mode and whose model is already staged, so no network is used.
func newFakeSidecarManager(t *testing.T) (*Manager, <-chan string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeSidecarEnv, "1")
	t.Setenv("SNZ_LLAMA_SERVER_BIN", exe)

	dir := t.TempDir()
	content := []byte("fake gguf")
	spec := testSpec("http://127.0.0.1:1/unused", content)
	spec.Dim = fakeSidecarDim
	if err := os.WriteFile(filepath.Join(dir, spec.FileName), content, 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(dir)
	m.spec = spec
	ready := make(chan string, 8)
	m.SetCallbacks(func(baseURL, _ string) { ready <- baseURL }, func() {})
	t.Cleanup(m.Shutdown)
	return m, ready
}

func waitReady(t *testing.T, m *Manager, ready <-chan string) string {
	t.Helper()
	select {
	case baseURL := <-ready:
		return baseURL
	case <-time.After(30 * time.Second):
		t.Fatalf("sidecar not ready within 30s; status=%+v", m.Status())
		return ""
	}
}

func TestEnsureInternalReadyRightAfterShutdownRestartsSidecar(t *testing.T) {
	m, ready := newFakeSidecarManager(t)
	m.EnsureInternalReady(context.Background())
	first := waitReady(t, m, ready)

	m.Shutdown()
	m.EnsureInternalReady(context.Background())
	second := waitReady(t, m, ready)

	if s := m.Status(); s.State != StateReady {
		t.Fatalf("status after restart = %q, want ready", s.State)
	}
	if got := m.BaseURL(); got != second {
		t.Fatalf("BaseURL = %q, want the restarted sidecar %q", got, second)
	}
	if first != second {
		if resp, err := http.Get(first + "/health"); err == nil {
			resp.Body.Close()
			t.Fatalf("the sidecar Shutdown stopped is still serving at %s", first)
		}
	}

	// By now the superseded run has finished winding down. Its exit must not mark
	// the new run stopped, or the next EnsureInternalReady would start a duplicate.
	time.Sleep(time.Second)
	m.EnsureInternalReady(context.Background())
	select {
	case baseURL := <-ready:
		t.Fatalf("a second sidecar started at %s beside the running one", baseURL)
	case <-time.After(time.Second):
	}
	if got := m.BaseURL(); got != second {
		t.Fatalf("BaseURL = %q, want the running sidecar %q", got, second)
	}
}

func TestShutdownLeavesStatusDisabled(t *testing.T) {
	m, ready := newFakeSidecarManager(t)
	m.EnsureInternalReady(context.Background())
	waitReady(t, m, ready)

	m.Shutdown()

	// Watch for a while too: the superseded run is still winding down after
	// Shutdown returns and must not write its state back.
	deadline := time.Now().Add(time.Second)
	for {
		s := m.Status()
		if s.State != StateDisabled {
			t.Fatalf("status after Shutdown = %q, want disabled", s.State)
		}
		if s.ModelID != m.spec.ModelID || s.Dim != m.spec.Dim {
			t.Fatalf("status after Shutdown = %+v, want the model identity kept", s)
		}
		if m.BaseURL() != "" {
			t.Fatal("BaseURL must be empty after Shutdown")
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
