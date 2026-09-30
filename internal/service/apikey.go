package service

import (
	"net/http"
	"net/url"
	"strings"

	"snzstudio/internal/config"
)

// LLM_API_KEY is bound to the origin of the default LLM endpoint rather than sent
// to whatever URL a request targets: participant endpoints arrive in preset files
// written by somebody else, and the review, image description and model-list
// endpoints are free-form, so without the binding the key follows any of them.

// llmAPIKeyFor returns LLM_API_KEY when dest shares the scheme, host and port of
// the default LLM endpoint, and "" otherwise.
func llmAPIKeyFor(s config.Settings, dest *url.URL) string {
	if s.LLMAPIKey == "" {
		return ""
	}
	base, err := url.Parse(strings.TrimSpace(s.LLMBaseURL))
	if err != nil || !sameOrigin(base, dest) {
		return ""
	}
	return s.LLMAPIKey
}

// embeddingAPIKeyFor returns EMBEDDING_API_KEY when it is set: the owner configured
// it for the embedding endpoint alone. Unset, it borrows LLM_API_KEY under the LLM
// key's origin rule. The bundled sidecar never receives a key.
func embeddingAPIKeyFor(s config.Settings, dest *url.URL) string {
	if s.EmbeddingMode == "internal" {
		return ""
	}
	if s.EmbeddingAPIKey != "" {
		return s.EmbeddingAPIKey
	}
	return llmAPIKeyFor(s, dest)
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil || a.Host == "" || b.Host == "" {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func setBearer(req *http.Request, apiKey string) {
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}
