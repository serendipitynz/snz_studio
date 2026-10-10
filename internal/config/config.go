// Package config ports the LLM / embedding related parts of backend/src/config.ts.
//
// The Node backend kept a single mutable `config` object that every service read
// live and that the configuration endpoint mutated via updateEditableConfiguration.
// Go services run concurrently, so instead of a shared mutable global this package
// hands out an immutable Settings snapshot (Config.Get) that callers read at the
// start of each request, and serialises mutations (Config.UpdateEditable) behind a
// lock. The seven user-editable fields are persisted to app-config.json exactly as
// the TS backend did.
//
// Infrastructure-only settings from config.ts (port, dataDir, uploadDir,
// sqlitePath) live with the process bootstrap (internal/bootstrap, app.go), not
// here — this package is scoped to what the service layer consumes.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Editable mirrors EditableAppConfiguration: the fields the configuration endpoint
// can read and write. They are persisted to app-config.json together with
// autoCheckUpdates (see persistedFile).
type Editable struct {
	LLMBaseURL        string `json:"llmBaseUrl"`
	LLMModel          string `json:"llmModel"`
	LLMResponseFormat string `json:"llmResponseFormat"`
	ReviewBaseURL     string `json:"reviewBaseUrl"`
	ReviewModel       string `json:"reviewModel"`
	EmbeddingBaseURL  string `json:"embeddingBaseUrl"`
	EmbeddingModel    string `json:"embeddingModel"`
	// EmbeddingMode selects the embedding source: "internal" (the bundled
	// ruri-v3-30m llama.cpp sidecar) or "external" (a user-configured
	// OpenAI-compatible endpoint via EmbeddingBaseURL/Model). When internal, the
	// sidecar endpoint is overlaid at read time by Config.Get without touching the
	// persisted EmbeddingBaseURL/Model.
	EmbeddingMode string `json:"embeddingMode"`
	// ImageDescriptionBaseURL falls back to LLMBaseURL at use time when empty.
	// ImageDescriptionModel does not fall back: the chat model is not necessarily
	// multimodal, and a text-only model would return a description of an image it
	// never saw, so an empty model disables image description instead.
	ImageDescriptionBaseURL string `json:"imageDescriptionBaseUrl"`
	ImageDescriptionModel   string `json:"imageDescriptionModel"`
}

// Settings is an immutable snapshot of every config value the service layer reads.
// It embeds Editable and adds the runtime-only values (API keys, timeouts, debug
// flags) that are sourced from the environment and never persisted.
type Settings struct {
	Editable
	LLMAPIKey    string
	LLMTimeoutMs int
	// EmbeddingAPIKey holds EMBEDDING_API_KEY only, or in internal mode the bundled
	// sidecar's per-launch key. The LLM_API_KEY fallback is applied per request
	// (service.embeddingAPIKeyFor) so that it follows the LLM key's origin rule
	// instead of reaching whatever the embedding endpoint is.
	EmbeddingAPIKey    string
	EmbeddingTimeoutMs int
	// ImageDescriptionTimeoutMs is separate from LLMTimeoutMs because a local
	// multimodal model spends far longer preprocessing an image than a chat turn
	// takes; raising the chat timeout instead would delay detecting a stalled chat.
	ImageDescriptionTimeoutMs int
	DebugChatFlow             bool
	DebugRetrieval            bool
	// LLMReasoning is the reasoning setting chosen per model, keyed by the LM
	// Studio API root and then by the model key, holding LM Studio's own vocabulary
	// (off / on / low / ...). A model without an entry runs at its own default. The
	// maps are shared between snapshots and never modified: SetReasoning replaces
	// them.
	LLMReasoning map[string]map[string]string
}

// Config holds the current Settings snapshot and the path it persists edits to.
type Config struct {
	mu            sync.RWMutex
	settings      Settings
	appConfigPath string

	// internalEmbedURL/Model/Key are the runtime-only overlay for the bundled
	// embedding sidecar. They are NOT persisted; Config.Get substitutes them into
	// the returned Settings when EmbeddingMode=="internal". Empty model means the
	// sidecar is not ready yet (retrieval degrades to FTS-only).
	internalEmbedURL   string
	internalEmbedModel string
	internalEmbedKey   string

	// autoCheckUpdates is persisted beside the editable fields but saved on its own
	// (SetAutoCheckUpdates): it is an app choice, not a connection setting, and the
	// configuration endpoint's save also warms models and may start a rebuild.
	autoCheckUpdates bool

	// reasoning is saved on its own (SetReasoning) for the same reason as
	// autoCheckUpdates, and because it is edited from a participant's card as well
	// as from the settings screen.
	reasoning map[string]map[string]string
}

// persistedFile is the shape UpdateEditable writes to app-config.json.
type persistedFile struct {
	Editable
	AutoCheckUpdates bool                         `json:"autoCheckUpdates"`
	LLMReasoning     map[string]map[string]string `json:"llmReasoning,omitempty"`
}

const (
	autoCheckUpdatesKey = "autoCheckUpdates"
	llmReasoningKey     = "llmReasoning"
)

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// firstNonEmptyEnv returns the value of the first set environment variable among
// keys, or fallback if none are set. Mirrors the `process.env.A ?? process.env.B
// ?? "default"` chains in config.ts.
func firstNonEmptyEnv(fallback string, keys ...string) string {
	for _, key := range keys {
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
	}
	return fallback
}

func parseIntEnv(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return n
}

func parseBoolFlag(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizeResponseFormat(value string) string {
	if value == "llm_jp_thinking" {
		return "llm_jp_thinking"
	}
	return "standard"
}

// normalizeEmbeddingMode canonicalises an embedding mode. It returns "internal"
// or "external" for recognised values, or "" for anything else so callers can
// decide to keep the current value (e.g. an UpdateEditable payload from an older
// frontend that omits the field must not silently flip the mode).
func normalizeEmbeddingMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "external":
		return "external"
	case "internal":
		return "internal"
	default:
		return ""
	}
}

// DefaultImageDescriptionTimeoutMs is the IMAGE_DESCRIPTION_TIMEOUT_MS default.
// See the .env.example entry for how the value was measured.
const DefaultImageDescriptionTimeoutMs = 180000

// Defaults builds the Settings from environment variables with the same defaults
// and fallback chains as config.ts. It does not read app-config.json.
func Defaults() Settings {
	llmBaseURL := getenv("LLM_BASE_URL", "http://127.0.0.1:1234/v1")
	llmTimeout := parseIntEnv(getenv("LLM_TIMEOUT_MS", ""), 60000)
	embeddingMode := normalizeEmbeddingMode(getenv("EMBEDDING_MODE", "internal"))
	if embeddingMode == "" {
		embeddingMode = "internal"
	}
	return Settings{
		Editable: Editable{
			LLMBaseURL:        llmBaseURL,
			LLMModel:          getenv("LLM_MODEL", "local-model"),
			LLMResponseFormat: normalizeResponseFormat(getenv("LLM_RESPONSE_FORMAT", "standard")),
			ReviewBaseURL:     firstNonEmptyEnv("http://127.0.0.1:1234/v1", "REVIEW_BASE_URL", "LLM_BASE_URL"),
			ReviewModel:       firstNonEmptyEnv("local-model", "REVIEW_MODEL", "LLM_MODEL"),
			EmbeddingBaseURL:  firstNonEmptyEnv("http://127.0.0.1:1234/v1", "EMBEDDING_BASE_URL", "LLM_BASE_URL"),
			EmbeddingModel:    getenv("EMBEDDING_MODEL", ""),
			EmbeddingMode:     embeddingMode,

			ImageDescriptionBaseURL: getenv("IMAGE_DESCRIPTION_BASE_URL", ""),
			ImageDescriptionModel:   getenv("IMAGE_DESCRIPTION_MODEL", ""),
		},
		LLMAPIKey:                 getenv("LLM_API_KEY", ""),
		LLMTimeoutMs:              llmTimeout,
		EmbeddingAPIKey:           getenv("EMBEDDING_API_KEY", ""),
		EmbeddingTimeoutMs:        parseIntEnv(firstNonEmptyEnv("", "EMBEDDING_TIMEOUT_MS", "LLM_TIMEOUT_MS"), 60000),
		ImageDescriptionTimeoutMs: parseIntEnv(getenv("IMAGE_DESCRIPTION_TIMEOUT_MS", ""), DefaultImageDescriptionTimeoutMs),
		DebugChatFlow:             parseBoolFlag(getenv("DEBUG_CHAT_FLOW", "")),
		DebugRetrieval:            parseBoolFlag(getenv("DEBUG_RETRIEVAL", "")),
	}
}

// New constructs a Config around an explicit Settings snapshot (used by tests and
// by the bootstrap once it has resolved the data directory).
func New(settings Settings, appConfigPath string) *Config {
	settings.LLMResponseFormat = normalizeResponseFormat(settings.LLMResponseFormat)
	if m := normalizeEmbeddingMode(settings.EmbeddingMode); m != "" {
		settings.EmbeddingMode = m
	} else if strings.TrimSpace(settings.EmbeddingModel) != "" {
		// An explicit Settings that carries an embedding model but no mode clearly
		// wants that external model active, so default to external (mirrors the
		// app-config.json migration in applyOverrides) rather than internal — which
		// would overlay an empty model and disable embeddings.
		settings.EmbeddingMode = "external"
	} else {
		settings.EmbeddingMode = "internal"
	}
	// On unless app-config.json says otherwise (decided in TASK-92): with the check
	// off by default, almost nobody would learn that a newer version exists.
	return &Config{settings: settings, appConfigPath: appConfigPath, autoCheckUpdates: true}
}

// Load builds Config from the environment defaults and then applies the persisted
// app-config.json overrides, mirroring config.ts (loadEnvFile is the OS env in Go,
// followed by applyAppConfigOverrides).
func Load(appConfigPath string) *Config {
	c := New(Defaults(), appConfigPath)
	c.applyOverrides(appConfigPath)
	return c
}

// applyOverrides reads app-config.json and applies any present editable fields,
// trimming strings, exactly like applyAppConfigOverrides in config.ts.
func (c *Config) applyOverrides(path string) {
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("config: cannot read %s, starting from defaults: %v", path, err)
		}
		return
	}
	var overrides map[string]json.RawMessage
	if err := json.Unmarshal(raw, &overrides); err != nil {
		// Moving the file aside keeps the user's settings recoverable: the next save
		// would otherwise overwrite them with the defaults this launch falls back to.
		backup := path + ".bak"
		if renameErr := os.Rename(path, backup); renameErr != nil {
			log.Printf("config: %s is not valid JSON (%v) and could not be moved to %s (%v); starting from defaults", path, err, backup, renameErr)
			return
		}
		log.Printf("config: %s is not valid JSON (%v); moved it to %s and started from defaults", path, err, backup)
		return
	}

	applyString := func(key string, dst *string, trim bool) {
		v, ok := overrides[key]
		if !ok {
			return
		}
		var s string
		if json.Unmarshal(v, &s) != nil {
			return
		}
		if trim {
			s = strings.TrimSpace(s)
		}
		*dst = s
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	applyString("llmBaseUrl", &c.settings.LLMBaseURL, true)
	applyString("llmModel", &c.settings.LLMModel, true)
	if v, ok := overrides["llmResponseFormat"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil && (s == "standard" || s == "llm_jp_thinking") {
			c.settings.LLMResponseFormat = s
		}
	}
	applyString("reviewBaseUrl", &c.settings.ReviewBaseURL, true)
	applyString("reviewModel", &c.settings.ReviewModel, true)
	applyString("embeddingBaseUrl", &c.settings.EmbeddingBaseURL, true)
	applyString("embeddingModel", &c.settings.EmbeddingModel, true)
	applyString("imageDescriptionBaseUrl", &c.settings.ImageDescriptionBaseURL, true)
	applyString("imageDescriptionModel", &c.settings.ImageDescriptionModel, true)
	if v, ok := overrides[autoCheckUpdatesKey]; ok {
		var enabled bool
		if json.Unmarshal(v, &enabled) == nil {
			c.autoCheckUpdates = enabled
		}
	}
	if v, ok := overrides[llmReasoningKey]; ok {
		var reasoning map[string]map[string]string
		if json.Unmarshal(v, &reasoning) == nil {
			c.reasoning = reasoning
		}
	}

	// embeddingMode override + migration. When the key is present we honour a
	// valid value; an invalid value keeps the env default. When the key is ABSENT
	// from an existing app-config.json (a pre-Track-B user), migrate by intent: a
	// non-empty persisted embeddingModel means they configured an external
	// endpoint, so keep them on external; otherwise default to internal. (A fresh
	// install has no app-config.json, so this function returns early above and the
	// Defaults() value — internal — stands.)
	if v, ok := overrides["embeddingMode"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			if m := normalizeEmbeddingMode(s); m != "" {
				c.settings.EmbeddingMode = m
			}
		}
	} else if strings.TrimSpace(c.settings.EmbeddingModel) != "" {
		c.settings.EmbeddingMode = "external"
	} else {
		c.settings.EmbeddingMode = "internal"
	}
}

// Get returns the current settings snapshot. Reads are cheap and lock-free for the
// caller's purposes (a value copy under a short read lock).
//
// When EmbeddingMode=="internal" the bundled sidecar endpoint is overlaid onto the
// returned snapshot WITHOUT mutating the persisted EmbeddingBaseURL/Model — so the
// EmbeddingClient/retrieval/sync layers reach the sidecar with no changes. If the
// sidecar is not ready (internalEmbedModel==""), EmbeddingModel comes back empty,
// which the EmbeddingClient treats as disabled (retrieval degrades to FTS-only).
// External users' persisted endpoint is left untouched and still used in external
// mode.
func (c *Config) Get() Settings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := c.settings
	s.LLMReasoning = c.reasoning
	if s.EmbeddingMode == "internal" {
		s.EmbeddingBaseURL = c.internalEmbedURL
		s.EmbeddingModel = c.internalEmbedModel
		s.EmbeddingAPIKey = c.internalEmbedKey
	}
	return s
}

// SetInternalEmbedding points the internal-mode overlay at the bundled sidecar's
// loopback endpoint, the key that launch requires, and the model id. Called from
// the sidecar manager's ready callback.
func (c *Config) SetInternalEmbedding(baseURL, apiKey, model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.internalEmbedURL = baseURL
	c.internalEmbedKey = apiKey
	c.internalEmbedModel = model
}

// ClearInternalEmbedding drops the internal overlay (e.g. on sidecar crash or when
// switching to external mode), so internal mode degrades to FTS-only.
func (c *Config) ClearInternalEmbedding() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.internalEmbedURL = ""
	c.internalEmbedKey = ""
	c.internalEmbedModel = ""
}

// GetEditable returns just the user-editable subset, mirroring
// getEditableConfiguration.
func (c *Config) GetEditable() Editable {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.settings.Editable
}

// UpdateEditable trims and applies the seven editable fields, persists them to
// app-config.json, and returns the resulting editable view. Mirrors
// updateEditableConfiguration (which writes a 2-space-indented JSON file with a
// trailing newline).
func (c *Config) UpdateEditable(input Editable) (Editable, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settings.LLMBaseURL = strings.TrimSpace(input.LLMBaseURL)
	c.settings.LLMModel = strings.TrimSpace(input.LLMModel)
	c.settings.LLMResponseFormat = normalizeResponseFormat(input.LLMResponseFormat)
	c.settings.ReviewBaseURL = strings.TrimSpace(input.ReviewBaseURL)
	c.settings.ReviewModel = strings.TrimSpace(input.ReviewModel)
	c.settings.EmbeddingBaseURL = strings.TrimSpace(input.EmbeddingBaseURL)
	c.settings.EmbeddingModel = strings.TrimSpace(input.EmbeddingModel)
	c.settings.ImageDescriptionBaseURL = strings.TrimSpace(input.ImageDescriptionBaseURL)
	c.settings.ImageDescriptionModel = strings.TrimSpace(input.ImageDescriptionModel)
	// Apply embeddingMode only when the payload carries a valid value; an empty or
	// unrecognised value keeps the current mode so an older frontend that omits the
	// field cannot silently flip an external user back to internal.
	if m := normalizeEmbeddingMode(input.EmbeddingMode); m != "" {
		c.settings.EmbeddingMode = m
	}
	editable := c.settings.Editable

	// The write stays under the lock so the file always ends with the update that
	// was applied last in memory; writing after Unlock let two concurrent saves land
	// in the opposite order. The file is a few hundred bytes, so readers blocked on
	// Get for the duration of the write wait only briefly.
	if c.appConfigPath != "" {
		encoded, err := json.MarshalIndent(persistedFile{Editable: editable, AutoCheckUpdates: c.autoCheckUpdates, LLMReasoning: c.reasoning}, "", "  ")
		if err != nil {
			return editable, err
		}
		encoded = append(encoded, '\n')
		if err := writeFileAtomic(c.appConfigPath, encoded, 0o644); err != nil {
			return editable, err
		}
	}
	return editable, nil
}

// AutoCheckUpdates reports whether the app checks for a newer version at startup.
func (c *Config) AutoCheckUpdates() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.autoCheckUpdates
}

// SetAutoCheckUpdates saves whether the app checks for a newer version at startup.
// Only that key of app-config.json is rewritten: writing the whole editable view
// would pin the environment's connection defaults into a file that had none, and
// they would then outlive a later change to the environment. On error nothing
// changes, on disk or in memory.
func (c *Config) SetAutoCheckUpdates(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.rewriteKey(autoCheckUpdatesKey, json.RawMessage(strconv.FormatBool(enabled))); err != nil {
		return err
	}
	c.autoCheckUpdates = enabled
	return nil
}

// SetReasoning saves the reasoning setting of one model at one LM Studio API
// root; an empty value removes it, so the model runs at its own default again.
// Like SetAutoCheckUpdates, only that key of app-config.json is rewritten, and on
// error nothing changes.
func (c *Config) SetReasoning(apiRoot, modelKey, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := make(map[string]map[string]string, len(c.reasoning)+1)
	for root, models := range c.reasoning {
		next[root] = models
	}
	models := make(map[string]string, len(next[apiRoot])+1)
	for key, v := range next[apiRoot] {
		models[key] = v
	}
	if value == "" {
		delete(models, modelKey)
	} else {
		models[modelKey] = value
	}
	if len(models) == 0 {
		delete(next, apiRoot)
	} else {
		next[apiRoot] = models
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err := c.rewriteKey(llmReasoningKey, encoded); err != nil {
		return err
	}
	c.reasoning = next
	return nil
}

// rewriteKey replaces one top-level key of app-config.json, leaving the others as
// they are on disk. The caller holds c.mu.
func (c *Config) rewriteKey(key string, value json.RawMessage) error {
	if c.appConfigPath == "" {
		return nil
	}
	var fields map[string]json.RawMessage
	raw, err := os.ReadFile(c.appConfigPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	fields[key] = value
	encoded, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return writeFileAtomic(c.appConfigPath, encoded, 0o644)
}

// writeFileAtomic replaces path with data via a temporary file in the same
// directory and a rename, so a crash mid-write leaves either the old file or the
// new one — never a truncated file that the next launch would fail to parse.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := writeAndSync(tmp, data, perm); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func writeAndSync(f *os.File, data []byte, perm os.FileMode) (err error) {
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Chmod(perm); err != nil {
		return err
	}
	return f.Sync()
}
