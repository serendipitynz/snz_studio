package service

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"snzstudio/internal/config"
	"snzstudio/internal/model"
	"snzstudio/internal/repository"
)

// Errors a turn can fail with. They are sentinels so the HTTP layer can map them
// to a status (409 for ErrTurnInProgress, 404 for ErrParticipantNotInChat) with
// errors.Is, matching how repository.ErrProjectReorderMismatch is handled.
var (
	ErrTurnInProgress            = errors.New("service: a turn is already running for this chat")
	ErrNotMultiAgentChat         = errors.New("service: chat is not a multi-agent chat")
	ErrChatNotFound              = errors.New("service: chat not found")
	ErrRosterEmpty               = errors.New("service: chat has no participants on its roster")
	ErrManualParticipantRequired = errors.New("service: turn rule is manual, so a participant must be named")
	ErrParticipantNotNameable    = errors.New("service: turn rule is round_robin, so a participant cannot be named")
	ErrParticipantNotInChat      = errors.New("service: participant does not belong to this chat")
	ErrParticipantRemoved        = errors.New("service: participant was removed from the roster")
	ErrEndpointUnavailable       = errors.New("service: participant endpoint did not accept the model")
)

// turnHistoryLimit caps how many past messages are mapped into a turn's prompt
// (design §4.3). Compressing older history through the summary service is still
// undecided (§8), so the cap is a constant rather than a setting.
const turnHistoryLimit = 20

// Speaker labels for messages that carry no participant_id: the human's own
// interventions and any assistant message left over from before the chat became
// multi-agent.
const (
	humanSpeakerLabel     = "ユーザー"
	assistantSpeakerLabel = "アシスタント"
)

// TurnEngine runs one turn of a multi-agent chat: it picks the speaker, builds
// that speaker's view of the conversation, streams the completion from the
// participant's own endpoint, and stores the result with its participant_id
// (docs/multi-agent-chat-design.md §4). It shares only the LLM client and the
// repositories with ChatService; the retrieval / memory / summary context of a
// single-assistant turn is deliberately not reused (§4.4).
type TurnEngine struct {
	chats        *repository.ChatRepository
	participants *repository.ParticipantRepository
	llm          *LLMClient
	cfg          *config.Config

	// running holds the chats with a turn in flight. The speaker is derived from
	// the last stored message and only becomes visible to the next request once
	// the turn finishes, so two overlapping requests would otherwise pick the same
	// participant and make it speak twice (§4.2 step 0).
	mu      sync.Mutex
	running map[string]bool
}

// NewTurnEngine builds a TurnEngine.
func NewTurnEngine(chats *repository.ChatRepository, participants *repository.ParticipantRepository, llm *LLMClient, cfg *config.Config) *TurnEngine {
	return &TurnEngine{
		chats:        chats,
		participants: participants,
		llm:          llm,
		cfg:          cfg,
		running:      map[string]bool{},
	}
}

func (e *TurnEngine) acquireTurn(chatID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running[chatID] {
		return false
	}
	e.running[chatID] = true
	return true
}

func (e *TurnEngine) releaseTurn(chatID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.running, chatID)
}

// RunTurn executes one turn of the chat and returns the stored message. A turn is
// one completion by one participant: continuous progression is the frontend
// calling this repeatedly (§4.1). participantID names the speaker under the
// manual turn rule and must be empty under round_robin.
//
// The turn is not bound to the caller's lifetime: the underlying stream runs on
// its own sliding deadline, so a disconnected client still gets the finished turn
// stored (§4.1). onDelta may be nil when no one is watching the stream.
func (e *TurnEngine) RunTurn(chatID, participantID string, onDelta func(string)) (*model.Message, error) {
	if !e.acquireTurn(chatID) {
		return nil, ErrTurnInProgress
	}
	defer e.releaseTurn(chatID)

	chat, err := e.chats.GetChat(chatID)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, ErrChatNotFound
	}
	if chat.Kind != model.ChatKindMultiAgent {
		return nil, ErrNotMultiAgentChat
	}

	messages, err := e.chats.ListMessages(chatID)
	if err != nil {
		return nil, err
	}
	speaker, err := e.selectSpeaker(chat, participantID, messages)
	if err != nil {
		return nil, err
	}

	if !e.endpointAccepts(speaker) {
		return nil, fmt.Errorf("%w: %s (%s at %s)", ErrEndpointUnavailable, speaker.DisplayName, speaker.ModelName, speaker.BaseURL)
	}

	knownSpeakers, err := e.participants.ListAll(chatID)
	if err != nil {
		return nil, err
	}

	log.Printf("[turn] completion start chatId=%s participantId=%s model=%s", chatID, speaker.ID, speaker.ModelName)
	var streamed strings.Builder
	result, err := e.llm.CreateChatCompletionStream(ChatCompletionInput{
		SystemPrompt: buildTurnSystemPrompt(chat, speaker, knownSpeakers),
		Messages:     mapHistoryForSpeaker(messages, speaker, knownSpeakers),
		UserInput:    buildTurnCue(speaker),
		Temperature:  float64Ptr(0.7),
		Target:       &CompletionTarget{BaseURL: speaker.BaseURL, Model: speaker.ModelName},
	}, func(chunk string) {
		streamed.WriteString(chunk)
		if onDelta != nil {
			onDelta(chunk)
		}
	})
	if err != nil {
		// No fallback to another participant, and no fallback text either (§4.2
		// step 2): a turn that did not happen must not enter the transcript, or
		// round_robin would advance past the participant that never spoke.
		log.Printf("[turn] completion failed chatId=%s participantId=%s reason=%v", chatID, speaker.ID, err)
		return nil, err
	}

	// The message is written once, after the stream completes, rather than being
	// created empty and filled in as ChatService does: a failed turn would
	// otherwise leave a blank participant message in the transcript, which
	// round_robin then reads as that participant having spoken.
	message, err := e.chats.AddMessage(repository.AddMessageInput{
		ChatID:          chatID,
		Role:            "assistant",
		Content:         result.Content,
		ResponseMs:      int64Ptr(result.ResponseMs),
		OutputTokens:    int64Ptr(result.OutputTokens),
		TokensPerSecond: float64Ptr(result.TokensPerSecond),
		ModelName:       strPtr(result.ModelName),
		ParticipantID:   strPtr(speaker.ID),
	})
	if err != nil {
		return nil, err
	}
	return &message, nil
}

// selectSpeaker applies the chat's turn rule. round_robin derives the speaker
// from the transcript instead of from server-side progression state, so a
// restarted server (or a second window) continues the cycle unchanged (§2).
func (e *TurnEngine) selectSpeaker(chat *model.Chat, participantID string, messages []model.Message) (*model.Participant, error) {
	participantID = strings.TrimSpace(participantID)
	if chat.TurnRule == model.TurnRuleManual {
		return e.namedSpeaker(chat.ID, participantID)
	}
	if participantID != "" {
		// Honouring it would let the UI nominate a speaker while a different
		// participant actually speaks, so the mismatch is reported instead.
		return nil, ErrParticipantNotNameable
	}

	roster, err := e.participants.ListRoster(chat.ID)
	if err != nil {
		return nil, err
	}
	if len(roster) == 0 {
		return nil, ErrRosterEmpty
	}
	last := lastParticipantID(messages)
	if last == "" {
		return &roster[0], nil
	}
	for i := range roster {
		if roster[i].ID == last {
			return &roster[(i+1)%len(roster)], nil
		}
	}
	// The last speaker is no longer on the roster, so the cycle has no position to
	// advance from; the roster's head restarts it (§4.2 step 1).
	return &roster[0], nil
}

func (e *TurnEngine) namedSpeaker(chatID, participantID string) (*model.Participant, error) {
	if participantID == "" {
		return nil, ErrManualParticipantRequired
	}
	participant, err := e.participants.GetParticipant(participantID)
	if err != nil {
		return nil, err
	}
	// chatID and participants.chat_id are two independent facts here, so this is a
	// real check rather than a restatement of the route (§3).
	if participant == nil || participant.ChatID != chatID {
		return nil, ErrParticipantNotInChat
	}
	if participant.DeletedAt != nil {
		return nil, ErrParticipantRemoved
	}
	return participant, nil
}

// endpointAccepts runs the pre-turn check of §4.2 step 2. EnsureModelLoaded only
// answers for LM Studio (it speaks the native /api/v1 routes), so a plain
// OpenAI-compatible endpoint falls through to CheckConnection rather than being
// rejected for not being LM Studio.
func (e *TurnEngine) endpointAccepts(participant *model.Participant) bool {
	if e.llm.EnsureModelLoaded(participant.ModelName, participant.BaseURL) {
		return true
	}
	return e.llm.CheckConnection(participant.BaseURL, participant.ModelName)
}

func lastParticipantID(messages []model.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if id := messages[i].ParticipantID; id != nil && *id != "" {
			return *id
		}
	}
	return ""
}

// speakerLabels maps participant id -> display name over every participant row,
// removed ones included: a past message keeps pointing at a participant that has
// since left the roster, and its speaker still has to be named (§3).
func speakerLabels(participants []model.Participant) map[string]string {
	labels := make(map[string]string, len(participants))
	for _, p := range participants {
		labels[p.ID] = p.DisplayName
	}
	return labels
}

// mapHistoryForSpeaker rewrites the transcript into the speaker's own point of
// view, which is what an OpenAI-compatible API can express: its own past turns
// become assistant messages, and everyone else's (participants and the human
// alike) become user messages prefixed with the speaker's name (§4.3).
func mapHistoryForSpeaker(messages []model.Message, speaker *model.Participant, participants []model.Participant) []model.Message {
	labels := speakerLabels(participants)
	mapped := make([]model.Message, 0, len(messages))
	for _, m := range messages {
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		if m.ParticipantID != nil && *m.ParticipantID == speaker.ID {
			mapped = append(mapped, model.Message{Role: "assistant", Content: m.Content})
			continue
		}
		mapped = append(mapped, model.Message{Role: "user", Content: speakerLabel(m, labels) + ": " + m.Content})
	}
	return lastN(mapped, turnHistoryLimit)
}

func speakerLabel(m model.Message, labels map[string]string) string {
	if m.ParticipantID != nil {
		if name, ok := labels[*m.ParticipantID]; ok && name != "" {
			return name
		}
	}
	if m.Role == "user" {
		return humanSpeakerLabel
	}
	return assistantSpeakerLabel
}

// buildTurnSystemPrompt assembles the speaker's system message: the chat-wide
// scene, the participant's role prompt, and the per-turn role reminder (§4.3).
// The reminder is repeated every turn because small models drift out of their
// role as the history grows and settle into agreeing with the previous speaker.
func buildTurnSystemPrompt(chat *model.Chat, speaker *model.Participant, participants []model.Participant) string {
	parts := make([]string, 0, 4)
	if scene := strings.TrimSpace(chat.ScenePrompt); scene != "" {
		parts = append(parts, scene)
	}
	if role := strings.TrimSpace(speaker.RolePrompt); role != "" {
		parts = append(parts, role)
	}
	if names := rosterNames(participants); names != "" {
		parts = append(parts, fmt.Sprintf("この会話の参加者: %s", names))
	}
	parts = append(parts, strings.Join([]string{
		fmt.Sprintf("あなたは「%s」としてのみ発言する。他の参加者の発言を代筆しない。", speaker.DisplayName),
		"発言の先頭に自分の名前や記号を付けない。本文だけを書く。",
		"直前の発言に同意するだけで終わらせず、自分の立場から具体的に述べる。",
		"1 回の発言は簡潔にまとめる。",
	}, "\n"))
	return strings.Join(parts, "\n\n")
}

func rosterNames(participants []model.Participant) string {
	names := make([]string, 0, len(participants))
	for _, p := range participants {
		if p.DeletedAt == nil && strings.TrimSpace(p.DisplayName) != "" {
			names = append(names, p.DisplayName)
		}
	}
	return strings.Join(names, ", ")
}

// buildTurnCue is the final user message of the prompt. A turn has no new human
// input, but LLMClient always appends one (and it is left unmodified, §1), so
// the slot carries the hand-off to this speaker instead of being left blank.
func buildTurnCue(speaker *model.Participant) string {
	return fmt.Sprintf("（進行）次は「%s」の番です。%s として発言してください。", speaker.DisplayName, speaker.DisplayName)
}
