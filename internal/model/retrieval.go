package model

// This file mirrors the retrieval / context / memory-organization interfaces in
// backend/src/lib/types.ts that the service layer (Phase 5) produces. JSON tags
// match the property names the React frontend expects (api/client.ts), so these
// structs serialise to the same shape the old Node backend produced.

// SearchReference mirrors the SearchReference interface. It is the base shape for
// every reference attached to an assistant turn or returned by the review API.
type SearchReference struct {
	SourceType string  `json:"sourceType"`
	SourceID   string  `json:"sourceId"`
	Label      string  `json:"label"`
	Excerpt    string  `json:"excerpt"`
	Score      float64 `json:"score"`
}

// RetrievedDocumentChunk mirrors the RetrievedDocumentChunk interface.
type RetrievedDocumentChunk struct {
	ChunkID    string  `json:"chunkId"`
	ChunkIndex int     `json:"chunkIndex"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
}

// RetrievedDocumentReference mirrors the RetrievedDocumentReference interface
// (which extends SearchReference with sourceType pinned to "document"). It is the
// rich, document-specific reference used to build the prompt context. Note the
// frontend only ever consumes the base SearchReference fields of a reference, so
// AssembledContext.References stores the flattened base shape; this richer struct
// is used internally by ContextService while assembling the prompt.
type RetrievedDocumentReference struct {
	SearchReference
	Category            string                   `json:"category,omitempty"`
	Chunks              []RetrievedDocumentChunk `json:"chunks"`
	IncludeFullDocument bool                     `json:"includeFullDocument"`
	FullDocumentContent string                   `json:"fullDocumentContent"`
	RetrievalMode       string                   `json:"retrievalMode"`
}

// AssembledContext mirrors the AssembledContext interface returned by
// ContextService.assemble and consumed by ChatService / ReviewService.
type AssembledContext struct {
	Project             Project           `json:"project"`
	Chat                Chat              `json:"chat"`
	Summary             string            `json:"summary"`
	RecentMessages      []Message         `json:"recentMessages"`
	PromptContext       string            `json:"promptContext"`
	References          []SearchReference `json:"references"`
	IsQuoteRequest      bool              `json:"isQuoteRequest"`
	TargetDocumentTitle *string           `json:"targetDocumentTitle"`
}

// MemoryOrganizationChange mirrors the MemoryOrganizationChange interface. The
// optional fields use omitempty so a "remove" change marshals without kind/title/
// content, matching the TS objects that omit undefined properties.
type MemoryOrganizationChange struct {
	Action   string `json:"action"`
	MemoryID string `json:"memoryId,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Title    string `json:"title,omitempty"`
	Content  string `json:"content,omitempty"`
	Reason   string `json:"reason"`
}

// MemoryOrganizationPlan mirrors the MemoryOrganizationPlan interface.
type MemoryOrganizationPlan struct {
	Summary string                     `json:"summary"`
	Changes []MemoryOrganizationChange `json:"changes"`
}
