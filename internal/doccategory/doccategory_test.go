package doccategory

import "testing"

func TestIsValid(t *testing.T) {
	for _, c := range []string{"world", "character", "rule", "plot", "timeline", "index", "story", "misc"} {
		if !IsValid(c) {
			t.Errorf("IsValid(%q) = false, want true", c)
		}
	}
	for _, c := range []string{"", "unknown", "World", "stories"} {
		if IsValid(c) {
			t.Errorf("IsValid(%q) = true, want false", c)
		}
	}
}

func TestInferEmpty(t *testing.T) {
	if got := Infer(Input{}); got != "misc" {
		t.Errorf("Infer(empty) = %q, want misc", got)
	}
	if got := Infer(Input{Title: "   "}); got != "misc" {
		t.Errorf("Infer(whitespace) = %q, want misc", got)
	}
}

func TestInferCategories(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want string
	}{
		{"story heading", Input{Title: "chapter", ContentText: "# 第1話 旅立ち\n本文..."}, "story"},
		{"plot", Input{Title: "構成", ContentText: "プロット概要と物語構想"}, "plot"},
		{"timeline", Input{ContentText: "時系列の整理"}, "timeline"},
		{"index", Input{ContentText: "用語索引と地理索引"}, "index"},
		{"character", Input{ContentText: "主要人物と家族構成のメモ"}, "character"},
		{"rule", Input{ContentText: "基礎ルールと運用メモ"}, "rule"},
		{"world", Input{ContentText: "世界設定と魔法体系"}, "world"},
		{"no hit", Input{ContentText: "本日は晴天なり"}, "misc"},
	}
	for _, c := range cases {
		if got := Infer(c.in); got != c.want {
			t.Errorf("%s: Infer = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestInferFullWidthSpace guards the `\s` -> `[\s\p{Z}]` parity fix: a heading
// with full-width (U+3000) spaces must still classify as story, which plain RE2
// `\s` (ASCII-only) would miss.
func TestInferFullWidthSpace(t *testing.T) {
	if got := Infer(Input{Title: "第　1　話"}); got != "story" {
		t.Errorf("Infer(full-width spaced heading) = %q, want story", got)
	}
}
