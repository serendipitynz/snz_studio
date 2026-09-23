package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// sidecar runs a single llama-server process bound to an ephemeral loopback port
// in embedding mode. It is owned by the Manager, which supervises restarts.
type sidecar struct {
	binPath   string
	modelPath string
	dim       int
	client    *http.Client

	mu      sync.Mutex
	cmd     *exec.Cmd
	baseURL string
}

func newSidecar(binPath, modelPath string, dim int) *sidecar {
	return &sidecar{
		binPath:   binPath,
		modelPath: modelPath,
		dim:       dim,
		client:    &http.Client{},
	}
}

// Start launches llama-server, waits for /health, and asserts the probe embedding
// has dim dimensions. On any failure the process is stopped and the error returned.
// The process is left running on success; call Wait to block on its exit and Stop
// to terminate it.
func (s *sidecar) Start(ctx context.Context) error {
	port, err := freePort()
	if err != nil {
		return err
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	args := []string{
		"-m", s.modelPath,
		"--embedding",
		"--pooling", "mean", // ruri uses mean pooling
		"-c", "2048",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		// CPU-only: the 37M model is tiny, and this keeps the sidecar portable
		// (matches the Windows CPU build) and free of GPU/JIT concerns.
		"-ngl", "0",
	}
	cmd := sidecarCommand(s.binPath, args) // platform-specific parent-death guard
	configureSysProcAttr(cmd)              // platform-specific process-group setup
	cmd.Dir = filepath.Dir(s.binPath)
	if err := cmd.Start(); err != nil {
		return err
	}

	s.mu.Lock()
	s.cmd = cmd
	s.baseURL = baseURL
	s.mu.Unlock()

	if err := s.waitHealthy(ctx, baseURL); err != nil {
		s.Stop()
		return err
	}
	if err := s.probeDim(ctx, baseURL); err != nil {
		s.Stop()
		return err
	}
	return nil
}

// BaseURL returns the sidecar's loopback origin (e.g. http://127.0.0.1:51234).
func (s *sidecar) BaseURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baseURL
}

// Wait blocks until the process exits, returning its exit error (nil on clean exit).
func (s *sidecar) Wait() error {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	if cmd == nil {
		return nil
	}
	return cmd.Wait()
}

// Stop terminates the process group. Safe to call multiple times.
func (s *sidecar) Stop() {
	s.mu.Lock()
	cmd := s.cmd
	s.cmd = nil
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	killProcessGroup(cmd)
}

func (s *sidecar) waitHealthy(ctx context.Context, baseURL string) error {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("sidecar: /health timed out")
		case <-tick.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
			if err != nil {
				return err
			}
			resp, err := s.client.Do(req)
			if err != nil {
				continue // not up yet
			}
			ok := resp.StatusCode == http.StatusOK
			resp.Body.Close()
			if ok {
				return nil
			}
		}
	}
}

// probeDim sends one embedding request and asserts the returned vector dimension,
// catching a wrong/incompatible GGUF before the sidecar is marked ready.
func (s *sidecar) probeDim(ctx context.Context, baseURL string) error {
	body, _ := json.Marshal(map[string]any{"input": []string{"probe"}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sidecar probe: status %d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	got := 0
	if len(out.Data) > 0 {
		got = len(out.Data[0].Embedding)
	}
	if got != s.dim {
		return fmt.Errorf("sidecar probe: embedding dim %d, want %d", got, s.dim)
	}
	return nil
}

// freePort asks the OS for an unused loopback TCP port. There is a benign TOCTOU
// window between closing the listener and llama-server binding it; acceptable for a
// local single-user app.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
