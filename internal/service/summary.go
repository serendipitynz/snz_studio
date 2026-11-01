package service

import (
	"regexp"
	"strings"

	"snzstudio/internal/config"
	"snzstudio/internal/model"
	"snzstudio/internal/util"
)

// SummaryService ports summaryService.ts: it keeps a rolling natural-language chat
// summary and generates chat titles, falling back to heuristics when the LLM is
// unreachable.
type SummaryService struct {
	llm *LLMClient
	cfg *config.Config
}

// NewSummaryService builds a SummaryService.
func NewSummaryService(llm *LLMClient, cfg *config.Config) *SummaryService {
	return &SummaryService{llm: llm, cfg: cfg}
}

func (s *SummaryService) text() *promptText {
	return promptTextFor(s.cfg.Get().ProcessingLanguage)
}

var (
	reTripleNewline  = regexp.MustCompile(`\n{3,}`)
	reTitleEdgeTrim  = regexp.MustCompile("^[\"'「『\\s\\p{Z}]+|[\"'」』\\s\\p{Z}]+$")
	reNewlineToSpace = regexp.MustCompile(`\n+`)
)

func summaryRoleLabel(role string, text *promptText) string {
	switch role {
	case "user":
		return text.humanSpeaker
	case "assistant":
		return text.assistantSpeaker
	default:
		return text.systemSpeaker
	}
}

// normalizeMessageForSummary mirrors normalizeMessageForSummary.
func normalizeMessageForSummary(m model.Message, text *promptText) string {
	return summaryRoleLabel(m.Role, text) + ": " + util.Truncate(collapseWhitespace(m.Content), 220)
}

func buildFallbackSummary(existingSummary string, recentMessages []model.Message, text *promptText) string {
	lines := make([]string, 0, 6)
	for _, m := range lastN(recentMessages, 6) {
		lines = append(lines, normalizeMessageForSummary(m, text))
	}
	recentTurns := joinLines(lines)

	sections := []string{}
	if existingSummary != "" {
		sections = append(sections, text.summaryFallbackPast+util.Truncate(existingSummary, 700))
	}
	if recentTurns != "" {
		sections = append(sections, text.summaryFallbackNow+recentTurns)
	}
	return util.Truncate(strings.Join(sections, "\n\n"), 1600)
}

// UpdateSummary mirrors updateSummary.
func (s *SummaryService) UpdateSummary(existingSummary string, recentMessages []model.Message) string {
	text := s.text()
	lines := make([]string, 0, 8)
	for _, m := range lastN(recentMessages, 8) {
		lines = append(lines, normalizeMessageForSummary(m, text))
	}
	transcript := joinLines(lines)
	if strings.TrimSpace(transcript) == "" {
		return existingSummary
	}

	existingSection := text.summaryNoneYet
	if existingSummary != "" {
		existingSection = text.summaryExisting + util.Truncate(existingSummary, 1200)
	}
	userInput := strings.Join([]string{
		existingSection,
		text.summaryRecent + transcript,
		text.summaryRequest,
	}, "\n\n")

	result, err := s.llm.CreateChatCompletion(ChatCompletionInput{
		SystemPrompt: text.summarySystem,
		Messages:     []model.Message{},
		UserInput:    userInput,
		Temperature:  float64Ptr(0.15),
	})
	if err != nil {
		return buildFallbackSummary(existingSummary, recentMessages, text)
	}
	return util.Truncate(strings.TrimSpace(reTripleNewline.ReplaceAllString(result.Content, "\n\n")), 1600)
}

// GenerateChatTitle mirrors generateChatTitle.
func (s *SummaryService) GenerateChatTitle(recentMessages []model.Message) string {
	text := s.text()
	nonSystem := make([]model.Message, 0, len(recentMessages))
	for _, m := range recentMessages {
		if m.Role != "system" {
			nonSystem = append(nonSystem, m)
		}
	}
	lines := make([]string, 0, 6)
	for _, m := range lastN(nonSystem, 6) {
		lines = append(lines, normalizeMessageForSummary(m, text))
	}
	transcript := joinLines(lines)
	if strings.TrimSpace(transcript) == "" {
		return ""
	}

	result, err := s.llm.CreateChatCompletion(ChatCompletionInput{
		SystemPrompt: text.titleSystem,
		Messages:     []model.Message{},
		UserInput:    text.titleRecent + transcript,
		Temperature:  float64Ptr(0.2),
	})
	if err != nil {
		for _, m := range recentMessages {
			if m.Role == "user" {
				return util.Truncate(collapseWhitespace(m.Content), 40)
			}
		}
		return util.Truncate("", 40)
	}

	cleaned := reTitleEdgeTrim.ReplaceAllString(result.Content, "")
	cleaned = reNewlineToSpace.ReplaceAllString(cleaned, " ")
	return util.Truncate(strings.TrimSpace(cleaned), 40)
}
