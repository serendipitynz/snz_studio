package preset

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"snzstudio/internal/model"
)

// TestBundledPresets checks the shipped files as data: every one carries the
// fields the picker groups and labels by, and the two presets the design fixes
// as the initial pair (§6) are among them.
func TestBundledPresets(t *testing.T) {
	presets := Bundled()
	if len(presets) < 2 {
		t.Fatalf("%d bundled presets, want at least the debate and improv pair", len(presets))
	}

	known := map[string]bool{}
	for _, g := range Groups {
		known[g] = true
	}
	ids := map[string]bool{}
	for _, p := range presets {
		if !known[p.Group] {
			t.Errorf("%s: group %q is not one of %v", p.ID, p.Group, Groups)
		}
		if p.Description == "" || p.ScenePrompt == "" {
			t.Errorf("%s: description and scenePrompt must both be set", p.ID)
		}
		for _, participant := range p.Participants {
			if participant.RolePrompt == "" {
				t.Errorf("%s: participant %q has no role prompt", p.ID, participant.DisplayName)
			}
		}
		ids[p.ID] = true
	}
	for _, id := range []string{"debate", "improv-late-night-diner"} {
		if !ids[id] {
			t.Errorf("bundled presets lack %q", id)
		}
	}

	if _, ok := Find("debate"); !ok {
		t.Fatal("Find(debate) = not found")
	}
	if _, ok := Find("no-such-preset"); ok {
		t.Fatal("Find(no-such-preset) found something")
	}
}

// TestBundledIsACopy guards the picker's list against being mutated through the
// slice a caller received.
func TestBundledIsACopy(t *testing.T) {
	first := Bundled()
	first[0].Title = "mutated"
	if Bundled()[0].Title == "mutated" {
		t.Fatal("Bundled() handed out its backing array")
	}
}

func TestParse(t *testing.T) {
	valid := `{
		"title": " 二人の対話 ",
		"scenePrompt": "静かな部屋。\n",
		"participants": [
			{"displayName": "甲", "rolePrompt": "あなたは甲です。"},
			{"displayName": "乙", "rolePrompt": "あなたは乙です。"}
		],
		"notes": "unknown fields are allowed"
	}`
	p, err := Parse([]byte(valid))
	if err != nil {
		t.Fatalf("Parse(valid): %v", err)
	}
	if p.Title != "二人の対話" || p.TurnRule != "round_robin" || p.ScenePrompt != "静かな部屋。" {
		t.Fatalf("Parse(valid) = %+v, want trimmed fields and the round_robin default", p)
	}

	cases := map[string]string{
		"not json":                               `{`,
		"no title":                               `{"participants":[{"displayName":"甲"},{"displayName":"乙"}]}`,
		"one participant":                        `{"title":"x","participants":[{"displayName":"甲"}]}`,
		"blank name":                             `{"title":"x","participants":[{"displayName":"甲"},{"displayName":"  "}]}`,
		"bad turn rule":                          `{"title":"x","turnRule":"auction","participants":[{"displayName":"甲"},{"displayName":"乙"}]}`,
		"facilitator rule without a facilitator": `{"title":"x","turnRule":"facilitator_alternating","participants":[{"displayName":"甲"},{"displayName":"乙"}]}`,
		"two facilitators":                       `{"title":"x","turnRule":"facilitator_alternating","participants":[{"displayName":"甲","facilitator":true},{"displayName":"乙","facilitator":true}]}`,
		"facilitator under another rule":         `{"title":"x","participants":[{"displayName":"甲","facilitator":true},{"displayName":"乙"}]}`,
		"two facilitators under weighted":        `{"title":"x","turnRule":"weighted","participants":[{"displayName":"甲","facilitator":true},{"displayName":"乙","facilitator":true}]}`,
	}
	for name, raw := range cases {
		_, err := Parse([]byte(raw))
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
		if err != nil && !strings.HasPrefix(err.Error(), ErrInvalid.Error()+": ") {
			t.Errorf("%s: err = %q, want the reason after the sentinel", name, err)
		}
	}
}

// TestParseWeighted: weighted reads the facilitator mark but runs without one,
// so the mark is optional there rather than required.
func TestParseWeighted(t *testing.T) {
	for _, raw := range []string{
		`{"title":"x","turnRule":"weighted","participants":[{"displayName":"甲"},{"displayName":"乙"}]}`,
		`{"title":"x","turnRule":"weighted","participants":[{"displayName":"甲","facilitator":true},{"displayName":"乙"}]}`,
	} {
		if p, err := Parse([]byte(raw)); err != nil || p.TurnRule != "weighted" {
			t.Errorf("Parse(%s) = %+v, %v, want a weighted preset", raw, p, err)
		}
	}
}

// TestParseFacilitator covers TASK-21 AC #5: the facilitator is preset data,
// marked on the roster entry because the participant id the chat stores does not
// exist until the preset is applied.
func TestParseFacilitator(t *testing.T) {
	p, err := Parse([]byte(`{
		"title": "卓",
		"turnRule": "facilitator_alternating",
		"participants": [
			{ "displayName": "プレイヤー" },
			{ "displayName": "GM", "facilitator": true }
		]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Participants[0].Facilitator || !p.Participants[1].Facilitator {
		t.Fatalf("facilitator marked on %+v", p.Participants)
	}

	trpg, ok := Find("trpg-table")
	if !ok {
		t.Fatal("bundled presets lack the TRPG table")
	}
	if trpg.TurnRule != "facilitator_alternating" {
		t.Fatalf("trpg-table turnRule = %q", trpg.TurnRule)
	}
	for _, participant := range trpg.Participants {
		if participant.Facilitator != (participant.DisplayName == "GM") {
			t.Errorf("trpg-table: %q facilitator = %v", participant.DisplayName, participant.Facilitator)
		}
		// The line-up only works as a TRPG table if the GM is the one holding the
		// scenario, so the two flags have to agree.
		if *participant.ReceivesProjectMaterial != (participant.DisplayName == "GM") {
			t.Errorf("trpg-table: %q receivesProjectMaterial = %v", participant.DisplayName, *participant.ReceivesProjectMaterial)
		}
	}
}

// TestParseReceivesProjectMaterial covers TASK-20 AC #3: the field is preset data,
// and a preset written before it existed keeps handing its whole roster the
// project's material.
func TestParseReceivesProjectMaterial(t *testing.T) {
	p, err := Parse([]byte(`{
		"title": "ダンジョン探索",
		"participants": [
			{ "displayName": "GM" },
			{ "displayName": "戦士", "receivesProjectMaterial": false },
			{ "displayName": "盗賊", "receivesProjectMaterial": true }
		]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []bool{true, false, true}
	for i, participant := range p.Participants {
		if participant.ReceivesProjectMaterial == nil {
			t.Fatalf("participants[%d].receivesProjectMaterial left nil; Validate must fill the default", i)
		}
		if *participant.ReceivesProjectMaterial != want[i] {
			t.Errorf("participants[%d] (%s).receivesProjectMaterial = %v, want %v", i, participant.DisplayName, *participant.ReceivesProjectMaterial, want[i])
		}
	}

	// Withholding the project material is what makes a hidden scenario possible,
	// so a bundled preset may do it — but only the presets built around one, and
	// never as the side effect of a copied roster entry.
	withholds := map[string]bool{"trpg-table": true}
	for _, bundledPreset := range Bundled() {
		for _, participant := range bundledPreset.Participants {
			if participant.ReceivesProjectMaterial == nil {
				t.Errorf("bundled %s: participant %q left receivesProjectMaterial nil; Validate must fill the default", bundledPreset.ID, participant.DisplayName)
				continue
			}
			if !*participant.ReceivesProjectMaterial && !withholds[bundledPreset.ID] {
				t.Errorf("bundled %s: participant %q does not receive the project material", bundledPreset.ID, participant.DisplayName)
			}
		}
	}
}

// TestParseStateSheet covers TASK-35: stateSheet is optional preset data on the
// preset and on each participant, trimmed like the other text fields and held
// to the limits the PATCH routes enforce, counted in runes.
func TestParseStateSheet(t *testing.T) {
	p, err := Parse([]byte(`{
		"title": "卓",
		"stateSheet": "  場所: 入口\n",
		"participants": [
			{ "displayName": "GM" },
			{ "displayName": "戦士", "stateSheet": "HP: 10/10" }
		]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.StateSheet != "場所: 入口" || p.Participants[0].StateSheet != "" || p.Participants[1].StateSheet != "HP: 10/10" {
		t.Fatalf("sheets = %q / %q / %q", p.StateSheet, p.Participants[0].StateSheet, p.Participants[1].StateSheet)
	}

	atLimit := func(n int) string { return strings.Repeat("あ", n) }
	for name, raw := range map[string]string{
		"shared at 400":      `{"title": "卓", "stateSheet": "` + atLimit(400) + `", "participants": [{"displayName": "A"}, {"displayName": "B"}]}`,
		"participant at 200": `{"title": "卓", "participants": [{"displayName": "A", "stateSheet": "` + atLimit(200) + `"}, {"displayName": "B"}]}`,
	} {
		if _, err := Parse([]byte(raw)); err != nil {
			t.Errorf("%s: Parse = %v, want accepted", name, err)
		}
	}
	for name, raw := range map[string]string{
		"shared at 401":      `{"title": "卓", "stateSheet": "` + atLimit(401) + `", "participants": [{"displayName": "A"}, {"displayName": "B"}]}`,
		"participant at 201": `{"title": "卓", "participants": [{"displayName": "A"}, {"displayName": "B", "stateSheet": "` + atLimit(201) + `"}]}`,
	} {
		if _, err := Parse([]byte(raw)); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "stateSheet") {
			t.Errorf("%s: Parse = %v, want ErrInvalid naming stateSheet", name, err)
		}
	}
}

// TestParseEndpoints covers TASK-62 AC #3: baseUrl and modelName are optional
// participant fields, trimmed like the others, and a preset without them parses
// to an empty pair (the workspace endpoint). No bundled preset carries one.
func TestParseEndpoints(t *testing.T) {
	p, err := Parse([]byte(`{
		"title": "持ち出した編成",
		"participants": [
			{ "displayName": "A", "baseUrl": " http://192.168.0.10:1234/v1 ", "modelName": " qwen3-8b " },
			{ "displayName": "B" }
		]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Participants[0].BaseURL != "http://192.168.0.10:1234/v1" || p.Participants[0].ModelName != "qwen3-8b" {
		t.Fatalf("participants[0] = %+v, want the trimmed endpoint and model", p.Participants[0])
	}
	if p.Participants[1].BaseURL != "" || p.Participants[1].ModelName != "" {
		t.Fatalf("participants[1] = %+v, want no endpoint", p.Participants[1])
	}

	for _, bundledPreset := range Bundled() {
		for i, participant := range bundledPreset.Participants {
			if participant.BaseURL != "" || participant.ModelName != "" {
				t.Errorf("%s: participants[%d] carries an endpoint or a model", bundledPreset.ID, i)
			}
		}
	}
}

// TestPresetCommands covers the preset half of TASK-65 AC #2 (design §4.8.7):
// commands fills the chat's commands, the TASK-37 top-level diceTarget reads
// as commands.roll.target, both at once or an unknown command is refused, and
// the bundled TRPG table is the one bundled preset that enables /roll.
func TestPresetCommands(t *testing.T) {
	parse := func(fields string) (*MultiAgentPreset, error) {
		return Parse([]byte(`{"title": "t", ` + fields + `"participants": [{"displayName": "A"}, {"displayName": "B"}]}`))
	}
	cases := []struct {
		name   string
		fields string
		want   model.ChatCommands
	}{
		{"omitted", ``, model.ChatCommands{}},
		{"empty", `"commands": {}, `, model.ChatCommands{}},
		{"roll with a target", `"commands": {"roll": {"target": 12}}, `, model.ChatCommands{Roll: &model.RollSettings{Target: 12}}},
		{"roll without settings", `"commands": {"roll": {}}, `, model.ChatCommands{Roll: &model.RollSettings{}}},
		{"diceTarget", `"diceTarget": 12, `, model.ChatCommands{Roll: &model.RollSettings{Target: 12}}},
		{"diceTarget 0", `"diceTarget": 0, `, model.ChatCommands{}},
	}
	for _, tc := range cases {
		p, err := parse(tc.fields)
		if err != nil {
			t.Fatalf("%s: Parse = %v", tc.name, err)
		}
		if p.Commands == nil || !reflect.DeepEqual(*p.Commands, tc.want) || p.DiceTarget != 0 {
			t.Errorf("%s: commands = %+v (diceTarget %d), want %+v folded in", tc.name, p.Commands, p.DiceTarget, tc.want)
		}
	}

	for name, fields := range map[string]string{
		"diceTarget below 0":     `"diceTarget": -1, `,
		"diceTarget above 9999":  `"diceTarget": 10000, `,
		"roll.target above 9999": `"commands": {"roll": {"target": 10000}}, `,
		"roll.target fractional": `"commands": {"roll": {"target": 12.5}}, `,
		"both forms":             `"commands": {"roll": {"target": 12}}, "diceTarget": 12, `,
		"an unknown command":     `"commands": {"add": {}}, `,
		"commands not an object": `"commands": ["roll"], `,
	} {
		if _, err := parse(fields); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: Parse = %v, want ErrInvalid", name, err)
		}
	}

	for _, p := range Bundled() {
		if p.ID == "trpg-table" {
			if p.Commands.Roll == nil || p.Commands.Roll.Target != 12 {
				t.Errorf("trpg-table commands = %+v, want roll against 12", p.Commands)
			}
			if !strings.Contains(p.ScenePrompt, "/roll 1d20+修正 行動の要約") || strings.Contains(p.ScenePrompt, "ダイスは使いません") {
				t.Errorf("trpg-table scene does not carry the /roll rule:\n%s", p.ScenePrompt)
			}
		} else if p.Commands.Roll != nil {
			t.Errorf("%s enables /roll, want only trpg-table to", p.ID)
		}
	}
}
