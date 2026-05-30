package service

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"snzstudio/internal/model"
	"snzstudio/internal/repository"
	"snzstudio/internal/util"
)

// MemoryService ports memoryService.ts: rule-based automatic memory extraction from
// user messages, plus explicit "覚えて" extraction backed by the LLM (with a
// heuristic fallback). Temporary chats are excluded by the caller (ChatService).
type MemoryService struct {
	memories *repository.MemoryRepository
	llm      *LLMClient
}

// NewMemoryService builds a MemoryService.
func NewMemoryService(memories *repository.MemoryRepository, llm *LLMClient) *MemoryService {
	return &MemoryService{memories: memories, llm: llm}
}

// durableCue is one rule in the DURABLE_CUES table.
type durableCue struct {
	regex *regexp.Regexp
	kind  string
	title string
}

// durableCues ports the DURABLE_CUES array, in order. The English cues carry the
// case-insensitive flag (the TS `/i`); the Japanese cues are case-sensitive (`/u`).
var durableCues = []durableCue{
	{regexp.MustCompile(`(?i)\b(i prefer|prefer|always|please use|use .* for|style|tone|format|answer in|respond in|avoid using)\b`), "procedural", "Working preference"},
	{regexp.MustCompile(`(を優先|を使って|を使いたい|は使わない|は禁止|避けてください|簡潔に|日本語で|英語で|箇条書きで|短く|詳しく|敬語で|常に|毎回|必ず|今後は|以後は)`), "procedural", "Working preference"},
	{regexp.MustCompile(`(?i)\b(my |i am |i work on|project uses|we use|my stack|our stack|this project uses)\b`), "semantic", "Project fact"},
	{regexp.MustCompile(`(このプロジェクトは|このアプリは|技術スタックは|フロントエンドは|バックエンドは|データベースは|前提です|使っています|採用しています|利用しています|で構成します|を使います|にします)`), "semantic", "Project fact"},
	{regexp.MustCompile(`(?i)\b(we decided|we shipped|last time|on \d{4}-\d{2}-\d{2}|previously)\b`), "episodic", "Project event"},
	{regexp.MustCompile(`(前回|以前|先ほど|さっき|今日|昨日|先週|\d{4}[-/年]\d{1,2}[-/月]\d{1,2}日?|に決めた|を決めた|変更した|追加した|削除した|対応した|採用した)`), "episodic", "Project event"},
}

var (
	reQuestionCues        = regexp.MustCompile(`[?？]|(ですか|ますか|でしょうか|できますか|してもいいですか|どうでしょう)`)
	reRememberCues        = regexp.MustCompile(`(?i)(覚えて|記憶して|メモリに保存|memory に保存|メモリ化|今後の前提に|今のことを覚えて|保存しておいて|残しておいて)`)
	reTransientRequest    = regexp.MustCompile(`(作成してください|書いてください|考えてください|提案してください|説明してください|要約してください|レビューしてください|翻訳してください|生成してください|直してください|修正してください|教えてください|検討してください|ください。?$|お願いします。?$)`)
	reDurableProcedural   = regexp.MustCompile(`(を優先|は使わない|は禁止|避けてください|簡潔に|日本語で|英語で|箇条書きで|短く|詳しく|敬語で|常に|毎回|必ず|今後は|以後は|文体|口調|フォーマット|形式)`)
	reHeadingPrefix       = regexp.MustCompile(`(?m)^#+[\s\p{Z}]*`)
	reTitleSentenceSplit  = regexp.MustCompile(`[。！？.!?\n]`)
	reNewlineRun          = regexp.MustCompile(`\n+`)
	maxMemoryLength       = 300
	maxMemoryTitleLength  = 48
	maxAutoSentenceLength = 320 // for the explicit-request normalization
)

func isSentenceTerminator(r rune) bool {
	switch r {
	case '.', '!', '?', '。', '！', '？':
		return true
	}
	return false
}

// splitSentences reproduces input.split(/\n|(?<=[.!?。！？])/): it breaks on newlines
// (consuming them) and after each sentence terminator (keeping the terminator with
// the preceding sentence). Empty fragments are preserved here and filtered by the
// caller's length check, matching the TS pipeline order (split → filter → slice).
func splitSentences(s string) []string {
	var out []string
	var cur strings.Builder
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
		if isSentenceTerminator(r) {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func generateMemoryTitle(content, kind string) string {
	stripped := reHeadingPrefix.ReplaceAllString(content, "")
	normalized := collapseWhitespace(stripped)
	firstSentence := strings.TrimSpace(reTitleSentenceSplit.Split(normalized, -1)[0])
	if firstSentence == "" {
		firstSentence = normalized
	}
	base := util.Truncate(firstSentence, maxMemoryTitleLength)
	if base != "" {
		return base
	}
	switch kind {
	case "procedural":
		return "Working preference"
	case "episodic":
		return "Project event"
	default:
		return "Project fact"
	}
}

// GenerateMemoryTitle derives a memory title from raw content, mirroring the
// helper the Node backend duplicated in index.ts for the manual-create route. It
// shares the exact logic used by automatic extraction so both paths title
// memories identically; the HTTP layer calls this instead of re-implementing it.
func GenerateMemoryTitle(content, kind string) string {
	return generateMemoryTitle(content, kind)
}

func inferKindFromText(input string) string {
	for _, cue := range durableCues {
		if cue.regex.MatchString(input) {
			return cue.kind
		}
	}
	return "episodic"
}

func shouldSkipAutoMemorySentence(sentence, kind string) bool {
	if kind != "procedural" {
		return false
	}
	if reDurableProcedural.MatchString(sentence) {
		return false
	}
	return reTransientRequest.MatchString(sentence)
}

func memoryRoleLabel(role string) string {
	switch role {
	case "assistant":
		return "assistant"
	case "user":
		return "user"
	default:
		return "system"
	}
}

func normalizeMessageForMemory(m model.Message) string {
	return memoryRoleLabel(m.Role) + ": " + util.Truncate(collapseWhitespace(m.Content), maxAutoSentenceLength)
}

// MaybeStoreFromUserMessage mirrors maybeStoreFromUserMessage: the rule-based
// extractor that records durable preferences/facts/events from a user message.
func (s *MemoryService) MaybeStoreFromUserMessage(projectID, chatID, content string) ([]model.Memory, error) {
	candidates := []string{}
	for _, raw := range splitSentences(content) {
		sentence := strings.TrimSpace(raw)
		if utf8.RuneCountInString(sentence) < 10 {
			continue
		}
		candidates = append(candidates, sentence)
		if len(candidates) == 6 {
			break
		}
	}

	created := []model.Memory{}
	for _, sentence := range candidates {
		if reQuestionCues.MatchString(sentence) {
			continue
		}
		if utf8.RuneCountInString(sentence) > maxMemoryLength {
			continue
		}
		var matched *durableCue
		for i := range durableCues {
			if durableCues[i].regex.MatchString(sentence) {
				matched = &durableCues[i]
				break
			}
		}
		if matched == nil {
			continue
		}
		if shouldSkipAutoMemorySentence(sentence, matched.kind) {
			continue
		}
		similar, err := s.memories.HasSimilarMemory(projectID, matched.title, sentence)
		if err != nil {
			return nil, err
		}
		if similar {
			continue
		}
		sourceChatID := chatID
		memory, err := s.memories.CreateMemory(repository.CreateMemoryInput{
			ProjectID:    projectID,
			Kind:         matched.kind,
			Title:        matched.title,
			Content:      sentence,
			SourceChatID: &sourceChatID,
			Source:       "chat",
			Locked:       false,
		})
		if err != nil {
			return nil, err
		}
		created = append(created, memory)
	}
	return created, nil
}

// MaybeStoreFromExplicitRequest mirrors maybeStoreFromExplicitRequest: when the user
// explicitly asks to remember something, extract one durable memory via the LLM,
// falling back to a heuristic when the LLM/JSON path does not yield one. Returns
// (nil, nil) when nothing should be stored.
func (s *MemoryService) MaybeStoreFromExplicitRequest(projectID, chatID, content string, recentMessages []model.Message) (*model.Memory, error) {
	if !reRememberCues.MatchString(content) {
		return nil, nil
	}

	recentContext := []model.Message{}
	for _, m := range recentMessages {
		if m.Role != "system" {
			recentContext = append(recentContext, m)
		}
	}
	recentContext = lastN(recentContext, 4)
	if len(recentContext) == 0 {
		return nil, nil
	}

	// LLM extraction attempt. Only LLM/JSON failures fall through to the heuristic
	// fallback; repository errors propagate.
	contextLines := make([]string, 0, len(recentContext))
	for _, m := range recentContext {
		contextLines = append(contextLines, normalizeMessageForMemory(m))
	}
	resp, llmErr := s.llm.CreateChatCompletion(ChatCompletionInput{
		SystemPrompt: strings.Join([]string{
			"あなたは project memory 抽出専用アシスタントです。",
			"ユーザーが『覚えて』と明示したので、直前の会話から project をまたいで有用な durable memory を 1 件だけ抽出してください。",
			"返答は JSON のみとし、形式は {\"kind\":\"semantic|procedural|episodic\",\"content\":\"...\"} または {\"skip\":true,\"reason\":\"...\"} にしてください。",
			"content は 1 から 2 文の短い日本語にしてください。",
			"雑談や一時的な話題は保存しないでください。",
		}, "\n"),
		Messages: []model.Message{},
		UserInput: strings.Join([]string{
			"recent conversation:",
			joinLines(contextLines),
			"",
			"latest user request: " + util.Truncate(content, 220),
		}, "\n"),
		Temperature: float64Ptr(0.1),
	})
	if llmErr == nil {
		if jsonText := extractJSONObject(resp.Content); jsonText != "" {
			var parsed struct {
				Skip    bool   `json:"skip"`
				Kind    string `json:"kind"`
				Content string `json:"content"`
			}
			if json.Unmarshal([]byte(jsonText), &parsed) == nil {
				normalizedContent := util.Truncate(strings.TrimSpace(parsed.Content), 220)
				kind := parsed.Kind
				if kind != "semantic" && kind != "procedural" && kind != "episodic" {
					kind = inferKindFromText(normalizedContent)
				}
				if !parsed.Skip && normalizedContent != "" {
					title := generateMemoryTitle(normalizedContent, kind)
					similar, err := s.memories.HasSimilarMemory(projectID, title, normalizedContent)
					if err != nil {
						return nil, err
					}
					if !similar {
						sourceChatID := chatID
						memory, err := s.memories.CreateMemory(repository.CreateMemoryInput{
							ProjectID:    projectID,
							Kind:         kind,
							Title:        title,
							Content:      normalizedContent,
							SourceChatID: &sourceChatID,
							Source:       "chat",
							Locked:       true,
						})
						if err != nil {
							return nil, err
						}
						return &memory, nil
					}
				}
			}
		}
	}

	// Heuristic fallback: the most recent assistant message (else user message).
	var fallbackSource *model.Message
	for i := len(recentContext) - 1; i >= 0; i-- {
		if recentContext[i].Role == "assistant" {
			fallbackSource = &recentContext[i]
			break
		}
	}
	if fallbackSource == nil {
		for i := len(recentContext) - 1; i >= 0; i-- {
			if recentContext[i].Role == "user" {
				fallbackSource = &recentContext[i]
				break
			}
		}
	}
	if fallbackSource == nil {
		return nil, nil
	}

	chosen := fallbackSource.Content
	for _, line := range reNewlineRun.Split(fallbackSource.Content, -1) {
		if strings.TrimSpace(line) != "" {
			chosen = line
			break
		}
	}
	normalizedContent := util.Truncate(collapseWhitespace(chosen), 220)
	if normalizedContent == "" {
		return nil, nil
	}

	kind := inferKindFromText(content + "\n" + normalizedContent)
	title := generateMemoryTitle(normalizedContent, kind)
	similar, err := s.memories.HasSimilarMemory(projectID, title, normalizedContent)
	if err != nil {
		return nil, err
	}
	if similar {
		return nil, nil
	}
	sourceChatID := chatID
	memory, err := s.memories.CreateMemory(repository.CreateMemoryInput{
		ProjectID:    projectID,
		Kind:         kind,
		Title:        title,
		Content:      normalizedContent,
		SourceChatID: &sourceChatID,
		Source:       "chat",
		Locked:       true,
	})
	if err != nil {
		return nil, err
	}
	return &memory, nil
}
