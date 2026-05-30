package service

import (
	"encoding/json"
	"fmt"
	"strings"

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
}

// NewMemoryOrganizerService builds a MemoryOrganizerService.
func NewMemoryOrganizerService(memories *repository.MemoryRepository, chats *repository.ChatRepository, llm *LLMClient, embeddingSync *EmbeddingSyncService) *MemoryOrganizerService {
	return &MemoryOrganizerService{memories: memories, chats: chats, llm: llm, embeddingSync: embeddingSync}
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
func buildFallbackPlan(memories []model.Memory, summaries []model.ChatSummary) model.MemoryOrganizationPlan {
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
				Reason:   fmt.Sprintf("同一内容の memory (%s) と重複しています。", existingID),
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
				Reason:  "直近の chat summary から継続判断に役立つ経緯を 1 件だけ補完します。",
			})
		}
	}

	summary := "整頓対象は見つかりませんでした。"
	if len(changes) > 0 {
		summary = "重複統合と recent summary 由来の補完候補を作成しました。"
	}
	return model.MemoryOrganizationPlan{Summary: summary, Changes: changes}
}

// AnalyzeProject mirrors analyzeProject.
func (s *MemoryOrganizerService) AnalyzeProject(projectID string) (model.MemoryOrganizationPlan, error) {
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
			Summary: "整理対象の memory と chat summary がありません。",
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
		"現在の memories:",
		memoryJSON,
		"recent chat summaries:",
		summaryJSON,
		strings.Join([]string{
			"次の JSON だけを返してください。",
			`{ "summary": "short summary", "changes": [`,
			`  { "action": "create"|"update"|"remove", "memoryId": "existing id for update/remove", "kind": "semantic|procedural|episodic", "title": "title", "content": "content", "reason": "reason" }`,
			"] }",
			"更新は既存 memory の整理に限定し、無意味な全面書き換えは避けてください。",
			"locked=true の memory は update/remove しないでください。",
			"雑談は memory にしないでください。",
			"changes は最大 8 件に抑えてください。",
		}, "\n"),
	}, "\n\n")

	resp, llmErr := s.llm.CreateChatCompletion(ChatCompletionInput{
		SystemPrompt: strings.Join([]string{
			"あなたは project memory の整理専用アシスタントです。",
			"semantic / procedural / episodic の区別を守り、重複統合、書き換え、不要候補の削除、新規 memory 候補の作成だけを提案してください。",
			"必ず JSON のみを返してください。",
		}, "\n"),
		Messages:    []model.Message{},
		UserInput:   userInput,
		Temperature: float64Ptr(0.1),
	})
	if llmErr != nil {
		return buildFallbackPlan(memories, summaries), nil
	}

	jsonText := extractJSONObject(resp.Content)
	var raw struct {
		Summary string                           `json:"summary"`
		Changes []model.MemoryOrganizationChange `json:"changes"`
	}
	if jsonText != "" {
		if err := json.Unmarshal([]byte(jsonText), &raw); err != nil {
			// A malformed JSON payload behaves like the TS catch: fall back.
			return buildFallbackPlan(memories, summaries), nil
		}
	}
	plan := sanitizePlan(raw.Summary, raw.Changes)
	if len(plan.Changes) > 0 || plan.Summary != "" {
		return plan, nil
	}
	return buildFallbackPlan(memories, summaries), nil
}

// ApplyProjectPlan mirrors applyProjectPlan.
func (s *MemoryOrganizerService) ApplyProjectPlan(projectID string, plan model.MemoryOrganizationPlan) ([]model.Memory, error) {
	existingList, err := s.memories.ListByProject(projectID)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]model.Memory, len(existingList))
	for _, memory := range existingList {
		existing[memory.ID] = memory
	}

	affectedMemoryIDs := []string{}
	for _, change := range plan.Changes {
		var current *model.Memory
		if change.MemoryID != "" {
			if m, ok := existing[change.MemoryID]; ok {
				current = &m
			}
		}

		switch {
		case change.Action == "remove" && change.MemoryID != "":
			if current != nil && current.Locked {
				continue
			}
			if _, err := s.memories.DeleteMemory(change.MemoryID); err != nil {
				return nil, err
			}

		case change.Action == "update" && change.MemoryID != "" && change.Kind != "" && change.Title != "" && change.Content != "":
			if current != nil && current.Locked {
				continue
			}
			updated, err := s.memories.UpdateMemory(repository.UpdateMemoryInput{
				MemoryID: change.MemoryID,
				Kind:     change.Kind,
				Title:    change.Title,
				Content:  change.Content,
			})
			if err != nil {
				return nil, err
			}
			if updated != nil {
				affectedMemoryIDs = append(affectedMemoryIDs, updated.ID)
			}

		case change.Action == "create" && change.Kind != "" && change.Title != "" && change.Content != "":
			created, err := s.memories.CreateMemory(repository.CreateMemoryInput{
				ProjectID: projectID,
				Kind:      change.Kind,
				Title:     change.Title,
				Content:   change.Content,
				Source:    "organized",
				Locked:    false,
			})
			if err != nil {
				return nil, err
			}
			affectedMemoryIDs = append(affectedMemoryIDs, created.ID)
		}
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
