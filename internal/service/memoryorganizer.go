package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"snzstudio/internal/config"
	"snzstudio/internal/model"
	"snzstudio/internal/repository"
)

// MemoryOrganizerService ports memoryOrganizerService.ts: it proposes a memory
// reorganization plan (dedupe / rewrite / remove / create) via the LLM, with a
// heuristic fallback, and applies an approved plan. Locked memories are never
// updated or removed.
type MemoryOrganizerService struct {
	memories      *repository.MemoryRepository
	chats         *repository.ChatRepository
	llm           *LLMClient
	embeddingSync *EmbeddingSyncService
	cfg           *config.Config
}

// NewMemoryOrganizerService builds a MemoryOrganizerService.
func NewMemoryOrganizerService(memories *repository.MemoryRepository, chats *repository.ChatRepository, llm *LLMClient, embeddingSync *EmbeddingSyncService, cfg *config.Config) *MemoryOrganizerService {
	return &MemoryOrganizerService{memories: memories, chats: chats, llm: llm, embeddingSync: embeddingSync, cfg: cfg}
}

func isMemoryKind(value string) bool {
	return value == "semantic" || value == "procedural" || value == "episodic"
}

// sanitizePlan mirrors sanitizePlan: it keeps only well-formed changes (each
// requiring a reason and the fields appropriate to its action).
func sanitizePlan(summary string, rawChanges []model.MemoryOrganizationChange) model.MemoryOrganizationPlan {
	changes := []model.MemoryOrganizationChange{}
	for _, change := range rawChanges {
		memoryID := change.MemoryID
		kind := ""
		if isMemoryKind(change.Kind) {
			kind = change.Kind
		}
		title := strings.TrimSpace(change.Title)
		content := strings.TrimSpace(change.Content)
		reason := strings.TrimSpace(change.Reason)
		if reason == "" {
			continue
		}

		switch {
		case change.Action == "remove" && memoryID != "":
			changes = append(changes, model.MemoryOrganizationChange{Action: "remove", MemoryID: memoryID, Reason: reason})
		case change.Action == "create" && kind != "" && title != "" && content != "":
			changes = append(changes, model.MemoryOrganizationChange{Action: "create", Kind: kind, Title: title, Content: content, Reason: reason})
		case change.Action == "update" && memoryID != "" && kind != "" && title != "" && content != "":
			changes = append(changes, model.MemoryOrganizationChange{Action: "update", MemoryID: memoryID, Kind: kind, Title: title, Content: content, Reason: reason})
		}
	}
	return model.MemoryOrganizationPlan{Summary: strings.TrimSpace(summary), Changes: changes}
}

// buildFallbackPlan mirrors buildFallbackPlan: dedupe identical memories (removing
// the non-locked duplicates) and, failing that, synthesize one episodic memory from
// recent chat summaries.
func buildFallbackPlan(memories []model.Memory, summaries []model.ChatSummary, text *promptText) model.MemoryOrganizationPlan {
	seen := map[string]string{}
	changes := []model.MemoryOrganizationChange{}

	for _, memory := range memories {
		key := memory.Kind + ":" + strings.ToLower(strings.TrimSpace(memory.Title)) + ":" + strings.ToLower(strings.TrimSpace(memory.Content))
		if existingID, ok := seen[key]; ok {
			if memory.Locked {
				continue
			}
			changes = append(changes, model.MemoryOrganizationChange{
				Action:   "remove",
				MemoryID: memory.ID,
				Reason:   fmt.Sprintf(text.organizerDuplicate, existingID),
			})
			continue
		}
		seen[key] = memory.ID
	}

	if len(changes) == 0 {
		hasSummary := false
		for _, summary := range summaries {
			if strings.TrimSpace(summary.Summary) != "" {
				hasSummary = true
				break
			}
		}
		if hasSummary {
			parts := []string{}
			for _, summary := range summaries {
				if strings.TrimSpace(summary.Summary) == "" {
					continue
				}
				parts = append(parts, strings.TrimSpace(summary.Summary))
				if len(parts) == 2 {
					break
				}
			}
			changes = append(changes, model.MemoryOrganizationChange{
				Action:  "create",
				Kind:    "episodic",
				Title:   "Recent project progress",
				Content: strings.Join(parts, " "),
				Reason:  text.organizerFromSummary,
			})
		}
	}

	summary := text.organizerNoChanges
	if len(changes) > 0 {
		summary = text.organizerProposed
	}
	return model.MemoryOrganizationPlan{Summary: summary, Changes: changes}
}

// AnalyzeProject mirrors analyzeProject.
func (s *MemoryOrganizerService) AnalyzeProject(projectID string) (model.MemoryOrganizationPlan, error) {
	text := promptTextFor(s.cfg.Get().ProcessingLanguage)
	memories, err := s.memories.ListByProject(projectID)
	if err != nil {
		return model.MemoryOrganizationPlan{}, err
	}
	summaries, err := s.chats.ListSummariesByProject(projectID, false)
	if err != nil {
		return model.MemoryOrganizationPlan{}, err
	}
	if len(summaries) > 8 {
		summaries = summaries[:8]
	}

	if len(memories) == 0 && len(summaries) == 0 {
		return model.MemoryOrganizationPlan{
			Summary: text.organizerNothing,
			Changes: []model.MemoryOrganizationChange{},
		}, nil
	}

	memoryPayload := make([]map[string]any, len(memories))
	for i, memory := range memories {
		memoryPayload[i] = map[string]any{
			"id":      memory.ID,
			"kind":    memory.Kind,
			"title":   memory.Title,
			"content": memory.Content,
			"source":  memory.Source,
			"locked":  memory.Locked,
		}
	}
	summaryPayload := make([]map[string]any, len(summaries))
	for i, summary := range summaries {
		summaryPayload[i] = map[string]any{
			"chatId":  summary.ChatID,
			"summary": summary.Summary,
		}
	}
	memoryJSON, err := marshalJSONIndentNoEscape(memoryPayload)
	if err != nil {
		return model.MemoryOrganizationPlan{}, err
	}
	summaryJSON, err := marshalJSONIndentNoEscape(summaryPayload)
	if err != nil {
		return model.MemoryOrganizationPlan{}, err
	}

	userInput := strings.Join([]string{
		text.organizerMemories,
		memoryJSON,
		"recent chat summaries:",
		summaryJSON,
		strings.Join(text.organizerInstructions, "\n"),
	}, "\n\n")

	resp, llmErr := s.llm.CreateChatCompletion(ChatCompletionInput{
		SystemPrompt: text.organizerSystem,
		Messages:     []model.Message{},
		UserInput:    userInput,
		Temperature:  float64Ptr(0.1),
	})
	if llmErr != nil {
		return buildFallbackPlan(memories, summaries, text), nil
	}

	jsonText := extractJSONObject(resp.Content)
	var raw struct {
		Summary string                           `json:"summary"`
		Changes []model.MemoryOrganizationChange `json:"changes"`
	}
	if jsonText != "" {
		if err := json.Unmarshal([]byte(jsonText), &raw); err != nil {
			// A malformed JSON payload behaves like the TS catch: fall back.
			return buildFallbackPlan(memories, summaries, text), nil
		}
	}
	plan := sanitizePlan(raw.Summary, raw.Changes)
	if len(plan.Changes) > 0 || plan.Summary != "" {
		return plan, nil
	}
	return buildFallbackPlan(memories, summaries, text), nil
}

// ErrMalformedPlan reports a plan with a change sanitizePlan would drop: an
// unknown action, an invalid kind, or a missing field.
var ErrMalformedPlan = errors.New("service: the memory organization plan has a malformed change")

// ApplyProjectPlan applies a plan the client sends back after AnalyzeProject. The
// plan is sanitized again because nothing guarantees it is the one analyze
// returned, and a plan that loses a change to sanitizing is rejected whole rather
// than applied without it: a merge is a create plus the removes of what it
// replaces, and dropping the create alone would delete the originals with nothing
// in their place. AnalyzeProject only returns sanitized plans, so this never
// rejects one it produced.
//
// The repository applies the plan in one transaction, skipping changes to
// memories outside the project or locked. Embeddings are synced only after the
// commit, so an embedding failure cannot leave the plan half-applied.
func (s *MemoryOrganizerService) ApplyProjectPlan(projectID string, plan model.MemoryOrganizationPlan) ([]model.Memory, error) {
	sanitized := sanitizePlan(plan.Summary, plan.Changes)
	if len(sanitized.Changes) != len(plan.Changes) {
		return nil, ErrMalformedPlan
	}
	affectedMemoryIDs, err := s.memories.ApplyOrganization(projectID, sanitized.Changes)
	if err != nil {
		return nil, err
	}

	if len(affectedMemoryIDs) > 0 {
		if err := s.embeddingSync.SyncMemories(affectedMemoryIDs); err != nil {
			return nil, err
		}
	}
	if err := s.memories.RebuildSearchIndex(); err != nil {
		return nil, err
	}
	return s.memories.ListByProject(projectID)
}
