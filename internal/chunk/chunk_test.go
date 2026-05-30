package chunk

import "testing"

func TestDocumentEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t  \r\n"} {
		if got := Document(in); len(got) != 0 {
			t.Errorf("Document(%q) = %#v, want empty", in, got)
		}
	}
}

func TestDocumentShort(t *testing.T) {
	got := Document("  hello world  ")
	if len(got) != 1 {
		t.Fatalf("Document short = %#v, want one chunk", got)
	}
	if got[0] != "hello world" {
		t.Errorf("chunk = %q, want trimmed %q", got[0], "hello world")
	}
}

func TestDocumentCRLFNormalized(t *testing.T) {
	got := Document("line1\r\nline2")
	if len(got) != 1 || got[0] != "line1\nline2" {
		t.Errorf("Document CRLF = %#v, want [\"line1\\nline2\"]", got)
	}
}

// TestDocumentWindowing verifies the 1000-rune window with 150-rune overlap
// (step 850): a 2000-rune body yields windows at offsets 0, 850, 1700.
func TestDocumentWindowing(t *testing.T) {
	runes := make([]rune, 2000)
	for i := range runes {
		runes[i] = rune('a' + i%26)
	}
	chunks := Document(string(runes))

	if len(chunks) != 3 {
		t.Fatalf("len(chunks) = %d, want 3", len(chunks))
	}
	want := []string{
		string(runes[0:1000]),
		string(runes[850:1850]),
		string(runes[1700:2000]),
	}
	for i := range want {
		if chunks[i] != want[i] {
			t.Errorf("chunk[%d] mismatch (len got %d want %d)", i, len([]rune(chunks[i])), len([]rune(want[i])))
		}
	}
}
