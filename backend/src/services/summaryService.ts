import { Message } from "../lib/types.js";
import { truncate } from "../lib/utils.js";
import { LlmClient } from "./llmClient.js";

function normalizeMessageForSummary(message: Message) {
  const roleLabel =
    message.role === "user"
      ? "ユーザー"
      : message.role === "assistant"
        ? "アシスタント"
        : "システム";

  return `${roleLabel}: ${truncate(message.content.replace(/\s+/g, " ").trim(), 220)}`;
}

function buildFallbackSummary(existingSummary: string, recentMessages: Message[]) {
  const recentTurns = recentMessages
    .slice(-6)
    .map((message) => normalizeMessageForSummary(message))
    .join("\n");

  const sections = [
    existingSummary ? `これまでの要約:\n${truncate(existingSummary, 700)}` : "",
    recentTurns ? `直近のやり取り:\n${recentTurns}` : ""
  ].filter(Boolean);

  return truncate(sections.join("\n\n"), 1600);
}

export class SummaryService {
  constructor(private readonly llm: LlmClient) {}

  async updateSummary(existingSummary: string, recentMessages: Message[]) {
    const transcript = recentMessages.slice(-8).map((message) => normalizeMessageForSummary(message)).join("\n");
    if (!transcript.trim()) {
      return existingSummary;
    }

    const systemPrompt = [
      "あなたは会話要約専用のアシスタントです。",
      "会話全体の継続に必要な内容だけを自然な日本語で要約してください。",
      "雑談や冗長な表現は落とし、決定事項、未解決事項、重要な前提、直近の変更だけを残してください。",
      "箇条書きではなく、2から5文程度の短い自然文でまとめてください。"
    ].join("\n");

    const userInput = [
      existingSummary ? `既存の要約:\n${truncate(existingSummary, 1200)}` : "既存の要約はありません。",
      `直近の会話:\n${transcript}`,
      "上記を踏まえて、今後の会話継続に使える自然な日本語の要約を更新してください。"
    ].join("\n\n");

    try {
      const summary = await this.llm.createChatCompletion({
        systemPrompt,
        messages: [],
        userInput,
        temperature: 0.15
      });

      return truncate(summary.content.replace(/\n{3,}/g, "\n\n").trim(), 1600);
    } catch {
      return buildFallbackSummary(existingSummary, recentMessages);
    }
  }

  async generateChatTitle(recentMessages: Message[]) {
    const transcript = recentMessages
      .filter((message) => message.role !== "system")
      .slice(-6)
      .map((message) => normalizeMessageForSummary(message))
      .join("\n");

    if (!transcript.trim()) {
      return "";
    }

    try {
      const response = await this.llm.createChatCompletion({
        systemPrompt: [
          "あなたは会話タイトル生成専用のアシスタントです。",
          "会話の内容を表す短い日本語タイトルだけを返してください。",
          "長さは 8 文字から 28 文字程度。",
          "引用符、接頭辞、説明文、句点は付けないでください。"
        ].join("\n"),
        messages: [],
        userInput: `直近の会話:\n${transcript}`,
        temperature: 0.2
      });

      return truncate(
        response.content
          .replace(/^["'「『\s]+|["'」』\s]+$/g, "")
          .replace(/\n+/g, " ")
          .trim(),
        40
      );
    } catch {
      const firstUserMessage = recentMessages.find((message) => message.role === "user")?.content ?? "";
      return truncate(firstUserMessage.replace(/\s+/g, " ").trim(), 40);
    }
  }
}
