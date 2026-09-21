package service

import (
	"fmt"
	"strings"

	"snzstudio/internal/model"
	"snzstudio/internal/util"
)

// Budget for the project material one multi-agent turn carries (design §4.4).
// The history of a turn is the last 30 utterances passed uncompressed
// (turnHistoryLimit), about 6,000 characters at ~200 a turn, and the window of a
// local small model is a few thousand tokens; the material stays under a third of
// the history so the conversation, not the project, fills the window. The counts
// are the single-assistant defaults halved (4 × 3 chunks, 4 memories there):
// a participant speaks in role and needs a fact or two, not a briefing. The total
// cap is a third of the history; the per-item sizes add up to about 2,300 when
// every item is at its limit, so the cap then drops the last memory or two. The
// total is what the window pays for, so it wins over the counts; it is measured
// on the section as written into the prompt, framing and separators included.
const (
	turnMaterialDocumentLimit     = 2
	turnMaterialChunksPerDocument = 2
	turnMaterialChunkChars        = 300
	turnMaterialMemoryLimit       = 3
	turnMaterialDescriptionChars  = 400
	turnMaterialTotalChars        = 2000

	// The retrieval query is the newest utterances first: the FTS side keeps only
	// the first 12 tokens (search.TokenizeSearchTerms), so what the speaker has to
	// answer must come before the scene, which never changes and would otherwise
	// pin every turn's query to the same tokens.
	turnMaterialRecentUtterances = 3
	turnMaterialSceneHeadChars   = 200
)

// TurnMaterial is the project material a turn's system prompt carries and the
// references stored with the utterance it produced. Both come from the same final
// selection, so what the UI lists as used is exactly what the prompt contained.
type TurnMaterial struct {
	Prompt     string
	References []model.SearchReference
}

// TurnMaterialAssembler is what TurnEngine asks for a turn's project material. It
// is an interface rather than the ContextService itself so a test can stand in a
// failing assembler: the engine's contract is to speak without material when
// assembly fails (§4.4), and that path is otherwise unreachable.
type TurnMaterialAssembler interface {
	// AssembleTurnMaterial takes the speaker because the material is per
	// participant: a speaker whose ReceivesProjectMaterial is false gets none of it
	// (a game master who sees the scenario while the players do not). Every
	// speaker that does receive it gets the same material; narrowing it to a
	// subset of the documents would need a different shape and is not what the
	// flag expresses.
	AssembleTurnMaterial(chat *model.Chat, speaker *model.Participant, messages []model.Message) (*TurnMaterial, error)
}

// AssembleTurnMaterial builds the project material of one multi-agent turn:
// the project description, the document passages and the memories that match the
// conversation's latest utterances. It is deliberately not Assemble (§4.4):
// PromptContext carries behavioural instructions — the project system prompt,
// procedural memory injected whole, the quote-mode order that fires on lines
// containing 「そのまま」 or 「引用」 — that a participant speaking in role must
// not receive, and a system prompt has no way to make the scene that follows
// override them.
//
// A speaker that does not receive the material is answered before anything is
// read or searched, rather than by discarding the result afterwards: a turn that
// must not know the scenario should not pay for retrieving it, and the empty
// TurnMaterial is what keeps the prompt and the stored references empty
// together.
func (s *ContextService) AssembleTurnMaterial(chat *model.Chat, speaker *model.Participant, messages []model.Message) (*TurnMaterial, error) {
	if !speaker.ReceivesProjectMaterial {
		return &TurnMaterial{}, nil
	}

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
	if query := buildTurnMaterialQuery(messages, chat.ScenePrompt); query != "" {
		documentRefs, err = s.retrieval.SearchDocuments(project.ID, query, turnMaterialDocumentLimit, turnMaterialChunksPerDocument)
		if err != nil {
			return nil, err
		}
		memoryRefs, err = s.retrieval.SearchMemories(project.ID, query, turnMaterialMemoryLimit)
		if err != nil {
			return nil, err
		}
	}
	return buildTurnMaterial(project, documentRefs, memoryRefs), nil
}

// buildTurnMaterialQuery is the one deterministic retrieval query of a turn:
// the latest utterance, the two before it, then the head of the scene. On the
// opening turn only the scene is there. The query is for retrieval only — none of
// the title matching, quote detection or full-document expansion of Assemble
// reads it.
func buildTurnMaterialQuery(messages []model.Message, scene string) string {
	parts := make([]string, 0, turnMaterialRecentUtterances+1)
	for i := len(messages) - 1; i >= 0 && len(parts) < turnMaterialRecentUtterances; i-- {
		if content := strings.TrimSpace(messages[i].Content); content != "" {
			parts = append(parts, content)
		}
	}
	if head := strings.TrimSpace(util.Truncate(scene, turnMaterialSceneHeadChars)); head != "" {
		parts = append(parts, head)
	}
	return strings.Join(parts, "\n")
}

type turnMaterialItem struct {
	text      string
	reference model.SearchReference
}

// buildTurnMaterial lays the material out in priority order — description,
// documents, memories — and stops at the first item that would push the total
// over the budget. The references are collected from the same loop, which is
// what keeps the stored list equal to the prompt's contents.
func buildTurnMaterial(project *model.Project, documentRefs []model.RetrievedDocumentReference, memoryRefs []model.SearchReference) *TurnMaterial {
	items := make([]turnMaterialItem, 0, 1+len(documentRefs)+len(memoryRefs))
	if description := strings.TrimSpace(project.Description); description != "" {
		excerpt := util.Truncate(description, turnMaterialDescriptionChars)
		items = append(items, turnMaterialItem{
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
		items = append(items, turnMaterialItem{text: formatTurnDocument(ref), reference: ref.SearchReference})
	}
	for _, ref := range memoryRefs {
		items = append(items, turnMaterialItem{
			text:      fmt.Sprintf("[メモリ] %s: %s", ref.Label, ref.Excerpt),
			reference: ref,
		})
	}

	// The budget is the whole section as it lands in the prompt — header and
	// separators included — since that is what the window pays for.
	const separator = "\n\n"
	sections := make([]string, 0, len(items))
	references := make([]model.SearchReference, 0, len(items))
	used := runeLen(turnMaterialHeader)
	for _, item := range items {
		if used+runeLen(separator)+runeLen(item.text) > turnMaterialTotalChars {
			break
		}
		used += runeLen(separator) + runeLen(item.text)
		sections = append(sections, item.text)
		references = append(references, item.reference)
	}
	if len(sections) == 0 {
		return &TurnMaterial{References: references}
	}
	return &TurnMaterial{
		Prompt:     turnMaterialHeader + separator + strings.Join(sections, separator),
		References: references,
	}
}

// turnMaterialHeader states what the material is for before the scene and the
// role arrive: the model reads the system prompt top to bottom, and material with
// no framing reads as the persona (a reference document answered as an assistant
// would) — the drift this section exists to avoid.
const turnMaterialHeader = "【プロジェクト資料】\n" +
	"以下はこの会話が属するプロジェクトの資料である。資料は話者の役割・口調・立場を変えない。会話の流れで必要になったときだけ、自分の役割のまま自然に使う。資料を要約したり、アシスタントとして解説したりしない。"

// formatTurnDocument renders one retrieved document as matched passages only.
// Neither the full-document expansion nor the retrieval-mode line of
// formatDocumentContext applies here: the first is what the budget rules out, the
// second is an instruction to an assistant, not information for a speaker.
func formatTurnDocument(ref model.RetrievedDocumentReference) string {
	lines := make([]string, 0, len(ref.Chunks))
	for _, chunk := range ref.Chunks {
		lines = append(lines, fmt.Sprintf("- %s", util.Truncate(chunk.Content, turnMaterialChunkChars)))
	}
	if len(lines) == 0 {
		lines = append(lines, "- "+util.Truncate(ref.Excerpt, turnMaterialChunkChars))
	}
	return fmt.Sprintf("[ドキュメント] %s\n%s", ref.Label, joinLines(lines))
}
