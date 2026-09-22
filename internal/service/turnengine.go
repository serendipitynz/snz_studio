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
	ErrParticipantNotNameable    = errors.New("service: turn rule derives the speaker, so a participant cannot be named")
	ErrParticipantNotInChat      = errors.New("service: participant does not belong to this chat")
	ErrParticipantRemoved        = errors.New("service: participant was removed from the roster")
	ErrEndpointUnavailable       = errors.New("service: participant endpoint did not accept the model")
	// ErrUtteranceOnlyDirective is found after generation, so it only ever
	// reaches a caller inside an open stream and has no HTTP status of its own.
	ErrUtteranceOnlyDirective = errors.New("service: the utterance held nothing but its addressee directive")
)

// turnHistoryLimit caps how many past messages are mapped into a turn's prompt
// (design §4.3). Older history is dropped rather than summarised (§8): a summary
// call per turn would double the latency of a turn on a local model, and the
// drift seen on small models is role drift, which the per-turn reminder
// addresses, rather than forgotten facts. 30 keeps a whole 20-question game and
// several rounds of a four-speaker roster in view at ~200 characters a turn.
const turnHistoryLimit = 30

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
// (docs/multi-agent-chat-design.md §4). It shares the LLM client and the
// repositories with ChatService, and takes the project's documents and memories
// through the project material assembler; the summary and the rest of a
// single-assistant turn's PromptContext are deliberately not reused (§4.4).
type TurnEngine struct {
	chats        *repository.ChatRepository
	participants *repository.ParticipantRepository
	llm          *LLMClient
	cfg          *config.Config
	material     TurnMaterialAssembler

	// running holds the chats with a turn in flight. The speaker is derived from
	// the last stored message and only becomes visible to the next request once
	// the turn finishes, so two overlapping requests would otherwise pick the same
	// participant and make it speak twice (§4.2 step 0).
	mu      sync.Mutex
	running map[string]bool

	// messageWrites holds one lock per chat for the writes that must not
	// interleave with each other: storing a human message, and applying a preset.
	// A lock is never removed once created — dropping one another goroutine is
	// about to take would need refcounting, to save a mutex per chat that has been
	// written to in this process.
	messageWrites map[string]*sync.Mutex
}

// NewTurnEngine builds a TurnEngine.
func NewTurnEngine(chats *repository.ChatRepository, participants *repository.ParticipantRepository, llm *LLMClient, cfg *config.Config, material TurnMaterialAssembler) *TurnEngine {
	return &TurnEngine{
		chats:         chats,
		participants:  participants,
		llm:           llm,
		cfg:           cfg,
		material:      material,
		running:       map[string]bool{},
		messageWrites: map[string]*sync.Mutex{},
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

// WithTurnExcluded runs fn while holding the chat's turn slot, and answers
// ErrTurnInProgress without running fn when a turn already holds it. It is for a
// write that invalidates a turn's premise rather than merely racing it: applying
// a preset deletes the roster outright, and a turn that picked its speaker before
// the delete would store a message whose participant_id no longer resolves —
// exactly the breakage the logical removal of §3 exists to prevent. Taking the
// same slot the turn takes is what makes the two mutually exclusive; a lock of
// its own would let them run side by side.
func (e *TurnEngine) WithTurnExcluded(chatID string, fn func() error) error {
	if !e.acquireTurn(chatID) {
		return ErrTurnInProgress
	}
	defer e.releaseTurn(chatID)
	return fn()
}

// WithMessageWrite runs fn while holding the chat's message-write lock, waiting
// for it rather than refusing. Storing a human message and applying a preset both
// take it, which is what stops an intervention from landing between the apply's
// emptiness check and its roster replacement — leaving a preset applied to a
// conversation that has already been spoken in.
//
// It is deliberately not the turn slot. Speaking into a conversation while a turn
// runs is intended (§4.4), so a human message must not be refused, nor made to
// wait out a completion that can take tens of seconds. A turn's own message needs
// no lock here: applying a preset holds the turn slot, so the two are already
// exclusive.
func (e *TurnEngine) WithMessageWrite(chatID string, fn func() error) error {
	lock := e.messageWriteLock(chatID)
	lock.Lock()
	defer lock.Unlock()
	return fn()
}

func (e *TurnEngine) messageWriteLock(chatID string) *sync.Mutex {
	e.mu.Lock()
	defer e.mu.Unlock()
	lock, ok := e.messageWrites[chatID]
	if !ok {
		lock = &sync.Mutex{}
		e.messageWrites[chatID] = lock
	}
	return lock
}

// SpeakerChoice is who a turn picked, announced before generation starts so the
// spectator view names the speaker from the engine's own choice rather than a
// copy of the turn rule (design §4.6.6).
type SpeakerChoice struct {
	Participant *model.Participant `json:"participant"`
	// ModelName is the model the completion will run on, the workspace default
	// when the participant inherits it.
	ModelName string `json:"modelName"`
	// Weights is the calculation that picked the speaker, one entry per
	// participant, for a rule that weighs the roster. The rules that derive the
	// speaker by position leave it empty (never nil, so the frame always carries
	// an array).
	Weights []SpeakerWeight `json:"weights"`
}

// SpeakerWeight is one participant's weight and the factors multiplied into it.
// It carries the display name as well as the id, so a participant added after
// the client last read the roster is still named in the breakdown.
type SpeakerWeight struct {
	ParticipantID string         `json:"participantId"`
	DisplayName   string         `json:"displayName"`
	Weight        float64        `json:"weight"`
	Factors       []WeightFactor `json:"factors"`
}

// WeightFactor is one named factor of a weight.
type WeightFactor struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// RunTurn executes one turn of the chat and returns the stored message. A turn is
// one completion by one participant: continuous progression is the frontend
// calling this repeatedly (§4.1). participantID names the speaker under the
// manual turn rule and must be empty under the rules that derive it.
//
// onSpeaker is called once, after every pre-turn check has passed and before
// generation starts: every refusal the HTTP layer maps to a status (409, 404,
// 400, 502) happens before it, and only failures after it can occur once the
// caller has committed to a stream (§4.6.6).
//
// The turn is not bound to the caller's lifetime: the underlying stream runs on
// its own sliding deadline, so a disconnected client still gets the finished turn
// stored (§4.1). onSpeaker and onDelta may be nil when no one is watching.
func (e *TurnEngine) RunTurn(chatID, participantID string, onSpeaker func(SpeakerChoice), onDelta func(string)) (*model.Message, error) {
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
	speaker, weights, err := e.selectSpeaker(chat, participantID, messages)
	if err != nil {
		return nil, err
	}

	// The target the completion will actually use: a participant leaves the
	// endpoint or the model blank to inherit the workspace setting, and the two
	// fall back independently. Resolving it here — through the same resolveTarget
	// the completion calls — is what lets the check and the error below name what
	// was really tried, instead of printing a blank where the inherited field was.
	//
	// The resolved values, not the participant's raw ones, are what the completion
	// is then handed: resolveTarget passes a non-empty field through unchanged, so
	// this pins the inheritance to the settings the check ran against. Handing over
	// the raw fields would let the completion re-resolve them from a snapshot taken
	// later, and a workspace setting saved mid-turn would send the turn to a
	// combination nothing checked.
	settings := e.cfg.Get()
	effectiveBaseURL, effectiveModel := resolveTarget(
		&CompletionTarget{BaseURL: speaker.BaseURL, Model: speaker.ModelName},
		settings.LLMBaseURL, settings.LLMModel,
	)
	target := &CompletionTarget{BaseURL: effectiveBaseURL, Model: effectiveModel}

	if !e.endpointAccepts(effectiveBaseURL, effectiveModel) {
		return nil, fmt.Errorf("%w: %s (%s at %s)", ErrEndpointUnavailable, speaker.DisplayName, effectiveModel, effectiveBaseURL)
	}
	if onSpeaker != nil {
		onSpeaker(SpeakerChoice{Participant: speaker, ModelName: effectiveModel, Weights: weights})
	}

	knownSpeakers, err := e.participants.ListAll(chatID)
	if err != nil {
		return nil, err
	}

	material := e.assembleMaterial(chat, speaker, messages)

	log.Printf("[turn] completion start chatId=%s participantId=%s model=%s references=%d", chatID, speaker.ID, effectiveModel, len(material.References))
	history, finalUserMessage := buildTurnPrompt(mapHistoryForSpeaker(messages, speaker, knownSpeakers), speaker)
	var streamed strings.Builder
	result, err := e.llm.CreateChatCompletionStream(ChatCompletionInput{
		SystemPrompt: buildTurnSystemPrompt(material.Prompt, chat, speaker, knownSpeakers),
		Messages:     history,
		UserInput:    finalUserMessage,
		Temperature:  float64Ptr(0.7),
		Target:       target,
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

	// Whom the utterance calls on is fixed here, once, for every rule: a chat
	// switched to weighted later reads the calls already in its window, and
	// re-reading bodies then would answer differently once a name has changed
	// (§4.6.5). The directive is only asked for under weighted, but a trailing one
	// is removed under any rule so no control syntax reaches the transcript.
	content, addressees := detectAddressees(result.Content, speaker.ID, onRoster(knownSpeakers))
	if strings.TrimSpace(content) == "" {
		return nil, ErrUtteranceOnlyDirective
	}

	// The message is written once, after the stream completes, rather than being
	// created empty and filled in as ChatService does: a failed turn would
	// otherwise leave a blank participant message in the transcript, which
	// round_robin then reads as that participant having spoken. The references go
	// in the same transaction for the same reason: stored separately, a failure
	// to store them would fail the turn with the utterance already in the
	// transcript, and round_robin would advance past a turn reported as failed.
	message, err := e.chats.AddMessageWithReferences(repository.AddMessageInput{
		ChatID:          chatID,
		Role:            "assistant",
		Content:         content,
		ResponseMs:      int64Ptr(result.ResponseMs),
		OutputTokens:    int64Ptr(result.OutputTokens),
		TokensPerSecond: float64Ptr(result.TokensPerSecond),
		ModelName:       strPtr(result.ModelName),
		ParticipantID:   strPtr(speaker.ID),

		AddressedParticipantIDs: addressees,
	}, referenceInputs(material.References))
	if err != nil {
		return nil, err
	}
	return &message, nil
}

// assembleMaterial never fails the turn: a broken document or memory search
// would otherwise stop every turn of an auto-advancing conversation, so the
// speaker goes on without material and the failure is left in the log (§4.4).
// This is distinct from the reference store failing after generation, which
// AddMessageWithReferences makes impossible to observe on its own. An embedding
// endpoint failure never reaches here — EmbeddingClient disables itself and
// retrieval continues on keywords, the same degradation a single-assistant turn
// gets.
func (e *TurnEngine) assembleMaterial(chat *model.Chat, speaker *model.Participant, messages []model.Message) *TurnMaterial {
	material, err := e.material.AssembleTurnMaterial(chat, speaker, messages)
	if err != nil {
		log.Printf("[turn] project material unavailable chatId=%s participantId=%s reason=%v", chat.ID, speaker.ID, err)
		return &TurnMaterial{}
	}
	return material
}

func referenceInputs(references []model.SearchReference) []repository.ReferenceInput {
	inputs := make([]repository.ReferenceInput, len(references))
	for i, ref := range references {
		inputs[i] = repository.ReferenceInput{
			SourceType: ref.SourceType,
			SourceID:   ref.SourceID,
			Label:      ref.Label,
			Excerpt:    ref.Excerpt,
			Score:      ref.Score,
		}
	}
	return inputs
}

// selectSpeaker applies the chat's turn rule. The derived rules read the speaker
// out of the transcript instead of server-side progression state, so a restarted
// server (or a second window) continues unchanged (§2). The weights are the
// calculation behind the pick, empty for the rules that pick by position.
func (e *TurnEngine) selectSpeaker(chat *model.Chat, participantID string, messages []model.Message) (*model.Participant, []SpeakerWeight, error) {
	participantID = strings.TrimSpace(participantID)
	if chat.TurnRule == model.TurnRuleManual {
		speaker, err := e.namedSpeaker(chat.ID, participantID)
		return speaker, []SpeakerWeight{}, err
	}
	if participantID != "" {
		// Honouring it would let the UI nominate a speaker while a different
		// participant actually speaks, so the mismatch is reported instead.
		return nil, nil, ErrParticipantNotNameable
	}

	roster, err := e.participants.ListRoster(chat.ID)
	if err != nil {
		return nil, nil, err
	}
	if len(roster) == 0 {
		return nil, nil, ErrRosterEmpty
	}
	if chat.TurnRule == model.TurnRuleWeighted {
		// A facilitator that is unset or has left the roster exempts no one, and
		// the rule goes on weighing everyone alike.
		speaker, weights := weightedSpeaker(roster, rosterIndexOf(roster, chat.FacilitatorID), messages)
		return speaker, weights, nil
	}
	if chat.TurnRule == model.TurnRuleFacilitatorAlternating {
		// An unset facilitator and one that has left the roster are the same case
		// here, and both fall through to round_robin rather than failing the turn
		// (§4.2 step 1): the rule is the chat's standing setting, so refusing would
		// stop an auto-advancing conversation on a state a removal can reach at any
		// time, and the panel can say what is missing where it can also be fixed.
		if facilitator := rosterIndexOf(roster, chat.FacilitatorID); facilitator >= 0 {
			return alternatingSpeaker(roster, facilitator, messages), []SpeakerWeight{}, nil
		}
	}
	return roundRobinSpeaker(roster, messages), []SpeakerWeight{}, nil
}

func rosterIndexOf(roster []model.Participant, participantID string) int {
	if participantID == "" {
		return -1
	}
	for i := range roster {
		if roster[i].ID == participantID {
			return i
		}
	}
	return -1
}

func roundRobinSpeaker(roster []model.Participant, messages []model.Message) *model.Participant {
	last := lastParticipantID(messages)
	if last == "" {
		return &roster[0]
	}
	for i := range roster {
		if roster[i].ID == last {
			return &roster[(i+1)%len(roster)]
		}
	}
	// The last speaker is no longer on the roster, so the cycle has no position to
	// advance from; the roster's head restarts it (§4.2 step 1).
	return &roster[0]
}

// alternatingSpeaker implements facilitator_alternating: the facilitator and the
// rest of the roster speak one utterance each in turn, the rest cycling in
// sort_order (§4.2 step 1).
//
// The human's own interventions carry no participant_id, and round_robin looks
// straight past them; here the last message is read whoever spoke it, so an
// intervention is answered by the facilitator. That is the same choice as the
// opening turn, where a game master sets the scene before anyone acts.
func alternatingSpeaker(roster []model.Participant, facilitator int, messages []model.Message) *model.Participant {
	others := make([]*model.Participant, 0, len(roster)-1)
	for i := range roster {
		if i != facilitator {
			others = append(others, &roster[i])
		}
	}
	if len(others) == 0 {
		return &roster[facilitator]
	}
	if last := lastSpeaker(messages); last != roster[facilitator].ID {
		return &roster[facilitator]
	}
	// The facilitator has just spoken, so the roster advances one past whoever
	// spoke before it. A previous speaker that has left the roster leaves the
	// cycle without a position, and the head restarts it — round_robin's fallback.
	previous := lastParticipantIDExcept(messages, roster[facilitator].ID)
	for i, other := range others {
		if other.ID == previous {
			return others[(i+1)%len(others)]
		}
	}
	return others[0]
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

// endpointAccepts runs the pre-turn check of §4.2 step 2 against the resolved
// target, not the participant's raw fields: a participant that inherits the
// workspace model would otherwise be checked with an empty model name, which
// EnsureModelLoaded always refuses. EnsureModelLoaded only answers for LM Studio
// (it speaks the native /api/v1 routes), so a plain OpenAI-compatible endpoint
// falls through to CheckConnection rather than being rejected for not being LM
// Studio.
func (e *TurnEngine) endpointAccepts(baseURL, modelName string) bool {
	if e.llm.EnsureModelLoaded(modelName, baseURL) {
		return true
	}
	return e.llm.CheckConnection(baseURL, modelName)
}

// lastSpeaker is the participant id of the last message, empty when the
// transcript is empty or its last message is the human's (or an assistant
// message left over from before the chat became multi-agent).
func lastSpeaker(messages []model.Message) string {
	if len(messages) == 0 {
		return ""
	}
	if id := messages[len(messages)-1].ParticipantID; id != nil {
		return *id
	}
	return ""
}

func lastParticipantIDExcept(messages []model.Message, excluded string) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if id := messages[i].ParticipantID; id != nil && *id != "" && *id != excluded {
			return *id
		}
	}
	return ""
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

// buildTurnSystemPrompt assembles the speaker's system message: the project's
// project material, the chat-wide scene, the participant's role prompt, and
// the per-turn role reminder (§4.3). The material comes first so that the scene
// and the role, which decide how the speaker talks, are the last word before the
// reminder. The reminder is repeated every turn because small models drift out of
// their role as the history grows and settle into agreeing with the previous
// speaker.
func buildTurnSystemPrompt(material string, chat *model.Chat, speaker *model.Participant, participants []model.Participant) string {
	parts := make([]string, 0, 5)
	if material = strings.TrimSpace(material); material != "" {
		parts = append(parts, material)
	}
	if scene := strings.TrimSpace(chat.ScenePrompt); scene != "" {
		parts = append(parts, scene)
	}
	if role := strings.TrimSpace(speaker.RolePrompt); role != "" {
		parts = append(parts, role)
	}
	if names := rosterNames(participants); names != "" {
		parts = append(parts, fmt.Sprintf("この会話の参加者: %s", names))
	}
	// Each line answers a failure seen with small models: they echo the
	// "name: body" shape the history is mapped into, write the other speakers'
	// lines as well as their own, settle into agreeing, and stop honouring the
	// scene's length rule once the history grows.
	reminder := []string{
		fmt.Sprintf("あなたは「%s」としてのみ発言する。他の参加者の発言や動作を代筆しない。1 回の発言に複数人分の会話を入れない。", speaker.DisplayName),
		"発言の先頭に自分の名前や記号を付けない。本文だけを書く。",
		"直前の発言のどこに反応しているかが分かるように述べる。同意するだけで終わらせず、自分の立場から具体的に述べる。",
		"発言の長さは場面設定の指定に従う。指定がなければ簡潔にまとめる。",
	}
	// Only weighted reads the call, so only weighted asks for the directive: under
	// the other rules it would invite the model to name someone the rule then
	// passes over, and their prompts stay as they were.
	if chat.TurnRule == model.TurnRuleWeighted {
		reminder = append(reminder, "次に話してほしい相手がいるときだけ、発言の最後の行に [次: 表示名] と書く（複数なら読点で区切る）。いなければ書かない。")
	}
	parts = append(parts, strings.Join(reminder, "\n"))
	return strings.Join(parts, "\n\n")
}

// onRoster keeps the participants still on the roster: only they can be called
// on, while ListAll also returns removed ones so past speakers keep their names.
func onRoster(participants []model.Participant) []model.Participant {
	roster := make([]model.Participant, 0, len(participants))
	for _, p := range participants {
		if p.DeletedAt == nil {
			roster = append(roster, p)
		}
	}
	return roster
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

// buildTurnPrompt splits the mapped transcript into the history and the final
// user message. A chat completion answers its last user message, and LLMClient
// always fills that slot (§1), so the utterance the speaker has to answer goes
// there rather than behind a hand-off line.
//
// Why: a model that reasons before answering reads a trailing "it is your turn"
// as an instruction about the conversation rather than as something to answer,
// spends the turn deciding whether it should speak at all, and returns reasoning
// with no content — which RunTurn can only fail. Measured against the bundled
// twenty-questions preset on gemma-4-e4b (2026-09-19): with the hand-off last,
// two of four turns came back empty and the others spent 184-454 tokens on an
// answer of one word; with the question last, four of four answered in 6 tokens.
//
// The hand-off remains for the turns with nothing to answer: the opening turn,
// and a manual re-nomination of the participant who just spoke.
func buildTurnPrompt(mapped []model.Message, speaker *model.Participant) ([]model.Message, string) {
	if n := len(mapped); n > 0 && mapped[n-1].Role == "user" {
		return mapped[:n-1], mapped[n-1].Content
	}
	return mapped, buildTurnCue(speaker)
}

// buildTurnCue is the hand-off used when the prompt has no utterance for the
// speaker to answer (see buildTurnPrompt).
func buildTurnCue(speaker *model.Participant) string {
	return fmt.Sprintf("（進行）次は「%s」の番です。%s として発言してください。", speaker.DisplayName, speaker.DisplayName)
}
