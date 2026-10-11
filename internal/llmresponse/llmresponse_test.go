package llmresponse

import "testing"

func TestParseAssistantResponseStandard(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"plain", "Hello world", "Hello world"},
		{"trim", "  spaced answer  ", "spaced answer"},
		{
			"full final markers",
			"<|start|>assistant<|channel|>final<|message|>Answer<|end|>",
			"Answer",
		},
		{
			// Standard mode merely strips the tags, so analysis text survives
			// concatenated to the final text — matching the TS behaviour.
			"analysis and final stripped",
			"<|channel|>analysis<|message|>thinking<|channel|>final<|message|>Answer",
			"thinkingAnswer",
		},
		{
			"bare message tag",
			"<|message|>Just the answer<|end|>",
			"Just the answer",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseAssistantResponse(tc.raw, FormatStandard); got != tc.want {
				t.Fatalf("ParseAssistantResponse(%q, standard) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestParseAssistantResponseLLMJPThinking(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			"final section without start prefix",
			"<|channel|>analysis<|message|>think hard<|channel|>final<|message|>The answer",
			"The answer",
		},
		{
			"full start final section",
			"<|start|>assistant<|channel|>final<|message|>Final answer<|end|>",
			"Final answer",
		},
		{
			// Whitespace is allowed before "final" (matched by \s*) but not between
			// "final" and "<|message|>", mirroring the TS final-section regex.
			"channel final with leading whitespace",
			"<|channel|>\nfinal<|message|>Spaced final<|end|>",
			"Spaced final",
		},
		{
			// Tagged input but no final section is treated as analysis-only and
			// dropped entirely.
			"analysis only returns empty",
			"<|channel|>analysis<|message|>only thinking here",
			"",
		},
		{
			"user asks prefix returns empty",
			"The user asks: what is the answer?",
			"",
		},
		{
			"plain tag-free text passes through",
			"Just a normal answer.",
			"Just a normal answer.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseAssistantResponse(tc.raw, FormatLLMJPThinking); got != tc.want {
				t.Fatalf("ParseAssistantResponse(%q, llm_jp_thinking) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestSanitizePromptContent(t *testing.T) {
	if got := SanitizePromptContent("   ", "user", FormatStandard); got != "" {
		t.Fatalf("blank input should sanitize to empty, got %q", got)
	}

	// In standard mode every role is run through cleanupStandardContent.
	if got := SanitizePromptContent("<|message|>hi<|end|>", "user", FormatStandard); got != "hi" {
		t.Fatalf("standard user sanitize = %q, want %q", got, "hi")
	}

	// In llm_jp_thinking mode an assistant message is parsed to its final answer.
	raw := "<|channel|>analysis<|message|>reasoning<|channel|>final<|message|>Clean answer"
	if got := SanitizePromptContent(raw, "assistant", FormatLLMJPThinking); got != "Clean answer" {
		t.Fatalf("llm_jp assistant sanitize = %q, want %q", got, "Clean answer")
	}

	// A non-assistant role in llm_jp_thinking mode falls back to standard cleanup.
	if got := SanitizePromptContent("<|message|>question<|end|>", "user", FormatLLMJPThinking); got != "question" {
		t.Fatalf("llm_jp user sanitize = %q, want %q", got, "question")
	}
}

func TestSplitThinking(t *testing.T) {
	cases := []struct {
		name         string
		raw          string
		wantThinking string
		wantRest     string
	}{
		{"no thinking", "Just the answer", "", "Just the answer"},
		{"closed block", "<think>weigh it</think>\n\nThe answer", "weigh it", "\n\nThe answer"},
		{"leading space before the tag", "\n <think>hmm</think>ok", "hmm", "ok"},
		{"opening tag still arriving", "<thi", "", ""},
		{"thinking still open", "<think>part of a thought", "part of a thought", ""},
		{"closing tag still arriving", "<think>a thought</thi", "a thought", ""},
		{"close without open is a quoted tag", "The closing tag is `</think>`.", "", "The closing tag is `</think>`."},
		{"both tags mentioned mid-answer", "Wrap it in <think>x</think> tags.", "", "Wrap it in <think>x</think> tags."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			thinking, rest := SplitThinking(tc.raw)
			if thinking != tc.wantThinking || rest != tc.wantRest {
				t.Fatalf("SplitThinking(%q) = (%q, %q), want (%q, %q)", tc.raw, thinking, rest, tc.wantThinking, tc.wantRest)
			}
		})
	}
}

func TestSanitizePromptContentDropsThinking(t *testing.T) {
	raw := "<think>private reasoning</think>Visible answer"
	for _, format := range []string{FormatStandard, FormatLLMJPThinking} {
		if got := SanitizePromptContent(raw, "assistant", format); got != "Visible answer" {
			t.Fatalf("assistant sanitize (%s) = %q, want %q", format, got, "Visible answer")
		}
	}
	quoted := "Close the block with `</think>`."
	if got := SanitizePromptContent(quoted, "assistant", FormatStandard); got != quoted {
		t.Fatalf("assistant sanitize of a quoted closing tag = %q, want it unchanged", got)
	}
	// A user who types the tags is quoting them, not thinking.
	if got := SanitizePromptContent(raw, "user", FormatStandard); got != raw {
		t.Fatalf("user sanitize = %q, want it unchanged", got)
	}
}
