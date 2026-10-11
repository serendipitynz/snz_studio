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
//
// OutputTokens counts every generated token, the reasoning included, and
// ReasoningTokens is the reasoning's share of it (0 when the model did not think).
type ChatCompletionResult struct {
	Content         string
	Reasoning       string
	ResponseMs      int64
	OutputTokens    int64
	ReasoningTokens int64
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
	return &LLMClient{cfg: cfg, http: newKeyedHTTPClient()}
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
// milliseconds; outputTokens is the endpoint-reported count when available, and
// content is what the estimate reads otherwise — the reasoning included, since
// the reported count includes it too.
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

// reasoningTokenCount is the reasoning's share of the output tokens: the count
// the endpoint reported (usage.completion_tokens_details.reasoning_tokens), or an
// estimate from the reasoning text, never more than the whole.
func reasoningTokenCount(reasoning string, reported *int64, outputTokens int64) int64 {
	count := estimateTokenCount(reasoning)
	if reported != nil {
		count = *reported
	}
	return max(0, min(count, outputTokens))
}

// generatedText is the text the token estimate reads: everything generated.
func generatedText(content, reasoning string) string {
	if reasoning == "" {
		return content
	}
	return reasoning + "\n" + content
}

// splitResponse separates a response into its reasoning and its visible content.
// Reasoning arrives in a field of its own (LM Studio sends reasoning_content for
// some models and reasoning for others), or inside the content between <think>
// tags; the two are joined when a model uses both.
func splitResponse(rawContent, rawReasoning, format string) (reasoning, content string) {
	thinking, rest := llmresponse.SplitThinking(rawContent)
	reasoning = strings.TrimSpace(rawReasoning)
	if thinking != "" {
		if reasoning != "" {
			reasoning += "\n\n"
		}
		reasoning += thinking
	}
	return reasoning, llmresponse.ParseAssistantResponse(rest, format)
}

// completionUsage is the usage block of a completion or of a stream's last chunk.
type completionUsage struct {
	CompletionTokens        *int64 `json:"completion_tokens"`
	CompletionTokensDetails *struct {
		ReasoningTokens *int64 `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u *completionUsage) reasoningTokens() *int64 {
	if u == nil || u.CompletionTokensDetails == nil {
		return nil
	}
	return u.CompletionTokensDetails.ReasoningTokens
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatCompletionBody struct {
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	// ReasoningEffort is left out unless a value was chosen for the model: an
	// OpenAI-compatible server other than LM Studio may reject a parameter it does
	// not know.
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
	Stream          bool           `json:"stream,omitempty"`
	StreamOptions   *streamOptions `json:"stream_options,omitempty"`
	Messages        []chatMessage  `json:"messages"`
}

// reasoningEffortFor returns the reasoning_effort to send to a model, or "" when
// none was chosen for it at that endpoint. The value is stored in LM Studio's
// vocabulary, but LM Studio's /v1/chat/completions accepts only OpenAI's
// (none / minimal / low / medium / high / xhigh) and answers off and on with 400.
// For a model that offers only off and on, any value but none turns thinking on,
// so on is sent as medium.
func reasoningEffortFor(s config.Settings, baseURL, modelName string) string {
	value := s.LLMReasoning[getLmStudioAPIRoot(baseURL)][modelName]
	switch value {
	case "off":
		return "none"
	case "on":
		return "medium"
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return value
	default:
		return ""
	}
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

func (c *LLMClient) authHeader(req *http.Request, s config.Settings) {
	setBearer(req, llmAPIKeyFor(s, req.URL))
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
		Model:           modelName,
		Temperature:     temperatureOrDefault(input.Temperature),
		ReasoningEffort: reasoningEffortFor(s, baseURL, modelName),
		Messages:        buildMessages(input, s.LLMResponseFormat),
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
	c.authHeader(req, s)

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
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				Reasoning        string `json:"reasoning"`
			} `json:"message"`
		} `json:"choices"`
		Usage *completionUsage `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		if isContextTimeout(err, ctx) {
			return nil, fmt.Errorf("LLM request timed out after %d ms", s.LLMTimeoutMs)
		}
		return nil, err
	}

	var reasoning, content string
	if len(data.Choices) > 0 {
		message := data.Choices[0].Message
		reasoning, content = splitResponse(strings.TrimSpace(message.Content), message.ReasoningContent+message.Reasoning, s.LLMResponseFormat)
	}
	if content == "" {
		return nil, errors.New("LLM response did not contain message content")
	}

	var completionTokens *int64
	if data.Usage != nil {
		completionTokens = data.Usage.CompletionTokens
	}
	elapsed := float64(time.Since(start).Microseconds()) / 1000.0
	respMs, tokens, tps := buildGenerationMetrics(generatedText(content, reasoning), elapsed, completionTokens)
	return &ChatCompletionResult{
		Content:         content,
		Reasoning:       reasoning,
		ResponseMs:      respMs,
		OutputTokens:    tokens,
		ReasoningTokens: reasoningTokenCount(reasoning, data.Usage.reasoningTokens(), tokens),
		TokensPerSecond: tps,
		ModelName:       modelName,
	}, nil
}

// StreamDelta is one change to the visible text, or to the reasoning, of a
// streaming completion. Reasoning marks a change to the reasoning: the two are
// separate texts, and a reasoning delta never touches the visible one.
//
// Both texts are re-parsed from the whole raw response on every chunk, so they do
// not only grow: a tag fragment shows as text until the rest of the tag arrives
// and removes it, and under llm_jp_thinking a late channel tag hides everything
// shown so far. Replace marks such a change, and Text is then the whole
// text, shown instead of everything streamed before it. Otherwise Text is
// appended. A whole-text replacement rather than "keep N characters, then append"
// because Go counts runes and the frontend counts UTF-16 code units.
type StreamDelta struct {
	Text      string
	Replace   bool
	Reasoning bool
}

// maxStreamEventBytes bounds the bytes held while waiting for an SSE event's
// "\n\n" separator. Every read resets the sliding timeout, so without a bound an
// endpoint that keeps sending data with no separator grows the buffer for as long
// as it keeps sending.
const maxStreamEventBytes = 1 << 20

// CreateChatCompletionStream performs a streaming completion, invoking onDelta
// whenever the visible text or the reasoning changes. Mirrors
// createChatCompletionStream, including the sliding timeout that resets on every
// received chunk.
func (c *LLMClient) CreateChatCompletionStream(input ChatCompletionInput, onDelta func(StreamDelta)) (*ChatCompletionResult, error) {
	s := c.cfg.Get()
	baseURL, modelName := resolveTarget(input.Target, s.LLMBaseURL, s.LLMModel)
	body := chatCompletionBody{
		Model:           modelName,
		Temperature:     temperatureOrDefault(input.Temperature),
		ReasoningEffort: reasoningEffortFor(s, baseURL, modelName),
		Stream:          true,
		StreamOptions:   &streamOptions{IncludeUsage: true},
		Messages:        buildMessages(input, s.LLMResponseFormat),
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
	c.authHeader(req, s)

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
		rawReasoning     strings.Builder
		visibleContent   string
		visibleReasoning string
		usage            *completionUsage
	)

	show := func(shown *string, next string, reasoning bool) {
		if next == *shown {
			return
		}
		if strings.HasPrefix(next, *shown) {
			onDelta(StreamDelta{Text: next[len(*shown):], Reasoning: reasoning})
		} else {
			onDelta(StreamDelta{Text: next, Replace: true, Reasoning: reasoning})
		}
		*shown = next
	}

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
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					Reasoning        string `json:"reasoning"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *completionUsage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(dataText), &payload); err != nil {
			return fmt.Errorf("LLM stream returned invalid JSON: %w", err)
		}
		if payload.Usage != nil && payload.Usage.CompletionTokens != nil {
			usage = payload.Usage
		}
		if len(payload.Choices) == 0 {
			return nil
		}
		delta := payload.Choices[0].Delta
		if delta.Content == "" && delta.ReasoningContent == "" && delta.Reasoning == "" {
			return nil
		}
		rawContent.WriteString(delta.Content)
		rawReasoning.WriteString(delta.ReasoningContent)
		rawReasoning.WriteString(delta.Reasoning)
		nextReasoning, nextVisible := splitResponse(rawContent.String(), rawReasoning.String(), s.LLMResponseFormat)
		show(&visibleReasoning, nextReasoning, true)
		show(&visibleContent, nextVisible, false)
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
			if len(buffer) > maxStreamEventBytes {
				return nil, fmt.Errorf("LLM stream sent more than %d bytes without an event separator", maxStreamEventBytes)
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

	reasoning, content := splitResponse(rawContent.String(), rawReasoning.String(), s.LLMResponseFormat)
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("LLM stream did not contain message content")
	}
	var completionTokens *int64
	if usage != nil {
		completionTokens = usage.CompletionTokens
	}
	elapsed := float64(time.Since(start).Microseconds()) / 1000.0
	respMs, tokens, tps := buildGenerationMetrics(generatedText(content, reasoning), elapsed, completionTokens)
	return &ChatCompletionResult{
		Content:         content,
		Reasoning:       reasoning,
		ResponseMs:      respMs,
		OutputTokens:    tokens,
		ReasoningTokens: reasoningTokenCount(reasoning, usage.reasoningTokens(), tokens),
		TokensPerSecond: tps,
		ModelName:       modelName,
	}, nil
}

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
func parseModelList(kind string, statusCode int, body io.Reader) ([]string, error) {
	// Streamed rather than read whole: an aggregator's /models runs to megabytes
	// (hundreds of entries carrying descriptions and pricing), and any cap on the
	// read would truncate it into a parse error reported as a failed connection.
	var reply modelListReply
	decodeErr := json.NewDecoder(body).Decode(&reply)
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
	c.authHeader(req, s)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return parseModelList("LLM", resp.StatusCode, resp.Body)
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
	c.authHeader(req, s)
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

// ReasoningChoice is a model's reasoning setting: the values LM Studio's
// /api/v1/models says it accepts, the one it runs at when none is chosen, and the
// one chosen for it here ("" when none is).
type ReasoningChoice struct {
	AllowedOptions []string `json:"allowedOptions"`
	Default        string   `json:"default"`
	Selected       string   `json:"selected"`
}

// ErrReasoningNotOffered marks a reasoning value the endpoint does not offer for
// the model, including every value at an endpoint that is not LM Studio.
var ErrReasoningNotOffered = errors.New("reasoning value is not offered for this model")

// ListReasoningChoices returns, by model key, the reasoning choice of every LLM at
// the endpoint that has reasoning options.
func (c *LLMClient) ListReasoningChoices(baseURL string) (map[string]ReasoningChoice, error) {
	choices, err := c.listReasoningOptions(baseURL)
	if err != nil {
		return nil, err
	}
	selected := c.cfg.Get().LLMReasoning[getLmStudioAPIRoot(baseURL)]
	for key, choice := range choices {
		choice.Selected = selected[key]
		choices[key] = choice
	}
	return choices, nil
}

// SetReasoning saves the reasoning value chosen for a model at an endpoint, after
// checking that the endpoint offers it. An empty value goes back to the model's
// default and needs no check, so it works while the endpoint is down.
func (c *LLMClient) SetReasoning(baseURL, modelName, value string) error {
	if value != "" {
		choices, err := c.listReasoningOptions(baseURL)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrReasoningNotOffered, err)
		}
		if !containsString(choices[modelName].AllowedOptions, value) {
			return fmt.Errorf("%w: %q for %s", ErrReasoningNotOffered, value, modelName)
		}
	}
	return c.cfg.SetReasoning(getLmStudioAPIRoot(baseURL), modelName, value)
}

// listReasoningOptions reads GET {lmRoot}/api/v1/models and returns, by model key,
// the reasoning options of every LLM that has them. A model whose reasoning is
// null cannot be switched through the API and is left out, and an endpoint that is
// not LM Studio fails the request or answers without models, so it yields none.
func (c *LLMClient) listReasoningOptions(baseURL string) (map[string]ReasoningChoice, error) {
	s := c.cfg.Get()
	timeout := time.Duration(minInt(s.LLMTimeoutMs, 5000)) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, getLmStudioAPIRoot(baseURL)+"/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	c.authHeader(req, s)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if !respOK(resp) {
		return nil, fmt.Errorf("LLM reasoning option request failed with %d", resp.StatusCode)
	}
	var data struct {
		Models []struct {
			Type         string `json:"type"`
			Key          string `json:"key"`
			Capabilities struct {
				Reasoning *struct {
					AllowedOptions []string `json:"allowed_options"`
					Default        string   `json:"default"`
				} `json:"reasoning"`
			} `json:"capabilities"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	out := map[string]ReasoningChoice{}
	for _, item := range data.Models {
		reasoning := item.Capabilities.Reasoning
		if item.Type != "llm" || item.Key == "" || reasoning == nil || len(reasoning.AllowedOptions) == 0 {
			continue
		}
		// LM Studio lists a model once per downloaded format, and the formats can
		// disagree (one with options, one with null); the first with options wins.
		if _, seen := out[item.Key]; !seen {
			out[item.Key] = ReasoningChoice{AllowedOptions: reasoning.AllowedOptions, Default: reasoning.Default}
		}
	}
	return out, nil
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
	c.authHeader(listReq, s)
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
	c.authHeader(loadReq, s)
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
