// Package model holds the domain entities returned by the repository (and later
// the service / HTTP) layers. It mirrors the relevant interfaces in
// backend/src/lib/types.ts. JSON tags match the property names the React
// frontend expects, so these structs serialise to the same shape the old Node
// backend produced. Nullable columns are pointers so they marshal to JSON null
// (rather than a zero value) when absent.
package model

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
}

// DocumentRecord mirrors the DocumentRecord interface. Type and Category are
// validated by the DB CHECK constraint / doccategory.IsValid respectively.
type DocumentRecord struct {
	ID          string   `json:"id"`
	ProjectID   string   `json:"projectId"`
	Type        string   `json:"type"`
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Note        string   `json:"note"`
	Tags        []string `json:"tags"`
	DerivedText string   `json:"derivedText"`
	ContentText string   `json:"contentText"`
	FilePath    *string  `json:"filePath"`
	MimeType    *string  `json:"mimeType"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

// Chat kinds and turn rules. The columns carry no CHECK constraint (the
// migration only documents the allowed values), so these are the single
// definition the Go layers validate and compare against.
const (
	ChatKindAssistant  = "assistant"
	ChatKindMultiAgent = "multi_agent"

	TurnRuleRoundRobin = "round_robin"
	TurnRuleManual     = "manual"
)

// Chat mirrors the Chat interface. Kind is "assistant" (the single-assistant
// chat) or "multi_agent"; TurnRule and ScenePrompt only carry meaning for the
// latter (see docs/multi-agent-chat-design.md §3).
type Chat struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	Title       string `json:"title"`
	IsTemporary bool   `json:"isTemporary"`
	Kind        string `json:"kind"`
	TurnRule    string `json:"turnRule"`
	ScenePrompt string `json:"scenePrompt"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// Message mirrors the Message interface. The metric fields are nil until the
// assistant turn is finalised. ParticipantID is nil for the conventional user /
// assistant messages and set for a multi-agent participant's turn; the speaker's
// display name is resolved through Participant, which is never hard-deleted.
type Message struct {
	ID              string   `json:"id"`
	ChatID          string   `json:"chatId"`
	Role            string   `json:"role"`
	Content         string   `json:"content"`
	CreatedAt       string   `json:"createdAt"`
	ResponseMs      *int64   `json:"responseMs"`
	OutputTokens    *int64   `json:"outputTokens"`
	TokensPerSecond *float64 `json:"tokensPerSecond"`
	ModelName       *string  `json:"modelName"`
	ParticipantID   *string  `json:"participantId"`
}

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
	CreatedAt               string  `json:"createdAt"`
	DeletedAt               *string `json:"deletedAt"`
}

// ChatSummary mirrors the ChatSummary interface.
type ChatSummary struct {
	ChatID    string `json:"chatId"`
	Summary   string `json:"summary"`
	UpdatedAt string `json:"updatedAt"`
}

// Memory mirrors the Memory interface.
type Memory struct {
	ID           string  `json:"id"`
	ProjectID    string  `json:"projectId"`
	Kind         string  `json:"kind"`
	Title        string  `json:"title"`
	Content      string  `json:"content"`
	SourceChatID *string `json:"sourceChatId"`
	Source       string  `json:"source"`
	Locked       bool    `json:"locked"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
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
