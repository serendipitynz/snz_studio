import { config } from "../config.js";

function debugInfo(message: string) {
  if (config.debugRetrieval) {
    console.info(message);
  }
}

function debugWarn(message: string) {
  if (config.debugRetrieval) {
    console.warn(message);
  }
}

function createHeaders() {
  const headers: Record<string, string> = {};
  if (config.embeddingApiKey) {
    headers.Authorization = `Bearer ${config.embeddingApiKey}`;
  }
  return headers;
}

function getLmStudioApiRoot(baseUrl: string) {
  return baseUrl.replace(/\/$/, "").replace(/(\/api)?\/v1$/i, "");
}

export class EmbeddingClient {
  private unavailableLogged = false;
  private disabled = !config.embeddingModel.trim();

  isEnabled() {
    return !this.disabled;
  }

  getModel() {
    return config.embeddingModel.trim();
  }

  refreshConfiguration() {
    this.disabled = !config.embeddingModel.trim();
    this.unavailableLogged = false;
  }

  async listModels(baseUrl = config.embeddingBaseUrl) {
    const headers = createHeaders();

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), Math.min(config.embeddingTimeoutMs, 5000));

    try {
      const response = await fetch(`${baseUrl.replace(/\/$/, "")}/models`, {
        method: "GET",
        headers,
        signal: controller.signal
      });

      if (!response.ok) {
        throw new Error(`Embedding model list request failed with ${response.status}`);
      }

      const data = (await response.json()) as {
        data?: Array<{ id?: string }>;
      };

      return (data.data?.map((item) => item.id).filter((item): item is string => Boolean(item)) ?? []).sort();
    } finally {
      clearTimeout(timeout);
    }
  }

  async listAvailableModels(baseUrl = config.embeddingBaseUrl) {
    const headers = createHeaders();
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), Math.min(config.embeddingTimeoutMs, 5000));

    try {
      const response = await fetch(`${getLmStudioApiRoot(baseUrl)}/api/v1/models`, {
        method: "GET",
        headers,
        signal: controller.signal
      });

      if (!response.ok) {
        throw new Error(`Embedding available model request failed with ${response.status}`);
      }

      const data = (await response.json()) as {
        models?: Array<{ type?: string; key?: string }>;
      };

      return (data.models ?? [])
        .filter((item) => item.type === "embedding" && item.key)
        .map((item) => String(item.key))
        .sort();
    } finally {
      clearTimeout(timeout);
    }
  }

  async ensureModelLoaded(model: string, baseUrl = config.embeddingBaseUrl) {
    if (!model.trim()) {
      return false;
    }

    const headers = {
      ...createHeaders(),
      "Content-Type": "application/json"
    };

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), Math.min(config.embeddingTimeoutMs, 20000));

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

      const modelEntry = (data.models ?? []).find((item) => item.type === "embedding" && item.key === model);
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

      if (!loadResponse.ok) {
        return false;
      }

      this.disabled = false;
      return true;
    } catch {
      return false;
    } finally {
      clearTimeout(timeout);
    }
  }

  async checkConnection() {
    if (!config.embeddingModel.trim()) {
      return false;
    }

    try {
      const models = await this.listModels();
      return !models.length || models.includes(config.embeddingModel);
    } catch {
      return false;
    }
  }

  async createEmbeddings(inputs: string[]) {
    const cleanedInputs = inputs.map((input) => input.trim()).filter(Boolean);
    if (this.disabled || !cleanedInputs.length) {
      return null;
    }

    const startedAt = Date.now();
    debugInfo(
      `[embedding] request start ${JSON.stringify({
        inputCount: cleanedInputs.length,
        totalChars: cleanedInputs.reduce((total, current) => total + current.length, 0),
        model: config.embeddingModel,
        baseUrl: config.embeddingBaseUrl
      })}`
    );

    const body = {
      model: config.embeddingModel,
      input: cleanedInputs
    };

    const headers: Record<string, string> = {
      "Content-Type": "application/json"
    };

    if (config.embeddingApiKey) {
      headers.Authorization = `Bearer ${config.embeddingApiKey}`;
    }

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), config.embeddingTimeoutMs);

    try {
      const response = await fetch(`${config.embeddingBaseUrl.replace(/\/$/, "")}/embeddings`, {
        method: "POST",
        headers,
        body: JSON.stringify(body),
        signal: controller.signal
      });

      if (!response.ok) {
        throw new Error(`Embedding request failed with ${response.status}`);
      }

      const data = (await response.json()) as {
        data?: Array<{ embedding?: number[] }>;
      };

      const embeddings = data.data?.map((item) => item.embedding ?? []) ?? [];
      if (embeddings.length !== cleanedInputs.length || embeddings.some((embedding) => !embedding.length)) {
        throw new Error("Embedding response did not contain valid vectors");
      }

      debugInfo(
        `[embedding] request done ${JSON.stringify({
          inputCount: cleanedInputs.length,
          durationMs: Date.now() - startedAt,
          vectorCount: embeddings.length,
          dimensions: embeddings[0]?.length ?? 0
        })}`
      );

      return embeddings;
    } catch (error) {
      this.disabled = true;
      const message = error instanceof Error ? error.message : "unknown embedding error";
      debugWarn(
        `[embedding] request failed ${JSON.stringify({
          inputCount: cleanedInputs.length,
          durationMs: Date.now() - startedAt,
          reason: message
        })}`
      );

      if (!this.unavailableLogged) {
        console.warn(`Embedding retrieval disabled: ${message}`);
        this.unavailableLogged = true;
      }

      return null;
    } finally {
      clearTimeout(timeout);
    }
  }

  async createEmbedding(input: string) {
    const embeddings = await this.createEmbeddings([input]);
    return embeddings?.[0] ?? null;
  }
}
