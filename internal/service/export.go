package service

import (
	"fmt"
	"strings"
	"time"

	"snzstudio/internal/model"
)

// exportRemovedSuffix marks a speaker whose participant row has left the roster.
// It matches the observation view's `multiAgent.speakerRemoved` so a transcript
// read beside the screen names the same speaker the same way.
const exportRemovedSuffix = "（除籍済み）"

const exportTimeLayout = "2006-01-02 15:04"

// BuildChatMarkdown renders a whole chat as a standalone markdown transcript.
//
// participants must be every row of the chat, removed ones included
// (repository.ParticipantRepository.ListAll): a past message keeps pointing at a
// participant that has since left the roster, and its speaker still has to be
// named (docs/multi-agent-chat-design.md §3).
//
// The scene, turn rule and roster sections carry no meaning for a
// single-assistant chat, so they are emitted only for a multi-agent one; the
// heading and the transcript are common to both kinds.
func BuildChatMarkdown(project *model.Project, chat *model.Chat, participants []model.Participant, messages []model.Message, now time.Time) string {
	var b strings.Builder

	title := strings.TrimSpace(chat.Title)
	if title == "" {
		title = "無題のチャット"
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	if project != nil {
		fmt.Fprintf(&b, "- プロジェクト: %s\n", strings.TrimSpace(project.Title))
	}
	fmt.Fprintf(&b, "- 出力日時: %s\n", now.Format(exportTimeLayout))

	if chat.Kind == model.ChatKindMultiAgent {
		writeSceneSection(&b, chat)
		writeTurnRuleSection(&b, chat)
		writeRosterSection(&b, participants)
	}

	writeTranscript(&b, participants, messages)
	return b.String()
}

func writeSceneSection(b *strings.Builder, chat *model.Chat) {
	b.WriteString("\n## 場面設定\n\n")
	if scene := strings.TrimSpace(chat.ScenePrompt); scene != "" {
		b.WriteString(scene)
		b.WriteString("\n")
	} else {
		b.WriteString("（未設定）\n")
	}
}

func writeTurnRuleSection(b *strings.Builder, chat *model.Chat) {
	b.WriteString("\n## ターン進行ルール\n\n")
	fmt.Fprintf(b, "%s\n", turnRuleDescription(chat.TurnRule))
}

// turnRuleDescription spells out the stored rule. An unrecognised value is
// passed through rather than replaced, so a rule added later still exports
// something truthful before this list catches up.
func turnRuleDescription(rule string) string {
	switch rule {
	case model.TurnRuleRoundRobin:
		return "round_robin（編成順に回す）"
	case model.TurnRuleManual:
		return "manual（1 ターンごとに発言者を指名する）"
	case "":
		return "（未設定）"
	default:
		return rule
	}
}

func writeRosterSection(b *strings.Builder, participants []model.Participant) {
	b.WriteString("\n## 編成\n\n")
	if len(participants) == 0 {
		b.WriteString("（参加者なし）\n")
		return
	}
	for _, p := range participants {
		fmt.Fprintf(b, "- %s%s\n", participantLabel(p), modelSuffix(p.ModelName))
	}
}

func writeTranscript(b *strings.Builder, participants []model.Participant, messages []model.Message) {
	b.WriteString("\n## 会話\n")
	if len(messages) == 0 {
		b.WriteString("\n（発言なし）\n")
		return
	}
	byID := participantsByID(participants)
	for _, m := range messages {
		fmt.Fprintf(b, "\n### %s\n\n", exportSpeakerLabel(m, byID))
		body := strings.TrimRight(m.Content, "\n")
		if strings.TrimSpace(body) == "" {
			body = "（空の発言）"
		}
		b.WriteString(body)
		b.WriteString("\n")
	}
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
func exportSpeakerLabel(m model.Message, byID map[string]model.Participant) string {
	if m.Role == "user" {
		return humanSpeakerLabel
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
			return participantLabel(p) + modelSuffix(modelName)
		}
	}
	// A multi-agent chat reaches here only for a participant row that is gone
	// entirely; rows are never hard-deleted, so in practice this is the
	// single-assistant chat's own assistant.
	return assistantSpeakerLabel + modelSuffix(modelName)
}

func participantLabel(p model.Participant) string {
	name := strings.TrimSpace(p.DisplayName)
	if name == "" {
		name = assistantSpeakerLabel
	}
	if p.DeletedAt != nil {
		return name + exportRemovedSuffix
	}
	return name
}

func modelSuffix(modelName string) string {
	if name := strings.TrimSpace(modelName); name != "" {
		return " (" + name + ")"
	}
	return ""
}
