package util

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestNowISOFormat(t *testing.T) {
	// Matches JS new Date().toISOString(): UTC, millisecond precision, "Z" suffix.
	want := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)
	got := NowISO()
	if !want.MatchString(got) {
		t.Errorf("NowISO() = %q, does not match ISO-8601 millisecond UTC form", got)
	}
}

func TestNewID(t *testing.T) {
	id := NewID("project")
	if !strings.HasPrefix(id, "project_") {
		t.Errorf("NewID prefix = %q, want project_ prefix", id)
	}
	// UUID portion should be 36 chars (8-4-4-4-12).
	if uuidPart := strings.TrimPrefix(id, "project_"); len(uuidPart) != 36 {
		t.Errorf("uuid part = %q (len %d), want 36", uuidPart, len(uuidPart))
	}
	if NewID("x") == NewID("x") {
		t.Error("NewID returned duplicate IDs")
	}
}

func TestParseTags(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ,, c ", []string{"a", "b", "c"}},
		{"単一", []string{"単一"}},
		{",,,", []string{}},
	}
	for _, c := range cases {
		got := ParseTags(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseTags(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}
