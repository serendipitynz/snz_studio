import { config } from "../config.js";
import { MessageRole } from "./types.js";

function cleanupStandardContent(input: string) {
  return input
    .replace(/<\|start\|>assistant<\|channel\|>final<\|message\|>/giu, "")
    .replace(/<\|start\|>assistant/giu, "")
    .replace(/<\|channel\|>\s*(?:analysis|final)\s*<\|message\|>/giu, "")
    .replace(/<\|message\|>/giu, "")
    .replace(/<\|end\|>/giu, "")
    .trim();
}

function extractTaggedSection(input: string, startPattern: RegExp) {
  const match = startPattern.exec(input);
  if (!match) {
    return null;
  }

  const sectionStart = match.index + match[0].length;
  const remaining = input.slice(sectionStart);
  const nextTagIndex = remaining.search(/<\|(?:start|channel|message|end)\|>/iu);
  const content = nextTagIndex >= 0 ? remaining.slice(0, nextTagIndex) : remaining;
  return content.trim();
}

function cleanupLlmpJThinkingContent(raw: string) {
  const finalSection =
    extractTaggedSection(raw, /<\|start\|>assistant<\|channel\|>final<\|message\|>/iu) ??
    extractTaggedSection(raw, /<\|channel\|>\s*final<\|message\|>/iu);

  if (finalSection) {
    return cleanupStandardContent(finalSection);
  }

  if (/<\|(?:start|channel|message|end)\|>/iu.test(raw) || /^The ?user asks:/iu.test(raw.trim())) {
    return "";
  }

  const noAnalysis = raw
    .replace(/<\|channel\|>\s*analysis<\|message\|>[\s\S]*?(?=<\|start\|>assistant<\|channel\|>final<\|message\|>|<\|channel\|>\s*final<\|message\|>|$)/giu, "")
    .replace(/<\|start\|>assistant/giu, "")
    .replace(/<\|channel\|>\s*analysis/giu, "")
    .replace(/<\|channel\|>\s*final/giu, "")
    .replace(/<\|message\|>/giu, "")
    .replace(/<\|end\|>/giu, "");

  return noAnalysis.trim();
}

export function parseAssistantResponse(raw: string) {
  if (config.llmResponseFormat === "llm_jp_thinking") {
    return cleanupLlmpJThinkingContent(raw);
  }

  return cleanupStandardContent(raw);
}

export function sanitizePromptContent(raw: string, role: MessageRole | "system") {
  const normalized = raw.trim();
  if (!normalized) {
    return "";
  }

  if (config.llmResponseFormat === "llm_jp_thinking") {
    if (role === "assistant") {
      const parsed = parseAssistantResponse(normalized);
      if (parsed) {
        return parsed;
      }
    }

    return cleanupStandardContent(normalized);
  }

  return cleanupStandardContent(normalized);
}
