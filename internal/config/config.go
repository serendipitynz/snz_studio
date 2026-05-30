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
	"os"
	"strconv"
	"strings"
	"sync"
)

// Editable mirrors EditableAppConfiguration: the fields the configuration endpoint
// can read and write, and the only fields persisted to app-config.json.
type Editable struct {
	LLMBaseURL        string `json:"llmBaseUrl"`
	LLMModel          string `json:"llmModel"`
	LLMResponseFormat string `json:"llmResponseFormat"`
	ReviewBaseURL     string `json:"reviewBaseUrl"`
	ReviewModel       string `json:"reviewModel"`
	EmbeddingBaseURL  string `json:"embeddingBaseUrl"`
	EmbeddingModel    string `json:"embeddingModel"`
}

// Settings is an immutable snapshot of every config value the service layer reads.
// It embeds Editable and adds the runtime-only values (API keys, timeouts, debug
// flags) that are sourced from the environment and never persisted.
type Settings struct {
	Editable
	LLMAPIKey          string
	LLMTimeoutMs       int
	EmbeddingAPIKey    string
	EmbeddingTimeoutMs int
	DebugChatFlow      bool
	DebugRetrieval     bool
}

// Config holds the current Settings snapshot and the path it persists edits to.
type Config struct {
	mu            sync.RWMutex
	settings      Settings
	appConfigPath string
}

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

// Defaults builds the Settings from environment variables with the same defaults
// and fallback chains as config.ts. It does not read app-config.json.
func Defaults() Settings {
	llmBaseURL := getenv("LLM_BASE_URL", "http://127.0.0.1:1234/v1")
	llmTimeout := parseIntEnv(getenv("LLM_TIMEOUT_MS", ""), 60000)
	return Settings{
		Editable: Editable{
			LLMBaseURL:        llmBaseURL,
			LLMModel:          getenv("LLM_MODEL", "local-model"),
			LLMResponseFormat: normalizeResponseFormat(getenv("LLM_RESPONSE_FORMAT", "standard")),
			ReviewBaseURL:     firstNonEmptyEnv("http://127.0.0.1:1234/v1", "REVIEW_BASE_URL", "LLM_BASE_URL"),
			ReviewModel:       firstNonEmptyEnv("local-model", "REVIEW_MODEL", "LLM_MODEL"),
			EmbeddingBaseURL:  firstNonEmptyEnv("http://127.0.0.1:1234/v1", "EMBEDDING_BASE_URL", "LLM_BASE_URL"),
			EmbeddingModel:    getenv("EMBEDDING_MODEL", ""),
		},
		LLMAPIKey:          getenv("LLM_API_KEY", ""),
		LLMTimeoutMs:       llmTimeout,
		EmbeddingAPIKey:    firstNonEmptyEnv("", "EMBEDDING_API_KEY", "LLM_API_KEY"),
		EmbeddingTimeoutMs: parseIntEnv(firstNonEmptyEnv("", "EMBEDDING_TIMEOUT_MS", "LLM_TIMEOUT_MS"), 60000),
		DebugChatFlow:      parseBoolFlag(getenv("DEBUG_CHAT_FLOW", "")),
		DebugRetrieval:     parseBoolFlag(getenv("DEBUG_RETRIEVAL", "")),
	}
}

// New constructs a Config around an explicit Settings snapshot (used by tests and
// by the bootstrap once it has resolved the data directory).
func New(settings Settings, appConfigPath string) *Config {
	settings.LLMResponseFormat = normalizeResponseFormat(settings.LLMResponseFormat)
	return &Config{settings: settings, appConfigPath: appConfigPath}
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
		return
	}
	var overrides map[string]json.RawMessage
	if err := json.Unmarshal(raw, &overrides); err != nil {
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
}

// Get returns the current settings snapshot. Reads are cheap and lock-free for the
// caller's purposes (a value copy under a short read lock).
func (c *Config) Get() Settings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.settings
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
	c.settings.LLMBaseURL = strings.TrimSpace(input.LLMBaseURL)
	c.settings.LLMModel = strings.TrimSpace(input.LLMModel)
	c.settings.LLMResponseFormat = normalizeResponseFormat(input.LLMResponseFormat)
	c.settings.ReviewBaseURL = strings.TrimSpace(input.ReviewBaseURL)
	c.settings.ReviewModel = strings.TrimSpace(input.ReviewModel)
	c.settings.EmbeddingBaseURL = strings.TrimSpace(input.EmbeddingBaseURL)
	c.settings.EmbeddingModel = strings.TrimSpace(input.EmbeddingModel)
	editable := c.settings.Editable
	path := c.appConfigPath
	c.mu.Unlock()

	if path != "" {
		encoded, err := json.MarshalIndent(editable, "", "  ")
		if err != nil {
			return editable, err
		}
		encoded = append(encoded, '\n')
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			return editable, err
		}
	}
	return editable, nil
}
