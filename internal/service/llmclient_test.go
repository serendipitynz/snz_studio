package service

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/llmresponse"
)

func testConfig(settings config.Settings) *config.Config {
	return config.New(settings, "")
}

func TestEstimateTokenCount(t *testing.T) {
	if got := estimateTokenCount("   "); got != 0 {
		t.Fatalf("blank => %d, want 0", got)
	}
	// "alpha beta gamma delta epsilon": wordLike=5 but the 30-char length term
	// dominates => ceil(30/4)=8.
	if got := estimateTokenCount("alpha beta gamma delta epsilon"); got != 8 {
		t.Fatalf("five words => %d, want 8 (char-length term wins)", got)
	}
	// Japanese: 9 kana => max(ceil(9/1.8)=5, ceil(9/4)=3, wordLike=1) = 5.
	if got := estimateTokenCount("あいうえおかきくけ"); got != 5 {
		t.Fatalf("nine kana => %d, want 5", got)
	}
}

func TestBuildGenerationMetrics(t *testing.T) {
	// Endpoint-reported tokens win over the estimate.
	tokensReported := int64(120)
	respMs, tokens, tps := buildGenerationMetrics("ignored", 2000, &tokensReported)
	if respMs != 2000 || tokens != 120 {
		t.Fatalf("metrics = (%d, %d, %v), want respMs 2000 tokens 120", respMs, tokens, tps)
	}
	if tps != 60 { // 120 tokens / 2s
		t.Fatalf("tokensPerSecond = %v, want 60", tps)
	}

	// elapsed rounds up to a 1ms floor; zero tokens => zero tps.
	respMs, tokens, tps = buildGenerationMetrics("", 0.2, int64Ptr(0))
	if respMs != 1 || tokens != 0 || tps != 0 {
		t.Fatalf("floor metrics = (%d, %d, %v), want (1, 0, 0)", respMs, tokens, tps)
	}
}

func TestGetLmStudioAPIRoot(t *testing.T) {
	cases := map[string]string{
		"http://127.0.0.1:1234/v1":     "http://127.0.0.1:1234",
		"http://127.0.0.1:1234/v1/":    "http://127.0.0.1:1234",
		"http://127.0.0.1:1234/api/v1": "http://127.0.0.1:1234",
		"http://host/openai/v1":        "http://host/openai",
		"http://host:5000":             "http://host:5000",
	}
	for input, want := range cases {
		if got := getLmStudioAPIRoot(input); got != want {
			t.Fatalf("getLmStudioAPIRoot(%q) = %q, want %q", input, got, want)
		}
	}
}

// sseServer starts a fake OpenAI-compatible /chat/completions endpoint that streams
// the given raw SSE body, so the parser can be exercised without LM Studio.
func sseServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func streamSettings(baseURL, format string) config.Settings {
	return config.Settings{
		Editable: config.Editable{
			LLMBaseURL:        baseURL,
			LLMModel:          "test-model",
			LLMResponseFormat: format,
		},
		LLMTimeoutMs: 5000,
	}
}

func TestCreateChatCompletionStream(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":", world"}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"!"}}],"usage":{"completion_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
		"",
	}, "\n")
	srv := sseServer(t, body)
	client := NewLLMClient(testConfig(streamSettings(srv.URL, llmresponse.FormatStandard)))

	var deltas []string
	result, err := client.CreateChatCompletionStream(ChatCompletionInput{
		SystemPrompt: "sys",
		UserInput:    "hi",
	}, func(chunk string) { deltas = append(deltas, chunk) })
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if result.Content != "Hello, world!" {
		t.Fatalf("content = %q, want %q", result.Content, "Hello, world!")
	}
	if strings.Join(deltas, "") != "Hello, world!" {
		t.Fatalf("deltas joined = %q, want %q", strings.Join(deltas, ""), "Hello, world!")
	}
	if result.OutputTokens != 7 {
		t.Fatalf("outputTokens = %d, want 7 (from usage)", result.OutputTokens)
	}
	if result.ModelName != "test-model" {
		t.Fatalf("modelName = %q, want test-model", result.ModelName)
	}
}

func TestCreateChatCompletionStreamLLMJPThinking(t *testing.T) {
	// The analysis channel must not surface; only the final answer streams out.
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"<|channel|>analysis<|message|>secret reasoning"}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"<|channel|>final<|message|>Visible "}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"answer"}}]}`,
		"",
		"data: [DONE]",
		"",
		"",
	}, "\n")
	srv := sseServer(t, body)
	client := NewLLMClient(testConfig(streamSettings(srv.URL, llmresponse.FormatLLMJPThinking)))

	var deltas []string
	result, err := client.CreateChatCompletionStream(ChatCompletionInput{UserInput: "hi"}, func(chunk string) {
		deltas = append(deltas, chunk)
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if result.Content != "Visible answer" {
		t.Fatalf("content = %q, want %q", result.Content, "Visible answer")
	}
	joined := strings.Join(deltas, "")
	if joined != "Visible answer" {
		t.Fatalf("streamed deltas = %q, want %q (analysis must be hidden)", joined, "Visible answer")
	}
	if strings.Contains(joined, "secret") {
		t.Fatalf("analysis content leaked into stream: %q", joined)
	}
}

func TestCreateChatCompletionNonStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"  Final answer  "}}],"usage":{"completion_tokens":3}}`)
	}))
	t.Cleanup(srv.Close)
	client := NewLLMClient(testConfig(streamSettings(srv.URL, llmresponse.FormatStandard)))

	result, err := client.CreateChatCompletion(ChatCompletionInput{UserInput: "hi"})
	if err != nil {
		t.Fatalf("completion error: %v", err)
	}
	if result.Content != "Final answer" {
		t.Fatalf("content = %q, want trimmed %q", result.Content, "Final answer")
	}
	if result.OutputTokens != 3 {
		t.Fatalf("outputTokens = %d, want 3", result.OutputTokens)
	}
}

func TestCreateChatCompletionErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	client := NewLLMClient(testConfig(streamSettings(srv.URL, llmresponse.FormatStandard)))

	if _, err := client.CreateChatCompletion(ChatCompletionInput{UserInput: "hi"}); err == nil {
		t.Fatal("expected error on 500 status, got nil")
	}
}
