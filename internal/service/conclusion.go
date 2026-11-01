package service

import (
	"errors"
	"fmt"
	"strings"

	"snzstudio/internal/model"
	"snzstudio/internal/util"
)

// ConclusionCharLimit caps the transcript a conclusion draft is generated from,
// counted in characters over the labelled lines the model receives. It is the
// budget the same default model already takes for a whole document
// (fullDocumentCharLimit), so a conversation is never asked to fit a larger
// window than the app hands that model elsewhere. A transcript over it is
// refused rather than clipped from the front: the head of a discussion is where
// its premises are, and a draft silently missing them reads as complete.
const ConclusionCharLimit = fullDocumentCharLimit

// conclusionRolePromptChars keeps the role summary a hint about stance rather
// than a second copy of every persona.
const conclusionRolePromptChars = 160

// ErrNothingToConclude is returned when the requested range holds no utterance.
var ErrNothingToConclude = errors.New("service: the conversation has no utterances to summarize")

// ConclusionTooLongError reports a transcript over ConclusionCharLimit, with
// the counted length so the caller can say by how much.
type ConclusionTooLongError struct {
	Chars int
	Limit int
}

func (e *ConclusionTooLongError) Error() string {
	return fmt.Sprintf("the conversation is %d characters, over the %d-character limit for a conclusion draft; choose a later starting utterance", e.Chars, e.Limit)
}

// DraftConclusion asks the default model what a multi-agent conversation
// decided and left open. It shares only the LLM client with UpdateSummary:
// that one keeps the last 8 messages at 220 characters each for continuity,
// while a conclusion has to see the whole range it is asked about. The draft is
// returned, never stored — it becomes a memory only through the save dialog
// (docs/multi-agent-chat-design.md §4.4), and chat_summaries stays the
// single-assistant flow's (§8.1).
//
// participants is every row of the chat, removed ones included, so an older
// speaker keeps their name and role.
func (s *SummaryService) DraftConclusion(messages []model.Message, participants []model.Participant) (string, error) {
	language := s.cfg.Get().ProcessingLanguage
	text := promptTextFor(language)
	byID := make(map[string]model.Participant, len(participants))
	for _, p := range participants {
		byID[p.ID] = p
	}

	lines := make([]string, 0, len(messages))
	spoke := map[string]bool{}
	chars := 0
	for _, m := range messages {
		// The rolls and effects go in with the body, as the speakers read them:
		// an outcome the conversation turned on is part of what it concluded.
		content := contentWithRecords(m, language)
		if m.Role == "system" || content == "" {
			continue
		}
		label := text.assistantSpeaker
		switch {
		case m.Role == "user":
			label = text.humanSpeaker
		case m.ParticipantID != nil:
			if p, ok := byID[*m.ParticipantID]; ok {
				label = p.DisplayName
				spoke[p.ID] = true
			}
		}
		line := label + ": " + content
		chars += runeLen(line)
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "", ErrNothingToConclude
	}
	if chars > ConclusionCharLimit {
		return "", &ConclusionTooLongError{Chars: chars, Limit: ConclusionCharLimit}
	}

	roles := []string{"- " + text.humanSpeaker + ": " + text.conclusionHumanRole}
	for _, p := range participants {
		if !spoke[p.ID] {
			continue
		}
		role := util.Truncate(collapseWhitespace(p.RolePrompt), conclusionRolePromptChars)
		if role == "" {
			role = text.conclusionNoRole
		}
		roles = append(roles, "- "+p.DisplayName+": "+role)
	}

	result, err := s.llm.CreateChatCompletion(ChatCompletionInput{
		SystemPrompt: text.conclusionSystem,
		Messages:     []model.Message{},
		UserInput: strings.Join([]string{
			text.conclusionRoster + joinLines(roles),
			text.conclusionTranscript + joinLines(lines),
			text.conclusionRequest,
		}, "\n\n"),
		Temperature: float64Ptr(0.15),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(reTripleNewline.ReplaceAllString(result.Content, "\n\n")), nil
}
