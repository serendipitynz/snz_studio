import { ChatRepository } from "../repositories/chatRepository.js";
import { ContextService } from "./contextService.js";
import { LlmClient } from "./llmClient.js";
import { MemoryService } from "./memoryService.js";
import { SummaryService } from "./summaryService.js";
import { truncate } from "../lib/utils.js";
import { EmbeddingSyncService } from "./embeddingSyncService.js";

export class ChatService {
  constructor(
    private readonly chats: ChatRepository,
    private readonly context: ContextService,
    private readonly llm: LlmClient,
    private readonly summary: SummaryService,
    private readonly memoryService: MemoryService,
    private readonly embeddingSync: EmbeddingSyncService
  ) {}

  async sendMessage(chatId: string, content: string) {
    const assembled = await this.context.assemble(chatId, content);
    const userMessage = this.chats.addMessage({
      chatId,
      role: "user",
      content
    });

    const createdMemories = this.memoryService.maybeStoreFromUserMessage({
      projectId: assembled.project.id,
      chatId,
      content
    });

    if (createdMemories.length) {
      await this.embeddingSync.syncMemories(createdMemories.map((memory) => memory.id));
    }

    const systemPrompt = [
      "You are a local project assistant.",
      "Use the provided project context when it is relevant, but avoid mentioning irrelevant references.",
      "Answer clearly and practically.",
      assembled.isQuoteRequest
        ? `The user is asking for document quotation${assembled.targetDocumentTitle ? ` from "${assembled.targetDocumentTitle}"` : ""}. Quote only from provided document material, preserve the original wording, and say clearly if the exact passage was not found.`
        : "",
      assembled.promptContext
    ]
      .filter(Boolean)
      .join("\n\n");

    let assistantContent: string;

    try {
      assistantContent = await this.llm.createChatCompletion({
        systemPrompt,
        messages: assembled.recentMessages,
        userInput: content,
        temperature: 0.25
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : "Unknown LLM error";
      assistantContent = [
        "Local LLM endpoint could not be reached, so this is a fallback response.",
        `Reason: ${message}`,
        assembled.references.length
          ? `Available references:\n${assembled.references
              .map((reference) => `- ${reference.label}: ${truncate(reference.excerpt, 160)}`)
              .join("\n")}`
          : "No project references were selected for this turn.",
        `User message: ${content}`
      ].join("\n\n");
    }

    const assistantMessage = this.chats.addMessage({
      chatId,
      role: "assistant",
      content: assistantContent
    });

    this.chats.replaceAssistantReferences(
      assistantMessage.id,
      assembled.references.map((reference) => ({
        sourceType: reference.sourceType,
        sourceId: reference.sourceId,
        label: reference.label,
        excerpt: reference.excerpt,
        score: reference.score
      }))
    );

    const updatedMessages = [...assembled.recentMessages, userMessage, assistantMessage];
    const updatedSummary = await this.summary.updateSummary(assembled.summary, updatedMessages);
    this.chats.upsertSummary(chatId, updatedSummary);

    return assistantMessage;
  }
}
