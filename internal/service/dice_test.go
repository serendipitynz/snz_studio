package service

import (
	"errors"
	"reflect"
	"testing"

	"snzstudio/internal/model"
)

// TestSplitRollCommand covers where a /roll is found (design §4.8.3 item 2): on
// the last line only, as a word of its own, also when it follows the body
// without a line break.
func TestSplitRollCommand(t *testing.T) {
	cases := []struct {
		name      string
		content   string
		wantFound bool
		wantBody  string
	}{
		{"own line", "岩棚を横歩きで渡る。\n/roll 1d20+3 岩棚を渡る", true, "岩棚を横歩きで渡る。"},
		{"appended after 。", "岩棚を横歩きで渡る。/roll 1d20+3 岩棚を渡る", true, "岩棚を横歩きで渡る。"},
		{"appended after a space", "I cross the ledge /roll 1d20+3", true, "I cross the ledge"},
		{"trailing blank lines", "渡る。\n/roll 1d20\n\n", true, "渡る。"},
		{"command only", "/roll 1d20", true, ""},
		{"not on the last line", "/roll 1d20\n渡る。", false, "/roll 1d20\n渡る。"},
		{"part of a path", "see https://example.com/roll 1d20", false, "see https://example.com/roll 1d20"},
		{"a longer word", "/rolling 1d20", false, "/rolling 1d20"},
	}
	for _, tc := range cases {
		body, found, _, _ := splitRollCommand(tc.content)
		if found != tc.wantFound || (found && body != tc.wantBody) {
			t.Errorf("%s: splitRollCommand = (%q, %v), want (%q, %v)", tc.name, body, found, tc.wantBody, tc.wantFound)
		}
	}
}

// TestParseRollCommand covers the fixed argument order of design §4.8.3 item 4
// and the bounds of an expression.
func TestParseRollCommand(t *testing.T) {
	valid := []struct {
		line string
		want rollCommand
	}{
		{"/roll 1d20+3 目標12 岩棚を渡る", rollCommand{line: "/roll 1d20+3 目標12 岩棚を渡る", expression: "1d20+3", count: 1, sides: 20, modifier: 3, target: 12, action: "岩棚を渡る"}},
		{"/roll 2d6", rollCommand{line: "/roll 2d6", expression: "2d6", count: 2, sides: 6}},
		{"/roll 1D20-2 扉を破る", rollCommand{line: "/roll 1D20-2 扉を破る", expression: "1d20-2", count: 1, sides: 20, modifier: -2, action: "扉を破る"}},
		// The minus sign the measurement's own rule text used, and full-width
		// forms an IME leaves behind.
		{"/roll 1d20−2", rollCommand{line: "/roll 1d20−2", expression: "1d20-2", count: 1, sides: 20, modifier: -2}},
		{"/roll　１d２０＋３　目標１２", rollCommand{line: "/roll　１d２０＋３　目標１２", expression: "1d20+3", count: 1, sides: 20, modifier: 3, target: 12}},
		{"/roll 1d20 目標 15 跳ぶ", rollCommand{line: "/roll 1d20 目標 15 跳ぶ", expression: "1d20", count: 1, sides: 20, target: 15, action: "跳ぶ"}},
		{"/roll 1d20+0 見張る", rollCommand{line: "/roll 1d20+0 見張る", expression: "1d20+0", count: 1, sides: 20, action: "見張る"}},
		{"/roll 20d100+999", rollCommand{line: "/roll 20d100+999", expression: "20d100+999", count: 20, sides: 100, modifier: 999}},
		// English tables write target; a target of 0 or なし compares nothing.
		{"/roll 1d20 target15 jump", rollCommand{line: "/roll 1d20 target15 jump", expression: "1d20", count: 1, sides: 20, target: 15, action: "jump"}},
		{"/roll 1d20 Target: 15 jump", rollCommand{line: "/roll 1d20 Target: 15 jump", expression: "1d20", count: 1, sides: 20, target: 15, action: "jump"}},
		{"/roll 1d20 targeted strike", rollCommand{line: "/roll 1d20 targeted strike", expression: "1d20", count: 1, sides: 20, action: "targeted strike"}},
		{"/roll 2d6 目標なし 丁半", rollCommand{line: "/roll 2d6 目標なし 丁半", expression: "2d6", count: 2, sides: 6, noTarget: true, action: "丁半"}},
		{"/roll 2d6 目標0", rollCommand{line: "/roll 2d6 目標0", expression: "2d6", count: 2, sides: 6, noTarget: true}},
		{"/roll 2d6 target 0 initiative", rollCommand{line: "/roll 2d6 target 0 initiative", expression: "2d6", count: 2, sides: 6, noTarget: true, action: "initiative"}},
	}
	for _, tc := range valid {
		got, err := parseRollCommand(tc.line)
		if err != nil {
			t.Errorf("parseRollCommand(%q) = %v", tc.line, err)
			continue
		}
		if !reflect.DeepEqual(*got, tc.want) {
			t.Errorf("parseRollCommand(%q) = %+v, want %+v", tc.line, *got, tc.want)
		}
	}

	invalid := []string{
		"/roll",
		"/roll d20",
		"/roll 1d20+",
		"/roll 0d6",
		"/roll 21d6",
		"/roll 1d1",
		"/roll 1d101",
		"/roll 1d20+1000",
		"/roll 1d20 目標10000",
		"/roll 1d20 目標",
		"/roll 1d20 目標 高い",
		"/roll 1d20 目標12.5 跳ぶ",
		"/roll 1d20 目標-1 跳ぶ",
		"/roll 1d20 目標abc 跳ぶ",
		"/roll 1d20 target12.5 jump",
		"/roll 1d20 target: high",
		"/roll 岩棚を渡る",
	}
	for _, line := range invalid {
		if got, err := parseRollCommand(line); !errors.Is(err, ErrInvalidRollCommand) {
			t.Errorf("parseRollCommand(%q) = %+v, %v, want ErrInvalidRollCommand", line, got, err)
		}
	}
}

// TestRollCommandRoll covers TASK-37 AC #2: the command's own target first, the
// chat's default otherwise, success at a total equal to the target, and the
// total alone when there is neither.
func TestRollCommandRoll(t *testing.T) {
	fixed := func(values ...int) func(int) int {
		return func(int) int {
			v := values[0]
			values = values[1:]
			return v
		}
	}
	cmd, err := parseRollCommand("/roll 1d20+3 目標12 岩棚を渡る")
	if err != nil {
		t.Fatal(err)
	}

	failed := cmd.roll(fixed(4), 0)
	if failed.Total != 7 || failed.Target != 12 || failed.Success == nil || *failed.Success {
		t.Fatalf("roll = %+v, want 7 against 12, failed", failed)
	}
	if !reflect.DeepEqual(failed.Dice, []int{4}) || failed.Modifier != 3 || failed.Command != "/roll 1d20+3 目標12 岩棚を渡る" {
		t.Fatalf("roll record = %+v, want the die, the modifier and the command kept", failed)
	}
	if met := cmd.roll(fixed(9), 20); met.Target != 12 || met.Success == nil || !*met.Success {
		t.Fatalf("roll = %+v, want the command's 12 over the default 20, and 12 to succeed", met)
	}

	plain, _ := parseRollCommand("/roll 2d6")
	if r := plain.roll(fixed(3, 5), 8); r.Total != 8 || r.Target != 8 || r.Success == nil || !*r.Success {
		t.Fatalf("roll = %+v, want the default target 8 met", r)
	}
	if r := plain.roll(fixed(3, 5), 0); r.Target != 0 || r.Success != nil {
		t.Fatalf("roll = %+v, want the total alone with no target", r)
	}
	none, _ := parseRollCommand("/roll 2d6 目標なし 丁半")
	if r := none.roll(fixed(3, 5), 12); r.Target != 0 || r.Success != nil || r.Total != 8 {
		t.Fatalf("roll = %+v, want 目標なし to record the total alone over the default 12", r)
	}

	for i := 0; i < 200; i++ {
		if v := rollDie(6); v < 1 || v > 6 {
			t.Fatalf("rollDie(6) = %d", v)
		}
	}
}

func TestDiceRollLine(t *testing.T) {
	failed, succeeded := false, true
	cases := []struct {
		roll model.DiceRoll
		want string
	}{
		{model.DiceRoll{Expression: "1d20+3", Action: "岩棚を渡る", Dice: []int{4}, Modifier: 3, Total: 7, Target: 12, Success: &failed}, "岩棚を渡る — 1d20+3 → 4+3 = 7（目標 12、失敗）"},
		{model.DiceRoll{Expression: "2d6-1", Dice: []int{3, 5}, Modifier: -1, Total: 7, Target: 7, Success: &succeeded}, "2d6-1 → 3+5-1 = 7（目標 7、成功）"},
		{model.DiceRoll{Expression: "2d6", Dice: []int{3, 5}, Total: 8}, "2d6 → 3+5 = 8"},
		{model.DiceRoll{Expression: "1d20", Action: "見張る", Dice: []int{14}, Total: 14}, "見張る — 1d20 → 14"},
	}
	for _, tc := range cases {
		if got := diceRollLine(tc.roll); got != tc.want {
			t.Errorf("diceRollLine = %q, want %q", got, tc.want)
		}
	}
}

// TestSplitDiceMarkerCommand covers TASK-63 AC #2: a result-free 【ダイス】
// opening the last line reads as the /roll the speaker meant, in the command's
// order or the mapping's; one carrying a result, one with nothing to roll, one
// inside a sentence and one not on the last line are not read.
func TestSplitDiceMarkerCommand(t *testing.T) {
	read := []struct {
		name       string
		content    string
		wantBody   string
		expression string
		action     string
	}{
		{"its own line", "盗賊道具で作動点を調べる。\n【ダイス】1d20+3 罠を外す", "盗賊道具で作動点を調べる。", "1d20+3", "罠を外す"},
		{"indented", "調べる。\n　【ダイス】1d20+3 罠を外す", "調べる。", "1d20+3", "罠を外す"},
		{"the mapping's order", "調べる。\n【ダイス】罠を外す — 1d20+3", "調べる。", "1d20+3", "罠を外す"},
		{"the dice before the dash", "調べる。\n【ダイス】1d20+3 — 罠を外す", "調べる。", "1d20+3", "罠を外す"},
		{"marker only", "【ダイス】2d6", "", "2d6", ""},
	}
	for _, tc := range read {
		body, found, cmd := splitDiceMarkerCommand(tc.content)
		if !found || cmd == nil {
			t.Fatalf("%s: splitDiceMarkerCommand(%q) found nothing", tc.name, tc.content)
		}
		if body != tc.wantBody || cmd.expression != tc.expression || cmd.action != tc.action {
			t.Errorf("%s: = (%q, %s, %q), want (%q, %s, %q)", tc.name, body, cmd.expression, cmd.action, tc.wantBody, tc.expression, tc.action)
		}
	}
	if _, _, cmd := splitDiceMarkerCommand("調べる。\n【ダイス】1d20+3 罠を外す"); cmd.line != "【ダイス】1d20+3 罠を外す" {
		t.Errorf("command line = %q, want what the speaker wrote", cmd.line)
	}

	notRead := []string{
		"【ダイス】1d20+3 → 14+3 = 17（目標 12、成功）",
		"【ダイス】罠を外す — 1d20+3 → 14+3 = 17",
		"【ダイス】1d20+3 = 17",
		"【ダイス】1d20+3 成功",
		"作動点を調べる。【ダイス】1d20+3 罠を外す",
		"調べる。\n【ダイス】",
		"【ダイス】d20 罠を外す",
		"【ダイス】1d20+3 罠を外す\n調べる。",
	}
	for _, content := range notRead {
		if body, found, cmd := splitDiceMarkerCommand(content); found || cmd != nil || body != content {
			t.Errorf("splitDiceMarkerCommand(%q) = (%q, %v, %+v), want it left alone", content, body, found, cmd)
		}
	}
}

// TestStripDiceMarkers covers TASK-63 AC #1: a line opening with 【ダイス】 is
// removed when it is shaped like a record — empty, a result, or dice that read
// as a /roll — and any other 【ダイス】 stays as text.
func TestStripDiceMarkers(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		want        string
		wantRemoved []string
	}{
		{"a forged result above the narration", "【ダイス】1d20+3 → 14+3 = 17（目標 12、成功）\n\nレンは罠を外した。", "レンは罠を外した。", []string{"【ダイス】1d20+3 → 14+3 = 17（目標 12、成功）"}},
		{"a bare marker line", "壁を調べる。\n【ダイス】\n続けて進む。", "壁を調べる。\n続けて進む。", []string{"【ダイス】"}},
		{"an outcome without an arrow", "【ダイス】15 成功\n罠を外した。", "罠を外した。", []string{"【ダイス】15 成功"}},
		{"a result-free roll", "【ダイス】1d20+3 罠を外す\n罠を外した。", "罠を外した。", []string{"【ダイス】1d20+3 罠を外す"}},
		{"a heading", "【ダイス】の確率について\n2d6 の分布を見る。", "【ダイス】の確率について\n2d6 の分布を見る。", nil},
		{"dice that do not read", "跳ぶ。\n【ダイス】d20 跳ぶ", "跳ぶ。\n【ダイス】d20 跳ぶ", nil},
		{"inside a sentence", "罠を外した。【ダイス】1d20 → 18\n先へ進む。", "罠を外した。【ダイス】1d20 → 18\n先へ進む。", nil},
		{"none", "罠を外した。", "罠を外した。", nil},
	}
	for _, tc := range cases {
		got, removed := stripDiceMarkers(tc.content)
		if got != tc.want || !reflect.DeepEqual(removed, tc.wantRemoved) {
			t.Errorf("%s: stripDiceMarkers = (%q, %q), want (%q, %q)", tc.name, got, removed, tc.want, tc.wantRemoved)
		}
	}
}
