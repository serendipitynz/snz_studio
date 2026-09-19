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
		"not json":        `{`,
		"no title":        `{"participants":[{"displayName":"甲"},{"displayName":"乙"}]}`,
		"one participant": `{"title":"x","participants":[{"displayName":"甲"}]}`,
		"blank name":      `{"title":"x","participants":[{"displayName":"甲"},{"displayName":"  "}]}`,
		"bad turn rule":   `{"title":"x","turnRule":"auction","participants":[{"displayName":"甲"},{"displayName":"乙"}]}`,
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
