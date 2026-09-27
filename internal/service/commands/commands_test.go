package commands

import (
	"errors"
	"testing"
)

// TestExtractWithRoll covers the pass §4.8.3 item 2 and §4.8.6 set, run
// through Extract rather than prepareUtterance.
func TestExtractWithRoll(t *testing.T) {
	got := Extract("岩棚を渡る。\n/roll 1d20+3 岩棚を渡る")
	if got.Content != "岩棚を渡る。" || got.CallText != "岩棚を渡る。" || got.Roll == nil || got.Roll.action != "岩棚を渡る" || got.RollErr != nil {
		t.Errorf("a /roll = %+v, want it removed and read", got)
	}

	got = Extract("調べる。\n【ダイス】1d20+3 罠を外す")
	if got.Content != "調べる。" || got.Roll == nil || got.Roll.line != "【ダイス】1d20+3 罠を外す" {
		t.Errorf("a mistyped roll = %+v, want it rolled as /roll", got)
	}

	got = Extract("【ダイス】1d20+3 → 14+3 = 17（目標 12、成功）\n\nレンは罠を外した。")
	if got.Content != "レンは罠を外した。" || got.Roll != nil || len(got.RemovedDice) != 1 {
		t.Errorf("a forged result = %+v, want it removed and not rolled", got)
	}

	// An unreadable /roll stays in the body but out of the name match.
	got = Extract("ミラ、見ていて。\n/roll 1d20 目標12.5 ミラを庇う")
	if got.Content != "ミラ、見ていて。\n/roll 1d20 目標12.5 ミラを庇う" || got.CallText != "ミラ、見ていて。" || !errors.Is(got.RollErr, ErrInvalidRollCommand) {
		t.Errorf("an unreadable /roll = %+v, want it kept as text and left out of CallText", got)
	}

}
