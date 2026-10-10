package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"snzstudio/internal/config"
	"snzstudio/internal/llmresponse"
)

// lmStudioModels is an /api/v1/models reply carrying the shapes seen on a real
// LM Studio: on/off and graded options, a null reasoning, an embedding model, and
// one model listed twice with the formats disagreeing.
const lmStudioModels = `{"models":[
	{"type":"llm","key":"google/gemma-4-12b","capabilities":{"reasoning":{"allowed_options":["off","on"],"default":"on"}}},
	{"type":"llm","key":"openai/gpt-oss-20b","capabilities":{"reasoning":{"allowed_options":["low","medium","high"],"default":"low"}}},
	{"type":"llm","key":"qwen3.8-27b","capabilities":{"reasoning":null}},
	{"type":"llm","key":"gemma-4-e4b-it-qat","capabilities":{"reasoning":{"allowed_options":["off","on"],"default":"on"}}},
	{"type":"llm","key":"gemma-4-e4b-it-qat","capabilities":{"reasoning":null}},
	{"type":"embedding","key":"text-embedding","capabilities":{"reasoning":{"allowed_options":["off","on"],"default":"on"}}}
]}`

// recordingLLM is a fake LM Studio: it serves lmStudioModels and records the raw
// body of every /chat/completions request.
type recordingLLM struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []map[string]json.RawMessage
}

func newRecordingLLM(t *testing.T) *recordingLLM {
	t.Helper()
	rec := &recordingLLM{}
	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/models":
			io.WriteString(w, lmStudioModels)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			var body map[string]json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.mu.Lock()
			rec.bodies = append(rec.bodies, body)
			rec.mu.Unlock()
			if string(body["stream"]) == "true" {
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
				return
			}
			io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(rec.Close)
	return rec
}

func (r *recordingLLM) lastReasoningEffort(t *testing.T) (string, bool) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.bodies) == 0 {
		t.Fatal("no completion request was made")
	}
	raw, ok := r.bodies[len(r.bodies)-1]["reasoning_effort"]
	if !ok {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("reasoning_effort is not a string: %s", raw)
	}
	return value, true
}

func TestChosenReasoningIsSentAsReasoningEffort(t *testing.T) {
	llm := newRecordingLLM(t)
	client := NewLLMClient(testConfig(streamSettings(llm.URL+"/v1", llmresponse.FormatStandard)))

	cases := []struct {
		model, chosen, want string
	}{
		{"google/gemma-4-12b", "off", "none"},
		{"google/gemma-4-12b", "on", "medium"},
		{"openai/gpt-oss-20b", "high", "high"},
	}
	for _, tc := range cases {
		if err := client.SetReasoning(llm.URL+"/v1", tc.model, tc.chosen); err != nil {
			t.Fatalf("SetReasoning(%s, %s): %v", tc.model, tc.chosen, err)
		}
		target := &CompletionTarget{Model: tc.model}
		if _, err := client.CreateChatCompletion(ChatCompletionInput{UserInput: "hi", Target: target}); err != nil {
			t.Fatalf("completion: %v", err)
		}
		if got, ok := llm.lastReasoningEffort(t); !ok || got != tc.want {
			t.Fatalf("%s chosen %s: reasoning_effort = %q (present %v), want %q", tc.model, tc.chosen, got, ok, tc.want)
		}
		if _, err := client.CreateChatCompletionStream(ChatCompletionInput{UserInput: "hi", Target: target}, func(StreamDelta) {}); err != nil {
			t.Fatalf("stream: %v", err)
		}
		if got, ok := llm.lastReasoningEffort(t); !ok || got != tc.want {
			t.Fatalf("%s chosen %s (stream): reasoning_effort = %q (present %v), want %q", tc.model, tc.chosen, got, ok, tc.want)
		}
	}
}

// A participant's turn reaches the client as a resolved target, so the choice is
// looked up by the target's endpoint and model, not the workspace's.
func TestChosenReasoningFollowsTheTarget(t *testing.T) {
	workspace := newRecordingLLM(t)
	participant := newRecordingLLM(t)
	client := NewLLMClient(testConfig(streamSettings(workspace.URL+"/v1", llmresponse.FormatStandard)))
	if err := client.SetReasoning(participant.URL+"/v1/", "google/gemma-4-12b", "off"); err != nil {
		t.Fatalf("SetReasoning: %v", err)
	}

	target := &CompletionTarget{BaseURL: participant.URL + "/v1", Model: "google/gemma-4-12b"}
	if _, err := client.CreateChatCompletionStream(ChatCompletionInput{UserInput: "hi", Target: target}, func(StreamDelta) {}); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if got, ok := participant.lastReasoningEffort(t); !ok || got != "none" {
		t.Fatalf("participant endpoint: reasoning_effort = %q (present %v), want none", got, ok)
	}

	// The same model name at the workspace endpoint has nothing chosen.
	target = &CompletionTarget{Model: "google/gemma-4-12b"}
	if _, err := client.CreateChatCompletion(ChatCompletionInput{UserInput: "hi", Target: target}); err != nil {
		t.Fatalf("completion: %v", err)
	}
	if got, ok := workspace.lastReasoningEffort(t); ok {
		t.Fatalf("workspace endpoint: reasoning_effort = %q, want it left out", got)
	}
}

func TestReasoningEffortIsLeftOutWithoutAChoice(t *testing.T) {
	llm := newRecordingLLM(t)
	settings := streamSettings(llm.URL+"/v1", llmresponse.FormatStandard)
	settings.LLMModel = "google/gemma-4-12b"
	client := NewLLMClient(testConfig(settings))
	if err := client.SetReasoning(llm.URL+"/v1", "openai/gpt-oss-20b", "high"); err != nil {
		t.Fatalf("SetReasoning: %v", err)
	}

	if _, err := client.CreateChatCompletion(ChatCompletionInput{UserInput: "hi"}); err != nil {
		t.Fatalf("completion: %v", err)
	}
	if got, ok := llm.lastReasoningEffort(t); ok {
		t.Fatalf("a model with nothing chosen sent reasoning_effort %q", got)
	}

	// Clearing a choice goes back to the model's default.
	if err := client.SetReasoning(llm.URL+"/v1", "openai/gpt-oss-20b", ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	target := &CompletionTarget{Model: "openai/gpt-oss-20b"}
	if _, err := client.CreateChatCompletionStream(ChatCompletionInput{UserInput: "hi", Target: target}, func(StreamDelta) {}); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if got, ok := llm.lastReasoningEffort(t); ok {
		t.Fatalf("a cleared choice still sent reasoning_effort %q", got)
	}
}

// An endpoint that is not LM Studio has no /api/v1/models, so nothing can be
// chosen for it and its requests never carry the parameter.
func TestReasoningCannotBeChosenAtAnEndpointOtherThanLMStudio(t *testing.T) {
	var sawReasoning bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, sawReasoning = body["reasoning_effort"]
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	t.Cleanup(srv.Close)
	client := NewLLMClient(testConfig(streamSettings(srv.URL+"/v1", llmresponse.FormatStandard)))

	err := client.SetReasoning(srv.URL+"/v1", "test-model", "off")
	if !errors.Is(err, ErrReasoningNotOffered) {
		t.Fatalf("SetReasoning at a non-LM Studio endpoint = %v, want ErrReasoningNotOffered", err)
	}
	choices, err := client.ListReasoningChoices(srv.URL + "/v1")
	if err == nil && len(choices) != 0 {
		t.Fatalf("a non-LM Studio endpoint listed reasoning choices: %v", choices)
	}
	if _, err := client.CreateChatCompletion(ChatCompletionInput{UserInput: "hi"}); err != nil {
		t.Fatalf("completion: %v", err)
	}
	if sawReasoning {
		t.Fatal("a request to a non-LM Studio endpoint carried reasoning_effort")
	}
}

func TestSetReasoningRejectsAValueTheModelDoesNotOffer(t *testing.T) {
	llm := newRecordingLLM(t)
	client := NewLLMClient(testConfig(streamSettings(llm.URL+"/v1", llmresponse.FormatStandard)))
	for _, tc := range []struct{ model, value string }{
		{"google/gemma-4-12b", "high"},
		{"openai/gpt-oss-20b", "off"},
		{"qwen3.8-27b", "off"},
		{"unknown-model", "on"},
	} {
		if err := client.SetReasoning(llm.URL+"/v1", tc.model, tc.value); !errors.Is(err, ErrReasoningNotOffered) {
			t.Fatalf("SetReasoning(%s, %s) = %v, want ErrReasoningNotOffered", tc.model, tc.value, err)
		}
	}
}

func TestListReasoningChoicesMatchesAllowedOptions(t *testing.T) {
	llm := newRecordingLLM(t)
	client := NewLLMClient(testConfig(streamSettings(llm.URL+"/v1", llmresponse.FormatStandard)))
	if err := client.SetReasoning(llm.URL+"/v1", "google/gemma-4-12b", "off"); err != nil {
		t.Fatalf("SetReasoning: %v", err)
	}

	choices, err := client.ListReasoningChoices(llm.URL + "/v1")
	if err != nil {
		t.Fatalf("ListReasoningChoices: %v", err)
	}
	want := map[string]ReasoningChoice{
		"google/gemma-4-12b": {AllowedOptions: []string{"off", "on"}, Default: "on", Selected: "off"},
		"openai/gpt-oss-20b": {AllowedOptions: []string{"low", "medium", "high"}, Default: "low"},
		"gemma-4-e4b-it-qat": {AllowedOptions: []string{"off", "on"}, Default: "on"},
	}
	if fmt.Sprint(choices) != fmt.Sprint(want) {
		t.Fatalf("choices = %v, want %v (null reasoning and embedding models left out)", choices, want)
	}
}

func TestReasoningEffortForIgnoresAnUnknownStoredValue(t *testing.T) {
	s := config.Settings{LLMReasoning: map[string]map[string]string{"http://h": {"m": "turbo"}}}
	if got := reasoningEffortFor(s, "http://h/v1", "m"); got != "" {
		t.Fatalf("reasoningEffortFor = %q, want empty for a value outside OpenAI's vocabulary", got)
	}
}
