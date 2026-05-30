package service

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"snzstudio/internal/model"
	"snzstudio/internal/repository"
	"snzstudio/internal/util"
)

// ContextService ports contextService.ts: it assembles the per-turn prompt context
// (project settings, rolling summary, recent turns, procedural memories, explicit
// document/chat references, and hybrid-retrieved documents/memories) and the
// reference list attached to the assistant turn.
type ContextService struct {
	projects  *repository.ProjectRepository
	chats     *repository.ChatRepository
	documents *repository.DocumentRepository
	memories  *repository.MemoryRepository
	retrieval *RetrievalService
}

// NewContextService builds a ContextService.
func NewContextService(projects *repository.ProjectRepository, chats *repository.ChatRepository, documents *repository.DocumentRepository, memories *repository.MemoryRepository, retrieval *RetrievalService) *ContextService {
	return &ContextService{projects: projects, chats: chats, documents: documents, memories: memories, retrieval: retrieval}
}

var (
	reQuoteRequest        = regexp.MustCompile(`(?i)(引用|quote|quoted|引用して|原文|そのまま|抜き出|抜粋|該当箇所|当該箇所)`)
	reFullDocumentRequest = regexp.MustCompile(`(?i)(全文|全体|全内容|全部|全編|full document|entire document|whole document|文書全体|ドキュメント全体)`)
	reDocumentReview      = regexp.MustCompile(`(?i)(要約|まとめ|summary|review|説明|整理|構造|outline|全体像|レビュー)`)
	reThisDocument        = regexp.MustCompile(`(?i)(この document|this document|この文書|このドキュメント)`)
	reChatReference       = regexp.MustCompile(`(?i)(チャット|chat)`)
	reGenericQueryNoise   = regexp.MustCompile(`(?i)(引用|quote|quoted|原文|そのまま|抜き出して|抜粋して|該当箇所|この document|this document|この文書|このドキュメント|全文|全体|要約|まとめ|説明|してください|お願いします)`)
)

const fullDocumentCharLimit = 12000

func normalizeForMatch(input string) string {
	return strings.ToLower(norm.NFKC.String(input))
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// resolveExplicitDocument mirrors resolveExplicitDocument.
func resolveExplicitDocument(userInput string, documents []model.DocumentRecord) *model.DocumentRecord {
	normalizedInput := normalizeForMatch(userInput)
	matching := []model.DocumentRecord{}
	for _, doc := range documents {
		normalizedTitle := normalizeForMatch(doc.Title)
		if runeLen(normalizedTitle) >= 2 && strings.Contains(normalizedInput, normalizedTitle) {
			matching = append(matching, doc)
		}
	}
	sort.SliceStable(matching, func(i, j int) bool {
		return runeLen(matching[i].Title) > runeLen(matching[j].Title)
	})
	if len(matching) > 0 {
		doc := matching[0]
		return &doc
	}
	if len(documents) == 1 && reThisDocument.MatchString(userInput) {
		doc := documents[0]
		return &doc
	}
	return nil
}

// buildDocumentFocusedQuery mirrors buildDocumentFocusedQuery.
func buildDocumentFocusedQuery(userInput, documentTitle string) string {
	out := strings.ReplaceAll(userInput, documentTitle, " ")
	out = reGenericQueryNoise.ReplaceAllString(out, " ")
	return strings.TrimSpace(out)
}

// resolveExplicitChat mirrors resolveExplicitChat.
func resolveExplicitChat(userInput string, chats []model.Chat, currentChatID string) *model.Chat {
	if !reChatReference.MatchString(userInput) {
		return nil
	}
	normalizedInput := normalizeForMatch(userInput)
	matching := []model.Chat{}
	for _, chat := range chats {
		if chat.ID == currentChatID {
			continue
		}
		title := strings.TrimSpace(chat.Title)
		if title == "" {
			continue
		}
		normalizedTitle := normalizeForMatch(title)
		if runeLen(normalizedTitle) >= 2 && strings.Contains(normalizedInput, normalizedTitle) {
			matching = append(matching, chat)
		}
	}
	sort.SliceStable(matching, func(i, j int) bool {
		return runeLen(matching[i].Title) > runeLen(matching[j].Title)
	})
	if len(matching) == 0 {
		return nil
	}
	chat := matching[0]
	return &chat
}

func shouldIncludeFullDocument(userInput string, document model.DocumentRecord) bool {
	documentText := firstNonEmpty(document.ContentText, document.DerivedText, document.Note)
	if documentText == "" || runeLen(documentText) > fullDocumentCharLimit {
		return false
	}
	return reQuoteRequest.MatchString(userInput) || reFullDocumentRequest.MatchString(userInput) || reDocumentReview.MatchString(userInput)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func formatDocumentContext(reference model.RetrievedDocumentReference) string {
	matchedChunks := "- no matching chunks were found"
	if len(reference.Chunks) > 0 {
		lines := make([]string, 0, len(reference.Chunks))
		for _, chunk := range reference.Chunks {
			lines = append(lines, fmt.Sprintf("- chunk %d: %s", chunk.ChunkIndex+1, util.Truncate(chunk.Content, 400)))
		}
		matchedChunks = joinLines(lines)
	}
	fullDocumentSection := ""
	if reference.IncludeFullDocument {
		fullDocumentSection = "\nFull document content:\n" + reference.FullDocumentContent
	}
	categoryPart := ""
	if reference.Category != "" {
		categoryPart = ":" + reference.Category
	}
	return fmt.Sprintf("[Document%s] %s\nRetrieval mode: %s\nMatched passages:\n%s%s",
		categoryPart, reference.Label, reference.RetrievalMode, matchedChunks, fullDocumentSection)
}

func formatChatContext(title, summary string, recentMessages []model.Message) string {
	recentTurns := "- no recent turns"
	if len(recentMessages) > 0 {
		lines := make([]string, 0, len(recentMessages))
		for _, m := range recentMessages {
			lines = append(lines, fmt.Sprintf("- %s: %s", m.Role, util.Truncate(m.Content, 220)))
		}
		recentTurns = joinLines(lines)
	}
	summaryText := summary
	if summaryText == "" {
		summaryText = "No summary available."
	}
	return fmt.Sprintf("[Referenced chat] %s\nSummary:\n%s\n\nRecent turns:\n%s", title, summaryText, recentTurns)
}

// Assemble mirrors assemble(chatId, userInput).
func (s *ContextService) Assemble(chatID, userInput string) (*model.AssembledContext, error) {
	chat, err := s.chats.GetChat(chatID)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, errors.New("Chat not found")
	}
	project, err := s.projects.GetProject(chat.ProjectID)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, errors.New("Project not found")
	}

	summary := ""
	if cs, err := s.chats.GetSummary(chatID); err != nil {
		return nil, err
	} else if cs != nil {
		summary = cs.Summary
	}

	proceduralMemories, err := s.memories.ListByProjectAndKind(project.ID, "procedural")
	if err != nil {
		return nil, err
	}
	if len(proceduralMemories) > 4 {
		proceduralMemories = proceduralMemories[:4]
	}

	projectDocuments, err := s.documents.ListByProject(project.ID)
	if err != nil {
		return nil, err
	}
	projectChats, err := s.chats.ListByProject(project.ID)
	if err != nil {
		return nil, err
	}

	explicitDocument := resolveExplicitDocument(userInput, projectDocuments)
	explicitChat := resolveExplicitChat(userInput, projectChats, chat.ID)
	isQuoteRequest := reQuoteRequest.MatchString(userInput)

	recentMessages, err := s.chats.ListRecentMessages(chatID, 6)
	if err != nil {
		return nil, err
	}

	explicitChatSummary := ""
	var explicitChatMessages []model.Message
	if explicitChat != nil {
		if cs, err := s.chats.GetSummary(explicitChat.ID); err != nil {
			return nil, err
		} else if cs != nil {
			explicitChatSummary = cs.Summary
		}
		explicitChatMessages, err = s.chats.ListRecentMessages(explicitChat.ID, 6)
		if err != nil {
			return nil, err
		}
	}

	documentRefs, err := s.retrieval.SearchDocuments(project.ID, userInput, 4, 3)
	if err != nil {
		return nil, err
	}
	memoryRefs, err := s.retrieval.SearchMemories(project.ID, userInput, 4)
	if err != nil {
		return nil, err
	}

	if explicitDocument != nil {
		focusedQuery := buildDocumentFocusedQuery(userInput, explicitDocument.Title)
		chunkLimit := 3
		if isQuoteRequest {
			chunkLimit = 5
		}
		quoteChunks, err := s.retrieval.SearchChunksInDocument(explicitDocument.ID, focusedQuery, chunkLimit)
		if err != nil {
			return nil, err
		}
		fullDocumentContent := firstNonEmpty(explicitDocument.ContentText, explicitDocument.DerivedText, explicitDocument.Note)

		excerpt := util.Truncate(fullDocumentContent, 220)
		if len(quoteChunks) > 0 {
			lines := make([]string, 0, len(quoteChunks))
			for _, chunk := range quoteChunks {
				lines = append(lines, fmt.Sprintf("[chunk %d] %s", chunk.ChunkIndex+1, util.Truncate(chunk.Content, 150)))
			}
			excerpt = joinLines(lines)
		}
		score := 1.02
		retrievalMode := "search"
		if isQuoteRequest {
			score = 1.15
			retrievalMode = "quote"
		}
		explicitReference := model.RetrievedDocumentReference{
			SearchReference: model.SearchReference{
				SourceType: "document",
				SourceID:   explicitDocument.ID,
				Label:      explicitDocument.Title,
				Excerpt:    excerpt,
				Score:      score,
			},
			Category:            explicitDocument.Category,
			Chunks:              quoteChunks,
			IncludeFullDocument: shouldIncludeFullDocument(userInput, *explicitDocument),
			FullDocumentContent: fullDocumentContent,
			RetrievalMode:       retrievalMode,
		}
		documentRefs = append([]model.RetrievedDocumentReference{explicitReference}, documentRefs...)
	}

	// Dedupe document refs by sourceId (keep first), then cap.
	normalizedDocumentRefs := dedupeDocumentRefs(documentRefs)
	docCap := 4
	if explicitDocument != nil {
		docCap = 3
	}
	if len(normalizedDocumentRefs) > docCap {
		normalizedDocumentRefs = normalizedDocumentRefs[:docCap]
	}

	references := []model.SearchReference{
		{
			SourceType: "project",
			SourceID:   project.ID,
			Label:      project.Title + " settings",
			Excerpt:    util.Truncate(firstNonEmpty(project.Description, project.SystemPrompt, project.Title), 220),
			Score:      1,
		},
	}
	if strings.TrimSpace(summary) != "" {
		references = append(references, model.SearchReference{
			SourceType: "summary",
			SourceID:   chat.ID,
			Label:      "Chat summary",
			Excerpt:    util.Truncate(summary, 220),
			Score:      0.92,
		})
	}
	if explicitChat != nil {
		title := explicitChat.Title
		if title == "" {
			title = "(undefined)"
		}
		chatExcerpt := explicitChatSummary
		if chatExcerpt == "" {
			contents := make([]string, 0, len(explicitChatMessages))
			for _, m := range explicitChatMessages {
				contents = append(contents, m.Content)
			}
			chatExcerpt = strings.Join(contents, " ")
		}
		references = append(references, model.SearchReference{
			SourceType: "chat",
			SourceID:   explicitChat.ID,
			Label:      "Chat: " + title,
			Excerpt:    util.Truncate(chatExcerpt, 220),
			Score:      0.9,
		})
	}
	for index, memory := range proceduralMemories {
		references = append(references, model.SearchReference{
			SourceType: "memory",
			SourceID:   memory.ID,
			Label:      "[procedural] " + memory.Title,
			Excerpt:    util.Truncate(memory.Content, 220),
			Score:      0.88 - float64(index)*0.02,
		})
	}
	for _, ref := range normalizedDocumentRefs {
		if !hasReference(references, ref.SourceType, ref.SourceID) {
			references = append(references, ref.SearchReference)
		}
	}
	for _, ref := range memoryRefs {
		if !hasReference(references, ref.SourceType, ref.SourceID) {
			references = append(references, ref)
		}
	}

	promptParts := []string{
		"Project title: " + project.Title,
	}
	if project.Description != "" {
		promptParts = append(promptParts, "Project description:\n"+project.Description)
	}
	if project.SystemPrompt != "" {
		promptParts = append(promptParts, "Project system prompt:\n"+project.SystemPrompt)
	}
	if summary != "" {
		promptParts = append(promptParts, "Chat summary:\n"+summary)
	}
	if len(proceduralMemories) > 0 {
		lines := make([]string, 0, len(proceduralMemories))
		for _, memory := range proceduralMemories {
			lines = append(lines, fmt.Sprintf("- %s: %s", memory.Title, memory.Content))
		}
		promptParts = append(promptParts, "Persistent procedural memory:\n"+joinLines(lines))
	}
	if isQuoteRequest {
		promptParts = append(promptParts, "Document quote mode is active. Quote only from the provided document passages or full document content. If the exact supporting text is not present, say so plainly.")
	}
	if explicitChat != nil {
		title := explicitChat.Title
		if title == "" {
			title = "(undefined)"
		}
		promptParts = append(promptParts, "Referenced project chat:\n"+formatChatContext(title, explicitChatSummary, explicitChatMessages))
	}
	if len(normalizedDocumentRefs) > 0 {
		docs := make([]string, 0, len(normalizedDocumentRefs))
		for _, ref := range normalizedDocumentRefs {
			docs = append(docs, formatDocumentContext(ref))
		}
		promptParts = append(promptParts, "Relevant project documents:\n"+strings.Join(docs, "\n\n"))
	}
	if len(memoryRefs) > 0 {
		lines := make([]string, 0, len(memoryRefs))
		for _, ref := range memoryRefs {
			lines = append(lines, fmt.Sprintf("- %s: %s", ref.Label, ref.Excerpt))
		}
		promptParts = append(promptParts, "Relevant project memories:\n"+joinLines(lines))
	}
	promptContext := strings.Join(promptParts, "\n\n")

	if len(references) > 8 {
		references = references[:8]
	}

	var targetDocumentTitle *string
	if explicitDocument != nil {
		title := explicitDocument.Title
		targetDocumentTitle = &title
	}

	return &model.AssembledContext{
		Project:             *project,
		Chat:                *chat,
		Summary:             summary,
		RecentMessages:      recentMessages,
		PromptContext:       promptContext,
		References:          references,
		IsQuoteRequest:      isQuoteRequest,
		TargetDocumentTitle: targetDocumentTitle,
	}, nil
}

func dedupeDocumentRefs(refs []model.RetrievedDocumentReference) []model.RetrievedDocumentReference {
	seen := map[string]bool{}
	out := make([]model.RetrievedDocumentReference, 0, len(refs))
	for _, ref := range refs {
		if seen[ref.SourceID] {
			continue
		}
		seen[ref.SourceID] = true
		out = append(out, ref)
	}
	return out
}

func hasReference(references []model.SearchReference, sourceType, sourceID string) bool {
	for _, ref := range references {
		if ref.SourceType == sourceType && ref.SourceID == sourceID {
			return true
		}
	}
	return false
}
