package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"snzstudio/internal/config"
	"snzstudio/internal/llmresponse"
	"snzstudio/internal/model"
)

// ChatCompletionResult mirrors the ChatCompletionResult interface returned by the
// LLM client. The metric fields are always populated (never nil) here; ChatService
// is responsible for deciding when to persist them as null.
type ChatCompletionResult struct {
	Content         string
	ResponseMs      int64
	OutputTokens    int64
	TokensPerSecond float64
	ModelName       string
}

// CompletionTarget overrides the base URL and/or model for a single completion,
// mirroring the optional `target` argument (used by the review service).
type CompletionTarget struct {
	BaseURL string
	Model   string
}

// ChatCompletionInput mirrors the createChatCompletion / createChatCompletionStream
// input object. Temperature is a pointer so a nil value selects the 0.25 default,
// mirroring `input.temperature ?? 0.25`.
type ChatCompletionInput struct {
	SystemPrompt string
	Messages     []model.Message
	UserInput    string
	Temperature  *float64
	Target       *CompletionTarget
}

// LLMClient ports llmClient.ts: an OpenAI-compatible chat client built on
// net/http + context (in place of fetch + AbortController). It reads the live
// config snapshot per call so a configuration change takes effect immediately.
type LLMClient struct {
	cfg  *config.Config
	http *http.Client
}

// NewLLMClient builds an LLMClient. The http.Client has no timeout of its own —
// per-request deadlines are enforced through the request context (a sliding
// deadline for streaming).
func NewLLMClient(cfg *config.Config) *LLMClient {
	return &LLMClient{cfg: cfg, http: &http.Client{}}
}

var (
	reWordLike = regexp.MustCompile(`[\p{L}\p{N}_-]+`)
	reJapanese = regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}ー]`)
)

// estimateTokenCount mirrors estimateTokenCount: a rough token estimate used when
// the endpoint did not report usage.completion_tokens.
func estimateTokenCount(input string) int64 {
	normalized := strings.TrimSpace(input)
	if normalized == "" {
		return 0
	}
	wordLike := int64(len(reWordLike.FindAllString(normalized, -1)))
	japaneseChars := int64(len(reJapanese.FindAllString(normalized, -1)))
	runeLen := int64(utf8.RuneCountInString(normalized))

	jp := int64(math.Ceil(float64(japaneseChars) / 1.8))
	byChars := int64(math.Ceil(float64(runeLen) / 4.0))
	best := wordLike
	if jp > best {
		best = jp
	}
	if byChars > best {
		best = byChars
	}
	return best
}

// buildGenerationMetrics mirrors buildGenerationMetrics. elapsedMs is wall-clock
// milliseconds; outputTokens is the endpoint-reported count when available.
func buildGenerationMetrics(content string, elapsedMs float64, outputTokens *int64) (responseMs, tokens int64, tokensPerSecond float64) {
	responseMs = int64(math.Round(elapsedMs))
	if responseMs < 1 {
		responseMs = 1
	}
	if outputTokens != nil {
		tokens = *outputTokens
	} else {
		tokens = estimateTokenCount(content)
	}
	if tokens < 0 {
		tokens = 0
	}
	if tokens > 0 {
		tokensPerSecond = round2(float64(tokens) / (float64(responseMs) / 1000.0))
	}
	return responseMs, tokens, tokensPerSecond
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatCompletionBody struct {
	Model         string         `json:"model"`
	Temperature   float64        `json:"temperature"`
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
	Messages      []chatMessage  `json:"messages"`
}

// resolveTarget mirrors `input.target?.x?.trim() || config.x`.
func resolveTarget(target *CompletionTarget, defaultBaseURL, defaultModel string) (baseURL, modelName string) {
	baseURL, modelName = defaultBaseURL, defaultModel
	if target != nil {
		if v := strings.TrimSpace(target.Model); v != "" {
			modelName = v
		}
		if v := strings.TrimSpace(target.BaseURL); v != "" {
			baseURL = v
		}
	}
	return baseURL, modelName
}

func temperatureOrDefault(t *float64) float64 {
	if t == nil {
		return 0.25
	}
	return *t
}

// buildMessages assembles the system + history + user messages, sanitising each
// content with the configured response format. Mirrors the body.messages array.
func buildMessages(input ChatCompletionInput, format string) []chatMessage {
	msgs := make([]chatMessage, 0, len(input.Messages)+2)
	msgs = append(msgs, chatMessage{Role: "system", Content: llmresponse.SanitizePromptContent(input.SystemPrompt, "system", format)})
	for _, m := range input.Messages {
		msgs = append(msgs, chatMessage{Role: m.Role, Content: llmresponse.SanitizePromptContent(m.Content, m.Role, format)})
	}
	msgs = append(msgs, chatMessage{Role: "user", Content: llmresponse.SanitizePromptContent(input.UserInput, "user", format)})
	return msgs
}

func (c *LLMClient) authHeader(req *http.Request, apiKey string) {
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}

func respOK(resp *http.Response) bool {
	return statusOK(resp.StatusCode)
}

func statusOK(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

func isContextTimeout(err error, ctx context.Context) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
		ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled
}

// CreateChatCompletion performs a non-streaming completion. Mirrors
// createChatCompletion.
func (c *LLMClient) CreateChatCompletion(input ChatCompletionInput) (*ChatCompletionResult, error) {
	s := c.cfg.Get()
	baseURL, modelName := resolveTarget(input.Target, s.LLMBaseURL, s.LLMModel)
	body := chatCompletionBody{
		Model:       modelName,
		Temperature: temperatureOrDefault(input.Temperature),
		Messages:    buildMessages(input, s.LLMResponseFormat),
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	timeout := time.Duration(s.LLMTimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stripTrailingSlash(baseURL)+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authHeader(req, s.LLMAPIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		if isContextTimeout(err, ctx) {
			return nil, fmt.Errorf("LLM request timed out after %d ms", s.LLMTimeoutMs)
		}
		return nil, err
	}
	defer resp.Body.Close()
	if !respOK(resp) {
		return nil, fmt.Errorf("LLM request failed with %d", resp.StatusCode)
	}

	var data struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			CompletionTokens *int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		if isContextTimeout(err, ctx) {
			return nil, fmt.Errorf("LLM request timed out after %d ms", s.LLMTimeoutMs)
		}
		return nil, err
	}

	rawContent := ""
	if len(data.Choices) > 0 {
		rawContent = strings.TrimSpace(data.Choices[0].Message.Content)
	}
	content := ""
	if rawContent != "" {
		content = llmresponse.ParseAssistantResponse(rawContent, s.LLMResponseFormat)
	}
	if content == "" {
		return nil, errors.New("LLM response did not contain message content")
	}

	var completionTokens *int64
	if data.Usage != nil {
		completionTokens = data.Usage.CompletionTokens
	}
	elapsed := float64(time.Since(start).Microseconds()) / 1000.0
	respMs, tokens, tps := buildGenerationMetrics(content, elapsed, completionTokens)
	return &ChatCompletionResult{
		Content:         content,
		ResponseMs:      respMs,
		OutputTokens:    tokens,
		TokensPerSecond: tps,
		ModelName:       modelName,
	}, nil
}

// CreateChatCompletionStream performs a streaming completion, invoking onDelta with
// each newly-visible fragment. Mirrors createChatCompletionStream, including the
// sliding timeout that resets on every received chunk.
func (c *LLMClient) CreateChatCompletionStream(input ChatCompletionInput, onDelta func(string)) (*ChatCompletionResult, error) {
	s := c.cfg.Get()
	baseURL, modelName := resolveTarget(input.Target, s.LLMBaseURL, s.LLMModel)
	body := chatCompletionBody{
		Model:         modelName,
		Temperature:   temperatureOrDefault(input.Temperature),
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
		Messages:      buildMessages(input, s.LLMResponseFormat),
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	// Sliding deadline: a timer cancels the context if no chunk arrives within the
	// timeout window, and every read resets it. Mirrors resetTimeout().
	timeout := time.Duration(s.LLMTimeoutMs) * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.AfterFunc(timeout, cancel)
	defer timer.Stop()
	resetTimeout := func() { timer.Reset(timeout) }

	start := time.Now()
	resetTimeout()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stripTrailingSlash(baseURL)+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authHeader(req, s.LLMAPIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		if isContextTimeout(err, ctx) {
			return nil, fmt.Errorf("LLM stream timed out after %d ms without receiving data", s.LLMTimeoutMs)
		}
		return nil, err
	}
	defer resp.Body.Close()
	if !respOK(resp) {
		return nil, fmt.Errorf("LLM request failed with %d", resp.StatusCode)
	}

	var (
		rawContent       strings.Builder
		visibleContent   string
		completionTokens *int64
	)

	handleChunk := func(chunk string) error {
		var dataLines []string
		for _, line := range reLineSplit.Split(chunk, -1) {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			value := strings.TrimLeftFunc(line[len("data:"):], unicode.IsSpace)
			if value != "" {
				dataLines = append(dataLines, value)
			}
		}
		if len(dataLines) == 0 {
			return nil
		}
		dataText := strings.Join(dataLines, "\n")
		if dataText == "[DONE]" {
			return nil
		}

		var payload struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				CompletionTokens *int64 `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(dataText), &payload); err != nil {
			return fmt.Errorf("LLM stream returned invalid JSON: %w", err)
		}
		if payload.Usage != nil && payload.Usage.CompletionTokens != nil {
			ct := *payload.Usage.CompletionTokens
			completionTokens = &ct
		}
		delta := ""
		if len(payload.Choices) > 0 {
			delta = payload.Choices[0].Delta.Content
		}
		if delta == "" {
			return nil
		}
		rawContent.WriteString(delta)
		nextVisible := llmresponse.ParseAssistantResponse(rawContent.String(), s.LLMResponseFormat)
		visibleDelta := sliceFromRune(nextVisible, utf8.RuneCountInString(visibleContent))
		visibleContent = nextVisible
		if visibleDelta != "" {
			onDelta(visibleDelta)
		}
		return nil
	}

	separator := []byte("\n\n")
	var buffer []byte
	tmp := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(tmp)
		if n > 0 {
			resetTimeout()
			buffer = append(buffer, tmp[:n]...)
			for {
				idx := bytes.Index(buffer, separator)
				if idx < 0 {
					break
				}
				rawChunk := strings.TrimSpace(string(buffer[:idx]))
				buffer = buffer[idx+2:]
				if rawChunk != "" {
					if err := handleChunk(rawChunk); err != nil {
						return nil, err
					}
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			if isContextTimeout(readErr, ctx) {
				return nil, fmt.Errorf("LLM stream timed out after %d ms without receiving data", s.LLMTimeoutMs)
			}
			return nil, readErr
		}
	}
	if rem := strings.TrimSpace(string(buffer)); rem != "" {
		if err := handleChunk(rem); err != nil {
			return nil, err
		}
	}

	content := llmresponse.ParseAssistantResponse(rawContent.String(), s.LLMResponseFormat)
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("LLM stream did not contain message content")
	}
	elapsed := float64(time.Since(start).Microseconds()) / 1000.0
	respMs, tokens, tps := buildGenerationMetrics(content, elapsed, completionTokens)
	return &ChatCompletionResult{
		Content:         content,
		ResponseMs:      respMs,
		OutputTokens:    tokens,
		TokensPerSecond: tps,
		ModelName:       modelName,
	}, nil
}

// modelListBodyLimit caps how much of a /models reply is read. A model list is a
// few KiB at most, and the body has to be held whole to be both decoded and
// quoted back in an error.
const modelListBodyLimit = 64 << 10

// modelListReply is an OpenAI-compatible /models response. Data is a pointer so
// that an endpoint which answered without a `data` field can be told apart from
// one that genuinely serves no models.
type modelListReply struct {
	Data *[]struct {
		ID string `json:"id"`
	} `json:"data"`
	Error json.RawMessage `json:"error"`
}

// parseModelList turns a /models reply into the sorted model ids, or into an
// error carrying the endpoint's own wording.
//
// A 2xx status is not by itself a successful listing. LM Studio answers a base
// URL that is missing its /v1 suffix with 200 and {"error": "Unexpected endpoint
// ..."}, which otherwise decodes as a working endpoint serving no models — the
// connection check then reports success for an address no turn can run against.
func parseModelList(kind string, statusCode int, body []byte) ([]string, error) {
	var reply modelListReply
	decodeErr := json.Unmarshal(body, &reply)
	serverMessage := ""
	if decodeErr == nil {
		serverMessage = modelListErrorMessage(reply.Error)
	}

	if !statusOK(statusCode) {
		if serverMessage != "" {
			return nil, fmt.Errorf("%s model list request failed with %d: %s", kind, statusCode, serverMessage)
		}
		return nil, fmt.Errorf("%s model list request failed with %d", kind, statusCode)
	}
	if decodeErr != nil {
		return nil, decodeErr
	}
	if serverMessage != "" {
		return nil, fmt.Errorf("%s model list request failed: %s", kind, serverMessage)
	}
	if reply.Data == nil {
		return nil, fmt.Errorf("%s model list response carried no model list; check that the base URL ends with /v1", kind)
	}

	out := []string{}
	for _, item := range *reply.Data {
		if item.ID != "" {
			out = append(out, item.ID)
		}
	}
	return sortedStrings(out), nil
}

// modelListErrorMessage reads the endpoint's own error wording, accepting both
// shapes in use: OpenAI's {"error": {"message": ...}} and LM Studio's
// {"error": "..."}.
func modelListErrorMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	var object struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &object) == nil {
		return strings.TrimSpace(object.Message)
	}
	return ""
}

// ListModels mirrors listModels: GET {base}/models, returning the sorted ids.
func (c *LLMClient) ListModels(baseURL string) ([]string, error) {
	s := c.cfg.Get()
	if baseURL == "" {
		baseURL = s.LLMBaseURL
	}
	timeout := time.Duration(minInt(s.LLMTimeoutMs, 5000)) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stripTrailingSlash(baseURL)+"/models", nil)
	if err != nil {
		return nil, err
	}
	c.authHeader(req, s.LLMAPIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, modelListBodyLimit))
	if err != nil {
		return nil, err
	}
	return parseModelList("LLM", resp.StatusCode, body)
}

// ListAvailableModels mirrors listAvailableModels: GET {lmRoot}/api/v1/models,
// filtered to LLM-type entries, returning the sorted keys.
func (c *LLMClient) ListAvailableModels(baseURL string) ([]string, error) {
	s := c.cfg.Get()
	if baseURL == "" {
		baseURL = s.LLMBaseURL
	}
	timeout := time.Duration(minInt(s.LLMTimeoutMs, 5000)) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, getLmStudioAPIRoot(baseURL)+"/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	c.authHeader(req, s.LLMAPIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if !respOK(resp) {
		return nil, fmt.Errorf("LLM available model request failed with %d", resp.StatusCode)
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
		if item.Type == "llm" && item.Key != "" {
			out = append(out, item.Key)
		}
	}
	return sortedStrings(out), nil
}

// EnsureModelLoaded mirrors ensureModelLoaded: ask LM Studio whether the model is
// loaded, loading it if necessary. Returns false on any error.
func (c *LLMClient) EnsureModelLoaded(modelKey, baseURL string) bool {
	if strings.TrimSpace(modelKey) == "" {
		return false
	}
	s := c.cfg.Get()
	if baseURL == "" {
		baseURL = s.LLMBaseURL
	}
	timeout := time.Duration(minInt(s.LLMTimeoutMs, 20000)) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	root := getLmStudioAPIRoot(baseURL)
	listReq, err := http.NewRequestWithContext(ctx, http.MethodGet, root+"/api/v1/models", nil)
	if err != nil {
		return false
	}
	c.authHeader(listReq, s.LLMAPIKey)
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
		if data.Models[i].Type == "llm" && data.Models[i].Key == modelKey {
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
	c.authHeader(loadReq, s.LLMAPIKey)
	loadResp, err := c.http.Do(loadReq)
	if err != nil {
		return false
	}
	defer loadResp.Body.Close()
	return respOK(loadResp)
}

// CheckConnection mirrors checkConnection: true when the endpoint reports no models
// or includes the requested model.
func (c *LLMClient) CheckConnection(baseURL, modelName string) bool {
	s := c.cfg.Get()
	if modelName == "" {
		modelName = s.LLMModel
	}
	if strings.TrimSpace(modelName) == "" {
		return false
	}
	models, err := c.ListModels(baseURL)
	if err != nil {
		return false
	}
	return len(models) == 0 || containsString(models, modelName)
}
