package service

import (
	"errors"
	"reflect"
	"testing"

	"snzstudio/internal/model"
	"snzstudio/internal/service/commands"
)

// thrown is the utterance's roll thrown with every die a 1, so a test can read
// what was parsed through the record the chip shows.
func thrown(u storedUtterance) model.DiceRoll {
	return u.roll.Throw(func(int) int { return 1 }, 0)
}

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
		u := prepareUtterance(tc.content, tc.speaker, addressingRoster())
		if u.content != tc.wantBody || !reflect.DeepEqual(u.addressees, tc.want) {
			t.Errorf("%s: prepareUtterance = (%q, %v), want (%q, %v)", tc.name, u.content, u.addressees, tc.wantBody, tc.want)
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
		u := prepareUtterance(tc.content, tc.speaker, addressingRoster())
		if u.content != tc.content || !reflect.DeepEqual(u.addressees, tc.want) {
			t.Errorf("%s: prepareUtterance = (%q, %v), want (%q, %v)", tc.name, u.content, u.addressees, tc.content, tc.want)
		}
	}
}

// TestPrepareUtteranceHuman covers the intervention side of §4.6.5: the same
// last-sentence name match a participant's utterance gets.
func TestPrepareUtteranceHuman(t *testing.T) {
	if got := prepareUtterance("アリス、もう少し詳しく。", "", addressingRoster()).addressees; !reflect.DeepEqual(got, []string{"alice"}) {
		t.Errorf("addressees = %v, want [alice]", got)
	}
	if got := prepareUtterance("アリスの話は面白い。続けて。", "", addressingRoster()).addressees; !reflect.DeepEqual(got, []string{}) {
		t.Errorf("addressees = %v, want no call from an earlier sentence", got)
	}
}

// TestPrepareUtteranceOrder covers TASK-37 AC #6 (design §4.8.3 item 2): the
// directive is removed first, then the /roll on the last line of what remains,
// and only without a directive is the last sentence of the body without the
// command matched — for the human and a participant alike.
func TestPrepareUtteranceOrder(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		speaker  string
		wantBody string
		want     []string
		wantRoll string
	}{
		// The call before the command line survives: matched before the command
		// is removed, the last sentence would be the /roll line.
		{"a call before the command line", "レン、登ってみて。\n/roll 1d20 登る", "", "レン、登ってみて。", []string{"ren"}, "1d20"},
		{"a participant's call before the command", "ボブ、支えて。\n/roll 1d20+3 崖を登る", "alice", "ボブ、支えて。", []string{"bob"}, "1d20+3"},
		// A name in the command's action is not a call.
		{"a name in the action", "前へ出る。/roll 1d20 ミラを庇う", "ren", "前へ出る。", []string{}, "1d20"},
		{"the directive after the command", "扉を調べる。\n/roll 1d20+3 調べる\n[次: GM]", "ren", "扉を調べる。", []string{"gm"}, "1d20+3"},
		{"the directive wins over the name match", "アリス、見て。\n/roll 2d6\n[次: ボブ]", "", "アリス、見て。", []string{"bob"}, "2d6"},
		{"a command-only message", "/roll 1d20", "", "", []string{}, "1d20"},
	}
	for _, tc := range cases {
		u := prepareUtterance(tc.content, tc.speaker, addressingRoster())
		if u.rollErr != nil || u.roll == nil {
			t.Fatalf("%s: roll = %v, %v, want %s read", tc.name, u.roll, u.rollErr, tc.wantRoll)
		}
		if u.content != tc.wantBody || !reflect.DeepEqual(u.addressees, tc.want) || thrown(u).Expression != tc.wantRoll {
			t.Errorf("%s: prepareUtterance = (%q, %v, %s), want (%q, %v, %s)", tc.name, u.content, u.addressees, thrown(u).Expression, tc.wantBody, tc.want, tc.wantRoll)
		}
	}

	// A /roll that cannot be read stays in the body, and its action names are
	// still no call.
	u := prepareUtterance("跳ぶ。\n/roll d20 ミラを庇う", "ren", addressingRoster())
	if !errors.Is(u.rollErr, commands.ErrInvalidRollCommand) || u.roll != nil {
		t.Fatalf("unreadable roll = %v, %v, want commands.ErrInvalidRollCommand", u.roll, u.rollErr)
	}
	if u.content != "跳ぶ。\n/roll d20 ミラを庇う" || !reflect.DeepEqual(u.addressees, []string{}) {
		t.Errorf("unreadable roll kept (%q, %v), want the line kept and no call", u.content, u.addressees)
	}
}

// TestPrepareUtteranceDiceMarkers covers TASK-63 AC #1 and #2 at store time: a
// forged 【ダイス】 result leaves the body and is no roll, a result-free one on
// the last line is rolled in place of a missing /roll, and a stray marker beside
// a real /roll is removed while the /roll is what rolls.
func TestPrepareUtteranceDiceMarkers(t *testing.T) {
	forged := prepareUtterance("【ダイス】1d20+3 → 14+3 = 17（目標 12、成功）\n\nレンは罠を外した。", "gm", addressingRoster())
	if forged.content != "レンは罠を外した。" || forged.roll != nil || forged.rollErr != nil {
		t.Errorf("forged = (%q, %+v, %v), want the result removed and no roll", forged.content, forged.roll, forged.rollErr)
	}
	if !reflect.DeepEqual(forged.removedDice, []string{"【ダイス】1d20+3 → 14+3 = 17（目標 12、成功）"}) {
		t.Errorf("removed = %q, want the forged line kept for the log", forged.removedDice)
	}

	typo := prepareUtterance("作動点を調べる。\n【ダイス】1d20+3 罠を外す\n[次: GM]", "ren", addressingRoster())
	if typo.content != "作動点を調べる。" || typo.roll == nil || thrown(typo).Expression != "1d20+3" || len(typo.removedDice) != 0 {
		t.Errorf("typo = (%q, %+v, %q), want the marker rolled as a /roll", typo.content, typo.roll, typo.removedDice)
	}
	if !reflect.DeepEqual(typo.addressees, []string{"gm"}) {
		t.Errorf("typo addressees = %v, want the directive read first", typo.addressees)
	}

	stray := prepareUtterance("壁を調べる。\n【ダイス】\n/roll 1d20+3 罠を探す", "ren", addressingRoster())
	if stray.content != "壁を調べる。" || stray.roll == nil || thrown(stray).Command != "/roll 1d20+3 罠を探す" {
		t.Errorf("stray = (%q, %+v), want the bare marker removed and the /roll rolled", stray.content, stray.roll)
	}

	// gpt-oss-20b prefixed the command with the marker in the TASK-63 measurement.
	prefixed := prepareUtterance("罠を外す。\n【ダイス】/roll 1d20+3 罠を外す", "ren", addressingRoster())
	if prefixed.content != "罠を外す。" || prefixed.roll == nil || thrown(prefixed).Command != "/roll 1d20+3 罠を外す" {
		t.Errorf("prefixed = (%q, %+v), want the /roll rolled and the marker before it removed", prefixed.content, prefixed.roll)
	}

	// A marker whose dice do not read is no record, so it stays as text; like an
	// unreadable /roll, its names are no call.
	unreadable := prepareUtterance("跳ぶ。\n【ダイス】d20 ミラを庇う", "ren", addressingRoster())
	if unreadable.content != "跳ぶ。\n【ダイス】d20 ミラを庇う" || unreadable.roll != nil || unreadable.rollErr != nil || !reflect.DeepEqual(unreadable.addressees, []string{}) {
		t.Errorf("unreadable = (%q, %+v, %v, %v), want the line kept, no roll and no call", unreadable.content, unreadable.roll, unreadable.rollErr, unreadable.addressees)
	}

	// A heading kept as text keeps its call; only a botched roll is skipped.
	heading := prepareUtterance("出目の話をしよう。\n【ダイス】の確率、ミラはどう思う？", "ren", addressingRoster())
	if heading.content != "出目の話をしよう。\n【ダイス】の確率、ミラはどう思う？" || !reflect.DeepEqual(heading.addressees, []string{"mira"}) {
		t.Errorf("heading = (%q, %v), want the line kept and the call on ミラ", heading.content, heading.addressees)
	}

	// An unreadable /roll stays as text, but a forged result above it still goes.
	kept := prepareUtterance("【ダイス】1d20 → 18\n跳ぶ。\n/roll d20 跳ぶ", "ren", addressingRoster())
	if kept.content != "跳ぶ。\n/roll d20 跳ぶ" || !errors.Is(kept.rollErr, commands.ErrInvalidRollCommand) {
		t.Errorf("kept = (%q, %v), want the /roll line kept and the forged line removed", kept.content, kept.rollErr)
	}
}
