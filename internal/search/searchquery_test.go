package search

import (
	"slices"
	"testing"
)

const guildRequest = "第3章で、主人公がギルドに入るまでの経緯と、その時に出会った仲間の名前と性格をまとめてください。あと、ギルドマスターの過去についても触れておいて。"

// devCorpusDocFreq is the prefix document frequency of guildRequest's terms in the
// development database's 212 chunks, measured on 2026-10-11 the way the
// retrieval service reads it (fts5vocab rows whose term starts with the query
// term, doc counts summed). The guild words are absent from that corpus.
var devCorpusDocFreq = map[string]int{
	"第": 150, "章": 72, "主人": 16, "公": 22, "ギルド": 0, "入る": 17, "まで": 92,
	"経緯": 2, "時": 150, "出会っ": 0, "仲間": 9, "名前": 16, "性格": 15, "まとめて": 5,
	"ださい": 15, "あと": 41, "ギルドマスター": 0, "過去": 9, "について": 32, "触れ": 31, "おい": 5,
}

const devCorpusRows = 212

func devCorpusFreq(term string) int { return devCorpusDocFreq[term] }

func TestQueryTermsDropsStopwordSplitBySegmenter(t *testing.T) {
	terms := QueryTerms(guildRequest)
	if slices.Contains(terms, "ださい") {
		t.Fatalf("QueryTerms kept the 「ください」 fragment: %v", terms)
	}
	for _, want := range []string{"性格", "過去", "ギルドマスター"} {
		if !slices.Contains(terms, want) {
			t.Fatalf("QueryTerms lost %q: %v", want, terms)
		}
	}
}

// TestSelectTermsPrefersRareTermsOverEarlyOnes: 「第」 and 「時」 are in more than
// half the rows, 「まで」 is among the commonest terms left past the limit, and
// 「ださい」 is the fragment of 「ください」; the old positional pick kept all four
// and lost 「性格」 and 「過去」.
func TestSelectTermsPrefersRareTermsOverEarlyOnes(t *testing.T) {
	selected := SelectTerms(QueryTerms(guildRequest), nil, devCorpusFreq, devCorpusRows)
	for _, want := range []string{"性格", "過去", "経緯", "仲間"} {
		if !slices.Contains(selected, want) {
			t.Errorf("selection lacks the rare term %q: %v", want, selected)
		}
	}
	for _, unwanted := range []string{"第", "時", "まで", "ださい"} {
		if slices.Contains(selected, unwanted) {
			t.Errorf("selection kept %q: %v", unwanted, selected)
		}
	}
	// The terms the corpus lacks ride outside the limit: they cannot pull a row in.
	for _, absent := range []string{"ギルド", "ギルドマスター", "出会っ"} {
		if !slices.Contains(selected, absent) {
			t.Errorf("selection dropped the absent term %q: %v", absent, selected)
		}
	}
	if present := len(selected) - 3; present != maxQueryTerms {
		t.Errorf("selected %d matching terms, want the limit %d: %v", present, maxQueryTerms, selected)
	}
}

func TestSelectTermsIgnoresWordOrder(t *testing.T) {
	reordered := "あと、ギルドマスターの過去についても触れておいて。主人公がギルドに入るまでの経緯と、その時に出会った仲間の名前と性格を、第3章でまとめてください。"
	want := FtsQuery(SelectTerms(QueryTerms(guildRequest), nil, devCorpusFreq, devCorpusRows))
	got := FtsQuery(SelectTerms(QueryTerms(reordered), nil, devCorpusFreq, devCorpusRows))
	if got != want {
		t.Fatalf("reordering the words changed the query\n  got  %s\n  want %s", got, want)
	}
}

func TestSelectTermsFillsBackgroundAfterPrimary(t *testing.T) {
	freq := map[string]int{"港": 40, "灯台": 30, "嵐": 25, "王都": 1, "砦": 2}
	primary := []string{"港", "灯台", "嵐"}
	background := []string{"王都", "砦", "港"}
	selected := SelectTerms(primary, background, func(term string) int { return freq[term] }, 1000)
	want := []string{"嵐", "灯台", "港", "王都", "砦"}
	if !slices.Equal(selected, want) {
		t.Fatalf("selection = %v, want primary by rarity, then background: %v", selected, want)
	}

	many := make([]string, maxQueryTerms)
	for i := range many {
		many[i] = string(rune('あ'+i)) + "語"
		freq[many[i]] = 5
	}
	selected = SelectTerms(many, background, func(term string) int { return freq[term] }, 1000)
	if slices.Contains(selected, "王都") {
		t.Fatalf("a background term took a slot primary terms filled: %v", selected)
	}
}

func TestSelectTermsKeepsCommonTermsWhenNothingElseMatches(t *testing.T) {
	freq := map[string]int{"第": 150, "時": 150}
	selected := SelectTerms([]string{"時", "第"}, nil, func(term string) int { return freq[term] }, devCorpusRows)
	if !slices.Equal(selected, []string{"時", "第"}) {
		t.Fatalf("selection = %v, want the common terms rather than an empty query", selected)
	}
}

func TestSelectTermsKeepsCommonTermsInSmallTables(t *testing.T) {
	freq := map[string]int{"主人公": 4, "剣": 1}
	selected := SelectTerms([]string{"主人公", "剣"}, nil, func(term string) int { return freq[term] }, 7)
	if !slices.Equal(selected, []string{"剣", "主人公"}) {
		t.Fatalf("selection = %v, want both terms of a 7-row table", selected)
	}
}

func TestFtsQueryEscapesAndPrefixMatches(t *testing.T) {
	if got := FtsQuery([]string{"竜", `a"b`}); got != `"竜"* OR "a""b"*` {
		t.Fatalf("FtsQuery = %s", got)
	}
	if got := FtsQuery(nil); got != "" {
		t.Fatalf("FtsQuery(nil) = %q, want empty", got)
	}
}
