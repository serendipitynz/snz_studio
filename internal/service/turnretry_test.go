package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"snzstudio/internal/config"
	"snzstudio/internal/llmresponse"
	"snzstudio/internal/model"
)

// scriptedReply is one /chat/completions answer: an SSE body, or a status with
// no stream, or a hang that outlasts the client's sliding timeout.
type scriptedReply struct {
	body   string
	status int
	hang   bool
}

// scriptedLLMServer answers each completion request with the next scripted
// reply, and records the prompts it was sent. Its /models and /api/v1 routes
// behave like turnLLMServer's.
type scriptedLLMServer struct {
	*httptest.Server

	mu       sync.Mutex
	replies  []scriptedReply
	requests []capturedRequest
}

func newScriptedLLMServer(t *testing.T, replies ...scriptedReply) *scriptedLLMServer {
	t.Helper()
	s := &scriptedLLMServer{replies: replies}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/"):
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/models"):
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[]}`)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			var body capturedRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.mu.Lock()
			index := len(s.requests)
			s.requests = append(s.requests, body)
			s.mu.Unlock()
			if index >= len(s.replies) {
				http.Error(w, "no scripted reply left", http.StatusInternalServerError)
				return
			}
			reply := s.replies[index]
			switch {
			case reply.hang:
				select {
				case <-r.Context().Done():
				case <-time.After(5 * time.Second):
				}
			case reply.status != 0:
				http.Error(w, "scripted failure", reply.status)
			default:
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, reply.body)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *scriptedLLMServer) captured() []capturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]capturedRequest(nil), s.requests...)
}

// emptyAnswer is a reasoning-only stream, the shape gemma-4-e4b was recorded
// returning (buildTurnPrompt), ending the way LM Studio ends a finished one.
func emptyAnswer(t *testing.T) scriptedReply {
	return scriptedReply{body: reasoningSSEBody(t, []map[string]string{{"reasoning": "誰が話すべきか考える"}}, 9, 9)}
}

func answer(t *testing.T, content string) scriptedReply {
	return scriptedReply{body: reasoningSSEBody(t, []map[string]string{{"reasoning": "考える"}, {"content": content}}, 6, 2)}
}

func collectDeltas(deltas *[]StreamDelta) func(StreamDelta) {
	return func(delta StreamDelta) { *deltas = append(*deltas, delta) }
}

func restarts(deltas []StreamDelta) int {
	n := 0
	for _, delta := range deltas {
		if delta.Restart {
			n++
		}
	}
	return n
}

// TestTurnEngineRetriesEmptyCompletion covers TASK-112 AC #1, the retry that
// succeeds: an empty answer is generated once more by the same speaker from the
// same prompt, the stream is told to start over first, and only the second
// answer is stored.
func TestTurnEngineRetriesEmptyCompletion(t *testing.T) {
	g := newTurnGraph(t)
	srv := newScriptedLLMServer(t, emptyAnswer(t), answer(t, "二度目の発言"))
	chat, roster := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "場面", srv.URL, "A", "B")

	var deltas []StreamDelta
	message, err := g.engine.RunTurn(chat.ID, "", nil, collectDeltas(&deltas))
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if message.Content != "二度目の発言" || message.Reasoning != "考える" {
		t.Fatalf("stored (%q, reasoning %q), want the second answer alone", message.Content, message.Reasoning)
	}
	if message.ParticipantID == nil || *message.ParticipantID != roster[0].ID {
		t.Fatalf("stored speaker = %v, want %s: the retry must not move the turn order", message.ParticipantID, roster[0].ID)
	}

	requests := srv.captured()
	if len(requests) != 2 {
		t.Fatalf("completion requests = %d, want 2", len(requests))
	}
	if !reflect.DeepEqual(requests[0], requests[1]) {
		t.Fatalf("the retry sent a different request:\n first: %+v\nsecond: %+v", requests[0], requests[1])
	}

	if restarts(deltas) != 1 {
		t.Fatalf("restarts = %d, want 1 in %+v", restarts(deltas), deltas)
	}
	var restartAt int
	for i, delta := range deltas {
		if delta.Restart {
			restartAt = i
		}
	}
	if restartAt == 0 || !deltas[0].Reasoning {
		t.Fatalf("deltas = %+v, want the first attempt's reasoning before the restart", deltas)
	}

	stored, err := g.chats.ListMessages(chat.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored messages = %d, want 1", len(stored))
	}
}

// TestTurnEngineFailsAfterTwoEmptyCompletions covers TASK-112 AC #1, the retry
// that is empty too: the turn fails after the second attempt, with no third,
// and nothing enters the transcript.
func TestTurnEngineFailsAfterTwoEmptyCompletions(t *testing.T) {
	g := newTurnGraph(t)
	srv := newScriptedLLMServer(t, emptyAnswer(t), emptyAnswer(t), answer(t, "三度目は要らない"))
	chat, _ := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "場面", srv.URL, "A", "B")

	var deltas []StreamDelta
	_, err := g.engine.RunTurn(chat.ID, "", nil, collectDeltas(&deltas))
	if !errors.Is(err, ErrEmptyCompletion) {
		t.Fatalf("RunTurn error = %v, want ErrEmptyCompletion", err)
	}
	if n := len(srv.captured()); n != 2 {
		t.Fatalf("completion requests = %d, want 2", n)
	}
	if restarts(deltas) != 1 {
		t.Fatalf("restarts = %d, want 1", restarts(deltas))
	}
	stored, err := g.chats.ListMessages(chat.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("stored messages = %+v, want none", stored)
	}
}

// TestTurnEngineDoesNotRetryOtherFailures covers TASK-112 AC #2: a failure that
// is not an empty answer fails the turn on the first attempt.
func TestTurnEngineDoesNotRetryOtherFailures(t *testing.T) {
	cases := []struct {
		name  string
		reply scriptedReply
	}{
		{"endpoint refuses the request", scriptedReply{status: http.StatusInternalServerError}},
		{"stream times out", scriptedReply{hang: true}},
		{"stream sends invalid JSON", scriptedReply{body: "data: {not json\n\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newTurnGraphWithConfig(t, newServiceTestDB(t), testConfig(config.Settings{
				Editable:     config.Editable{LLMBaseURL: "http://unused.invalid/v1", LLMModel: "default-model"},
				LLMTimeoutMs: 200,
			}))
			srv := newScriptedLLMServer(t, tc.reply, answer(t, "再試行してはいけない"))
			chat, _ := g.newMultiAgentChat(t, model.TurnRuleRoundRobin, "場面", srv.URL, "A", "B")

			var deltas []StreamDelta
			_, err := g.engine.RunTurn(chat.ID, "", nil, collectDeltas(&deltas))
			if err == nil || errors.Is(err, ErrEmptyCompletion) {
				t.Fatalf("RunTurn error = %v, want the endpoint's failure", err)
			}
			if n := len(srv.captured()); n != 1 {
				t.Fatalf("completion requests = %d, want 1", n)
			}
			if restarts(deltas) != 0 {
				t.Fatalf("restarts = %d, want 0", restarts(deltas))
			}
			stored, err := g.chats.ListMessages(chat.ID)
			if err != nil {
				t.Fatalf("ListMessages: %v", err)
			}
			if len(stored) != 0 {
				t.Fatalf("stored messages = %+v, want none", stored)
			}
		})
	}
}

// TestCreateChatCompletionStreamEmptyAnswer pins what an empty answer reports:
// ErrEmptyCompletion, with the finish reason and how much raw text arrived, so
// the log can tell a reasoning-only answer from markup that shaped to nothing.
func TestCreateChatCompletionStreamEmptyAnswer(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning":"考える"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"content":"<think></think>"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: {"choices":[],"usage":{"completion_tokens":4,"completion_tokens_details":{"reasoning_tokens":2}}}`,
		"data: [DONE]",
		"",
	}, "\n\n")
	srv := sseServer(t, body)
	client := NewLLMClient(testConfig(streamSettings(srv.URL, llmresponse.FormatStandard)))

	_, err := client.CreateChatCompletionStream(ChatCompletionInput{UserInput: "hi"}, func(StreamDelta) {})
	var empty *EmptyCompletionError
	if !errors.As(err, &empty) || !errors.Is(err, ErrEmptyCompletion) {
		t.Fatalf("error = %v, want an *EmptyCompletionError", err)
	}
	if err.Error() != "LLM stream did not contain message content" {
		t.Fatalf("message = %q, want the wording the turn error has always shown", err.Error())
	}
	if empty.FinishReason != "stop" || empty.ContentChars != len("<think></think>") || empty.ReasoningChars != 3 ||
		empty.CompletionTokens == nil || *empty.CompletionTokens != 4 || empty.ReasoningTokens == nil || *empty.ReasoningTokens != 2 {
		t.Fatalf("detail = %s", empty.Detail())
	}
}
