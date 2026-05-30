package search

import (
	"encoding/json"
	"os"
	"testing"
)

// goldenCase mirrors the JSON produced by tools/segmenter-parity/gen_golden.ts,
// i.e. the reference output of the original Node implementation.
type goldenCase struct {
	Label           string   `json:"label"`
	Input           string   `json:"input"`
	Segments        []string `json:"segments"`
	BuildSearchText string   `json:"buildSearchText"`
	Terms           []string `json:"terms"`
	FtsQuery        string   `json:"ftsQuery"`
}

func loadGolden(t *testing.T) []goldenCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatalf("read golden: %v (regenerate with: pnpm exec tsx tools/segmenter-parity/gen_golden.ts)", err)
	}
	var cases []goldenCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("golden corpus is empty")
	}
	return cases
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// truncate keeps test failure output readable for large sample-doc inputs.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func TestSegmentParity(t *testing.T) {
	cases := loadGolden(t)
	mismatches := 0
	for _, c := range cases {
		got := Segment(c.Input)
		if !eqStrings(got, c.Segments) {
			mismatches++
			if mismatches <= 5 {
				t.Errorf("Segment mismatch [%s] input=%q\n  want %d tokens: %#v\n  got  %d tokens: %#v",
					c.Label, truncate(c.Input, 40), len(c.Segments), c.Segments, len(got), got)
			}
		}
	}
	if mismatches > 0 {
		t.Errorf("Segment: %d/%d cases mismatched", mismatches, len(cases))
	}
}

func TestBuildSearchTextParity(t *testing.T) {
	cases := loadGolden(t)
	for _, c := range cases {
		if got := BuildSearchText(c.Input); got != c.BuildSearchText {
			t.Errorf("BuildSearchText mismatch [%s] input=%q\n  want %q\n  got  %q",
				c.Label, truncate(c.Input, 40), truncate(c.BuildSearchText, 120), truncate(got, 120))
		}
	}
}

func TestTokenizeSearchTermsParity(t *testing.T) {
	cases := loadGolden(t)
	for _, c := range cases {
		if got := TokenizeSearchTerms(c.Input, 12); !eqStrings(got, c.Terms) {
			t.Errorf("TokenizeSearchTerms mismatch [%s] input=%q\n  want %#v\n  got  %#v",
				c.Label, truncate(c.Input, 40), c.Terms, got)
		}
	}
}

func TestToFtsQueryParity(t *testing.T) {
	cases := loadGolden(t)
	for _, c := range cases {
		if got := ToFtsQuery(c.Input); got != c.FtsQuery {
			t.Errorf("ToFtsQuery mismatch [%s] input=%q\n  want %q\n  got  %q",
				c.Label, truncate(c.Input, 40), c.FtsQuery, got)
		}
	}
}
