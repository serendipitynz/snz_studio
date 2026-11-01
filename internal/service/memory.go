package service

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"snzstudio/internal/config"
	"snzstudio/internal/lang"
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
	cfg      *config.Config
}

// NewMemoryService builds a MemoryService.
func NewMemoryService(memories *repository.MemoryRepository, llm *LLMClient, cfg *config.Config) *MemoryService {
	return &MemoryService{memories: memories, llm: llm, cfg: cfg}
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

// englishDurableCues widens the English cues above for a user working in English,
// the way the Japanese cues are written for one working in Japanese. The Japanese
// rules stay in the table: a project can hold both languages, and nothing is gained
// by no longer noticing what was noticed before the switch.
var englishDurableCues = []durableCue{
	{regexp.MustCompile(`(?i)\b(i prefer|prefer|always|never|please use|don't use|do not use|use .* for|style|tone|format|answer in|respond in|reply in|avoid using|from now on|be concise|keep it short|in bullet points)\b`), "procedural", "Working preference"},
	durableCues[1],
	{regexp.MustCompile(`(?i)\b(my |i am |i work on|project uses|we use|we are using|my stack|our stack|this project uses|this project is|this app|our (frontend|backend|database)|is built with|is written in)\b`), "semantic", "Project fact"},
	durableCues[3],
	{regexp.MustCompile(`(?i)\b(we decided|we shipped|we changed|we added|we removed|we adopted|last time|last week|yesterday|today|earlier|on \d{4}-\d{2}-\d{2}|previously)\b`), "episodic", "Project event"},
	durableCues[5],
}

// memoryCues is what the rule-based extractor reads a message with. A processing
// language picks the set; see japaneseMemoryCues and englishMemoryCues.
type memoryCues struct {
	durable           []durableCue
	remember          *regexp.Regexp
	transientRequest  *regexp.Regexp
	durableProcedural *regexp.Regexp
}

func memoryCuesFor(language lang.Language) *memoryCues {
	if language == lang.English {
		return &englishMemoryCues
	}
	return &japaneseMemoryCues
}

const (
	rememberCuesPattern      = `覚えて|記憶して|メモリに保存|memory に保存|メモリ化|今後の前提に|今のことを覚えて|保存しておいて|残しておいて`
	transientRequestPattern  = `作成してください|書いてください|考えてください|提案してください|説明してください|要約してください|レビューしてください|翻訳してください|生成してください|直してください|修正してください|教えてください|検討してください|ください。?$|お願いします。?$`
	durableProceduralPattern = `を優先|は使わない|は禁止|避けてください|簡潔に|日本語で|英語で|箇条書きで|短く|詳しく|敬語で|常に|毎回|必ず|今後は|以後は|文体|口調|フォーマット|形式`
)

var (
	japaneseMemoryCues = memoryCues{
		durable:           durableCues,
		remember:          regexp.MustCompile(`(?i)(` + rememberCuesPattern + `)`),
		transientRequest:  regexp.MustCompile(`(` + transientRequestPattern + `)`),
		durableProcedural: regexp.MustCompile(`(` + durableProceduralPattern + `)`),
	}
	englishMemoryCues = memoryCues{
		durable:           englishDurableCues,
		remember:          regexp.MustCompile(`(?i)(` + rememberCuesPattern + `|\bremember (this|that|it)\b|\bplease remember\b|\bdon't forget\b|\bsave (this|that|it) (to|in) memory\b)`),
		transientRequest:  regexp.MustCompile(`(?i)(` + transientRequestPattern + `|^(please |can you |could you )?(write|create|draft|think of|suggest|explain|summari[sz]e|review|translate|generate|fix|correct|tell me|consider)\b)`),
		durableProcedural: regexp.MustCompile(`(?i)(` + durableProceduralPattern + `|\b(prefer|always|never|don't use|do not use|from now on|be concise|bullet points|tone|style|format|answer in|respond in|reply in)\b)`),
	}
)

var (
	reQuestionCues        = regexp.MustCompile(`[?？]|(ですか|ますか|でしょうか|できますか|してもいいですか|どうでしょう)`)
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

// InferMemoryKind is inferKindFromText for the route that prefills the kind of
// a memory the user is about to confirm (the multi-agent save dialog), so the
// dialog's default agrees with what automatic extraction would have chosen.
func InferMemoryKind(content string, language lang.Language) string {
	return inferKindFromText(content, memoryCuesFor(language))
}

func inferKindFromText(input string, cues *memoryCues) string {
	for _, cue := range cues.durable {
		if cue.regex.MatchString(input) {
			return cue.kind
		}
	}
	return "episodic"
}

func shouldSkipAutoMemorySentence(sentence, kind string, cues *memoryCues) bool {
	if kind != "procedural" {
		return false
	}
	if cues.durableProcedural.MatchString(sentence) {
		return false
	}
	return cues.transientRequest.MatchString(sentence)
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
	cues := memoryCuesFor(s.cfg.Get().ProcessingLanguage)
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
		for i := range cues.durable {
			if cues.durable[i].regex.MatchString(sentence) {
				matched = &cues.durable[i]
				break
			}
		}
		if matched == nil {
			continue
		}
		if shouldSkipAutoMemorySentence(sentence, matched.kind, cues) {
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
	language := s.cfg.Get().ProcessingLanguage
	cues := memoryCuesFor(language)
	if !cues.remember.MatchString(content) {
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
		SystemPrompt: promptTextFor(language).memoryExtractionSystem,
		Messages:     []model.Message{},
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
					kind = inferKindFromText(normalizedContent, cues)
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

	kind := inferKindFromText(content+"\n"+normalizedContent, cues)
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
