package lang

import "testing"

func TestParse(t *testing.T) {
	for _, value := range []string{"ja", "en"} {
		if got, ok := Parse(value); !ok || string(got) != value {
			t.Errorf("Parse(%q) = %q, %v", value, got, ok)
		}
	}
	for _, value := range []string{"", "fr", "EN"} {
		if _, ok := Parse(value); ok {
			t.Errorf("Parse(%q) accepted it", value)
		}
	}
}
