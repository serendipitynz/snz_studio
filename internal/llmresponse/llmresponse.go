// Package llmresponse ports backend/src/lib/llmResponse.ts. It strips the channel
// markup some local models (notably the llm_jp_thinking format) emit, keeping only
// the user-facing final answer.
//
// Unlike the TS module, which read config.llmResponseFormat directly, these
// functions take the format as a parameter. That keeps the package a pure,
// dependency-free leaf (no import of config) and trivially unit-testable without a
// live LLM endpoint — exactly the "reasoning 除去は endpoint 無しで単体テスト" goal.
//
// Parity note on whitespace: JS `\s` under the `u` flag matches Unicode whitespace,
// whereas Go's `\s` is ASCII-only. The `\s*` occurrences between channel markers
// are widened to `[\s\p{Z}]*` to match, following the same correction made in
// internal/doccategory.
package llmresponse

import (
	"regexp"
	"strings"
	"unicode"
)

// Response format identifiers, mirroring EditableAppConfiguration["llmResponseFormat"].
const (
	FormatStandard      = "standard"
	FormatLLMJPThinking = "llm_jp_thinking"
)

var (
	// cleanupStandardContent replacements, applied in order.
	reStartAssistantFinal = regexp.MustCompile(`(?i)<\|start\|>assistant<\|channel\|>final<\|message\|>`)
	reStartAssistant      = regexp.MustCompile(`(?i)<\|start\|>assistant`)
	reChannelAnalysisFin  = regexp.MustCompile(`(?i)<\|channel\|>[\s\p{Z}]*(?:analysis|final)[\s\p{Z}]*<\|message\|>`)
	reMessage             = regexp.MustCompile(`(?i)<\|message\|>`)
	reEnd                 = regexp.MustCompile(`(?i)<\|end\|>`)

	// reAnyTag matches any tag boundary that ends a tagged section.
	reAnyTag = regexp.MustCompile(`(?i)<\|(?:start|channel|message|end)\|>`)

	// llm_jp_thinking final-section start patterns.
	reFinalSectionFull = regexp.MustCompile(`(?i)<\|start\|>assistant<\|channel\|>final<\|message\|>`)
	reFinalSection     = regexp.MustCompile(`(?i)<\|channel\|>[\s\p{Z}]*final<\|message\|>`)

	// detection patterns used when no explicit final section is present.
	reUserAsksPrefix = regexp.MustCompile(`(?i)^The ?user asks:`)

	// noAnalysis cleanup replacements. RE2 has no lookahead, so reAnalysisBlock
	// drops the analysis block through end-of-string rather than up to the next
	// final marker. This is faithful in practice: this branch is only reached when
	// raw contains no channel tags at all (the reAnyTag guard above returns "" for
	// any tagged input), so none of these tag patterns actually match here.
	reAnalysisBlock   = regexp.MustCompile(`(?is)<\|channel\|>[\s\p{Z}]*analysis<\|message\|>.*$`)
	reStartAssistant2 = regexp.MustCompile(`(?i)<\|start\|>assistant`)
	reChannelAnalysis = regexp.MustCompile(`(?i)<\|channel\|>[\s\p{Z}]*analysis`)
	reChannelFinal    = regexp.MustCompile(`(?i)<\|channel\|>[\s\p{Z}]*final`)
	reMessage2        = regexp.MustCompile(`(?i)<\|message\|>`)
	reEnd2            = regexp.MustCompile(`(?i)<\|end\|>`)
)

func cleanupStandardContent(input string) string {
	out := reStartAssistantFinal.ReplaceAllString(input, "")
	out = reStartAssistant.ReplaceAllString(out, "")
	out = reChannelAnalysisFin.ReplaceAllString(out, "")
	out = reMessage.ReplaceAllString(out, "")
	out = reEnd.ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}

// extractTaggedSection returns the trimmed text following the first match of
// startPattern, up to the next tag boundary (or end of input). Returns ("", false)
// when startPattern does not match. Mirrors extractTaggedSection.
func extractTaggedSection(input string, startPattern *regexp.Regexp) (string, bool) {
	loc := startPattern.FindStringIndex(input)
	if loc == nil {
		return "", false
	}
	remaining := input[loc[1]:]
	if next := reAnyTag.FindStringIndex(remaining); next != nil {
		return strings.TrimSpace(remaining[:next[0]]), true
	}
	return strings.TrimSpace(remaining), true
}

func cleanupLLMJPThinkingContent(raw string) string {
	if section, ok := extractTaggedSection(raw, reFinalSectionFull); ok {
		return cleanupStandardContent(section)
	}
	if section, ok := extractTaggedSection(raw, reFinalSection); ok {
		return cleanupStandardContent(section)
	}

	if reAnyTag.MatchString(raw) || reUserAsksPrefix.MatchString(strings.TrimSpace(raw)) {
		return ""
	}

	noAnalysis := reAnalysisBlock.ReplaceAllString(raw, "")
	noAnalysis = reStartAssistant2.ReplaceAllString(noAnalysis, "")
	noAnalysis = reChannelAnalysis.ReplaceAllString(noAnalysis, "")
	noAnalysis = reChannelFinal.ReplaceAllString(noAnalysis, "")
	noAnalysis = reMessage2.ReplaceAllString(noAnalysis, "")
	noAnalysis = reEnd2.ReplaceAllString(noAnalysis, "")
	return strings.TrimSpace(noAnalysis)
}

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// SplitThinking separates the reasoning a model wrote into its answer, between
// <think> and </think>, from the rest. It is for an endpoint or model that does
// not send reasoning in a field of its own.
//
// Only a section the response opens with counts. raw may be a stream cut at any
// point, so an opening tag still arriving yields nothing, and a closing tag still
// arriving is kept out of the thinking. A </think> with no opening tag is left in
// the answer: a template that writes <think> into the prompt would produce one,
// but so does an answer that quotes the tag, and nothing in the response tells
// the two apart.
func SplitThinking(raw string) (thinking, rest string) {
	trimmed := strings.TrimLeftFunc(raw, unicode.IsSpace)
	if trimmed != "" && len(trimmed) < len(thinkOpen) && strings.HasPrefix(thinkOpen, trimmed) {
		return "", ""
	}
	if after, ok := strings.CutPrefix(trimmed, thinkOpen); ok {
		if before, rest, closed := strings.Cut(after, thinkClose); closed {
			return strings.TrimSpace(before), rest
		}
		return strings.TrimSpace(withoutPartialSuffix(after, thinkClose)), ""
	}
	return "", raw
}

// withoutPartialSuffix drops a trailing proper prefix of tag from s.
func withoutPartialSuffix(s, tag string) string {
	for n := len(tag) - 1; n > 0; n-- {
		if strings.HasSuffix(s, tag[:n]) {
			return s[:len(s)-n]
		}
	}
	return s
}

// ParseAssistantResponse returns the user-facing content of a raw assistant
// response, stripping channel markup. Mirrors parseAssistantResponse(raw).
func ParseAssistantResponse(raw, format string) string {
	if format == FormatLLMJPThinking {
		return cleanupLLMJPThinkingContent(raw)
	}
	return cleanupStandardContent(raw)
}

// SanitizePromptContent cleans a message before it is sent back to the model.
// Mirrors sanitizePromptContent(raw, role). An assistant message loses any
// <think> section: a reply stored before thinking was split out may still hold
// one, and the model's reasoning is not fed back as history.
func SanitizePromptContent(raw, role, format string) string {
	normalized := strings.TrimSpace(raw)
	if role == "assistant" {
		_, rest := SplitThinking(normalized)
		normalized = strings.TrimSpace(rest)
	}
	if normalized == "" {
		return ""
	}
	if format == FormatLLMJPThinking {
		if role == "assistant" {
			if parsed := ParseAssistantResponse(normalized, format); parsed != "" {
				return parsed
			}
		}
		return cleanupStandardContent(normalized)
	}
	return cleanupStandardContent(normalized)
}
