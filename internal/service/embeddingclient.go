package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"snzstudio/internal/config"
	"snzstudio/internal/embed"
)

// EmbeddingClient ports embeddingClient.ts: an OpenAI-compatible embedding client
// that disables itself when the endpoint cannot be reached (returning nil
// thereafter) so that retrieval silently degrades to FTS-only when no embedding
// endpoint is available. Unlike the TS client, a request the endpoint answers with
// an error does not disable it: one oversized input would otherwise stop every
// later embedding until the configuration is refreshed.
//
// The TS client relied on JS being single-threaded to guard its disabled flag;
// here a mutex protects the mutable state since Go callers may run concurrently.
type EmbeddingClient struct {
	cfg  *config.Config
	http *http.Client

	mu                sync.Mutex
	disabled          bool
	unavailableLogged bool
}

// NewEmbeddingClient builds an EmbeddingClient. It starts disabled when no
// embedding model is configured, mirroring `disabled = !config.embeddingModel.trim()`.
func NewEmbeddingClient(cfg *config.Config) *EmbeddingClient {
	s := cfg.Get()
	return &EmbeddingClient{
		cfg:      cfg,
		http:     &http.Client{},
		disabled: strings.TrimSpace(s.EmbeddingModel) == "",
	}
}

// IsEnabled reports whether embeddings are currently available.
func (c *EmbeddingClient) IsEnabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.disabled
}

// GetModel returns the trimmed embedding model id. Mirrors getModel().
func (c *EmbeddingClient) GetModel() string {
	return strings.TrimSpace(c.cfg.Get().EmbeddingModel)
}

// RefreshConfiguration re-evaluates the disabled flag after a configuration change.
// Mirrors refreshConfiguration().
func (c *EmbeddingClient) RefreshConfiguration() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disabled = strings.TrimSpace(c.cfg.Get().EmbeddingModel) == ""
	c.unavailableLogged = false
}

// PrefixScheme is the asymmetric query/document prefix pair for the active embedding
// model. The zero value (empty prefixes) is a no-op, used for models that do not
// require prefixes.
type PrefixScheme struct {
	Query    string
	Document string
}

// Active reports whether this scheme applies a prefix (non-empty).
func (p PrefixScheme) Active() bool { return p.Query != "" || p.Document != "" }

// ActivePrefixScheme returns the prefix scheme for the currently-active embedding
// model. It is non-empty only for the bundled ruri model (ModernBERT-Ja, which uses
// an asymmetric 検索クエリ:/検索文書: scheme); external/unknown models get a no-op
// scheme. The prefixes are baked into stored vectors, so changing them (or the
// model) requires a full RebuildAll. Matching on the model id also correctly applies
// the prefixes when a user points an external endpoint at the same ruri model.
func (c *EmbeddingClient) ActivePrefixScheme() PrefixScheme {
	if strings.TrimSpace(c.cfg.Get().EmbeddingModel) == embed.RuriV3_30m.ModelID {
		return PrefixScheme{Query: embed.RuriV3_30m.QueryPrefix, Document: embed.RuriV3_30m.DocumentPrefix}
	}
	return PrefixScheme{}
}

func (c *EmbeddingClient) authHeader(req *http.Request, apiKey string) {
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}

// ListModels mirrors the embedding listModels.
func (c *EmbeddingClient) ListModels(baseURL string) ([]string, error) {
	s := c.cfg.Get()
	if baseURL == "" {
		baseURL = s.EmbeddingBaseURL
	}
	timeout := time.Duration(minInt(s.EmbeddingTimeoutMs, 5000)) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stripTrailingSlash(baseURL)+"/models", nil)
	if err != nil {
		return nil, err
	}
	c.authHeader(req, s.EmbeddingAPIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return parseModelList("Embedding", resp.StatusCode, resp.Body)
}

// ListAvailableModels mirrors the embedding listAvailableModels (type=="embedding").
func (c *EmbeddingClient) ListAvailableModels(baseURL string) ([]string, error) {
	s := c.cfg.Get()
	if baseURL == "" {
		baseURL = s.EmbeddingBaseURL
	}
	timeout := time.Duration(minInt(s.EmbeddingTimeoutMs, 5000)) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, getLmStudioAPIRoot(baseURL)+"/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	c.authHeader(req, s.EmbeddingAPIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if !respOK(resp) {
		return nil, fmt.Errorf("Embedding available model request failed with %d", resp.StatusCode)
	}
	var data struct {
		Models []struct {
			Type string `json:"type"`
			Key  string `json:"key"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	out := []string{}
	for _, item := range data.Models {
		if item.Type == "embedding" && item.Key != "" {
			out = append(out, item.Key)
		}
	}
	return sortedStrings(out), nil
}

// EnsureModelLoaded mirrors the embedding ensureModelLoaded; on success it clears
// the disabled flag.
func (c *EmbeddingClient) EnsureModelLoaded(modelKey, baseURL string) bool {
	if strings.TrimSpace(modelKey) == "" {
		return false
	}
	s := c.cfg.Get()
	if baseURL == "" {
		baseURL = s.EmbeddingBaseURL
	}
	timeout := time.Duration(minInt(s.EmbeddingTimeoutMs, 20000)) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	root := getLmStudioAPIRoot(baseURL)
	listReq, err := http.NewRequestWithContext(ctx, http.MethodGet, root+"/api/v1/models", nil)
	if err != nil {
		return false
	}
	c.authHeader(listReq, s.EmbeddingAPIKey)
	listResp, err := c.http.Do(listReq)
	if err != nil {
		return false
	}
	defer listResp.Body.Close()
	if !respOK(listResp) {
		return false
	}
	var data struct {
		Models []struct {
			Type            string `json:"type"`
			Key             string `json:"key"`
			LoadedInstances []struct {
				ID string `json:"id"`
			} `json:"loaded_instances"`
		} `json:"models"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&data); err != nil {
		return false
	}

	var entry *struct {
		Type            string `json:"type"`
		Key             string `json:"key"`
		LoadedInstances []struct {
			ID string `json:"id"`
		} `json:"loaded_instances"`
	}
	for i := range data.Models {
		if data.Models[i].Type == "embedding" && data.Models[i].Key == modelKey {
			entry = &data.Models[i]
			break
		}
	}
	if entry == nil {
		return false
	}
	if len(entry.LoadedInstances) > 0 {
		return true
	}

	loadBody, _ := json.Marshal(map[string]string{"model": modelKey})
	loadReq, err := http.NewRequestWithContext(ctx, http.MethodPost, root+"/api/v1/models/load", bytes.NewReader(loadBody))
	if err != nil {
		return false
	}
	loadReq.Header.Set("Content-Type", "application/json")
	c.authHeader(loadReq, s.EmbeddingAPIKey)
	loadResp, err := c.http.Do(loadReq)
	if err != nil {
		return false
	}
	defer loadResp.Body.Close()
	if !respOK(loadResp) {
		return false
	}

	c.mu.Lock()
	c.disabled = false
	c.mu.Unlock()
	return true
}

// CheckConnection mirrors the embedding checkConnection.
func (c *EmbeddingClient) CheckConnection() bool {
	s := c.cfg.Get()
	if strings.TrimSpace(s.EmbeddingModel) == "" {
		return false
	}
	models, err := c.ListModels("")
	if err != nil {
		return false
	}
	return len(models) == 0 || containsString(models, s.EmbeddingModel)
}

// CreateEmbeddings mirrors createEmbeddings: returns the embedding vectors, or nil
// when embeddings are disabled, there is nothing to embed, or the request fails.
// Only a failure to reach the endpoint disables the client; an error response, a
// timeout or a malformed body fails this request alone. It never returns an error
// to the caller — callers treat nil as "skip", exactly like the TS `null` return.
func (c *EmbeddingClient) CreateEmbeddings(inputs []string) [][]float64 {
	cleaned := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if trimmed := strings.TrimSpace(input); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	c.mu.Lock()
	disabled := c.disabled
	c.mu.Unlock()
	if disabled || len(cleaned) == 0 {
		return nil
	}

	s := c.cfg.Get()
	body, err := json.Marshal(map[string]any{
		"model": s.EmbeddingModel,
		"input": cleaned,
	})
	if err != nil {
		c.disable(err)
		return nil
	}

	timeout := time.Duration(s.EmbeddingTimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stripTrailingSlash(s.EmbeddingBaseURL)+"/embeddings", bytes.NewReader(body))
	if err != nil {
		c.disable(err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	c.authHeader(req, s.EmbeddingAPIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logRequestFailure(err)
		} else {
			c.disable(err)
		}
		return nil
	}
	defer resp.Body.Close()
	if !respOK(resp) {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		logRequestFailure(fmt.Errorf("Embedding request failed with %d: %s", resp.StatusCode, bytes.TrimSpace(detail)))
		return nil
	}

	var data struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		logRequestFailure(err)
		return nil
	}

	embeddings := make([][]float64, 0, len(data.Data))
	for _, item := range data.Data {
		embeddings = append(embeddings, item.Embedding)
	}
	if len(embeddings) != len(cleaned) {
		logRequestFailure(fmt.Errorf("Embedding response did not contain valid vectors"))
		return nil
	}
	for _, embedding := range embeddings {
		if len(embedding) == 0 {
			logRequestFailure(fmt.Errorf("Embedding response did not contain valid vectors"))
			return nil
		}
	}
	return embeddings
}

// CreateEmbedding mirrors createEmbedding: a single-input convenience that returns
// nil when embeddings are unavailable.
func (c *EmbeddingClient) CreateEmbedding(input string) []float64 {
	embeddings := c.CreateEmbeddings([]string{input})
	if len(embeddings) == 0 {
		return nil
	}
	return embeddings[0]
}

func logRequestFailure(err error) {
	log.Printf("Embedding request failed (embeddings stay enabled): %v", err)
}

// disable marks the client disabled and logs the reason once, mirroring the catch
// block of createEmbeddings.
func (c *EmbeddingClient) disable(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disabled = true
	if !c.unavailableLogged {
		log.Printf("Embedding retrieval disabled: %v", err)
		c.unavailableLogged = true
	}
}
