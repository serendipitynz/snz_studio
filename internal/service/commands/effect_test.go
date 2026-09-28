package commands

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"snzstudio/internal/model"
)

var effectRoster = []model.Participant{
	{ID: "gm", DisplayName: "GM"},
	{ID: "ren", DisplayName: "レン (斥候)"},
	{ID: "mira", DisplayName: "ミラ (神官戦士)"},
}

var effectsEnabled = model.ChatCommands{Add: &model.EffectSettings{}, Use: &model.EffectSettings{}, Set: &model.EffectSettings{}}

func readEffect(t *testing.T, line string) *Effect {
	t.Helper()
	got := Extract(line, effectsEnabled, effectRoster)
	if got.EffectErr != nil || got.Effect == nil {
		t.Fatalf("Extract(%q) = %+v, want an effect read", line, got)
	}
	return got.Effect
}

// TestParseEffect covers the three forms of design §4.8.4 and how an owner is
// named (§4.8.8): by the name before its note, by the full display name with
// its space, or as 共通.
func TestParseEffect(t *testing.T) {
	cases := []struct {
		line string
		want Effect
	}{
		{"/add レン HP -3", Effect{kind: "add", owner: "レン (斥候)", participantID: "ren", item: "HP", amount: -3}},
		{"/add レン (斥候) HP +2", Effect{kind: "add", owner: "レン (斥候)", participantID: "ren", item: "HP", amount: 2}},
		{"/add ミラ HP ４", Effect{kind: "add", owner: "ミラ (神官戦士)", participantID: "mira", item: "HP", amount: 4}},
		{"/add gm 残り −1", Effect{kind: "add", owner: "GM", participantID: "gm", item: "残り", amount: -1}},
		{"/use レン たいまつ", Effect{kind: "use", owner: "レン (斥候)", participantID: "ren", item: "たいまつ", amount: -1}},
		{"/set 共通 場所 第二坑道の 分かれ道", Effect{kind: "set", owner: "共通", item: "場所", value: "第二坑道の 分かれ道"}},
		{"/set 共通 場所: 第二坑道", Effect{kind: "set", owner: "共通", item: "場所", value: "第二坑道"}},
		{"/set 共通　時刻：夜", Effect{kind: "set", owner: "共通", item: "時刻", value: "夜"}},
	}
	for _, tc := range cases {
		got := readEffect(t, tc.line)
		tc.want.line = tc.line
		if !reflect.DeepEqual(*got, tc.want) {
			t.Errorf("parse %q = %+v, want %+v", tc.line, *got, tc.want)
		}
	}

	dice := readEffect(t, "/add レン HP -1d6+1")
	if dice.dice == nil || dice.dice.expression != "1d6+1" || !dice.negative || dice.amount != 0 {
		t.Errorf("dice = %+v, want -(1d6+1)", dice)
	}
}

// TestParseEffectRefused covers the lines no effect is read from, which stay
// in the body with the error for the caller.
func TestParseEffectRefused(t *testing.T) {
	roster := append([]model.Participant{}, effectRoster...)
	roster = append(roster, model.Participant{ID: "buyer1", DisplayName: "買い手 (部長)"}, model.Participant{ID: "buyer2", DisplayName: "買い手 (情シス)"})
	for _, line := range []string{
		"/add ガルド HP -3",    // not on the roster
		"/add 買い手 HP -3",    // shared by two participants
		"/add レン HP",        // no amount
		"/add レン HP -3 落下",  // more than one amount
		"/add レン HP 1d1",    // a die needs two sides
		"/add レン HP -10000", // beyond the bound
		"/add レン HP 三",      // not a number
		"/use レン たいまつ 2",    // /use takes nothing more
		"/set 共通 場所",        // no value
		"/set 共通",           // no item
		"/add レンHP -3",      // the owner runs into the item
	} {
		got := Extract("転ぶ。\n"+line, effectsEnabled, roster)
		if !errors.Is(got.EffectErr, ErrInvalidEffectCommand) || got.Effect != nil {
			t.Errorf("Extract(%q) = %+v, want ErrInvalidEffectCommand", line, got)
			continue
		}
		if got.Content != "転ぶ。\n"+line || got.CallText != "転ぶ。" {
			t.Errorf("Extract(%q) = (%q, %q), want the line kept in the body and out of the call text", line, got.Content, got.CallText)
		}
	}
}

// TestExtractEffectCommand covers where an effect command is found: on the last
// line, after the body as /roll is, only where the chat enables it, and first
// among the commands on the line.
func TestExtractEffectCommand(t *testing.T) {
	got := Extract("レンは足を滑らせた。/add レン HP -3", effectsEnabled, effectRoster)
	if got.Content != "レンは足を滑らせた。" || got.Effect == nil || got.Effect.amount != -3 {
		t.Errorf("appended = %+v, want the command taken off the body", got)
	}

	onlyAdd := model.ChatCommands{Add: &model.EffectSettings{}}
	if got := Extract("たいまつを灯す。\n/use レン たいまつ", onlyAdd, effectRoster); got.Effect != nil || got.Content != "たいまつを灯す。\n/use レン たいまつ" {
		t.Errorf("disabled /use = %+v, want the line kept as text", got)
	}
	if got := Extract("/set 共通 場所 坑道", model.ChatCommands{}, effectRoster); got.Effect != nil || got.Content != "/set 共通 場所 坑道" {
		t.Errorf("nothing enabled = %+v, want the line kept as text", got)
	}

	both := model.ChatCommands{Roll: &model.RollSettings{}, Add: &model.EffectSettings{}}
	if got := Extract("/add レン HP -1d6 /roll 1d20", both, effectRoster); !errors.Is(got.EffectErr, ErrInvalidEffectCommand) || got.Roll != nil {
		t.Errorf("add first = %+v, want the /add read with the /roll as its arguments", got)
	}
	if got := Extract("/roll 1d20 /add レン HP -1", both, effectRoster); got.Roll == nil || got.Effect != nil {
		t.Errorf("roll first = %+v, want the /roll read", got)
	}
	if got := Extract("/address the crowd", both, effectRoster); got.Effect != nil || got.EffectErr != nil {
		t.Errorf("a longer word = %+v, want no command", got)
	}

	forged := Extract("【ダイス】1d20 → 18（目標 12、成功）\nレンは跳んだ。\n/add ミラ HP -2", both, effectRoster)
	if forged.Content != "レンは跳んだ。" || forged.Effect == nil || len(forged.RemovedDice) != 1 {
		t.Errorf("forged = %+v, want the forged 【ダイス】 line removed with /roll enabled", forged)
	}
}

func fixedDie(values ...int) func(int) int {
	return func(int) int {
		v := values[0]
		values = values[1:]
		return v
	}
}

// TestApplyEffect covers TASK-36 AC #1, #3 and #5: set and add reach the sheet,
// and what fails a precondition or the limit leaves it as it was.
func TestApplyEffect(t *testing.T) {
	const sheet = "HP: 7/10\nたいまつ: 2 本\n包帯: 0\n所持品: ロープ"
	cases := []struct {
		line      string
		wantSheet string
		want      model.StateEffect
	}{
		{"/add レン HP -3", "HP: 4/10\nたいまつ: 2 本\n包帯: 0\n所持品: ロープ",
			model.StateEffect{Kind: "add", Delta: -3, Item: "HP", Before: strPtr("7/10"), After: "4/10", Applied: true}},
		{"/add レン hp -9", "HP: -2/10\nたいまつ: 2 本\n包帯: 0\n所持品: ロープ",
			model.StateEffect{Kind: "add", Delta: -9, Item: "hp", Before: strPtr("7/10"), After: "-2/10", Applied: true}},
		{"/use レン たいまつ", "HP: 7/10\nたいまつ: 1 本\n包帯: 0\n所持品: ロープ",
			model.StateEffect{Kind: "use", Delta: -1, Item: "たいまつ", Before: strPtr("2 本"), After: "1 本", Applied: true}},
		{"/use レン 包帯", sheet,
			model.StateEffect{Kind: "use", Delta: -1, Item: "包帯", Before: strPtr("0"), Reason: model.EffectNotPositive}},
		{"/use レン 聖水", sheet,
			model.StateEffect{Kind: "use", Delta: -1, Item: "聖水", Reason: model.EffectMissingItem}},
		{"/add レン 所持品 +1", sheet,
			model.StateEffect{Kind: "add", Delta: 1, Item: "所持品", Before: strPtr("ロープ"), Reason: model.EffectNotInteger}},
		{"/set レン 所持品 ロープ、鍵", "HP: 7/10\nたいまつ: 2 本\n包帯: 0\n所持品: ロープ、鍵",
			model.StateEffect{Kind: "set", Item: "所持品", Value: "ロープ、鍵", Before: strPtr("ロープ"), After: "ロープ、鍵", Applied: true}},
		{"/set レン 状態 毒", sheet + "\n状態: 毒",
			model.StateEffect{Kind: "set", Item: "状態", Value: "毒", After: "毒", Applied: true}},
		{"/set レン 状態 " + strings.Repeat("毒", 200), sheet,
			model.StateEffect{Kind: "set", Item: "状態", Value: strings.Repeat("毒", 200), Reason: model.EffectOverLimit}},
	}
	for _, tc := range cases {
		effect := readEffect(t, tc.line)
		gotSheet, got := effect.Apply(sheet, fixedDie())
		tc.want.Command, tc.want.Owner, tc.want.ParticipantID = tc.line, "レン (斥候)", "ren"
		if gotSheet != tc.wantSheet || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s:\n got (%q, %+v)\nwant (%q, %+v)", tc.line, gotSheet, got, tc.wantSheet, tc.want)
		}
	}
}

// TestApplyEffectDice covers an /add that rolls its amount: the sign takes the
// whole expression away, and the dice are thrown only when the value can take
// them.
func TestApplyEffectDice(t *testing.T) {
	effect := readEffect(t, "/add ミラ HP -1d6+1")
	sheet, got := effect.Apply("HP: 14/14", fixedDie(3))
	if sheet != "HP: 10/14" || got.Delta != -4 || !reflect.DeepEqual(got.Dice, []int{3}) || got.Modifier != 1 || got.Expression != "-1d6+1" {
		t.Errorf("dice = (%q, %+v), want 14 - (3+1)", sheet, got)
	}
	if EffectLine(got) != "ミラ (神官戦士) HP -1d6+1（3+1 = 4）: 14/14 → 10/14" {
		t.Errorf("EffectLine = %q", EffectLine(got))
	}

	thrown := false
	_, missing := effect.Apply("MP: 3", func(int) int { thrown = true; return 1 })
	if thrown || missing.Applied || missing.Reason != model.EffectMissingItem || missing.Dice != nil {
		t.Errorf("missing = %+v (thrown %v), want no dice thrown for an effect that cannot apply", missing, thrown)
	}
}

// TestApplyEffectSheetShapes covers the sheet forms a human writes: full-width
// colon and digits, an empty value, an empty sheet, and the shared sheet's own
// limit.
func TestApplyEffectSheetShapes(t *testing.T) {
	sheet, got := readEffect(t, "/add レン HP -1").Apply("HP：７/10", fixedDie())
	if sheet != "HP：6/10" || !got.Applied {
		t.Errorf("full width = (%q, %+v)", sheet, got)
	}
	sheet, _ = readEffect(t, "/set レン 状態 毒").Apply("状態:", fixedDie())
	if sheet != "状態: 毒" {
		t.Errorf("empty value = %q, want the value set apart from the colon", sheet)
	}
	sheet, got = readEffect(t, "/set 共通 場所 坑道").Apply("", fixedDie())
	if sheet != "場所: 坑道" || got.Before != nil || got.ParticipantID != "" {
		t.Errorf("empty sheet = (%q, %+v)", sheet, got)
	}
	long := strings.Repeat("あ", 250)
	if _, got := readEffect(t, "/set 共通 メモ "+long).Apply("", fixedDie()); !got.Applied {
		t.Errorf("shared = %+v, want 250 characters within the shared sheet's 400", got)
	}
	if _, got := readEffect(t, "/set レン メモ "+long).Apply("", fixedDie()); got.Applied || got.Reason != model.EffectOverLimit {
		t.Errorf("participant = %+v, want over the participant sheet's 200", got)
	}
}

// TestEffectLine covers how an effect reads in the transcript, applied or not.
func TestEffectLine(t *testing.T) {
	cases := map[string]model.StateEffect{
		"レン HP -3: 7/10 → 4/10": {Kind: "add", Owner: "レン", Item: "HP", Delta: -3, Before: strPtr("7/10"), After: "4/10", Applied: true},
		"レン たいまつ -1 — 適用されず（たいまつが 1 未満）":    {Kind: "use", Owner: "レン", Item: "たいまつ", Delta: -1, Before: strPtr("0"), Reason: model.EffectNotPositive},
		"レン 聖水 -1 — 適用されず（聖水の行が無い）":         {Kind: "use", Owner: "レン", Item: "聖水", Delta: -1, Reason: model.EffectMissingItem},
		"レン HP -1d6 — 適用されず（HPの値が整数で始まらない）": {Kind: "add", Owner: "レン", Item: "HP", Expression: "-1d6", Before: strPtr("元気"), Reason: model.EffectNotInteger},
		"共通 場所: （無し） → 坑道":                  {Kind: "set", Owner: "共通", Item: "場所", Value: "坑道", After: "坑道", Applied: true},
		"共通 場所: 入口 → 坑道":                    {Kind: "set", Owner: "共通", Item: "場所", Value: "坑道", Before: strPtr("入口"), After: "坑道", Applied: true},
		"共通 メモ → 長文 — 適用されず（状態が 400 字を超える）": {Kind: "set", Owner: "共通", Item: "メモ", Value: "長文", Reason: model.EffectOverLimit},
		"レン メモ → 長文 — 適用されず（状態が 200 字を超える）": {Kind: "set", Owner: "レン", ParticipantID: "ren", Item: "メモ", Value: "長文", Reason: model.EffectOverLimit},
		"ミラ HP +2d6（3+4 = 7）: 3/14 → 10/14": {Kind: "add", Owner: "ミラ", Item: "HP", Expression: "+2d6", Dice: []int{3, 4}, Delta: 7, Before: strPtr("3/14"), After: "10/14", Applied: true},
	}
	// A record edited by hand in the database must not panic the prompt build.
	cases["レン HP -3:  → 4/10"] = model.StateEffect{Kind: "add", Owner: "レン", Item: "HP", Delta: -3, After: "4/10", Applied: true}
	for want, r := range cases {
		if got := EffectLine(r); got != want {
			t.Errorf("EffectLine(%+v) = %q, want %q", r, got, want)
		}
	}
}

func strPtr(s string) *string {
	return &s
}

// TestExtractEffectLineAbove covers design §4.8.8's second place an effect
// command is read from: above the last line, where a game master writes the
// damage and goes on narrating, on a line of its own or run into a sentence.
// The last line keeps precedence, only the first readable one runs, one that
// cannot be read there is prose, and /roll is still read from the last line
// alone.
func TestExtractEffectLineAbove(t *testing.T) {
	both := model.ChatCommands{Roll: &model.RollSettings{}, Add: &model.EffectSettings{}, Use: &model.EffectSettings{}, Set: &model.EffectSettings{}}
	cases := []struct {
		name, body, wantContent string
		wantEffect              string // the command read, "" for none
		wantRoll                bool
	}{
		{"own line mid-body", "岩が肩を打つ。\n\n/add レン HP -1d6\n\nレンは立ち上がる。ミラ、どうする？",
			"岩が肩を打つ。\n\nレンは立ち上がる。ミラ、どうする？", "/add レン HP -1d6", false},
		{"indented, first line", "  /set 共通 場所 第二坑道\n一行は奥へ進む。", "一行は奥へ進む。", "/set 共通 場所 第二坑道", false},
		{"run into a sentence", "岩が肩を打つ。/add レン HP -1d6  \nレンは立ち上がる。",
			"岩が肩を打つ。\nレンは立ち上がる。", "/add レン HP -1d6", false},
		{"explaining a command", "/add は「/add レン HP -3」のように書く。\n以上。",
			"/add は「/add レン HP -3」のように書く。\n以上。", "", false},
		{"first readable", "/add ガルド HP -3\n倒れる。\n/add ミラ HP -1\n立つ。",
			"/add ガルド HP -3\n倒れる。\n立つ。", "/add ミラ HP -1", false},
		{"the last line wins", "/add レン HP -1\n岩棚を渡る。\n/roll 1d20+3 岩棚を渡る",
			"/add レン HP -1\n岩棚を渡る。", "", true},
		{"a dice marker ending the text wins", "/add レン HP -1\n調べる。\n【ダイス】1d20+3 罠を外す",
			"/add レン HP -1\n調べる。", "", true},
		{"first of two", "/use レン たいまつ\n灯す。\n/use ミラ 包帯\n巻く。",
			"灯す。\n/use ミラ 包帯\n巻く。", "/use レン たいまつ", false},
		{"no /roll above the last line", "/roll 1d20\n跳ぶ。", "/roll 1d20\n跳ぶ。", "", false},
		{"leftmost on its line", "/set 共通 メモ /add レン HP -3\n続く。", "続く。", "/set 共通 メモ /add レン HP -3", false},
	}
	for _, tc := range cases {
		got := Extract(tc.body, both, effectRoster)
		command := ""
		if got.Effect != nil {
			command = got.Effect.line
		}
		if got.Content != tc.wantContent || command != tc.wantEffect || (got.Roll != nil) != tc.wantRoll || got.EffectErr != nil {
			t.Errorf("%s: Extract = (%q, effect %q, roll %v, %v), want (%q, %q, roll %v)", tc.name, got.Content, command, got.Roll != nil, got.EffectErr, tc.wantContent, tc.wantEffect, tc.wantRoll)
		}
	}

	unreadable := Extract("転ぶ。\n/add ガルド HP -3\n立つ。", both, effectRoster)
	if unreadable.EffectErr != nil || unreadable.Effect != nil || unreadable.Content != "転ぶ。\n/add ガルド HP -3\n立つ。" {
		t.Errorf("unreadable = %+v, want the line kept as prose with no error", unreadable)
	}
}
