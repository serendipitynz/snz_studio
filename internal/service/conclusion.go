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
	byID := make(map[string]model.Participant, len(participants))
	for _, p := range participants {
		byID[p.ID] = p
	}

	lines := make([]string, 0, len(messages))
	spoke := map[string]bool{}
	chars := 0
	for _, m := range messages {
		if m.Role == "system" || strings.TrimSpace(m.Content) == "" {
			continue
		}
		label := assistantSpeakerLabel
		switch {
		case m.Role == "user":
			label = humanSpeakerLabel
		case m.ParticipantID != nil:
			if p, ok := byID[*m.ParticipantID]; ok {
				label = p.DisplayName
				spoke[p.ID] = true
			}
		}
		line := label + ": " + strings.TrimSpace(m.Content)
		chars += runeLen(line)
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "", ErrNothingToConclude
	}
	if chars > ConclusionCharLimit {
		return "", &ConclusionTooLongError{Chars: chars, Limit: ConclusionCharLimit}
	}

	roles := []string{"- " + humanSpeakerLabel + ": 会話を見ている人間。この人の発言は採用・却下の判断として扱う"}
	for _, p := range participants {
		if !spoke[p.ID] {
			continue
		}
		role := util.Truncate(collapseWhitespace(p.RolePrompt), conclusionRolePromptChars)
		if role == "" {
			role = "（役割の指定なし）"
		}
		roles = append(roles, "- "+p.DisplayName+": "+role)
	}

	result, err := s.llm.CreateChatCompletion(ChatCompletionInput{
		SystemPrompt: conclusionSystemPrompt,
		Messages:     []model.Message{},
		UserInput: strings.Join([]string{
			"参加者と役割:\n" + joinLines(roles),
			"会話:\n" + joinLines(lines),
			"この会話で何が決まり、何が未決かを、指定の形式で書いてください。",
		}, "\n\n"),
		Temperature: float64Ptr(0.15),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(reTripleNewline.ReplaceAllString(result.Content, "\n\n")), nil
}

// conclusionSystemPrompt separates decided from open, and keeps an objection
// that a participant raised because its role told it to (a critic, a devil's
// advocate) out of the decisions unless someone else took it up. Without that
// line a small model reports the last strong objection as the outcome.
var conclusionSystemPrompt = strings.Join([]string{
	"あなたは議論の結論を抜き出す専用のアシスタントです。",
	"会話は、役割を与えられた複数の参加者と、それを見ている人間（ユーザー）の発言です。",
	"誰が何を言ったかを並べるのではなく、会話の結果として何が決まったか、何が決まっていないかを書いてください。",
	"",
	"- 決定: 参加者が合意した判断、またはユーザーが採用した判断。それぞれに短い理由を添える。",
	"- 未決: 意見が分かれたまま、または結論が出ていない論点。",
	"- 参加者が役割（反対役・批判役・懐疑役など）に沿って出した反論や懸念は、他の参加者が同意したかユーザーが採用した場合を除き、決定に入れない。扱うなら未決に書く。",
	"- ユーザーが却下した案は決定に入れない。",
	"- 会話に無い推測や一般論を足さない。",
	"- 決定が無ければ「決定」の下に「- なし」と書く。未決も同じ。",
	"",
	"出力は次の形式だけにしてください。前置きや締めの文は書かないでください。",
	"決定:",
	"- （判断）— （理由）",
	"未決:",
	"- （論点）",
}, "\n")
