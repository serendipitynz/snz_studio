package service

import (
	"reflect"
	"testing"

	"snzstudio/internal/model"
)

func addressingRoster() []model.Participant {
	return []model.Participant{
		{ID: "gm", DisplayName: "GM"},
		{ID: "alice", DisplayName: "アリス"},
		{ID: "bob", DisplayName: "ボブ"},
		{ID: "rin", DisplayName: "リ"},
		{ID: "ren", DisplayName: "レン (斥候)"},
		{ID: "mira", DisplayName: "ミラ（神官戦士）"},
		{ID: "buyer-it", DisplayName: "買い手 (情シス担当)"},
		{ID: "buyer-head", DisplayName: "買い手 (部長)"},
	}
}

// TestDetectAddresseesDirective covers the trailing directive of design §4.6.5
// (a): it decides the call on its own, is always removed, and drops only the
// names that do not resolve.
func TestDetectAddresseesDirective(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		speaker  string
		wantBody string
		want     []string
	}{
		{"one name", "扉が開いた。\n[次: アリス]", "gm", "扉が開いた。", []string{"alice"}},
		{"two names with 読点", "どうする？\n[次: アリス、ボブ]", "gm", "どうする？", []string{"alice", "bob"}},
		{"full-width brackets and colon", "どうする？\n［次：ボブ］", "gm", "どうする？", []string{"bob"}},
		{"trailing blank lines", "どうする？\n[次: ボブ]\n\n", "gm", "どうする？", []string{"bob"}},
		{"an unknown name is dropped alone", "どうする？\n[次: 魔王、ボブ]", "gm", "どうする？", []string{"bob"}},
		{"no match still removes the line", "どうする？\n[次: 魔王]", "gm", "どうする？", []string{}},
		{"oneself is dropped", "続けます。\n[次: GM、アリス]", "gm", "続けます。", []string{"alice"}},
		// The directive wins over the name match: ボブ in the body is not a call
		// once the directive names someone else.
		{"directive over the name match", "ボブはどう思う？\n[次: アリス]", "gm", "ボブはどう思う？", []string{"alice"}},
		// The bundled trpg-table labels its players "レン (斥候)"; a game master
		// calls them by the name alone (seen live on gemma-4-e4b).
		{"the name before a parenthesised note", "どうする？\n[次: レン, ミラ]", "gm", "どうする？", []string{"ren", "mira"}},
		{"the full display name still matches", "どうする？\n[次: レン (斥候)]", "gm", "どうする？", []string{"ren"}},
		{"appended to the last sentence", "扉が開いた。[次: ボブ]", "gm", "扉が開いた。", []string{"bob"}},
		// Both buyers shorten to 買い手, so it calls no one; the full name still does.
		{"a short form two participants share", "いかが？\n[次: 買い手]", "gm", "いかが？", []string{}},
		{"the full name of one of them", "いかが？\n[次: 買い手 (部長)]", "gm", "いかが？", []string{"buyer-head"}},
	}
	for _, tc := range cases {
		body, got := detectAddressees(tc.content, tc.speaker, addressingRoster())
		if body != tc.wantBody || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: detectAddressees = (%q, %v), want (%q, %v)", tc.name, body, got, tc.wantBody, tc.want)
		}
	}
}

// TestDetectAddresseesNameMatch covers §4.6.5 (b): without a directive the last
// sentence is matched against the roster, oneself excluded, names shorter than two
// characters skipped, and a match on the whole roster kept.
func TestDetectAddresseesNameMatch(t *testing.T) {
	cases := []struct {
		name    string
		content string
		speaker string
		want    []string
	}{
		{"last sentence only", "アリスの案は面白い。ボブ、どう思う？", "gm", []string{"bob"}},
		{"an earlier sentence is not a call", "ボブの言う通りだ。先へ進もう。", "gm", []string{}},
		{"oneself is not a call", "アリスとしては、アリスの案に賛成です。", "alice", []string{}},
		{"a one-character name is skipped", "リ、どう？", "gm", []string{}},
		{"the whole roster is kept", "GM、アリス、ボブ、みんなどう思う？", "", []string{"gm", "alice", "bob"}},
		{"closing quote after the boundary", "彼は言った。「アリス、来て。」", "gm", []string{"alice"}},
		{"case-insensitive latin names", "What do you think, gm?", "alice", []string{"gm"}},
		{"the name before a parenthesised note", "扉の向こうから音がする。レン、ミラ、どうする？", "gm", []string{"ren", "mira"}},
		{"a shared short form calls only the one named in full", "では条件を。買い手 (部長)、いかがですか？", "gm", []string{"buyer-head"}},
		{"a decimal does not cut the sentence", "アリス、3.5 でどう？", "gm", []string{"alice"}},
		{"a directive-shaped line not at the end stays content", "[次: アリス]\n以上です。", "gm", []string{}},
	}
	for _, tc := range cases {
		body, got := detectAddressees(tc.content, tc.speaker, addressingRoster())
		if body != tc.content || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: detectAddressees = (%q, %v), want (%q, %v)", tc.name, body, got, tc.content, tc.want)
		}
	}
}

// TestDetectHumanAddressees covers the intervention side of §4.6.5: the same
// last-sentence name match a participant's utterance gets.
func TestDetectHumanAddressees(t *testing.T) {
	if got := DetectHumanAddressees("アリス、もう少し詳しく。", addressingRoster()); !reflect.DeepEqual(got, []string{"alice"}) {
		t.Errorf("DetectHumanAddressees = %v, want [alice]", got)
	}
	if got := DetectHumanAddressees("アリスの話は面白い。続けて。", addressingRoster()); !reflect.DeepEqual(got, []string{}) {
		t.Errorf("DetectHumanAddressees = %v, want no call from an earlier sentence", got)
	}
}
