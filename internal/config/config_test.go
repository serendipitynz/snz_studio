package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsEnvFallbacks(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://example/v1")
	t.Setenv("LLM_TIMEOUT_MS", "1234")
	// REVIEW_BASE_URL / EMBEDDING_BASE_URL unset => fall back to LLM_BASE_URL.
	os.Unsetenv("REVIEW_BASE_URL")
	os.Unsetenv("EMBEDDING_BASE_URL")
	os.Unsetenv("EMBEDDING_TIMEOUT_MS")
	t.Setenv("LLM_API_KEY", "secret")

	s := Defaults()
	if s.LLMBaseURL != "http://example/v1" {
		t.Fatalf("LLMBaseURL = %q", s.LLMBaseURL)
	}
	if s.ReviewBaseURL != "http://example/v1" || s.EmbeddingBaseURL != "http://example/v1" {
		t.Fatalf("review/embedding base should fall back to LLM base, got %q / %q", s.ReviewBaseURL, s.EmbeddingBaseURL)
	}
	if s.LLMTimeoutMs != 1234 || s.EmbeddingTimeoutMs != 1234 {
		t.Fatalf("timeouts = %d / %d, want 1234 (embedding falls back to LLM)", s.LLMTimeoutMs, s.EmbeddingTimeoutMs)
	}
	if s.EmbeddingAPIKey != "secret" {
		t.Fatalf("embedding api key should fall back to LLM key, got %q", s.EmbeddingAPIKey)
	}
	if s.LLMResponseFormat != "standard" {
		t.Fatalf("default response format = %q, want standard", s.LLMResponseFormat)
	}
}

func TestLoadAppliesOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app-config.json")
	if err := os.WriteFile(path, []byte(`{"llmModel":"  override-model  ","llmResponseFormat":"llm_jp_thinking","embeddingModel":"emb"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := Load(path)
	got := c.GetEditable()
	if got.LLMModel != "override-model" {
		t.Fatalf("llmModel = %q, want trimmed override-model", got.LLMModel)
	}
	if got.LLMResponseFormat != "llm_jp_thinking" {
		t.Fatalf("llmResponseFormat = %q, want llm_jp_thinking", got.LLMResponseFormat)
	}
	if got.EmbeddingModel != "emb" {
		t.Fatalf("embeddingModel = %q, want emb", got.EmbeddingModel)
	}
}

func TestUpdateEditablePersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app-config.json")
	c := New(Defaults(), path)

	updated, err := c.UpdateEditable(Editable{
		LLMBaseURL:        "  http://h/v1  ",
		LLMModel:          " m ",
		LLMResponseFormat: "weird-value",
		EmbeddingModel:    " e ",
	})
	if err != nil {
		t.Fatalf("UpdateEditable: %v", err)
	}
	if updated.LLMBaseURL != "http://h/v1" || updated.LLMModel != "m" || updated.EmbeddingModel != "e" {
		t.Fatalf("fields not trimmed: %+v", updated)
	}
	if updated.LLMResponseFormat != "standard" {
		t.Fatalf("invalid response format should normalize to standard, got %q", updated.LLMResponseFormat)
	}

	// The file should be valid JSON reflecting the update.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	var persisted Editable
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("persisted config is not valid JSON: %v", err)
	}
	if persisted.LLMModel != "m" {
		t.Fatalf("persisted llmModel = %q, want m", persisted.LLMModel)
	}

	// A freshly loaded config should observe the persisted values.
	if reloaded := Load(path).GetEditable(); reloaded.LLMModel != "m" {
		t.Fatalf("reloaded llmModel = %q, want m", reloaded.LLMModel)
	}
}

func TestEmbeddingModeDefaultInternal(t *testing.T) {
	os.Unsetenv("EMBEDDING_MODE")
	os.Unsetenv("EMBEDDING_MODEL")
	if s := Defaults(); s.EmbeddingMode != "internal" {
		t.Fatalf("default EmbeddingMode = %q, want internal", s.EmbeddingMode)
	}
}

func TestInternalOverlayAndDegrade(t *testing.T) {
	os.Unsetenv("EMBEDDING_MODE")
	os.Unsetenv("EMBEDDING_MODEL")
	c := New(Defaults(), "") // internal mode, no persisted external model

	// Before the sidecar is ready, internal mode must degrade to FTS-only: Get's
	// EmbeddingModel comes back empty regardless of any persisted base URL.
	if got := c.Get(); got.EmbeddingModel != "" {
		t.Fatalf("not-ready internal EmbeddingModel = %q, want empty", got.EmbeddingModel)
	}

	c.SetInternalEmbedding("http://127.0.0.1:9999/v1", "ruri-v3-30m")
	got := c.Get()
	if got.EmbeddingModel != "ruri-v3-30m" || got.EmbeddingBaseURL != "http://127.0.0.1:9999/v1" || got.EmbeddingAPIKey != "" {
		t.Fatalf("ready internal overlay = %+v, want sidecar url/model and empty key", got.Editable)
	}

	c.ClearInternalEmbedding()
	if got := c.Get(); got.EmbeddingModel != "" {
		t.Fatalf("after clear, internal EmbeddingModel = %q, want empty", got.EmbeddingModel)
	}
}

func TestExternalModeNoOverlay(t *testing.T) {
	s := Defaults()
	s.EmbeddingMode = "external"
	s.EmbeddingBaseURL = "http://ext/v1"
	s.EmbeddingModel = "ext-model"
	c := New(s, "")

	// Even with an internal overlay set, external mode must keep the persisted
	// external endpoint/model.
	c.SetInternalEmbedding("http://internal/v1", "ruri-v3-30m")
	got := c.Get()
	if got.EmbeddingModel != "ext-model" || got.EmbeddingBaseURL != "http://ext/v1" {
		t.Fatalf("external mode overlaid = %+v, want persisted ext values", got.Editable)
	}
}

func TestEmbeddingModeMigration(t *testing.T) {
	os.Unsetenv("EMBEDDING_MODE")
	os.Unsetenv("EMBEDDING_MODEL")
	write := func(t *testing.T, body string) string {
		p := filepath.Join(t.TempDir(), "app-config.json")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		return p
	}

	// Legacy external user (embeddingModel set, no embeddingMode) => external.
	if m := Load(write(t, `{"embeddingModel":"my-ext","embeddingBaseUrl":"http://ext/v1"}`)).GetEditable().EmbeddingMode; m != "external" {
		t.Fatalf("legacy-with-model migration = %q, want external", m)
	}
	// Legacy user with no embedding model and no mode => internal.
	if m := Load(write(t, `{"llmModel":"m"}`)).GetEditable().EmbeddingMode; m != "internal" {
		t.Fatalf("legacy-no-model migration = %q, want internal", m)
	}
	// An explicit embeddingMode wins over the migration heuristic.
	if m := Load(write(t, `{"embeddingModel":"x","embeddingMode":"internal"}`)).GetEditable().EmbeddingMode; m != "internal" {
		t.Fatalf("explicit mode = %q, want internal", m)
	}
}

func TestEmbeddingModeRoundTripAndKeepCurrent(t *testing.T) {
	os.Unsetenv("EMBEDDING_MODE")
	os.Unsetenv("EMBEDDING_MODEL")
	path := filepath.Join(t.TempDir(), "app-config.json")
	c := New(Defaults(), path)

	if _, err := c.UpdateEditable(Editable{LLMBaseURL: "http://h/v1", EmbeddingMode: "external", EmbeddingModel: "e"}); err != nil {
		t.Fatalf("UpdateEditable: %v", err)
	}
	if m := Load(path).GetEditable().EmbeddingMode; m != "external" {
		t.Fatalf("round-trip EmbeddingMode = %q, want external", m)
	}

	// An empty/omitted mode in a later update must not flip the current mode.
	if _, err := c.UpdateEditable(Editable{LLMBaseURL: "http://h/v1", EmbeddingMode: ""}); err != nil {
		t.Fatalf("UpdateEditable(empty mode): %v", err)
	}
	if m := c.GetEditable().EmbeddingMode; m != "external" {
		t.Fatalf("empty-mode update flipped mode to %q, want external kept", m)
	}
}

func TestImageDescriptionSettings(t *testing.T) {
	t.Setenv("LLM_MODEL", "chat-model")
	t.Setenv("LLM_TIMEOUT_MS", "1234")
	os.Unsetenv("IMAGE_DESCRIPTION_BASE_URL")
	os.Unsetenv("IMAGE_DESCRIPTION_MODEL")
	os.Unsetenv("IMAGE_DESCRIPTION_TIMEOUT_MS")

	s := Defaults()
	// The model must not inherit LLM_MODEL (that would describe images with a
	// model that may not see them), and the timeout must not inherit LLM_TIMEOUT_MS.
	if s.ImageDescriptionModel != "" || s.ImageDescriptionBaseURL != "" {
		t.Fatalf("image description = %q / %q, want both empty", s.ImageDescriptionBaseURL, s.ImageDescriptionModel)
	}
	if s.ImageDescriptionTimeoutMs != DefaultImageDescriptionTimeoutMs {
		t.Fatalf("timeout = %d, want %d", s.ImageDescriptionTimeoutMs, DefaultImageDescriptionTimeoutMs)
	}

	t.Setenv("IMAGE_DESCRIPTION_TIMEOUT_MS", "5000")
	if got := Defaults().ImageDescriptionTimeoutMs; got != 5000 {
		t.Fatalf("timeout = %d, want 5000 from IMAGE_DESCRIPTION_TIMEOUT_MS", got)
	}

	path := filepath.Join(t.TempDir(), "app-config.json")
	c := New(Defaults(), path)
	editable := c.GetEditable()
	editable.ImageDescriptionBaseURL = " http://vision/v1 "
	editable.ImageDescriptionModel = " gemma "
	if _, err := c.UpdateEditable(editable); err != nil {
		t.Fatalf("UpdateEditable: %v", err)
	}
	reloaded := Load(path).Get()
	if reloaded.ImageDescriptionBaseURL != "http://vision/v1" || reloaded.ImageDescriptionModel != "gemma" {
		t.Fatalf("reloaded = %q / %q, want trimmed persisted values", reloaded.ImageDescriptionBaseURL, reloaded.ImageDescriptionModel)
	}
}
