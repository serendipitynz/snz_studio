import { config } from "../config.js";
import { ChatRepository } from "../repositories/chatRepository.js";
import { ContextService } from "./contextService.js";
import { LlmClient } from "./llmClient.js";

export class ReviewService {
  constructor(
    private readonly chats: ChatRepository,
    private readonly context: ContextService,
    private readonly llm: LlmClient
  ) {}

  async reviewMessage(messageId: string) {
    const prepared = await this.prepareReview(messageId);

    const review = await this.llm.createChatCompletion({
      systemPrompt: prepared.systemPrompt,
      messages: [],
      userInput: prepared.userInput,
      temperature: 0.15,
      target: prepared.target
    });

    return {
      review: review.content,
      references: prepared.references
    };
  }

  async reviewMessageStream(messageId: string, onDelta: (chunk: string) => void) {
    const prepared = await this.prepareReview(messageId);

    const review = await this.llm.createChatCompletionStream({
      systemPrompt: prepared.systemPrompt,
      messages: [],
      userInput: prepared.userInput,
      temperature: 0.15,
      target: prepared.target,
      onDelta
    });

    return {
      review: (await review).content,
      references: prepared.references
    };
  }

  private async prepareReview(messageId: string) {
    const message = this.chats.getMessage(messageId);
    if (!message) {
      throw new Error("Message not found");
    }

    if (message.role !== "assistant") {
      throw new Error("Only assistant messages can be reviewed");
    }

    const assembled = await this.context.assemble(message.chatId, message.content);
    const target = {
      baseUrl: config.reviewBaseUrl || config.llmBaseUrl,
      model: config.reviewModel || config.llmModel
    };

    return {
      systemPrompt: [
        "あなたは創作文レビュー専用の編集者です。",
        "与えられた文章を書き直さず、レビューだけを返してください。",
        "日本語で、短く具体的に指摘してください。",
        "特に次を見てください: 設定整合、人物の一貫性、時系列、用語ぶれ、文体の不自然さ、冗長さ、説明過多。",
        "大きな問題がなければ、その旨を述べたうえで軽い改善提案だけを返してください。",
        "出力は markdown で、`Overall`、`Issues`、`Suggestions` の 3 セクションにしてください。",
        assembled.promptContext
      ].join("\n\n"),
      userInput: `Review this draft:\n\n${message.content}`,
      target,
      references: assembled.references
    };
  }
}
