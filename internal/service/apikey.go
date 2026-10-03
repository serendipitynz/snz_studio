package service

import (
	"errors"
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
// key's origin rule. In internal mode the only key is the bundled sidecar's
// per-launch one, sent to the sidecar's origin alone: a model-list request can
// still carry a URL the user typed for an external endpoint.
func embeddingAPIKeyFor(s config.Settings, dest *url.URL) string {
	if s.EmbeddingMode == "internal" {
		base, err := url.Parse(strings.TrimSpace(s.EmbeddingBaseURL))
		if err != nil || !sameOrigin(base, dest) {
			return ""
		}
		return s.EmbeddingAPIKey
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

// newKeyedHTTPClient builds the http.Client for requests that may carry an API
// key. net/http keeps Authorization across a redirect to the same hostname on
// another port or scheme, and to its subdomains — wider than the origin the key
// was checked against — so a redirect that leaves the first request's origin
// drops it.
func newKeyedHTTPClient() *http.Client {
	return &http.Client{CheckRedirect: keepAuthWithinOrigin}
}

func keepAuthWithinOrigin(req *http.Request, via []*http.Request) error {
	// Mirrors net/http's default limit, which a custom CheckRedirect replaces.
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if !sameOrigin(via[0].URL, req.URL) {
		req.Header.Del("Authorization")
	}
	return nil
}
