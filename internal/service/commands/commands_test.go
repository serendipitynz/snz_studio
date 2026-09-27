package commands

import (
	"errors"
	"reflect"
	"testing"

	"snzstudio/internal/model"
)

var rollEnabled = model.ChatCommands{Roll: &model.RollSettings{Target: 12}}

// TestExtractWithoutRoll covers TASK-65 AC #3: a chat that has not enabled
// /roll stores its /roll and 【ダイス】 lines as the text they are, rolls
// nothing and lets the name match read the whole body.
func TestExtractWithoutRoll(t *testing.T) {
	for _, body := range []string{
		"岩棚を渡る。\n/roll 1d20+3 岩棚を渡る",
		"調べる。\n【ダイス】1d20+3 罠を外す",
		"【ダイス】1d20+3 → 14+3 = 17（目標 12、成功）\n\nレンは罠を外した。",
		"/roll",
		"/roll 1d20 目標12.5",
	} {
		got := Extract(body, model.ChatCommands{})
		want := Result{Content: body, CallText: body}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Extract(%q, none) = %+v, want the body untouched", body, got)
		}
	}
}

// TestExtractWithRoll covers the pass §4.8.3 item 2 and §4.8.6 set where /roll
// is enabled, now run through Extract rather than prepareUtterance.
func TestExtractWithRoll(t *testing.T) {
	got := Extract("岩棚を渡る。\n/roll 1d20+3 岩棚を渡る", rollEnabled)
	if got.Content != "岩棚を渡る。" || got.CallText != "岩棚を渡る。" || got.Roll == nil || got.Roll.action != "岩棚を渡る" || got.RollErr != nil {
		t.Errorf("a /roll = %+v, want it removed and read", got)
	}

	got = Extract("調べる。\n【ダイス】1d20+3 罠を外す", rollEnabled)
	if got.Content != "調べる。" || got.Roll == nil || got.Roll.line != "【ダイス】1d20+3 罠を外す" {
		t.Errorf("a mistyped roll = %+v, want it rolled as /roll", got)
	}

	got = Extract("【ダイス】1d20+3 → 14+3 = 17（目標 12、成功）\n\nレンは罠を外した。", rollEnabled)
	if got.Content != "レンは罠を外した。" || got.Roll != nil || len(got.RemovedDice) != 1 {
		t.Errorf("a forged result = %+v, want it removed and not rolled", got)
	}

	// An unreadable /roll stays in the body but out of the name match.
	got = Extract("ミラ、見ていて。\n/roll 1d20 目標12.5 ミラを庇う", rollEnabled)
	if got.Content != "ミラ、見ていて。\n/roll 1d20 目標12.5 ミラを庇う" || got.CallText != "ミラ、見ていて。" || !errors.Is(got.RollErr, ErrInvalidRollCommand) {
		t.Errorf("an unreadable /roll = %+v, want it kept as text and left out of CallText", got)
	}

	// A {} roll is enabled too: the key is what enables a command.
	if got := Extract("/roll 1d6", model.ChatCommands{Roll: &model.RollSettings{}}); got.Roll == nil {
		t.Errorf("roll without a target = %+v, want the command read", got)
	}
}
