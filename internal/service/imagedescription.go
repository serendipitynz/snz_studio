package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"snzstudio/internal/config"
)

// Errors DescribeImage can fail with, as sentinels so the HTTP layer maps them to
// 4xx/5xx with errors.Is instead of fail()'s blanket 500.
var (
	ErrImageDescriptionDisabled  = errors.New("service: image description model is not configured")
	ErrUnsupportedImageFormat    = errors.New("service: image format is not supported for description")
	ErrImageTooLarge             = errors.New("service: image is too large for description")
	ErrImageDescriptionEndpoint  = errors.New("service: image description endpoint failed")
	ErrImageDescriptionEmptyText = errors.New("service: image description endpoint returned no text")
)

// MaxImageDescriptionBytes caps the image sent for description. Only this path is
// capped — storing an image has no limit, since it only lands on local disk —
// because the request carries the image base64-encoded (about 1.33x) inside one
// JSON body held in memory. Downscaling is not done here but in the UI before
// upload (see ImageDocumentDialog): the runtime measured, LM Studio with Gemma 4,
// rejects anything much above one megapixel outright instead of resizing it, and
// the standard library has no WebP decoder or resampler to do it server-side
// without a new dependency.
const MaxImageDescriptionBytes = 10 << 20

// imageDescriptionFormats is an allowlist, not a denylist: http.DetectContentType
// reports HEIC/AVIF as application/octet-stream and SVG as text/xml or text/plain,
// so a denylist would let every format it cannot recognise through. These three
// are the ones OpenAI-compatible multimodal endpoints commonly accept.
var imageDescriptionFormats = []string{"image/png", "image/jpeg", "image/webp"}

// ImageDescriptionFormats returns the MIME types DescribeImage accepts, for the
// UI to disable the action up front. The UI check is guidance only; DescribeImage
// is what rejects.
func ImageDescriptionFormats() []string {
	return append([]string(nil), imageDescriptionFormats...)
}

const imageDescriptionPrompt = `この画像を、あとでキーワード検索とチャットの参考資料として使うための説明文にしてください。

- 日本語で、簡潔に書く。
- 写っている物・人物の様子・場所、画像内の文字 (読める範囲でそのまま)、図や表の構造、固有名を優先して書く。
- 写っていないこと、読み取れないことは推測で書かない。
- 前置きや結びの挨拶は書かず、説明文だけを返す。`

// ImageDescriptionService turns an image into a draft description with a
// multimodal model. It stores nothing: the caller decides what to persist.
type ImageDescriptionService struct {
	cfg  *config.Config
	http *http.Client
}

// NewImageDescriptionService builds an ImageDescriptionService. Deadlines come
// from the request context, as in LLMClient.
func NewImageDescriptionService(cfg *config.Config) *ImageDescriptionService {
	return &ImageDescriptionService{cfg: cfg, http: &http.Client{}}
}

// Enabled reports whether an image description model is configured.
func (s *ImageDescriptionService) Enabled() bool {
	return strings.TrimSpace(s.cfg.Get().ImageDescriptionModel) != ""
}

// Separate from chatMessage on purpose: chat and multi-agent turns keep sending a
// plain string content, and only this request carries an image part.
type visionContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *visionImageURL `json:"image_url,omitempty"`
}

type visionImageURL struct {
	URL string `json:"url"`
}

type visionMessage struct {
	Role    string              `json:"role"`
	Content []visionContentPart `json:"content"`
}

type visionCompletionBody struct {
	Model       string          `json:"model"`
	Temperature float64         `json:"temperature"`
	Messages    []visionMessage `json:"messages"`
}

// DescribeImage sends image to the configured multimodal model once and returns
// its description. The format is sniffed from the bytes rather than taken from the
// caller, so the type written into the data URL always matches the bytes encoded
// after it, whichever route the image arrived by.
func (s *ImageDescriptionService) DescribeImage(ctx context.Context, image []byte) (string, error) {
	settings := s.cfg.Get()
	modelName := strings.TrimSpace(settings.ImageDescriptionModel)
	if modelName == "" {
		return "", ErrImageDescriptionDisabled
	}
	if len(image) > MaxImageDescriptionBytes {
		return "", ErrImageTooLarge
	}
	mimeType, err := sniffImageDescriptionFormat(image)
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimSpace(settings.ImageDescriptionBaseURL)
	if baseURL == "" {
		baseURL = settings.LLMBaseURL
	}
	body, err := json.Marshal(visionCompletionBody{
		Model:       modelName,
		Temperature: 0.2,
		Messages: []visionMessage{{
			Role: "user",
			Content: []visionContentPart{
				{Type: "text", Text: imageDescriptionPrompt},
				{Type: "image_url", ImageURL: &visionImageURL{URL: "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(image)}},
			},
		}},
	})
	if err != nil {
		return "", err
	}

	timeoutMs := settings.ImageDescriptionTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = config.DefaultImageDescriptionTimeoutMs
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stripTrailingSlash(baseURL)+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if settings.LLMAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+settings.LLMAPIKey)
	}

	resp, err := s.http.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("%w: timed out after %d ms", ErrImageDescriptionEndpoint, timeoutMs)
		}
		return "", fmt.Errorf("%w: %v", ErrImageDescriptionEndpoint, err)
	}
	defer resp.Body.Close()

	var reply struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error json.RawMessage `json:"error"`
	}
	decodeErr := json.NewDecoder(resp.Body).Decode(&reply)
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%w: timed out after %d ms", ErrImageDescriptionEndpoint, timeoutMs)
	}
	// The endpoint's own wording is kept because it is the only place a model that
	// cannot take images says so (LM Studio answers such a model with an error
	// body); one that silently drops the image part is caught only by the human
	// review of the draft.
	serverMessage := ""
	if decodeErr == nil {
		serverMessage = modelListErrorMessage(reply.Error)
	}
	if !statusOK(resp.StatusCode) {
		if serverMessage != "" {
			return "", fmt.Errorf("%w with %d: %s", ErrImageDescriptionEndpoint, resp.StatusCode, serverMessage)
		}
		return "", fmt.Errorf("%w with %d", ErrImageDescriptionEndpoint, resp.StatusCode)
	}
	if decodeErr != nil {
		return "", fmt.Errorf("%w: invalid response: %v", ErrImageDescriptionEndpoint, decodeErr)
	}
	if serverMessage != "" {
		return "", fmt.Errorf("%w: %s", ErrImageDescriptionEndpoint, serverMessage)
	}

	text := ""
	if len(reply.Choices) > 0 {
		text = strings.TrimSpace(reply.Choices[0].Message.Content)
	}
	if text == "" {
		return "", ErrImageDescriptionEmptyText
	}
	return text, nil
}

// sniffImageDescriptionFormat returns the allowlisted MIME type of image, judged
// from its leading bytes.
func sniffImageDescriptionFormat(image []byte) (string, error) {
	detected := http.DetectContentType(image)
	for _, allowed := range imageDescriptionFormats {
		if detected == allowed {
			return allowed, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, detected)
}
