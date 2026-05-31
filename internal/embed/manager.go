package embed

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// State is the lifecycle state of the internal embedding sidecar.
type State string

const (
	StateDisabled    State = "disabled"    // no binary found, or never started
	StateDownloading State = "downloading" // fetching the model GGUF
	StateStarting    State = "starting"    // launching llama-server / waiting for health
	StateReady       State = "ready"       // serving embeddings
	StateError       State = "error"       // gave up; retrieval stays FTS-only
)

// Status is the JSON-serialisable snapshot returned by GET /api/embedding/status.
type Status struct {
	State      State  `json:"state"`
	ModelID    string `json:"modelId"`
	Dim        int    `json:"dim"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Err        string `json:"error,omitempty"`
}

const maxConsecutiveFailures = 5

// Manager downloads the model and supervises the llama-server sidecar. It is
// decoupled from config/service: it reports readiness via onReady(baseURL, modelID)
// and loss via onLost(), which the HTTP layer wires to config's internal overlay.
type Manager struct {
	modelsDir string
	spec      ModelSpec
	binPath   string // resolved llama-server path; "" if not found
	client    *http.Client

	onReady func(baseURL, modelID string)
	onLost  func()

	mu      sync.Mutex
	status  Status
	sidecar *sidecar
	started bool
	cancel  context.CancelFunc
}

// NewManager builds a Manager for the default model under modelsDir. The
// llama-server binary is resolved from $SNZ_LLAMA_SERVER_BIN (dev override) or the
// per-OS bundled location; if neither exists, internal mode degrades to FTS-only.
func NewManager(modelsDir string) *Manager {
	return &Manager{
		modelsDir: modelsDir,
		spec:      RuriV3_30m,
		binPath:   resolveServerBinary(),
		client:    &http.Client{},
		status:    Status{State: StateDisabled, ModelID: RuriV3_30m.ModelID, Dim: RuriV3_30m.Dim},
	}
}

// SetCallbacks registers the ready/lost hooks. Call before EnsureInternalReady.
func (m *Manager) SetCallbacks(onReady func(baseURL, modelID string), onLost func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onReady = onReady
	m.onLost = onLost
}

// EnsureInternalReady starts the download+sidecar pipeline if it is not already
// running or ready. It is asynchronous (returns immediately) and idempotent.
func (m *Manager) EnsureInternalReady(ctx context.Context) {
	m.mu.Lock()
	if m.binPath == "" {
		m.status = Status{State: StateError, ModelID: m.spec.ModelID, Dim: m.spec.Dim, Err: "llama-server binary not found"}
		m.mu.Unlock()
		return
	}
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()

	go m.run(runCtx)
}

func (m *Manager) run(ctx context.Context) {
	m.setState(StateDownloading, "")
	modelPath, err := downloadModel(ctx, m.client, m.spec, m.modelsDir, func(d, t int64) {
		m.setProgress(d, t)
	})
	if err != nil {
		m.fail(err)
		m.markStopped()
		return
	}
	m.superviseSidecar(ctx, modelPath)
}

// superviseSidecar runs the sidecar, restarting it with exponential backoff after a
// crash, up to maxConsecutiveFailures. A clean shutdown (ctx cancel) exits quietly.
func (m *Manager) superviseSidecar(ctx context.Context, modelPath string) {
	defer m.markStopped()
	failures := 0
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if failures >= maxConsecutiveFailures {
			m.fail(fmt.Errorf("sidecar failed %d times; staying FTS-only", failures))
			return
		}

		m.setState(StateStarting, "")
		sc := newSidecar(m.binPath, modelPath, m.spec.Dim)
		if err := sc.Start(ctx); err != nil {
			sc.Stop()
			if ctx.Err() != nil {
				return
			}
			failures++
			m.fail(err)
			if !sleep(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		failures = 0
		backoff = time.Second
		m.markReady(sc)
		if cb := m.readyCallback(); cb != nil {
			cb(sc.BaseURL(), m.spec.ModelID)
		}

		exitErr := sc.Wait()
		if ctx.Err() != nil {
			return // intentional shutdown
		}
		// Unexpected exit: drop the overlay and retry after backoff.
		if cb := m.lostCallback(); cb != nil {
			cb()
		}
		failures++
		m.fail(fmt.Errorf("sidecar exited unexpectedly: %v", exitErr))
		if !sleep(ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

// BaseURL returns the sidecar's loopback origin if ready, else "".
func (m *Manager) BaseURL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State != StateReady || m.sidecar == nil {
		return ""
	}
	return m.sidecar.BaseURL()
}

// Status returns a snapshot of the current state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// Shutdown stops the pipeline and the sidecar process.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	cancel := m.cancel
	sc := m.sidecar
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if sc != nil {
		sc.Stop()
	}
}

func (m *Manager) setState(state State, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.State = state
	m.status.Err = errMsg
}

func (m *Manager) setProgress(downloaded, total int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.Downloaded = downloaded
	m.status.Total = total
}

func (m *Manager) markReady(sc *sidecar) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sidecar = sc
	m.status = Status{
		State:      StateReady,
		ModelID:    m.spec.ModelID,
		Dim:        m.spec.Dim,
		Downloaded: m.spec.SizeBytes,
		Total:      m.spec.SizeBytes,
	}
}

func (m *Manager) fail(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.State = StateError
	if err != nil {
		m.status.Err = err.Error()
	}
	m.sidecar = nil
}

func (m *Manager) markStopped() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = false
}

func (m *Manager) readyCallback() func(string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.onReady
}

func (m *Manager) lostCallback() func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.onLost
}

// resolveServerBinary finds the llama-server binary, in order: the
// SNZ_LLAMA_SERVER_BIN dev override, the per-OS bundled location (packaged app), then
// the cwd-relative dev fallback build/sidecar/<os>-<arch>/. Returns an ABSOLUTE path
// (so the sidecar's DYLD_LIBRARY_PATH/cwd resolve correctly) or "" if none exists.
func resolveServerBinary() string {
	if p := strings.TrimSpace(os.Getenv("SNZ_LLAMA_SERVER_BIN")); p != "" {
		return absIfExists(p)
	}
	if p := absIfExists(defaultServerBinaryPath()); p != "" {
		return p
	}
	return absIfExists(devServerBinaryPath())
}

// absIfExists returns the absolute form of path if it exists as a regular file, else "".
func absIfExists(path string) string {
	if !fileExists(path) {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// sleep waits for d or ctx cancellation; it returns false if ctx was cancelled.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func nextBackoff(d time.Duration) time.Duration {
	const max = 30 * time.Second
	if d *= 2; d > max {
		return max
	}
	return d
}
