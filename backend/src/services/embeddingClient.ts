import { config } from "../config.js";

export class EmbeddingClient {
  private unavailableLogged = false;
  private disabled = !config.embeddingModel.trim();

  isEnabled() {
    return !this.disabled;
  }

  getModel() {
    return config.embeddingModel.trim();
  }

  async createEmbeddings(inputs: string[]) {
    const cleanedInputs = inputs.map((input) => input.trim()).filter(Boolean);
    if (this.disabled || !cleanedInputs.length) {
      return null;
    }

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

      return embeddings;
    } catch (error) {
      this.disabled = true;

      if (!this.unavailableLogged) {
        const message = error instanceof Error ? error.message : "unknown embedding error";
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
