package preset

import (
	"strings"

	"snzstudio/internal/model"
)

// FromChat builds the preset that recreates the chat's current line-up: its
// settings, its state sheets and its roster in roster order, endpoints and
// models included (TASK-62). roster must be the chat's current roster
// (ParticipantRepository.ListRoster): a removed participant is not part of the
// line-up, and the transcript is not preset data.
//
// The result has been through Validate, the same check an imported preset goes
// through, so a preset FromChat returns is one the import accepts. A chat that
// would not make one — fewer than two participants, or facilitator_alternating
// with its facilitator off the roster — is refused with ErrInvalid instead of
// being exported as a file that would then fail to load.
func FromChat(chat *model.Chat, roster []model.Participant) (*MultiAgentPreset, error) {
	title := strings.TrimSpace(chat.Title)
	// A preset cannot carry an empty title (Validate refuses one).
	if title == "" {
		title = model.UntitledChatTitle
	}
	commands := chat.Commands
	if commands.Roll != nil {
		roll := *commands.Roll
		commands.Roll = &roll
	}
	// The chat keeps its facilitator id when the rule changes to one that reads
	// none, and a preset refuses the mark under such a rule; only the rules that
	// read it carry it over.
	marksFacilitator := chat.TurnRule == model.TurnRuleFacilitatorAlternating || chat.TurnRule == model.TurnRuleWeighted

	p := &MultiAgentPreset{
		Title:        title,
		TurnRule:     chat.TurnRule,
		ScenePrompt:  chat.ScenePrompt,
		StateSheet:   chat.StateSheet,
		Commands:     &commands,
		Participants: make([]Participant, 0, len(roster)),
	}
	for _, participant := range roster {
		receives := participant.ReceivesProjectMaterial
		p.Participants = append(p.Participants, Participant{
			DisplayName:             participant.DisplayName,
			RolePrompt:              participant.RolePrompt,
			ReceivesProjectMaterial: &receives,
			Facilitator:             marksFacilitator && participant.ID == chat.FacilitatorID,
			StateSheet:              participant.StateSheet,
			BaseURL:                 participant.BaseURL,
			ModelName:               participant.ModelName,
		})
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}
