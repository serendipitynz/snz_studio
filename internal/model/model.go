// Package model holds the domain entities returned by the repository (and later
// the service / HTTP) layers. It mirrors the relevant interfaces in
// backend/src/lib/types.ts. JSON tags match the property names the React
// frontend expects, so these structs serialise to the same shape the old Node
// backend produced. Nullable columns are pointers so they marshal to JSON null
// (rather than a zero value) when absent.
package model

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Project mirrors the Project interface. chatCount is derived (COUNT of chats).
type Project struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	SystemPrompt string `json:"systemPrompt"`
	SortOrder    int    `json:"sortOrder"`
	ChatCount    int    `json:"chatCount"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	// LastActivityAt is the later of UpdatedAt and its chats' updated_at: a
	// message bumps only the chat, so UpdatedAt alone misses the project's use.
	LastActivityAt string `json:"lastActivityAt"`
}

// DocumentRecord mirrors the DocumentRecord interface. Type and Category are
// validated by the DB CHECK constraint / doccategory.IsValid respectively.
//
// SharedWithAll true puts the document in the common project material: a
// multi-agent turn reaches it even when its speaker does not receive the project
// material (docs/multi-agent-chat-design.md §4.4). It is an explicit value, never
// derived from Category — see the design note, and the default is false.
type DocumentRecord struct {
	ID            string   `json:"id"`
	ProjectID     string   `json:"projectId"`
	Type          string   `json:"type"`
	Category      string   `json:"category"`
	Title         string   `json:"title"`
	Note          string   `json:"note"`
	Tags          []string `json:"tags"`
	DerivedText   string   `json:"derivedText"`
	ContentText   string   `json:"contentText"`
	SharedWithAll bool     `json:"sharedWithAll"`
	FilePath      *string  `json:"filePath"`
	MimeType      *string  `json:"mimeType"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
}

// Chat kinds and turn rules. The columns carry no CHECK constraint (the
// migration only documents the allowed values), so these are the single
// definition the Go layers validate and compare against.
const (
	ChatKindAssistant  = "assistant"
	ChatKindMultiAgent = "multi_agent"

	TurnRuleRoundRobin = "round_robin"
	TurnRuleManual     = "manual"
	// TurnRuleFacilitatorAlternating picks the facilitator and the rest of the
	// roster one utterance each in turn, the rest cycling in sort_order
	// (docs/multi-agent-chat-design.md §2). It is not the future rule where a
	// facilitator model names the next speaker (§7).
	TurnRuleFacilitatorAlternating = "facilitator_alternating"
	// TurnRuleWeighted gives every roster participant a weight — the product of
	// the factors that apply to it — and the heaviest speaks, ties going to the
	// longest silent and then to sort_order (§4.6.1). A call a message made
	// raises its addressees while it is unanswered.
	TurnRuleWeighted = "weighted"
)

// UntitledChatTitle names a chat whose title is empty in what the app writes
// out of it: the markdown export's heading, and the title of a preset exported
// from it, which becomes the chat title wherever that preset is applied without
// one. It is Japanese like the rest of those files (the markdown export's
// section headings), not following the UI language: both files are written by
// the server, which does not know it.
const UntitledChatTitle = "無題のチャット"

// Chat mirrors the Chat interface. Kind is "assistant" (the single-assistant
// chat) or "multi_agent"; TurnRule, ScenePrompt, FacilitatorID and StateSheet only carry
// meaning for the latter (see docs/multi-agent-chat-design.md §3).
//
// FacilitatorID names the participant TurnRuleFacilitatorAlternating
// interleaves. Empty means unset, and the id may also name a participant that
// has since left the roster; both make the rule fall back to round_robin's
// derivation (§4.2 step 1). TurnRuleWeighted reads the same setting to exempt
// the facilitator from the recent-speaker factor, and without one exempts no one.
type Chat struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	Title         string `json:"title"`
	IsTemporary   bool   `json:"isTemporary"`
	Kind          string `json:"kind"`
	TurnRule      string `json:"turnRule"`
	ScenePrompt   string `json:"scenePrompt"`
	FacilitatorID string `json:"facilitatorId"`
	StateSheet    string `json:"stateSheet"`
	// Commands is the slash commands the chat's store-time pass handles
	// (design §4.8.7).
	Commands  ChatCommands `json:"commands"`
	CreatedAt string       `json:"createdAt"`
	UpdatedAt string       `json:"updatedAt"`
}

// ChatCommands is a chat's commands as the JSON the chat, the API and a preset
// share: command name → that command's settings, a present key enabling the
// command (design §4.8.7). A nil field is a command the chat does not use.
type ChatCommands struct {
	Roll *RollSettings `json:"roll,omitempty"`
	// The effect commands (design §4.8.8) have no settings; each is enabled on
	// its own, so a table can let the models /set the place without letting them
	// /add to anyone's HP.
	Add *EffectSettings `json:"add,omitempty"`
	Use *EffectSettings `json:"use,omitempty"`
	Set *EffectSettings `json:"set,omitempty"`
}

// EffectSettings is an effect command's settings, of which there are none yet:
// the key being present is what enables the command.
type EffectSettings struct{}

// RollSettings is /roll's settings. Target is the default a /roll without
// 目標N is compared against; 0 compares nothing and records the total alone
// (design §4.8.3 item 4).
type RollSettings struct {
	Target int `json:"target"`
}

// ErrInvalidCommands marks a commands object the chat cannot store. The HTTP
// layer maps it to 400, so the wrapped message says what is wrong.
var ErrInvalidCommands = errors.New("model: invalid commands")

// UnmarshalJSON refuses a command name the app does not know, so a misspelt
// command is reported rather than silently left disabled (design §4.8.7). A
// null value enables the command with its default settings, like {}.
func (c *ChatCommands) UnmarshalJSON(raw []byte) error {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return fmt.Errorf("%w: commands must be an object of command name to settings", ErrInvalidCommands)
	}
	*c = ChatCommands{}
	for name, value := range entries {
		switch name {
		case "roll":
			c.Roll = &RollSettings{}
			if err := unmarshalSettings(value, c.Roll); err != nil {
				return fmt.Errorf("%w: commands.roll: %v", ErrInvalidCommands, err)
			}
		case "add", "use", "set":
			settings := &EffectSettings{}
			if err := unmarshalSettings(value, settings); err != nil {
				return fmt.Errorf("%w: commands.%s: %v", ErrInvalidCommands, name, err)
			}
			switch name {
			case "add":
				c.Add = settings
			case "use":
				c.Use = settings
			default:
				c.Set = settings
			}
		default:
			return fmt.Errorf("%w: unknown command %q (known: roll, add, use, set)", ErrInvalidCommands, name)
		}
	}
	return nil
}

func unmarshalSettings(raw json.RawMessage, settings any) error {
	if string(raw) == "null" {
		return nil
	}
	return json.Unmarshal(raw, settings)
}

// Validate checks the ranges the settings' types cannot.
func (c ChatCommands) Validate() error {
	if c.Roll != nil && (c.Roll.Target < 0 || c.Roll.Target > DiceTargetMax) {
		return fmt.Errorf("%w: commands.roll.target must be a whole number from 0 to %d", ErrInvalidCommands, DiceTargetMax)
	}
	return nil
}

// RecentChat is a chat listed across projects (the dashboard's recent chats),
// carrying its project's title so the row can name where it lives.
type RecentChat struct {
	Chat
	ProjectTitle string `json:"projectTitle"`
}

// Message mirrors the Message interface. The metric fields are nil until the
// assistant turn is finalised. ParticipantID is nil for the conventional user /
// assistant messages and set for a multi-agent participant's turn; the speaker's
// display name is resolved through Participant, which is never hard-deleted.
//
// AddressedParticipantIDs is the set of participants the message called on,
// fixed when it was stored and never re-derived from Content (design §4.6.5).
// It is empty, never nil, for a message that called on no one.
//
// DiceRolls is what the message's /roll threw, fixed when it was stored with the
// command line taken out of Content (design §4.8.3 item 2). It is empty, never
// nil, for a message that rolled nothing.
//
// StateEffects is what the message's effect command did to a state sheet, or
// why it did nothing, fixed when it was stored (design §4.8.8). It is empty,
// never nil, for a message that carried no effect command.
//
// Reasoning is what the model thought before answering, kept for reading only:
// it is never part of a prompt. OutputTokens includes ReasoningTokens, which is
// nil when the model did not think.
type Message struct {
	ID              string   `json:"id"`
	ChatID          string   `json:"chatId"`
	Role            string   `json:"role"`
	Content         string   `json:"content"`
	Reasoning       string   `json:"reasoning"`
	CreatedAt       string   `json:"createdAt"`
	ResponseMs      *int64   `json:"responseMs"`
	OutputTokens    *int64   `json:"outputTokens"`
	ReasoningTokens *int64   `json:"reasoningTokens"`
	TokensPerSecond *float64 `json:"tokensPerSecond"`
	ModelName       *string  `json:"modelName"`
	ParticipantID   *string  `json:"participantId"`

	AddressedParticipantIDs []string      `json:"addressedParticipantIds"`
	DiceRolls               []DiceRoll    `json:"diceRolls"`
	StateEffects            []StateEffect `json:"stateEffects"`
}

// DiceRoll is one /roll the app threw for a message (design §4.8.1 判定の記録).
// Target 0 means nothing was compared, and Success is then nil.
type DiceRoll struct {
	Command    string `json:"command"`
	Expression string `json:"expression"`
	Action     string `json:"action"`
	Dice       []int  `json:"dice"`
	Modifier   int    `json:"modifier"`
	Total      int    `json:"total"`
	Target     int    `json:"target"`
	Success    *bool  `json:"success"`
}

// StateEffect is one effect command's outcome on a state sheet (design §4.8.8
// 効果の記録). Kind is the command: add, use or set. ParticipantID is empty for
// the chat's shared sheet, and Owner is 共通 or the participant's display name
// as it stood. Delta is what add and use changed the leading number by, with
// Expression, Dice and Modifier set when add rolled it. Before is nil when the
// sheet had no line for the item. An effect that was not applied left the
// sheet as it was, has no After, and says why in Reason.
type StateEffect struct {
	Command       string  `json:"command"`
	Kind          string  `json:"kind"`
	Owner         string  `json:"owner"`
	ParticipantID string  `json:"participantId"`
	Item          string  `json:"item"`
	Delta         int     `json:"delta"`
	Expression    string  `json:"expression,omitempty"`
	Dice          []int   `json:"dice,omitempty"`
	Modifier      int     `json:"modifier,omitempty"`
	Value         string  `json:"value,omitempty"`
	Before        *string `json:"before"`
	After         string  `json:"after"`
	Applied       bool    `json:"applied"`
	Reason        string  `json:"reason,omitempty"`
}

// Why an effect was not applied (design §4.8.8).
const (
	EffectMissingItem = "missing_item" // add / use: the owner's sheet has no line for the item
	EffectNotInteger  = "not_integer"  // add / use: the value does not start with a whole number
	EffectNotPositive = "not_positive" // use: the leading number is below 1
	EffectOverLimit   = "over_limit"   // the sheet would exceed its limit
)

// DiceTargetMax bounds a target, whether a /roll names it or the chat holds it
// as the default. The highest total a /roll can reach is 20d100+999, so a
// larger target could never be met and is refused as a slip instead.
const DiceTargetMax = 9999

// Participant is one speaker of a multi-agent chat: a display name, a role
// prompt, and the endpoint (BaseURL + ModelName) its turns are generated
// against. DeletedAt nil means the participant is on the roster; non-nil means
// it was removed from the roster and the row survives only so past messages
// keep resolving to a name (docs/multi-agent-chat-design.md §3).
//
// ReceivesProjectMaterial false excludes the participant's turns from the
// project material: nothing is retrieved, nothing is added to the system
// prompt and no reference is stored (§4.4). It defaults to true, so a roster is
// one where everyone reads the same material until a participant is taken out of
// it — a game master who knows the scenario the players must not.
type Participant struct {
	ID                      string  `json:"id"`
	ChatID                  string  `json:"chatId"`
	DisplayName             string  `json:"displayName"`
	RolePrompt              string  `json:"rolePrompt"`
	BaseURL                 string  `json:"baseUrl"`
	ModelName               string  `json:"modelName"`
	SortOrder               int     `json:"sortOrder"`
	ReceivesProjectMaterial bool    `json:"receivesProjectMaterial"`
	StateSheet              string  `json:"stateSheet"`
	CreatedAt               string  `json:"createdAt"`
	DeletedAt               *string `json:"deletedAt"`
}

// The state sheet limits, counted in runes (design §4.7.3 item 4). They are
// checked when a sheet is stored and never applied by truncating the prompt, so
// what the panel shows is exactly what the model reads.
const (
	ChatStateSheetMaxRunes        = 400
	ParticipantStateSheetMaxRunes = 200
)

// ChatSummary mirrors the ChatSummary interface.
type ChatSummary struct {
	ChatID    string `json:"chatId"`
	Summary   string `json:"summary"`
	UpdatedAt string `json:"updatedAt"`
}

// Memory mirrors the Memory interface.
//
// SharedWithAll has the same meaning as on DocumentRecord (§4.4). Its default at
// creation comes from Source, which records where the memory came from rather
// than guessing it: a memory saved from an utterance of the conversation
// ("multi_agent") was heard by every participant and is already shared, while a
// manually added one or one extracted from a single-assistant chat was never
// spoken there.
type Memory struct {
	ID            string  `json:"id"`
	ProjectID     string  `json:"projectId"`
	Kind          string  `json:"kind"`
	Title         string  `json:"title"`
	Content       string  `json:"content"`
	SourceChatID  *string `json:"sourceChatId"`
	Source        string  `json:"source"`
	Locked        bool    `json:"locked"`
	SharedWithAll bool    `json:"sharedWithAll"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

// AssistantMessageReference mirrors the AssistantMessageReference interface.
type AssistantMessageReference struct {
	ID                 string  `json:"id"`
	AssistantMessageID string  `json:"assistantMessageId"`
	SourceType         string  `json:"sourceType"`
	SourceID           string  `json:"sourceId"`
	Label              string  `json:"label"`
	Excerpt            string  `json:"excerpt"`
	Score              float64 `json:"score"`
	CreatedAt          string  `json:"createdAt"`
}

// MessageWithReferences mirrors the MessageWithReferences interface. The
// embedded Message is anonymous so its fields are inlined into the JSON object,
// matching the TS object spread `{ ...message, references }`.
type MessageWithReferences struct {
	Message
	References []AssistantMessageReference `json:"references"`
}
