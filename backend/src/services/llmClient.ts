import { config } from "../config.js";
import { Message } from "../lib/types.js";

function createHeaders() {
  const headers: Record<string, string> = {};
  if (config.llmApiKey) {
    headers.Authorization = `Bearer ${config.llmApiKey}`;
  }
  return headers;
}

function getLmStudioApiRoot(baseUrl: string) {
  return baseUrl.replace(/\/$/, "").replace(/(\/api)?\/v1$/i, "");
}

export class LlmClient {
  async listModels(baseUrl = config.llmBaseUrl) {
    const headers = createHeaders();

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), Math.min(config.llmTimeoutMs, 5000));

    try {
      const response = await fetch(`${baseUrl.replace(/\/$/, "")}/models`, {
        method: "GET",
        headers,
        signal: controller.signal
      });

      if (!response.ok) {
        throw new Error(`LLM model list request failed with ${response.status}`);
      }

      const data = (await response.json()) as {
        data?: Array<{ id?: string }>;
      };

      return (data.data?.map((item) => item.id).filter((item): item is string => Boolean(item)) ?? []).sort();
    } finally {
      clearTimeout(timeout);
    }
  }

  async listAvailableModels(baseUrl = config.llmBaseUrl) {
    const headers = createHeaders();
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), Math.min(config.llmTimeoutMs, 5000));

    try {
      const response = await fetch(`${getLmStudioApiRoot(baseUrl)}/api/v1/models`, {
        method: "GET",
        headers,
        signal: controller.signal
      });

      if (!response.ok) {
        throw new Error(`LLM available model request failed with ${response.status}`);
      }

      const data = (await response.json()) as {
        models?: Array<{ type?: string; key?: string }>;
      };

      return (data.models ?? [])
        .filter((item) => item.type === "llm" && item.key)
        .map((item) => String(item.key))
        .sort();
    } finally {
      clearTimeout(timeout);
    }
  }

  async ensureModelLoaded(model: string, baseUrl = config.llmBaseUrl) {
    if (!model.trim()) {
      return false;
    }

    const headers = {
      ...createHeaders(),
      "Content-Type": "application/json"
    };

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), Math.min(config.llmTimeoutMs, 20000));

    try {
      const root = getLmStudioApiRoot(baseUrl);
      const listResponse = await fetch(`${root}/api/v1/models`, {
        method: "GET",
        headers: createHeaders(),
        signal: controller.signal
      });

      if (!listResponse.ok) {
        return false;
      }

      const data = (await listResponse.json()) as {
        models?: Array<{ type?: string; key?: string; loaded_instances?: Array<{ id?: string }> }>;
      };

      const modelEntry = (data.models ?? []).find((item) => item.type === "llm" && item.key === model);
      if (!modelEntry) {
        return false;
      }

      if (modelEntry.loaded_instances?.length) {
        return true;
      }

      const loadResponse = await fetch(`${root}/api/v1/models/load`, {
        method: "POST",
        headers,
        body: JSON.stringify({
          model
        }),
        signal: controller.signal
      });

      return loadResponse.ok;
    } catch {
      return false;
    } finally {
      clearTimeout(timeout);
    }
  }

  async checkConnection() {
    if (!config.llmModel.trim()) {
      return false;
    }

    try {
      const models = await this.listModels();
      return !models.length || models.includes(config.llmModel);
    } catch {
      return false;
    }
  }

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
