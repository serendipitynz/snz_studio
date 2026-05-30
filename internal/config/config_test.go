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
