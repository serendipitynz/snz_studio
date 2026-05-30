package service

import (
	"regexp"
	"strings"

	"snzstudio/internal/model"
	"snzstudio/internal/util"
)

// SummaryService ports summaryService.ts: it keeps a rolling natural-language chat
// summary and generates chat titles, falling back to heuristics when the LLM is
// unreachable.
type SummaryService struct {
	llm *LLMClient
}

// NewSummaryService builds a SummaryService.
func NewSummaryService(llm *LLMClient) *SummaryService {
	return &SummaryService{llm: llm}
}

var (
	reTripleNewline  = regexp.MustCompile(`\n{3,}`)
	reTitleEdgeTrim  = regexp.MustCompile("^[\"'「『\\s\\p{Z}]+|[\"'」』\\s\\p{Z}]+$")
	reNewlineToSpace = regexp.MustCompile(`\n+`)
)

func summaryRoleLabel(role string) string {
	switch role {
	case "user":
		return "ユーザー"
	case "assistant":
		return "アシスタント"
	default:
		return "システム"
	}
}

// normalizeMessageForSummary mirrors normalizeMessageForSummary.
func normalizeMessageForSummary(m model.Message) string {
	return summaryRoleLabel(m.Role) + ": " + util.Truncate(collapseWhitespace(m.Content), 220)
}

func buildFallbackSummary(existingSummary string, recentMessages []model.Message) string {
	lines := make([]string, 0, 6)
	for _, m := range lastN(recentMessages, 6) {
		lines = append(lines, normalizeMessageForSummary(m))
	}
	recentTurns := joinLines(lines)

	sections := []string{}
	if existingSummary != "" {
		sections = append(sections, "これまでの要約:\n"+util.Truncate(existingSummary, 700))
	}
	if recentTurns != "" {
		sections = append(sections, "直近のやり取り:\n"+recentTurns)
	}
	return util.Truncate(strings.Join(sections, "\n\n"), 1600)
}

// UpdateSummary mirrors updateSummary.
func (s *SummaryService) UpdateSummary(existingSummary string, recentMessages []model.Message) string {
	lines := make([]string, 0, 8)
	for _, m := range lastN(recentMessages, 8) {
		lines = append(lines, normalizeMessageForSummary(m))
	}
	transcript := joinLines(lines)
	if strings.TrimSpace(transcript) == "" {
		return existingSummary
	}

	systemPrompt := strings.Join([]string{
		"あなたは会話要約専用のアシスタントです。",
		"会話全体の継続に必要な内容だけを自然な日本語で要約してください。",
		"雑談や冗長な表現は落とし、決定事項、未解決事項、重要な前提、直近の変更だけを残してください。",
		"箇条書きではなく、2から5文程度の短い自然文でまとめてください。",
	}, "\n")

	existingSection := "既存の要約はありません。"
	if existingSummary != "" {
		existingSection = "既存の要約:\n" + util.Truncate(existingSummary, 1200)
	}
	userInput := strings.Join([]string{
		existingSection,
		"直近の会話:\n" + transcript,
		"上記を踏まえて、今後の会話継続に使える自然な日本語の要約を更新してください。",
	}, "\n\n")

	result, err := s.llm.CreateChatCompletion(ChatCompletionInput{
		SystemPrompt: systemPrompt,
		Messages:     []model.Message{},
		UserInput:    userInput,
		Temperature:  float64Ptr(0.15),
	})
	if err != nil {
		return buildFallbackSummary(existingSummary, recentMessages)
	}
	return util.Truncate(strings.TrimSpace(reTripleNewline.ReplaceAllString(result.Content, "\n\n")), 1600)
}

// GenerateChatTitle mirrors generateChatTitle.
func (s *SummaryService) GenerateChatTitle(recentMessages []model.Message) string {
	nonSystem := make([]model.Message, 0, len(recentMessages))
	for _, m := range recentMessages {
		if m.Role != "system" {
			nonSystem = append(nonSystem, m)
		}
	}
	lines := make([]string, 0, 6)
	for _, m := range lastN(nonSystem, 6) {
		lines = append(lines, normalizeMessageForSummary(m))
	}
	transcript := joinLines(lines)
	if strings.TrimSpace(transcript) == "" {
		return ""
	}

	result, err := s.llm.CreateChatCompletion(ChatCompletionInput{
		SystemPrompt: strings.Join([]string{
			"あなたは会話タイトル生成専用のアシスタントです。",
			"会話の内容を表す短い日本語タイトルだけを返してください。",
			"長さは 8 文字から 28 文字程度。",
			"引用符、接頭辞、説明文、句点は付けないでください。",
		}, "\n"),
		Messages:    []model.Message{},
		UserInput:   "直近の会話:\n" + transcript,
		Temperature: float64Ptr(0.2),
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
