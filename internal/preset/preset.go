// Package preset holds the multi-agent conversation presets bundled with the
// app and the parser that turns a preset JSON — bundled or supplied by the user
// — into something the chat creation route can apply
// (docs/multi-agent-chat-design.md §6). A preset is data, not behaviour: the
// engine stays one, and a preset only says which roster, turn rule and scene a
// new chat starts from.
package preset

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"snzstudio/internal/model"
)

// The bundled presets ship inside the binary so the desktop app has them
// wherever it is installed. They are JSON rather than Go literals so a preset
// can be authored, and imported later, without touching the engine.
//
//go:embed bundled/*.json
var bundledFS embed.FS

// Groups are the values of MultiAgentPreset.Group, in the order the picker lists
// them. Discussion presets head towards a conclusion, drama presets reveal the
// characters' circumstances, hosted presets have a host handing the floor
// around, and pair presets alternate two speakers.
var Groups = []string{"discussion", "drama", "hosted", "pair"}

// ErrInvalid marks a preset the parser or validator refused. The HTTP layer maps
// it to 400 with errors.Is, so the wrapped message must say what is wrong.
var ErrInvalid = errors.New("preset: invalid preset")

// Participant is one roster entry of a preset. The endpoint and model are not
// preset data: they are chosen at use time, and an empty pair falls back to the
// workspace endpoint when a turn runs.
//
// ReceivesProjectMaterial is a pointer so an omitted field can mean the default
// (true) rather than false — every preset written before the field existed must
// keep handing its whole roster the project's material. Validate fills it in, so
// a parsed preset never carries nil.
type Participant struct {
	DisplayName             string `json:"displayName"`
	RolePrompt              string `json:"rolePrompt"`
	ReceivesProjectMaterial *bool  `json:"receivesProjectMaterial"`
}

// MultiAgentPreset is the shape of one preset JSON. ID and Group only mean
// something for bundled presets; an imported preset may leave both empty.
type MultiAgentPreset struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Group        string        `json:"group"`
	TurnRule     string        `json:"turnRule"`
	ScenePrompt  string        `json:"scenePrompt"`
	Participants []Participant `json:"participants"`
}

var bundled = mustLoadBundled()

// Bundled returns the presets shipped with the app in picker order (the file
// names carry a numeric prefix that fixes it).
func Bundled() []MultiAgentPreset {
	return append([]MultiAgentPreset(nil), bundled...)
}

// Find returns the bundled preset with the given id.
func Find(id string) (*MultiAgentPreset, bool) {
	for i := range bundled {
		if bundled[i].ID == id {
			p := bundled[i]
			return &p, true
		}
	}
	return nil, false
}

// Parse decodes and validates a preset JSON. Unknown fields are ignored so a
// hand-written file can carry its own notes; the required fields are checked by
// Validate.
func Parse(raw []byte) (*MultiAgentPreset, error) {
	var p MultiAgentPreset
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate trims the fields and checks what applying the preset needs: a title
// (it becomes the chat title when none is given), a known turn rule (an empty
// one reads as round_robin, the chat default) and a roster of at least two named
// participants, which is what makes a chat multi-agent (§2). It also fills the
// omitted defaults, so what applies a preset reads the values rather than
// repeating the rules: the turn rule and each participant's receivesProjectMaterial.
func (p *MultiAgentPreset) Validate() error {
	p.ID = strings.TrimSpace(p.ID)
	p.Title = strings.TrimSpace(p.Title)
	p.Description = strings.TrimSpace(p.Description)
	p.Group = strings.TrimSpace(p.Group)
	p.TurnRule = strings.TrimSpace(p.TurnRule)
	p.ScenePrompt = strings.TrimSpace(p.ScenePrompt)

	if p.Title == "" {
		return fmt.Errorf("%w: title is required", ErrInvalid)
	}
	if p.TurnRule == "" {
		p.TurnRule = model.TurnRuleRoundRobin
	}
	if p.TurnRule != model.TurnRuleRoundRobin && p.TurnRule != model.TurnRuleManual {
		return fmt.Errorf("%w: turnRule must be \"round_robin\" or \"manual\"", ErrInvalid)
	}
	if len(p.Participants) < 2 {
		return fmt.Errorf("%w: participants must have at least two entries", ErrInvalid)
	}
	for i := range p.Participants {
		p.Participants[i].DisplayName = strings.TrimSpace(p.Participants[i].DisplayName)
		p.Participants[i].RolePrompt = strings.TrimSpace(p.Participants[i].RolePrompt)
		if p.Participants[i].DisplayName == "" {
			return fmt.Errorf("%w: participants[%d].displayName is required", ErrInvalid, i)
		}
		if p.Participants[i].ReceivesProjectMaterial == nil {
			receives := true
			p.Participants[i].ReceivesProjectMaterial = &receives
		}
	}
	return nil
}

// mustLoadBundled reads every embedded preset at package init. A broken bundled
// file is a build defect, not a runtime condition, so it panics the way
// regexp.MustCompile does; the package test exercises the same load.
func mustLoadBundled() []MultiAgentPreset {
	entries, err := bundledFS.ReadDir("bundled")
	if err != nil {
		panic(fmt.Sprintf("preset: read bundled dir: %v", err))
	}
	presets := make([]MultiAgentPreset, 0, len(entries))
	seen := map[string]string{}
	for _, entry := range entries {
		raw, err := bundledFS.ReadFile("bundled/" + entry.Name())
		if err != nil {
			panic(fmt.Sprintf("preset: read %s: %v", entry.Name(), err))
		}
		p, err := Parse(raw)
		if err != nil {
			panic(fmt.Sprintf("preset: %s: %v", entry.Name(), err))
		}
		if p.ID == "" {
			panic(fmt.Sprintf("preset: %s: bundled presets need an id", entry.Name()))
		}
		if other, dup := seen[p.ID]; dup {
			panic(fmt.Sprintf("preset: %s and %s share id %q", other, entry.Name(), p.ID))
		}
		seen[p.ID] = entry.Name()
		presets = append(presets, *p)
	}
	return presets
}
