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

	instructions := promptTextFor(settings.ProcessingLanguage).reviewSystem
	systemPrompt := strings.Join(append(append([]string{}, instructions...), assembled.PromptContext), "\n\n")

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

// ReviewMessageStream mirrors reviewMessageStream. Only the review's own text
// streams: the review dialog has no place for the reviewer's reasoning.
func (s *ReviewService) ReviewMessageStream(messageID string, onDelta func(StreamDelta)) (*ReviewResult, error) {
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
	}, func(delta StreamDelta) {
		if !delta.Reasoning {
			onDelta(delta)
		}
	})
	if err != nil {
		return nil, err
	}
	return &ReviewResult{Review: review.Content, References: prepared.references}, nil
}
