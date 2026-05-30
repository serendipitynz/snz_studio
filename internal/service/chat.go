package service

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"snzstudio/internal/config"
	"snzstudio/internal/llmresponse"
	"snzstudio/internal/model"
	"snzstudio/internal/repository"
	"snzstudio/internal/util"
)

// ChatService ports chatService.ts: it drives a chat turn end to end. The streaming
// path uses the streaming-save contract — create an empty assistant message, update
// its content on each delta, then finalize metrics/references/summary — and falls
// back to a reference excerpt when the LLM is unreachable.
type ChatService struct {
	chats         *repository.ChatRepository
	context       *ContextService
	llm           *LLMClient
	summary       *SummaryService
	memoryService *MemoryService
	embeddingSync *EmbeddingSyncService
	cfg           *config.Config
}

// NewChatService builds a ChatService.
func NewChatService(chats *repository.ChatRepository, context *ContextService, llm *LLMClient, summary *SummaryService, memoryService *MemoryService, embeddingSync *EmbeddingSyncService, cfg *config.Config) *ChatService {
	return &ChatService{
		chats:         chats,
		context:       context,
		llm:           llm,
		summary:       summary,
		memoryService: memoryService,
		embeddingSync: embeddingSync,
		cfg:           cfg,
	}
}

// generation carries the produced content and its (optional) metrics. The metric
// pointers are nil for the fallback response, mirroring the TS `| null` fields.
type generation struct {
	content         string
	responseMs      *int64
	outputTokens    *int64
	tokensPerSecond *float64
	modelName       *string
}

func generationFromResult(result *ChatCompletionResult) generation {
	return generation{
		content:         result.Content,
		responseMs:      int64Ptr(result.ResponseMs),
		outputTokens:    int64Ptr(result.OutputTokens),
		tokensPerSecond: float64Ptr(result.TokensPerSecond),
		modelName:       strPtr(result.ModelName),
	}
}

func strPtr(s string) *string { return &s }

type preparedTurn struct {
	assembled    *model.AssembledContext
	userMessage  model.Message
	systemPrompt string
}

// SendMessage mirrors sendMessage: a non-streaming turn.
func (s *ChatService) SendMessage(chatID, content string) (*model.Message, error) {
	prepared, err := s.prepareTurn(chatID, content)
	if err != nil {
		return nil, err
	}

	log.Printf("[chat] completion start %s", logFields(chatID, "sync"))
	result, llmErr := s.llm.CreateChatCompletion(ChatCompletionInput{
		SystemPrompt: prepared.systemPrompt,
		Messages:     prepared.assembled.RecentMessages,
		UserInput:    content,
		Temperature:  float64Ptr(0.25),
	})
	var gen generation
	if llmErr != nil {
		log.Printf("[chat] completion failed %s reason=%v", logFields(chatID, "sync"), llmErr)
		gen = generation{content: s.buildFallbackResponse(prepared.assembled.References, content, llmErr.Error())}
	} else {
		gen = generationFromResult(result)
	}

	return s.persistAssistantTurn(chatID, prepared.assembled, prepared.userMessage, gen)
}

// SendMessageStream mirrors sendMessageStream: a streaming turn that persists deltas
// as they arrive.
func (s *ChatService) SendMessageStream(chatID, content string, onDelta func(string)) (*model.Message, error) {
	prepared, err := s.prepareTurn(chatID, content)
	if err != nil {
		return nil, err
	}

	assistantMessage, err := s.chats.AddMessage(repository.AddMessageInput{
		ChatID:  chatID,
		Role:    "assistant",
		Content: "",
	})
	if err != nil {
		return nil, err
	}

	log.Printf("[chat] completion start %s", logFields(chatID, "stream"))
	var streamedContent strings.Builder
	result, llmErr := s.llm.CreateChatCompletionStream(ChatCompletionInput{
		SystemPrompt: prepared.systemPrompt,
		Messages:     prepared.assembled.RecentMessages,
		UserInput:    content,
		Temperature:  float64Ptr(0.25),
	}, func(chunk string) {
		streamedContent.WriteString(chunk)
		// Best-effort live persistence, matching the TS which does not await/check.
		_, _ = s.chats.UpdateMessageContent(assistantMessage.ID, streamedContent.String())
		onDelta(chunk)
	})

	var gen generation
	if llmErr != nil {
		log.Printf("[chat] completion failed %s reason=%v", logFields(chatID, "stream"), llmErr)
		fallbackContent := s.buildFallbackResponse(prepared.assembled.References, content, llmErr.Error())
		gen = generation{content: fallbackContent}
		_, _ = s.chats.UpdateMessageContent(assistantMessage.ID, fallbackContent)
		onDelta(fallbackContent)
	} else {
		gen = generationFromResult(result)
	}

	return s.persistExistingAssistantTurn(chatID, assistantMessage.ID, prepared.assembled, prepared.userMessage, gen)
}

func (s *ChatService) prepareTurn(chatID, content string) (*preparedTurn, error) {
	assembled, err := s.context.Assemble(chatID, content)
	if err != nil {
		return nil, err
	}

	userMessage, err := s.chats.AddMessage(repository.AddMessageInput{
		ChatID:  chatID,
		Role:    "user",
		Content: content,
	})
	if err != nil {
		return nil, err
	}

	var createdMemories []model.Memory
	var explicitMemory *model.Memory
	if !assembled.Chat.IsTemporary {
		createdMemories, err = s.memoryService.MaybeStoreFromUserMessage(assembled.Project.ID, chatID, content)
		if err != nil {
			return nil, err
		}
		explicitMemory, err = s.memoryService.MaybeStoreFromExplicitRequest(assembled.Project.ID, chatID, content, assembled.RecentMessages)
		if err != nil {
			return nil, err
		}
		if explicitMemory != nil {
			createdMemories = append(createdMemories, *explicitMemory)
		}
	}

	if len(createdMemories) > 0 {
		ids := make([]string, len(createdMemories))
		for i, memory := range createdMemories {
			ids[i] = memory.ID
		}
		if err := s.embeddingSync.SyncMemories(ids); err != nil {
			return nil, err
		}
	}

	settings := s.cfg.Get()
	parts := []string{
		"You are a local project assistant.",
		"Use the provided project context when it is relevant, but avoid mentioning irrelevant references.",
		"Answer clearly and practically.",
	}
	if settings.LLMResponseFormat == llmresponse.FormatLLMJPThinking {
		parts = append(parts, "Return only the final user-facing answer. Do not emit analysis, reasoning traces, or any tagged channel markup.")
	}
	if assembled.IsQuoteRequest {
		from := ""
		if assembled.TargetDocumentTitle != nil {
			from = fmt.Sprintf(` from "%s"`, *assembled.TargetDocumentTitle)
		}
		parts = append(parts, fmt.Sprintf("The user is asking for document quotation%s. Quote only from provided document material, preserve the original wording, and say clearly if the exact passage was not found.", from))
	}
	if explicitMemory != nil {
		parts = append(parts, fmt.Sprintf("A new %s memory was just saved from the recent conversation: %s\nIf it fits naturally, briefly acknowledge that it has been remembered.", explicitMemory.Kind, explicitMemory.Content))
	}
	if assembled.Chat.IsTemporary {
		parts = append(parts, "This is a temporary chat. Do not treat this conversation as durable project memory unless the user later converts the chat into a regular one.")
	}
	if assembled.PromptContext != "" {
		parts = append(parts, assembled.PromptContext)
	}
	systemPrompt := strings.Join(parts, "\n\n")

	return &preparedTurn{assembled: assembled, userMessage: userMessage, systemPrompt: systemPrompt}, nil
}

func (s *ChatService) persistAssistantTurn(chatID string, assembled *model.AssembledContext, userMessage model.Message, gen generation) (*model.Message, error) {
	assistantMessage, err := s.chats.AddMessage(repository.AddMessageInput{
		ChatID:          chatID,
		Role:            "assistant",
		Content:         gen.content,
		ResponseMs:      gen.responseMs,
		OutputTokens:    gen.outputTokens,
		TokensPerSecond: gen.tokensPerSecond,
		ModelName:       gen.modelName,
	})
	if err != nil {
		return nil, err
	}
	return s.finishTurn(chatID, &assistantMessage, assembled, userMessage)
}

func (s *ChatService) persistExistingAssistantTurn(chatID, assistantMessageID string, assembled *model.AssembledContext, userMessage model.Message, gen generation) (*model.Message, error) {
	assistantMessage, err := s.chats.FinalizeMessage(repository.FinalizeMessageInput{
		MessageID:       assistantMessageID,
		Content:         gen.content,
		ResponseMs:      gen.responseMs,
		OutputTokens:    gen.outputTokens,
		TokensPerSecond: gen.tokensPerSecond,
		ModelName:       gen.modelName,
	})
	if err != nil {
		return nil, err
	}
	if assistantMessage == nil {
		assistantMessage, err = s.chats.GetMessage(assistantMessageID)
		if err != nil {
			return nil, err
		}
	}
	if assistantMessage == nil {
		return nil, errors.New("Assistant message could not be finalized")
	}
	return s.finishTurn(chatID, assistantMessage, assembled, userMessage)
}

// finishTurn applies the post-generation steps shared by both persist paths:
// references, rolling summary, and chat title.
func (s *ChatService) finishTurn(chatID string, assistantMessage *model.Message, assembled *model.AssembledContext, userMessage model.Message) (*model.Message, error) {
	refInputs := make([]repository.ReferenceInput, len(assembled.References))
	for i, ref := range assembled.References {
		refInputs[i] = repository.ReferenceInput{
			SourceType: ref.SourceType,
			SourceID:   ref.SourceID,
			Label:      ref.Label,
			Excerpt:    ref.Excerpt,
			Score:      ref.Score,
		}
	}
	if err := s.chats.ReplaceAssistantReferences(assistantMessage.ID, refInputs); err != nil {
		return nil, err
	}

	updatedMessages := make([]model.Message, 0, len(assembled.RecentMessages)+2)
	updatedMessages = append(updatedMessages, assembled.RecentMessages...)
	updatedMessages = append(updatedMessages, userMessage, *assistantMessage)

	updatedSummary := s.summary.UpdateSummary(assembled.Summary, updatedMessages)
	if err := s.chats.UpsertSummary(chatID, updatedSummary); err != nil {
		return nil, err
	}

	if strings.TrimSpace(assembled.Chat.Title) == "" {
		nextTitle := s.summary.GenerateChatTitle(updatedMessages)
		if strings.TrimSpace(nextTitle) != "" {
			if _, err := s.chats.UpdateChatTitle(chatID, nextTitle); err != nil {
				return nil, err
			}
		}
	}

	return assistantMessage, nil
}

func (s *ChatService) buildFallbackResponse(references []model.SearchReference, content, reason string) string {
	referenceSection := "No project references were selected for this turn."
	if len(references) > 0 {
		lines := make([]string, 0, len(references))
		for _, ref := range references {
			lines = append(lines, fmt.Sprintf("- %s: %s", ref.Label, util.Truncate(ref.Excerpt, 160)))
		}
		referenceSection = "Available references:\n" + joinLines(lines)
	}
	return strings.Join([]string{
		"Local LLM endpoint could not be reached, so this is a fallback response.",
		"Reason: " + reason,
		referenceSection,
		"User message: " + content,
	}, "\n\n")
}

func logFields(chatID, mode string) string {
	return fmt.Sprintf("chatId=%s mode=%s", chatID, mode)
}
