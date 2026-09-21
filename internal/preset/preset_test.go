package preset

import (
	"errors"
	"strings"
	"testing"
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
