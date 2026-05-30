package service

import (
	"errors"
	"strings"

	"snzstudio/internal/config"
	"snzstudio/internal/model"
	"snzstudio/internal/repository"
)

// ReviewResult is the result of reviewing an assistant message.
type ReviewResult struct {
	Review     string                  `json:"review"`
	References []model.SearchReference `json:"references"`
}

// ReviewService ports reviewService.ts: it reviews an assistant draft using the
// (optionally separate) review model, with the same assembled project context the
// chat turn used.
type ReviewService struct {
	chats   *repository.ChatRepository
	context *ContextService
	llm     *LLMClient
	cfg     *config.Config
}

// NewReviewService builds a ReviewService.
func NewReviewService(chats *repository.ChatRepository, context *ContextService, llm *LLMClient, cfg *config.Config) *ReviewService {
	return &ReviewService{chats: chats, context: context, llm: llm, cfg: cfg}
}

type preparedReview struct {
	systemPrompt string
	userInput    string
	target       *CompletionTarget
	references   []model.SearchReference
}

func (s *ReviewService) prepareReview(messageID string) (*preparedReview, error) {
	message, err := s.chats.GetMessage(messageID)
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, errors.New("Message not found")
	}
	if message.Role != "assistant" {
		return nil, errors.New("Only assistant messages can be reviewed")
	}

	assembled, err := s.context.Assemble(message.ChatID, message.Content)
	if err != nil {
		return nil, err
	}

	settings := s.cfg.Get()
	baseURL := settings.ReviewBaseURL
	if baseURL == "" {
		baseURL = settings.LLMBaseURL
	}
	modelName := settings.ReviewModel
	if modelName == "" {
		modelName = settings.LLMModel
	}

	systemPrompt := strings.Join([]string{
		"あなたは創作文レビュー専用の編集者です。",
		"与えられた文章を書き直さず、レビューだけを返してください。",
		"日本語で、短く具体的に指摘してください。",
		"特に次を見てください: 設定整合、人物の一貫性、時系列、用語ぶれ、文体の不自然さ、冗長さ、説明過多。",
		"大きな問題がなければ、その旨を述べたうえで軽い改善提案だけを返してください。",
		"出力は markdown で、`Overall`、`Issues`、`Suggestions` の 3 セクションにしてください。",
		assembled.PromptContext,
	}, "\n\n")

	return &preparedReview{
		systemPrompt: systemPrompt,
		userInput:    "Review this draft:\n\n" + message.Content,
		target:       &CompletionTarget{BaseURL: baseURL, Model: modelName},
		references:   assembled.References,
	}, nil
}

// ReviewMessage mirrors reviewMessage.
func (s *ReviewService) ReviewMessage(messageID string) (*ReviewResult, error) {
	prepared, err := s.prepareReview(messageID)
	if err != nil {
		return nil, err
	}
	review, err := s.llm.CreateChatCompletion(ChatCompletionInput{
		SystemPrompt: prepared.systemPrompt,
		Messages:     []model.Message{},
		UserInput:    prepared.userInput,
		Temperature:  float64Ptr(0.15),
		Target:       prepared.target,
	})
	if err != nil {
		return nil, err
	}
	return &ReviewResult{Review: review.Content, References: prepared.references}, nil
}

// ReviewMessageStream mirrors reviewMessageStream.
func (s *ReviewService) ReviewMessageStream(messageID string, onDelta func(string)) (*ReviewResult, error) {
	prepared, err := s.prepareReview(messageID)
	if err != nil {
		return nil, err
	}
	review, err := s.llm.CreateChatCompletionStream(ChatCompletionInput{
		SystemPrompt: prepared.systemPrompt,
		Messages:     []model.Message{},
		UserInput:    prepared.userInput,
		Temperature:  float64Ptr(0.15),
		Target:       prepared.target,
	}, onDelta)
	if err != nil {
		return nil, err
	}
	return &ReviewResult{Review: review.Content, References: prepared.references}, nil
}
