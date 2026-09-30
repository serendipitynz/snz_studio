package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"snzstudio/internal/config"
)

// keyServer answers every endpoint the LLM, embedding and image description
// clients call, and records the Authorization header each request carried.
type keyServer struct {
	*httptest.Server
	mu   sync.Mutex
	auth []string
}

func newKeyServer(t *testing.T) *keyServer {
	t.Helper()
	ks := &keyServer{}
	ks.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ks.mu.Lock()
		ks.auth = append(ks.auth, r.Header.Get("Authorization"))
		ks.mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/chat/completions") && strings.Contains(string(body), `"stream":true`):
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
		case strings.HasSuffix(r.URL.Path, "/embeddings"):
			io.WriteString(w, `{"data":[{"embedding":[0.5]}]}`)
		case r.URL.Path == "/api/v1/models":
			io.WriteString(w, `{"models":[{"type":"llm","key":"m","loaded_instances":[]},{"type":"embedding","key":"e","loaded_instances":[]}]}`)
		case r.URL.Path == "/api/v1/models/load":
			io.WriteString(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "/models"):
			io.WriteString(w, `{"data":[{"id":"m"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ks.Close)
	return ks
}

// take returns the Authorization headers recorded since the last call.
func (ks *keyServer) take() []string {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	got := ks.auth
	ks.auth = nil
	return got
}

func wantAuth(t *testing.T, label string, got []string, want string) {
	t.Helper()
	if len(got) == 0 {
		t.Fatalf("%s: no request reached the server", label)
	}
	for _, header := range got {
		if header != want {
			t.Fatalf("%s: Authorization = %q, want %q", label, header, want)
		}
	}
}

func keySettings(defaultBase string) config.Settings {
	return config.Settings{
		Editable: config.Editable{
			LLMBaseURL:            defaultBase + "/v1",
			LLMModel:              "m",
			LLMResponseFormat:     "standard",
			ImageDescriptionModel: "vision",
			EmbeddingMode:         "external",
		},
		LLMAPIKey:                 "llm-secret",
		LLMTimeoutMs:              5000,
		EmbeddingTimeoutMs:        5000,
		ImageDescriptionTimeoutMs: 5000,
	}
}

// TestLLMKeyStaysWithDefaultOrigin covers every LLM request path that can be sent
// somewhere other than the default endpoint: a participant or review target, the
// model lists of the configuration screen, model loading and image description.
func TestLLMKeyStaysWithDefaultOrigin(t *testing.T) {
	home := newKeyServer(t)
	other := newKeyServer(t)
	const withKey = "Bearer llm-secret"

	for _, tc := range []struct {
		name   string
		server *keyServer
		want   string
	}{
		{"default origin", home, withKey},
		{"other origin", other, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := keySettings(home.URL)
			settings.ImageDescriptionBaseURL = tc.server.URL + "/v1"
			cfg := testConfig(settings)
			client := NewLLMClient(cfg)
			target := &CompletionTarget{BaseURL: tc.server.URL + "/v1", Model: "m"}
			tc.server.take()

			if _, err := client.CreateChatCompletion(ChatCompletionInput{UserInput: "hi", Target: target}); err != nil {
				t.Fatalf("completion: %v", err)
			}
			wantAuth(t, "completion (review)", tc.server.take(), tc.want)

			if _, err := client.CreateChatCompletionStream(ChatCompletionInput{UserInput: "hi", Target: target}, func(string) {}); err != nil {
				t.Fatalf("stream: %v", err)
			}
			wantAuth(t, "stream (participant, review)", tc.server.take(), tc.want)

			if _, err := client.ListModels(tc.server.URL + "/v1"); err != nil {
				t.Fatalf("list models: %v", err)
			}
			wantAuth(t, "model list", tc.server.take(), tc.want)

			if _, err := client.ListAvailableModels(tc.server.URL + "/v1"); err != nil {
				t.Fatalf("list available models: %v", err)
			}
			wantAuth(t, "available model list", tc.server.take(), tc.want)

			if !client.EnsureModelLoaded("m", tc.server.URL+"/v1") {
				t.Fatal("EnsureModelLoaded = false")
			}
			wantAuth(t, "model load", tc.server.take(), tc.want)

			if _, err := NewImageDescriptionService(cfg).DescribeImage(context.Background(), pngBytes); err != nil {
				t.Fatalf("describe image: %v", err)
			}
			wantAuth(t, "image description", tc.server.take(), tc.want)
		})
	}
}

func TestLLMKeyFollowsDefaultWhenTargetIsEmpty(t *testing.T) {
	home := newKeyServer(t)
	client := NewLLMClient(testConfig(keySettings(home.URL)))

	if _, err := client.CreateChatCompletion(ChatCompletionInput{UserInput: "hi"}); err != nil {
		t.Fatalf("completion: %v", err)
	}
	if _, err := client.ListModels(""); err != nil {
		t.Fatalf("list models: %v", err)
	}
	wantAuth(t, "default endpoint", home.take(), "Bearer llm-secret")
}

func TestEmbeddingKey(t *testing.T) {
	home := newKeyServer(t)
	other := newKeyServer(t)

	for _, tc := range []struct {
		name         string
		server       *keyServer
		embeddingKey string
		mode         string
		want         string
	}{
		{"explicit key goes to its endpoint", other, "emb-secret", "external", "Bearer emb-secret"},
		{"borrowed LLM key at the default origin", home, "", "external", "Bearer llm-secret"},
		{"borrowed LLM key withheld from another origin", other, "", "external", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := keySettings(home.URL)
			settings.EmbeddingBaseURL = tc.server.URL + "/v1"
			settings.EmbeddingModel = "e"
			settings.EmbeddingAPIKey = tc.embeddingKey
			settings.EmbeddingMode = tc.mode
			client := NewEmbeddingClient(testConfig(settings))
			tc.server.take()

			if got := client.CreateEmbedding("text"); got == nil {
				t.Fatal("CreateEmbedding returned nil")
			}
			if _, err := client.ListModels(""); err != nil {
				t.Fatalf("list models: %v", err)
			}
			wantAuth(t, "embedding", tc.server.take(), tc.want)
		})
	}
}

func TestEmbeddingKeyNeverReachesInternalSidecar(t *testing.T) {
	settings := keySettings("http://127.0.0.1:1234")
	settings.EmbeddingMode = "internal"
	settings.EmbeddingAPIKey = "emb-secret"
	dest, _ := url.Parse("http://127.0.0.1:1234/v1/embeddings")
	if got := embeddingAPIKeyFor(settings, dest); got != "" {
		t.Fatalf("internal mode key = %q, want empty", got)
	}
}

func TestSameOrigin(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"https://api.example.com/v1", "https://API.example.com:443/other", true},
		{"http://127.0.0.1:1234/v1", "http://127.0.0.1:1234/api/v1/models", true},
		{"http://example.com/v1", "http://example.com:80", true},
		{"http://127.0.0.1:1234/v1", "http://localhost:1234/v1", false},
		{"http://127.0.0.1:1234/v1", "http://127.0.0.1:1235/v1", false},
		{"https://api.example.com/v1", "http://api.example.com/v1", false},
		{"https://api.example.com/v1", "https://api.example.com.evil.test/v1", false},
		{"", "http://127.0.0.1:1234/v1", false},
	} {
		t.Run(fmt.Sprintf("%s vs %s", tc.a, tc.b), func(t *testing.T) {
			a, _ := url.Parse(tc.a)
			b, _ := url.Parse(tc.b)
			if got := sameOrigin(a, b); got != tc.want {
				t.Fatalf("sameOrigin = %v, want %v", got, tc.want)
			}
		})
	}
}
