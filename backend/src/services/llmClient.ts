import { config } from "../config.js";
import { Message } from "../lib/types.js";

export class LlmClient {
  async createChatCompletion(input: { systemPrompt: string; messages: Message[]; userInput: string }) {
    const body = {
      model: config.llmModel,
      temperature: 0.25,
      messages: [
        { role: "system", content: input.systemPrompt },
        ...input.messages.map((message) => ({
          role: message.role,
          content: message.content
        })),
        { role: "user", content: input.userInput }
      ]
    };

    const headers: Record<string, string> = {
      "Content-Type": "application/json"
    };

    if (config.llmApiKey) {
      headers.Authorization = `Bearer ${config.llmApiKey}`;
    }

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), config.llmTimeoutMs);

    try {
      console.log("Sending request to LLM with body:", JSON.stringify(body, null, 2));
      const response = await fetch(`${config.llmBaseUrl.replace(/\/$/, "")}/chat/completions`, {
        method: "POST",
        headers,
        body: JSON.stringify(body),
        signal: controller.signal
      });

      if (!response.ok) {
        throw new Error(`LLM request failed with ${response.status}`);
      }

      const data = (await response.json()) as {
        choices?: Array<{ message?: { content?: string } }>;
      };

      const content = data.choices?.[0]?.message?.content?.trim();
      if (!content) {
        throw new Error("LLM response did not contain message content");
      }

      return content;
    } finally {
      clearTimeout(timeout);
    }
  }
}
