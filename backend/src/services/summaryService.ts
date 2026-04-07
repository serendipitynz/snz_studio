import { Message } from "../lib/types.js";
import { truncate } from "../lib/utils.js";

export class SummaryService {
  updateSummary(existingSummary: string, recentMessages: Message[]) {
    const condensedMessages = recentMessages
      .slice(-10)
      .map((message) => `${message.role}: ${truncate(message.content.replace(/\s+/g, " ").trim(), 220)}`)
      .join("\n");

    const sections = [
      existingSummary ? `Previous summary:\n${truncate(existingSummary, 700)}` : "",
      condensedMessages ? `Recent turns:\n${condensedMessages}` : ""
    ].filter(Boolean);

    return truncate(sections.join("\n\n"), 1800);
  }
}
