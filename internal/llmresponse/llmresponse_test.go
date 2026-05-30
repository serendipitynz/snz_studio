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
