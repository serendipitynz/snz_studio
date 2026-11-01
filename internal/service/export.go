package service

import (
	"fmt"
	"strings"
	"time"

	"snzstudio/internal/lang"
	"snzstudio/internal/model"
	"snzstudio/internal/service/commands"
)

const exportTimeLayout = "2006-01-02 15:04"

// BuildChatMarkdown renders a whole chat as a standalone markdown transcript.
//
// participants must be every row of the chat, removed ones included
// (repository.ParticipantRepository.ListAll): a past message keeps pointing at a
// participant that has since left the roster, and its speaker still has to be
// named (docs/multi-agent-chat-design.md §3).
//
// The headings and labels follow the processing language rather than the display
// language: the transcript the file carries was written in the former, and the
// server does not know the latter.
//
// The scene, state, turn rule and roster sections carry no meaning for a
// single-assistant chat, so they are emitted only for a multi-agent one; the
// heading and the transcript are common to both kinds.
func BuildChatMarkdown(project *model.Project, chat *model.Chat, participants []model.Participant, messages []model.Message, now time.Time, language lang.Language) string {
	var b strings.Builder
	text := promptTextFor(language)

	title := singleLine(chat.Title)
	if title == "" {
		title = model.UntitledChatTitle(language)
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	if project != nil {
		fmt.Fprintf(&b, text.exportProject, singleLine(project.Title))
	}
	fmt.Fprintf(&b, text.exportGeneratedAt, now.Format(exportTimeLayout))

	if chat.Kind == model.ChatKindMultiAgent {
		writeSceneSection(&b, chat, text)
		writeStateSection(&b, chat, participants, text)
		writeTurnRuleSection(&b, chat, participants, text)
		writeRosterSection(&b, participants, text)
	}

	writeTranscript(&b, participants, messages, language)
	return b.String()
}

func writeSceneSection(b *strings.Builder, chat *model.Chat, text *promptText) {
	b.WriteString(text.exportScene)
	if scene := strings.TrimSpace(chat.ScenePrompt); scene != "" {
		b.WriteString(scene)
		b.WriteString("\n")
	} else {
		b.WriteString(text.exportUnset + "\n")
	}
}

// writeStateSection exports the state as it stands now, in the order the turn
// prompt carries it (§4.7.3 item 3). Like the prompt's section it is left out
// when every sheet is empty, so a conversation that never used state exports
// as it did before the sheets existed.
func writeStateSection(b *strings.Builder, chat *model.Chat, participants []model.Participant, text *promptText) {
	blocks := make([]string, 0, len(participants)+1)
	if shared := strings.TrimSpace(chat.StateSheet); shared != "" {
		blocks = append(blocks, text.exportStateShared+stateSheetList(shared))
	}
	for _, p := range participants {
		if p.DeletedAt != nil {
			continue
		}
		if sheet := strings.TrimSpace(p.StateSheet); sheet != "" {
			blocks = append(blocks, fmt.Sprintf("### %s\n\n%s", participantLabel(p, text), stateSheetList(sheet)))
		}
	}
	if len(blocks) == 0 {
		return
	}
	b.WriteString(text.exportState)
	b.WriteString(strings.Join(blocks, "\n"))
}

// stateSheetList writes a sheet one list item per line. A sheet is lines of
// "name: value" rather than markdown, and left as it is a renderer would join
// the lines into one paragraph.
func stateSheetList(sheet string) string {
	var b strings.Builder
	for _, line := range strings.Split(sheet, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			b.WriteString("- " + line + "\n")
		}
	}
	return b.String()
}

func writeTurnRuleSection(b *strings.Builder, chat *model.Chat, participants []model.Participant, text *promptText) {
	b.WriteString(text.exportTurnRule)
	fmt.Fprintf(b, "%s\n", turnRuleDescription(chat, participants, text))
}

// turnRuleDescription spells out the stored rule. An unrecognised value is
// passed through rather than replaced, so a rule added later still exports
// something truthful before this list catches up.
func turnRuleDescription(chat *model.Chat, participants []model.Participant, text *promptText) string {
	switch chat.TurnRule {
	case model.TurnRuleRoundRobin:
		return text.ruleRoundRobin
	case model.TurnRuleManual:
		return text.ruleManual
	case model.TurnRuleFacilitatorAlternating:
		// The export says what the conversation actually did, so an unresolvable
		// facilitator reports the fallback the engine took rather than the setting
		// the chat carries (§4.2 step 1).
		if name := facilitatorName(chat.FacilitatorID, participants); name != "" {
			return fmt.Sprintf(text.ruleFacilitator, name)
		}
		return text.ruleNoFacilitator
	case model.TurnRuleWeighted:
		if name := facilitatorName(chat.FacilitatorID, participants); name != "" {
			return fmt.Sprintf(text.ruleWeightedNamed, name)
		}
		return text.ruleWeighted
	case "":
		return text.exportUnset
	default:
		return chat.TurnRule
	}
}

func facilitatorName(facilitatorID string, participants []model.Participant) string {
	if facilitatorID == "" {
		return ""
	}
	for _, p := range participants {
		if p.ID == facilitatorID && p.DeletedAt == nil {
			return singleLine(p.DisplayName)
		}
	}
	return ""
}

func writeRosterSection(b *strings.Builder, participants []model.Participant, text *promptText) {
	b.WriteString(text.exportRoster)
	if len(participants) == 0 {
		b.WriteString(text.exportNoRoster)
		return
	}
	for _, p := range participants {
		fmt.Fprintf(b, "- %s%s\n", participantLabel(p, text), modelSuffix(p.ModelName))
	}
}

func writeTranscript(b *strings.Builder, participants []model.Participant, messages []model.Message, language lang.Language) {
	text := promptTextFor(language)
	b.WriteString(text.exportTranscript)
	if len(messages) == 0 {
		b.WriteString(text.exportNoMessages)
		return
	}
	byID := participantsByID(participants)
	for _, m := range messages {
		fmt.Fprintf(b, "\n### %s\n\n", exportSpeakerLabel(m, byID, text))
		body := strings.TrimRight(m.Content, "\n")
		if strings.TrimSpace(body) != "" {
			b.WriteString(body)
			b.WriteString("\n")
		} else if len(m.DiceRolls) == 0 && len(m.StateEffects) == 0 {
			b.WriteString(text.exportEmptyMessage)
		}
		writeCommandRecords(b, m, strings.TrimSpace(body) != "", language)
	}
}

// writeCommandRecords writes the rolls the app threw and the effects it applied
// for a message, set apart from the body as a quote so a reader away from the
// app can tell them from what the speaker wrote, as the chips do on screen
// (design §4.8.3 item 3, §4.8.8).
func writeCommandRecords(b *strings.Builder, m model.Message, afterBody bool, language lang.Language) {
	lines := CommandRecordLines(m, language)
	if len(lines) == 0 {
		return
	}
	if afterBody {
		b.WriteString("\n")
	}
	for _, line := range lines {
		b.WriteString("> " + singleLine(line) + "\n")
	}
}

// CommandRecordLines is a message's rolls and effects as a reader outside the
// prompt sees them — the markdown export and the memory draft (design §4.4) —
// with 🎲 and 📝 rather than the prompt's 【ダイス】 and 【効果】: a memory
// reaches every conversation in the project, and in one with the command
// enabled a bracketed line in the material would read as a record the app just
// made.
func CommandRecordLines(m model.Message, language lang.Language) []string {
	lines := make([]string, 0, len(m.DiceRolls)+len(m.StateEffects))
	for _, r := range m.DiceRolls {
		lines = append(lines, "🎲 "+commands.DiceRollLine(r, language))
	}
	for _, r := range m.StateEffects {
		lines = append(lines, "📝 "+commands.EffectLine(r, language))
	}
	return lines
}

func participantsByID(participants []model.Participant) map[string]model.Participant {
	byID := make(map[string]model.Participant, len(participants))
	for _, p := range participants {
		byID[p.ID] = p
	}
	return byID
}

// exportSpeakerLabel names the speaker of one utterance. It follows the
// observation view's rule (MultiAgentChatPage.speakerLabel) rather than the
// turn engine's speakerLabel, because a transcript read away from the app also
// has to say which model spoke and that the speaker has since been removed.
//
// The model name comes from the message when it recorded one — that is the
// model the turn actually ran against, which the participant's current setting
// may no longer be.
func exportSpeakerLabel(m model.Message, byID map[string]model.Participant, text *promptText) string {
	if m.Role == "user" {
		return text.humanSpeaker
	}
	modelName := ""
	if m.ModelName != nil {
		modelName = *m.ModelName
	}
	if m.ParticipantID != nil {
		if p, ok := byID[*m.ParticipantID]; ok {
			if modelName == "" {
				modelName = p.ModelName
			}
			return participantLabel(p, text) + modelSuffix(modelName)
		}
	}
	// A multi-agent chat reaches here only for a participant row that is gone
	// entirely; rows are never hard-deleted, so in practice this is the
	// single-assistant chat's own assistant.
	return text.assistantSpeaker + modelSuffix(modelName)
}

// participantLabel matches the observation view's `multiAgent.speakerRemoved` for
// a removed participant, so a transcript read beside the screen names the same
// speaker the same way.
func participantLabel(p model.Participant, text *promptText) string {
	name := singleLine(p.DisplayName)
	if name == "" {
		name = text.assistantSpeaker
	}
	if p.DeletedAt != nil {
		return name + text.exportRemoved
	}
	return name
}

func modelSuffix(modelName string) string {
	if name := singleLine(modelName); name != "" {
		return " (" + name + ")"
	}
	return ""
}

// singleLine flattens a value that is interpolated into a heading or a list
// item. Titles, display names and model names are free text — the API and the
// preset JSON both accept a newline in one — and a newline there would end the
// line it sits on and let the rest be read as markdown of its own, so a display
// name carrying "### " would attribute the following message to a speaker that
// does not exist. Message bodies and the scene are deliberately not passed
// through here: multi-line markdown is the point of those.
func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
