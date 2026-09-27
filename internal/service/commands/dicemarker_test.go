package commands

import (
	"reflect"
	"testing"
)

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
		// An outcome word in the action is part of what the player does, not a
		// result: the app rolls, as it would for the same /roll.
		{"an outcome word in the action", "説得する。\n【ダイス】1d20+3 交渉を成功させる", "説得する。", "1d20+3", "交渉を成功させる"},
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
		"【ダイス】15 成功",
		"【ダイス】13（成功）",
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
		{"a heading with an outcome word", "【ダイス】の判定で失敗した話\n続ける。", "【ダイス】の判定で失敗した話\n続ける。", nil},
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
