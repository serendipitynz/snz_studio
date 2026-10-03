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

	// callbackMu serialises the ready/lost callbacks with Shutdown, so a run that
	// Shutdown superseded cannot re-apply the internal overlay after the caller
	// cleared it.
	callbackMu sync.Mutex
	// modelMu gives one run at a time the model files (the seed copy and the
	// download's .part).
	modelMu sync.Mutex

	mu      sync.Mutex
	status  Status
	sidecar *sidecar
	started bool
	cancel  context.CancelFunc
	// gen identifies the current run. A superseded run keeps going until it notices
	// its cancelled context, so its state writes are dropped by generation rather
	// than allowed to overwrite its successor's.
	gen uint64
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
	m.gen++
	gen := m.gen
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()

	go m.run(runCtx, gen)
}

func (m *Manager) run(ctx context.Context, gen uint64) {
	m.setState(gen, StateDownloading, "")
	// A run that Shutdown superseded can still be writing the .part file after a
	// chunk it read before the cancel; waiting for it keeps this run's resume
	// offset from going stale under that write.
	m.modelMu.Lock()
	// Packaged builds ship the GGUF inside the app bundle; seed it into the
	// per-user models dir so downloadModel verifies and skips the network.
	seedBundledModel(m.modelsDir, m.spec)
	modelPath, err := downloadModel(ctx, m.client, m.spec, m.modelsDir, func(d, t int64) {
		m.setProgress(gen, d, t)
	})
	m.modelMu.Unlock()
	if err != nil {
		m.fail(gen, err)
		m.markStopped(gen)
		return
	}
	m.superviseSidecar(ctx, gen, modelPath)
}

// superviseSidecar runs the sidecar, restarting it with exponential backoff after a
// crash, up to maxConsecutiveFailures. A clean shutdown (ctx cancel) exits quietly.
func (m *Manager) superviseSidecar(ctx context.Context, gen uint64, modelPath string) {
	defer m.markStopped(gen)
	failures := 0
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if failures >= maxConsecutiveFailures {
			m.fail(gen, fmt.Errorf("sidecar failed %d times; staying FTS-only", failures))
			return
		}

		m.setState(gen, StateStarting, "")
		sc := newSidecar(m.binPath, modelPath, m.spec.Dim, m.spec.ContextLength)
		if err := sc.Start(ctx); err != nil {
			sc.Stop()
			if ctx.Err() != nil {
				return
			}
			failures++
			m.fail(gen, err)
			if !sleep(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		failures = 0
		backoff = time.Second
		if !m.becomeReady(gen, sc) {
			sc.Stop() // Shutdown ran while this sidecar was starting and never saw it
			return
		}

		exitErr := sc.Wait()
		if ctx.Err() != nil {
			return // intentional shutdown
		}
		// Unexpected exit: drop the overlay and retry after backoff.
		m.reportLost(gen)
		failures++
		m.fail(gen, fmt.Errorf("sidecar exited unexpectedly: %v", exitErr))
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

// Shutdown stops the pipeline and the sidecar process. On return the status is
// disabled and EnsureInternalReady starts a fresh run, even while the superseded
// run is still winding down.
func (m *Manager) Shutdown() {
	m.callbackMu.Lock()
	m.mu.Lock()
	m.gen++
	cancel := m.cancel
	sc := m.sidecar
	m.cancel = nil
	m.sidecar = nil
	m.started = false
	m.status = Status{State: StateDisabled, ModelID: m.spec.ModelID, Dim: m.spec.Dim}
	m.mu.Unlock()
	m.callbackMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if sc != nil {
		sc.Stop()
	}
}

func (m *Manager) setState(gen uint64, state State, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gen != m.gen {
		return
	}
	m.status.State = state
	m.status.Err = errMsg
}

func (m *Manager) setProgress(gen uint64, downloaded, total int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gen != m.gen {
		return
	}
	m.status.Downloaded = downloaded
	m.status.Total = total
}

// becomeReady publishes sc as the serving sidecar and fires onReady. It reports
// false, publishing nothing, when Shutdown has superseded this run.
func (m *Manager) becomeReady(gen uint64, sc *sidecar) bool {
	m.callbackMu.Lock()
	defer m.callbackMu.Unlock()
	m.mu.Lock()
	if gen != m.gen {
		m.mu.Unlock()
		return false
	}
	m.sidecar = sc
	m.status = Status{
		State:      StateReady,
		ModelID:    m.spec.ModelID,
		Dim:        m.spec.Dim,
		Downloaded: m.spec.SizeBytes,
		Total:      m.spec.SizeBytes,
	}
	onReady := m.onReady
	m.mu.Unlock()
	if onReady != nil {
		onReady(sc.BaseURL(), m.spec.ModelID)
	}
	return true
}

// reportLost fires onLost unless Shutdown has superseded this run; by then the
// overlay belongs to the caller that shut it down, or to the next run.
func (m *Manager) reportLost(gen uint64) {
	m.callbackMu.Lock()
	defer m.callbackMu.Unlock()
	m.mu.Lock()
	current := gen == m.gen
	onLost := m.onLost
	m.mu.Unlock()
	if current && onLost != nil {
		onLost()
	}
}

func (m *Manager) fail(gen uint64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gen != m.gen {
		return
	}
	m.status.State = StateError
	if err != nil {
		m.status.Err = err.Error()
	}
	m.sidecar = nil
}

func (m *Manager) markStopped(gen uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gen != m.gen {
		return
	}
	m.started = false
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
