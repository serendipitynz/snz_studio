package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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

	var shown streamDisplay
	result, err := client.CreateChatCompletionStream(ChatCompletionInput{
		SystemPrompt: "sys",
		UserInput:    "hi",
	}, shown.apply)
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if result.Content != "Hello, world!" {
		t.Fatalf("content = %q, want %q", result.Content, "Hello, world!")
	}
	if shown.text != "Hello, world!" || shown.replaces != 0 {
		t.Fatalf("shown = %q after %d replaces, want %q from appends only", shown.text, shown.replaces, "Hello, world!")
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

	var shown streamDisplay
	var history []string
	result, err := client.CreateChatCompletionStream(ChatCompletionInput{UserInput: "hi"}, func(delta StreamDelta) {
		shown.apply(delta)
		history = append(history, shown.text)
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if result.Content != "Visible answer" {
		t.Fatalf("content = %q, want %q", result.Content, "Visible answer")
	}
	if shown.text != "Visible answer" {
		t.Fatalf("shown = %q, want %q (analysis must be hidden)", shown.text, "Visible answer")
	}
	for _, text := range history {
		if strings.Contains(text, "secret") {
			t.Fatalf("analysis content leaked into stream: %q", text)
		}
	}
}

// streamDisplay applies StreamDeltas the way the frontend does: a delta is
// appended, a replacement stands in for everything shown before it. The answer
// and the reasoning are kept apart, as the frontend draws them apart.
type streamDisplay struct {
	text      string
	reasoning string
	replaces  int
}

func (d *streamDisplay) apply(delta StreamDelta) {
	target := &d.text
	if delta.Reasoning {
		target = &d.reasoning
	}
	if delta.Replace {
		*target = delta.Text
		d.replaces++
		return
	}
	*target += delta.Text
}

// sseBody frames each content fragment as its own completion chunk event.
func sseBody(t *testing.T, fragments []string) string {
	t.Helper()
	var b strings.Builder
	for _, fragment := range fragments {
		payload, err := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"delta": map[string]string{"content": fragment}}},
		})
		if err != nil {
			t.Fatalf("marshal chunk: %v", err)
		}
		b.WriteString("data: " + string(payload) + "\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

// TestCreateChatCompletionStreamFollowsVisibleText covers TASK-82 AC #2: when the
// visible text shrinks or changes course mid-stream (a tag fragment shown as text
// until the rest of the tag arrives; text shown before a late channel tag), what
// the stream has shown after each chunk is exactly the visible text of the
// response so far — nothing dropped, nothing doubled — and ends as the result.
func TestCreateChatCompletionStreamFollowsVisibleText(t *testing.T) {
	cases := []struct {
		name      string
		format    string
		fragments []string
		want      string
	}{
		{
			name:      "standard: end tag split across chunks",
			format:    llmresponse.FormatStandard,
			fragments: []string{"Hello <|en", "d|> wor", "ld", " and more"},
			want:      "Hello  world and more",
		},
		{
			name:      "standard: message tag split three ways",
			format:    llmresponse.FormatStandard,
			fragments: []string{"前置き<", "|mess", "age|>本文", "の続き"},
			want:      "前置き本文の続き",
		},
		{
			name:   "llm_jp_thinking: text shown before a late final channel",
			format: llmresponse.FormatLLMJPThinking,
			fragments: []string{
				"Draft", " text", "<|chan", "nel|>final<|message|>Real ", "answer",
			},
			want: "Real answer",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := sseServer(t, sseBody(t, tc.fragments))
			client := NewLLMClient(testConfig(streamSettings(srv.URL, tc.format)))

			var shown streamDisplay
			var history []string
			result, err := client.CreateChatCompletionStream(ChatCompletionInput{UserInput: "hi"}, func(delta StreamDelta) {
				shown.apply(delta)
				history = append(history, shown.text)
			})
			if err != nil {
				t.Fatalf("stream error: %v", err)
			}
			if result.Content != tc.want {
				t.Fatalf("content = %q, want %q", result.Content, tc.want)
			}
			if shown.text != result.Content {
				t.Fatalf("shown at the end = %q, want the result %q", shown.text, result.Content)
			}
			if shown.replaces == 0 {
				t.Fatalf("no replacement sent; the case does not exercise a shrinking visible text")
			}

			var want []string
			var raw, last string
			for _, fragment := range tc.fragments {
				raw += fragment
				if visible := llmresponse.ParseAssistantResponse(raw, tc.format); visible != last {
					want = append(want, visible)
					last = visible
				}
			}
			if !reflect.DeepEqual(history, want) {
				t.Fatalf("shown after each change = %q, want the visible text so far %q", history, want)
			}
		})
	}
}

// TestCreateChatCompletionStreamBoundsUnseparatedData covers TASK-82 AC #1: an
// endpoint that keeps sending data with no "\n\n" separator fails the stream
// once the pending bytes pass the bound, instead of being buffered for as long
// as it keeps sending.
func TestCreateChatCompletionStreamBoundsUnseparatedData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		line := strings.Repeat("x", 64<<10) + "\n"
		for sent := 0; sent <= 4*maxStreamEventBytes; sent += len(line) {
			if _, err := io.WriteString(w, line); err != nil {
				return // the client gave up, as it should
			}
		}
	}))
	t.Cleanup(srv.Close)
	client := NewLLMClient(testConfig(streamSettings(srv.URL, llmresponse.FormatStandard)))

	_, err := client.CreateChatCompletionStream(ChatCompletionInput{UserInput: "hi"}, func(StreamDelta) {})
	if err == nil || !strings.Contains(err.Error(), "without an event separator") {
		t.Fatalf("err = %v, want the unseparated-data bound", err)
	}
}

// TestCreateChatCompletionStreamLongResponse checks the bound applies to one
// pending event, not to the response: a stream whose events add up to well past
// it still completes.
func TestCreateChatCompletionStreamLongResponse(t *testing.T) {
	fragment := strings.Repeat("あ", 32<<10)
	var fragments []string
	for len(fragments)*len(fragment) <= maxStreamEventBytes+maxStreamEventBytes/4 {
		fragments = append(fragments, fragment)
	}
	srv := sseServer(t, sseBody(t, fragments))
	client := NewLLMClient(testConfig(streamSettings(srv.URL, llmresponse.FormatStandard)))

	result, err := client.CreateChatCompletionStream(ChatCompletionInput{UserInput: "hi"}, func(StreamDelta) {})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if want := strings.Join(fragments, ""); result.Content != want {
		t.Fatalf("content has %d bytes, want %d", len(result.Content), len(want))
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

// TestParseModelList covers the replies a local endpoint actually gives, including
// the one that used to read as success: LM Studio answers a base URL missing its
// /v1 suffix with 200 and an error body.
func TestParseModelList(t *testing.T) {
	models, err := parseModelList("LLM", 200, strings.NewReader(`{"data":[{"id":"beta"},{"id":""},{"id":"alpha"}]}`))
	if err != nil {
		t.Fatalf("well-formed list => %v", err)
	}
	if len(models) != 2 || models[0] != "alpha" || models[1] != "beta" {
		t.Fatalf("models = %v, want [alpha beta]", models)
	}

	// An endpoint serving no models is a reachable endpoint, so `data: []` stays a
	// success — and must marshal as [] rather than null.
	models, err = parseModelList("LLM", 200, strings.NewReader(`{"object":"list","data":[]}`))
	if err != nil {
		t.Fatalf("empty list => %v", err)
	}
	if models == nil {
		t.Fatal("empty list returned a nil slice, which marshals to JSON null")
	}
	if len(models) != 0 {
		t.Fatalf("empty list => %v, want no models", models)
	}

	// LM Studio, base URL missing /v1.
	_, err = parseModelList("LLM", 200, strings.NewReader(`{"error":"Unexpected endpoint or method. (GET /models)"}`))
	if err == nil {
		t.Fatal("200 with an error body was accepted as a model list")
	}
	if !strings.Contains(err.Error(), "Unexpected endpoint or method") {
		t.Fatalf("error = %q, want the endpoint's own wording", err)
	}

	// A 200 that is neither a list nor an error: no `data` field to read.
	_, err = parseModelList("LLM", 200, strings.NewReader(`{"object":"list"}`))
	if err == nil {
		t.Fatal("200 without a data field was accepted as a model list")
	}

	// Non-2xx keeps the status and gains the server's wording, in OpenAI's shape.
	_, err = parseModelList("LLM", 404, strings.NewReader(`{"error":{"message":"no such route","type":"invalid_request_error"}}`))
	if err == nil {
		t.Fatal("404 was accepted as a model list")
	}
	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "no such route") {
		t.Fatalf("error = %q, want both the status and the server's message", err)
	}

	// Non-2xx with an unreadable body still reports the status.
	_, err = parseModelList("LLM", 502, strings.NewReader("<html>bad gateway</html>"))
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("error = %v, want the 502 status", err)
	}

	// An aggregator's list runs to megabytes, well past any size a reply from a
	// single local server would suggest. Truncating one would surface as a parse
	// error and be reported to the user as a failed connection.
	entries := make([]string, 0, 2000)
	for i := 0; i < 2000; i++ {
		entries = append(entries, fmt.Sprintf(
			`{"id":"vendor/model-%04d","object":"model","description":%q,"pricing":{"prompt":"0.000001"}}`,
			i, strings.Repeat("long description ", 8)))
	}
	large := `{"object":"list","data":[` + strings.Join(entries, ",") + `]}`
	if len(large) < 64<<10 {
		t.Fatalf("fixture is %d bytes, too small to exercise a large reply", len(large))
	}
	models, err = parseModelList("LLM", 200, strings.NewReader(large))
	if err != nil {
		t.Fatalf("large list => %v", err)
	}
	if len(models) != 2000 {
		t.Fatalf("large list => %d models, want 2000", len(models))
	}
}

// reasoningSSEBody frames each delta object as its own completion chunk, then a
// usage chunk shaped like LM Studio's.
func reasoningSSEBody(t *testing.T, deltas []map[string]string, completionTokens, reasoningTokens int64) string {
	t.Helper()
	var b strings.Builder
	for _, delta := range deltas {
		payload, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta}}})
		if err != nil {
			t.Fatalf("marshal chunk: %v", err)
		}
		b.WriteString("data: " + string(payload) + "\n\n")
	}
	fmt.Fprintf(&b, `data: {"choices":[],"usage":{"completion_tokens":%d,"completion_tokens_details":{"reasoning_tokens":%d}}}`+"\n\n", completionTokens, reasoningTokens)
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

// TestCreateChatCompletionStreamSeparatesReasoning covers TASK-99 AC #1: the
// reasoning LM Studio sends in delta.reasoning_content (gemma) or delta.reasoning
// (gpt-oss), and the reasoning a model writes between <think> tags, stream as
// reasoning and never reach the answer.
func TestCreateChatCompletionStreamSeparatesReasoning(t *testing.T) {
	cases := []struct {
		name   string
		deltas []map[string]string
	}{
		{"reasoning_content", []map[string]string{
			{"reasoning_content": "Weigh"}, {"reasoning_content": " the options."},
			{"content": "The answer"}, {"content": " is 391."},
		}},
		{"reasoning", []map[string]string{
			{"reasoning": "Weigh"}, {"reasoning": " the options."},
			{"content": "The answer"}, {"content": " is 391."},
		}},
		{"think tags in the content", []map[string]string{
			{"content": "<thi"}, {"content": "nk>Weigh"}, {"content": " the options.</th"},
			{"content": "ink>\n\nThe answer"}, {"content": " is 391."},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := sseServer(t, reasoningSSEBody(t, tc.deltas, 12, 5))
			client := NewLLMClient(testConfig(streamSettings(srv.URL, llmresponse.FormatStandard)))

			var shown streamDisplay
			var answerBeforeReasoningEnded bool
			result, err := client.CreateChatCompletionStream(ChatCompletionInput{UserInput: "hi"}, func(delta StreamDelta) {
				shown.apply(delta)
				if shown.text != "" && !strings.HasSuffix(shown.reasoning, "options.") {
					answerBeforeReasoningEnded = true
				}
			})
			if err != nil {
				t.Fatalf("stream error: %v", err)
			}
			if result.Content != "The answer is 391." || shown.text != result.Content {
				t.Fatalf("content = %q, shown = %q, want both %q", result.Content, shown.text, "The answer is 391.")
			}
			if result.Reasoning != "Weigh the options." || shown.reasoning != result.Reasoning {
				t.Fatalf("reasoning = %q, shown = %q, want both %q", result.Reasoning, shown.reasoning, "Weigh the options.")
			}
			if answerBeforeReasoningEnded {
				t.Fatal("answer text was shown while the reasoning was still incomplete")
			}
			if result.OutputTokens != 12 || result.ReasoningTokens != 5 {
				t.Fatalf("tokens = %d (reasoning %d), want 12 (reasoning 5) from usage", result.OutputTokens, result.ReasoningTokens)
			}
		})
	}
}

func TestCreateChatCompletionNonStreamSeparatesReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Final answer","reasoning_content":"Think first."}}],"usage":{"completion_tokens":9}}`)
	}))
	t.Cleanup(srv.Close)
	client := NewLLMClient(testConfig(streamSettings(srv.URL, llmresponse.FormatStandard)))

	result, err := client.CreateChatCompletion(ChatCompletionInput{UserInput: "hi"})
	if err != nil {
		t.Fatalf("completion error: %v", err)
	}
	if result.Content != "Final answer" || result.Reasoning != "Think first." {
		t.Fatalf("content = %q, reasoning = %q", result.Content, result.Reasoning)
	}
	// No reasoning_tokens reported: the share is estimated from the text.
	if want := estimateTokenCount("Think first."); result.ReasoningTokens != want {
		t.Fatalf("reasoningTokens = %d, want the estimate %d", result.ReasoningTokens, want)
	}
}
