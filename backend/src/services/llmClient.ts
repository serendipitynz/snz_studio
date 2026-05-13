import { config } from "../config.js";
import { parseAssistantResponse, sanitizePromptContent } from "../lib/llmResponse.js";
import { Message } from "../lib/types.js";

function createHeaders() {
  const headers: Record<string, string> = {};
  if (config.llmApiKey) {
    headers.Authorization = `Bearer ${config.llmApiKey}`;
  }
  return headers;
}

type CompletionTarget = {
  baseUrl?: string;
  model?: string;
};

function getLmStudioApiRoot(baseUrl: string) {
  return baseUrl.replace(/\/$/, "").replace(/(\/api)?\/v1$/i, "");
}

function estimateTokenCount(input: string) {
  const normalized = input.trim();
  if (!normalized) {
    return 0;
  }

  const wordLike = normalized.match(/[\p{L}\p{N}_-]+/gu)?.length ?? 0;
  const japaneseChars = normalized.match(/[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}ー]/gu)?.length ?? 0;
  return Math.max(wordLike, Math.ceil(japaneseChars / 1.8), Math.ceil(normalized.length / 4));
}

function buildGenerationMetrics(content: string, elapsedMs: number, outputTokens?: number | null) {
  const safeElapsedMs = Math.max(1, Math.round(elapsedMs));
  const tokens = Math.max(0, outputTokens ?? estimateTokenCount(content));
  return {
    responseMs: safeElapsedMs,
    outputTokens: tokens,
    tokensPerSecond: tokens > 0 ? Number((tokens / (safeElapsedMs / 1000)).toFixed(2)) : 0
  };
}

function isAbortError(error: unknown) {
  if (error instanceof DOMException) {
    return error.name === "AbortError";
  }

  if (error instanceof Error) {
    return error.name === "AbortError" || /aborted/i.test(error.message);
  }

  return false;
}

export interface ChatCompletionResult {
  content: string;
  responseMs: number;
  outputTokens: number;
  tokensPerSecond: number;
  modelName: string;
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

  async checkConnection(baseUrl = config.llmBaseUrl, model = config.llmModel) {
    if (!model.trim()) {
      return false;
    }

    try {
      const models = await this.listModels(baseUrl);
      return !models.length || models.includes(model);
    } catch {
      return false;
    }
  }

  async createChatCompletion(input: {
    systemPrompt: string;
    messages: Message[];
    userInput: string;
    temperature?: number;
    target?: CompletionTarget;
  }) {
    const startedAt = performance.now();
    const model = input.target?.model?.trim() || config.llmModel;
    const baseUrl = input.target?.baseUrl?.trim() || config.llmBaseUrl;
    const body = {
      model,
      temperature: input.temperature ?? 0.25,
      messages: [
        { role: "system", content: sanitizePromptContent(input.systemPrompt, "system") },
        ...input.messages.map((message) => ({
          role: message.role,
          content: sanitizePromptContent(message.content, message.role)
        })),
        { role: "user", content: sanitizePromptContent(input.userInput, "user") }
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
      const response = await fetch(`${baseUrl.replace(/\/$/, "")}/chat/completions`, {
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
        usage?: { completion_tokens?: number };
      };

      const rawContent = data.choices?.[0]?.message?.content?.trim();
      const content = rawContent ? parseAssistantResponse(rawContent) : "";
      if (!content) {
        throw new Error("LLM response did not contain message content");
      }

      return {
        content,
        ...buildGenerationMetrics(content, performance.now() - startedAt, data.usage?.completion_tokens),
        modelName: model
      };
    } catch (error) {
      if (isAbortError(error)) {
        throw new Error(`LLM request timed out after ${config.llmTimeoutMs} ms`);
      }

      throw error;
    } finally {
      clearTimeout(timeout);
    }
  }

  async createChatCompletionStream(input: {
    systemPrompt: string;
    messages: Message[];
    userInput: string;
    onDelta: (chunk: string) => void;
    temperature?: number;
    target?: CompletionTarget;
  }) {
    const startedAt = performance.now();
    const model = input.target?.model?.trim() || config.llmModel;
    const baseUrl = input.target?.baseUrl?.trim() || config.llmBaseUrl;
    const body = {
      model,
      temperature: input.temperature ?? 0.25,
      stream: true,
      stream_options: {
        include_usage: true
      },
      messages: [
        { role: "system", content: sanitizePromptContent(input.systemPrompt, "system") },
        ...input.messages.map((message) => ({
          role: message.role,
          content: sanitizePromptContent(message.content, message.role)
        })),
        { role: "user", content: sanitizePromptContent(input.userInput, "user") }
      ]
    };

    const headers: Record<string, string> = {
      "Content-Type": "application/json"
    };

    if (config.llmApiKey) {
      headers.Authorization = `Bearer ${config.llmApiKey}`;
    }

    const controller = new AbortController();
    let timeout = setTimeout(() => controller.abort(), config.llmTimeoutMs);
    const resetTimeout = () => {
      clearTimeout(timeout);
      timeout = setTimeout(() => controller.abort(), config.llmTimeoutMs);
    };

    try {
      resetTimeout();
      const response = await fetch(`${baseUrl.replace(/\/$/, "")}/chat/completions`, {
        method: "POST",
        headers,
        body: JSON.stringify(body),
        signal: controller.signal
      });

      if (!response.ok) {
        throw new Error(`LLM request failed with ${response.status}`);
      }

      if (!response.body) {
        throw new Error("LLM stream did not include a response body");
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      let rawContent = "";
      let visibleContent = "";
      let completionTokens: number | null = null;

      const handleChunk = (chunk: string) => {
        const lines = chunk.split(/\r?\n/);
        const dataLines = lines
          .filter((line) => line.startsWith("data:"))
          .map((line) => line.slice(5).trimStart())
          .filter(Boolean);

        if (!dataLines.length) {
          return;
        }

        const dataText = dataLines.join("\n");
        if (dataText === "[DONE]") {
          return;
        }

        const payload = JSON.parse(dataText) as {
          choices?: Array<{ delta?: { content?: string } }>;
          usage?: { completion_tokens?: number };
        };

        if (payload.usage?.completion_tokens != null) {
          completionTokens = payload.usage.completion_tokens;
        }

        const delta = payload.choices?.[0]?.delta?.content ?? "";
        if (!delta) {
          return;
        }

        rawContent += delta;
        const nextVisibleContent = parseAssistantResponse(rawContent);
        const visibleDelta = nextVisibleContent.slice(visibleContent.length);
        visibleContent = nextVisibleContent;
        if (visibleDelta) {
          input.onDelta(visibleDelta);
        }
      };

      while (true) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }

        resetTimeout();

        buffer += decoder.decode(value, { stream: true });

        let separatorIndex = buffer.indexOf("\n\n");
        while (separatorIndex >= 0) {
          const rawChunk = buffer.slice(0, separatorIndex).trim();
          buffer = buffer.slice(separatorIndex + 2);
          if (rawChunk) {
            handleChunk(rawChunk);
          }
          separatorIndex = buffer.indexOf("\n\n");
        }
      }

      buffer += decoder.decode();
      if (buffer.trim()) {
        handleChunk(buffer.trim());
      }

      const content = parseAssistantResponse(rawContent);
      if (!content.trim()) {
        throw new Error("LLM stream did not contain message content");
      }

      return {
        content,
        ...buildGenerationMetrics(content, performance.now() - startedAt, completionTokens),
        modelName: model
      };
    } catch (error) {
      if (isAbortError(error)) {
        throw new Error(`LLM stream timed out after ${config.llmTimeoutMs} ms without receiving data`);
      }

      throw error;
    } finally {
      clearTimeout(timeout);
    }
  }
}
