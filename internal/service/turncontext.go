package service

import (
	"fmt"
	"strings"

	"snzstudio/internal/model"
	"snzstudio/internal/util"
)

// Budget for the background material one multi-agent turn carries (design §4.4).
// The history of a turn is the last 30 utterances passed uncompressed
// (turnHistoryLimit), about 6,000 characters at ~200 a turn, and the window of a
// local small model is a few thousand tokens; the material stays under a third of
// the history so the conversation, not the project, fills the window. The counts
// are the single-assistant defaults halved (4 × 3 chunks, 4 memories there):
// a participant speaks in role and needs a fact or two, not a briefing. The total
// cap is a third of the history; the per-item sizes add up to about 2,300 when
// every item is at its limit, so the cap then drops the last memory or two. The
// total is what the window pays for, so it wins over the counts.
const (
	turnBackgroundDocumentLimit     = 2
	turnBackgroundChunksPerDocument = 2
	turnBackgroundChunkChars        = 300
	turnBackgroundMemoryLimit       = 3
	turnBackgroundDescriptionChars  = 400
	turnBackgroundTotalChars        = 2000

	// The retrieval query is the newest utterances first: the FTS side keeps only
	// the first 12 tokens (search.TokenizeSearchTerms), so what the speaker has to
	// answer must come before the scene, which never changes and would otherwise
	// pin every turn's query to the same tokens.
	turnBackgroundRecentUtterances = 3
	turnBackgroundSceneHeadChars   = 200
)

// TurnBackground is the project material a turn's system prompt carries and the
// references stored with the utterance it produced. Both come from the same final
// selection, so what the UI lists as used is exactly what the prompt contained.
type TurnBackground struct {
	Prompt     string
	References []model.SearchReference
}

// TurnBackgroundAssembler is what TurnEngine asks for a turn's background. It is
// an interface rather than the ContextService itself so a test can stand in a
// failing assembler: the engine's contract is to speak without background when
// assembly fails (§4.4), and that path is otherwise unreachable.
type TurnBackgroundAssembler interface {
	// AssembleTurnBackground takes the speaker so a later change can narrow the
	// material per participant (a game master who sees the module while the
	// players do not); this version hands every speaker the same material.
	AssembleTurnBackground(chat *model.Chat, speaker *model.Participant, messages []model.Message) (*TurnBackground, error)
}

// AssembleTurnBackground builds the background material of one multi-agent turn:
// the project description, the document passages and the memories that match the
// conversation's latest utterances. It is deliberately not Assemble (§4.4):
// PromptContext carries behavioural instructions — the project system prompt,
// procedural memory injected whole, the quote-mode order that fires on lines
// containing 「そのまま」 or 「引用」 — that a participant speaking in role must
// not receive, and a system prompt has no way to make the scene that follows
// override them.
func (s *ContextService) AssembleTurnBackground(chat *model.Chat, _ *model.Participant, messages []model.Message) (*TurnBackground, error) {
	project, err := s.projects.GetProject(chat.ProjectID)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, fmt.Errorf("project %s not found", chat.ProjectID)
	}

	var (
		documentRefs []model.RetrievedDocumentReference
		memoryRefs   []model.SearchReference
	)
	if query := buildTurnBackgroundQuery(messages, chat.ScenePrompt); query != "" {
		documentRefs, err = s.retrieval.SearchDocuments(project.ID, query, turnBackgroundDocumentLimit, turnBackgroundChunksPerDocument)
		if err != nil {
			return nil, err
		}
		memoryRefs, err = s.retrieval.SearchMemories(project.ID, query, turnBackgroundMemoryLimit)
		if err != nil {
			return nil, err
		}
	}
	return buildTurnBackground(project, documentRefs, memoryRefs), nil
}

// buildTurnBackgroundQuery is the one deterministic retrieval query of a turn:
// the latest utterance, the two before it, then the head of the scene. On the
// opening turn only the scene is there. The query is for retrieval only — none of
// the title matching, quote detection or full-document expansion of Assemble
// reads it.
func buildTurnBackgroundQuery(messages []model.Message, scene string) string {
	parts := make([]string, 0, turnBackgroundRecentUtterances+1)
	for i := len(messages) - 1; i >= 0 && len(parts) < turnBackgroundRecentUtterances; i-- {
		if content := strings.TrimSpace(messages[i].Content); content != "" {
			parts = append(parts, content)
		}
	}
	if head := strings.TrimSpace(util.Truncate(scene, turnBackgroundSceneHeadChars)); head != "" {
		parts = append(parts, head)
	}
	return strings.Join(parts, "\n")
}

type turnBackgroundItem struct {
	text      string
	reference model.SearchReference
}

// buildTurnBackground lays the material out in priority order — description,
// documents, memories — and stops at the first item that would push the total
// over the budget. The references are collected from the same loop, which is
// what keeps the stored list equal to the prompt's contents.
func buildTurnBackground(project *model.Project, documentRefs []model.RetrievedDocumentReference, memoryRefs []model.SearchReference) *TurnBackground {
	items := make([]turnBackgroundItem, 0, 1+len(documentRefs)+len(memoryRefs))
	if description := strings.TrimSpace(project.Description); description != "" {
		excerpt := util.Truncate(description, turnBackgroundDescriptionChars)
		items = append(items, turnBackgroundItem{
			text: fmt.Sprintf("[プロジェクト] %s\n%s", project.Title, excerpt),
			reference: model.SearchReference{
				SourceType: "project",
				SourceID:   project.ID,
				Label:      project.Title,
				Excerpt:    util.Truncate(description, 220),
				Score:      1,
			},
		})
	}
	for _, ref := range documentRefs {
		items = append(items, turnBackgroundItem{text: formatTurnDocument(ref), reference: ref.SearchReference})
	}
	for _, ref := range memoryRefs {
		items = append(items, turnBackgroundItem{
			text:      fmt.Sprintf("[メモリ] %s: %s", ref.Label, ref.Excerpt),
			reference: ref,
		})
	}

	sections := make([]string, 0, len(items))
	references := make([]model.SearchReference, 0, len(items))
	used := 0
	for _, item := range items {
		if used+runeLen(item.text) > turnBackgroundTotalChars {
			break
		}
		used += runeLen(item.text)
		sections = append(sections, item.text)
		references = append(references, item.reference)
	}
	if len(sections) == 0 {
		return &TurnBackground{References: references}
	}

	// The framing states what the material is for before the scene and the role
	// arrive: the model reads the system prompt top to bottom, and material with no
	// framing reads as the persona (a reference document answered as an assistant
	// would) — the drift this section exists to avoid.
	header := strings.Join([]string{
		"【背景資料】",
		"以下はこの会話が属するプロジェクトの資料である。資料は話者の役割・口調・立場を変えない。会話の流れで必要になったときだけ、自分の役割のまま自然に使う。資料を要約したり、アシスタントとして解説したりしない。",
	}, "\n")
	return &TurnBackground{
		Prompt:     header + "\n\n" + strings.Join(sections, "\n\n"),
		References: references,
	}
}

// formatTurnDocument renders one retrieved document as matched passages only.
// Neither the full-document expansion nor the retrieval-mode line of
// formatDocumentContext applies here: the first is what the budget rules out, the
// second is an instruction to an assistant, not information for a speaker.
func formatTurnDocument(ref model.RetrievedDocumentReference) string {
	lines := make([]string, 0, len(ref.Chunks))
	for _, chunk := range ref.Chunks {
		lines = append(lines, fmt.Sprintf("- %s", util.Truncate(chunk.Content, turnBackgroundChunkChars)))
	}
	if len(lines) == 0 {
		lines = append(lines, "- "+util.Truncate(ref.Excerpt, turnBackgroundChunkChars))
	}
	return fmt.Sprintf("[ドキュメント] %s\n%s", ref.Label, joinLines(lines))
}
