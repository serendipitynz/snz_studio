import fs from "node:fs";
import path from "node:path";

const cwd = process.cwd();
const envPath = path.resolve(cwd, ".env");

loadEnvFile(envPath);

const dataDir = process.env.DATA_DIR
  ? path.resolve(process.env.DATA_DIR)
  : path.resolve(cwd, "data");

fs.mkdirSync(dataDir, { recursive: true });

const appConfigPath = path.resolve(dataDir, "app-config.json");

export interface EditableAppConfiguration {
  llmBaseUrl: string;
  llmModel: string;
  llmResponseFormat: "standard" | "llm_jp_thinking";
  reviewBaseUrl: string;
  reviewModel: string;
  embeddingBaseUrl: string;
  embeddingModel: string;
}

export const config = {
  port: Number(process.env.PORT ?? 8787),
  appOrigin: process.env.APP_ORIGIN ?? "http://127.0.0.1:5173",
  dataDir,
  appConfigPath,
  uploadDir: process.env.UPLOAD_DIR
    ? path.resolve(process.env.UPLOAD_DIR)
    : path.resolve(dataDir, "uploads"),
  sqlitePath: process.env.SQLITE_PATH
    ? path.resolve(process.env.SQLITE_PATH)
    : path.resolve(dataDir, "app.sqlite"),
  llmBaseUrl: process.env.LLM_BASE_URL ?? "http://127.0.0.1:1234/v1",
  llmModel: process.env.LLM_MODEL ?? "local-model",
  llmResponseFormat: (process.env.LLM_RESPONSE_FORMAT as EditableAppConfiguration["llmResponseFormat"] | undefined) ?? "standard",
  llmApiKey: process.env.LLM_API_KEY ?? "",
  llmTimeoutMs: Number(process.env.LLM_TIMEOUT_MS ?? 60000),
  reviewBaseUrl: process.env.REVIEW_BASE_URL ?? process.env.LLM_BASE_URL ?? "http://127.0.0.1:1234/v1",
  reviewModel: process.env.REVIEW_MODEL ?? process.env.LLM_MODEL ?? "local-model",
  embeddingBaseUrl: process.env.EMBEDDING_BASE_URL ?? process.env.LLM_BASE_URL ?? "http://127.0.0.1:1234/v1",
  embeddingModel: process.env.EMBEDDING_MODEL ?? "",
  embeddingApiKey: process.env.EMBEDDING_API_KEY ?? process.env.LLM_API_KEY ?? "",
  embeddingTimeoutMs: Number(process.env.EMBEDDING_TIMEOUT_MS ?? process.env.LLM_TIMEOUT_MS ?? 60000),
  debugChatFlow: /^(1|true|yes|on)$/i.test(process.env.DEBUG_CHAT_FLOW ?? ""),
  debugRetrieval: /^(1|true|yes|on)$/i.test(process.env.DEBUG_RETRIEVAL ?? "")
};

applyAppConfigOverrides();

export function getEditableConfiguration(): EditableAppConfiguration {
  return {
    llmBaseUrl: config.llmBaseUrl,
    llmModel: config.llmModel,
    llmResponseFormat: config.llmResponseFormat,
    reviewBaseUrl: config.reviewBaseUrl,
    reviewModel: config.reviewModel,
    embeddingBaseUrl: config.embeddingBaseUrl,
    embeddingModel: config.embeddingModel
  };
}

export function updateEditableConfiguration(input: EditableAppConfiguration) {
  config.llmBaseUrl = input.llmBaseUrl.trim();
  config.llmModel = input.llmModel.trim();
  config.llmResponseFormat = input.llmResponseFormat;
  config.reviewBaseUrl = input.reviewBaseUrl.trim();
  config.reviewModel = input.reviewModel.trim();
  config.embeddingBaseUrl = input.embeddingBaseUrl.trim();
  config.embeddingModel = input.embeddingModel.trim();

  fs.writeFileSync(config.appConfigPath, `${JSON.stringify(getEditableConfiguration(), null, 2)}\n`, "utf8");
  return getEditableConfiguration();
}

function applyAppConfigOverrides() {
  if (!fs.existsSync(appConfigPath)) {
    return;
  }

  const overrides = safeJsonParse<Partial<EditableAppConfiguration>>(fs.readFileSync(appConfigPath, "utf8"), {});
  if (typeof overrides.llmBaseUrl === "string") {
    config.llmBaseUrl = overrides.llmBaseUrl.trim();
  }
  if (typeof overrides.llmModel === "string") {
    config.llmModel = overrides.llmModel.trim();
  }
  if (overrides.llmResponseFormat === "standard" || overrides.llmResponseFormat === "llm_jp_thinking") {
    config.llmResponseFormat = overrides.llmResponseFormat;
  }
  if (typeof overrides.reviewBaseUrl === "string") {
    config.reviewBaseUrl = overrides.reviewBaseUrl.trim();
  }
  if (typeof overrides.reviewModel === "string") {
    config.reviewModel = overrides.reviewModel.trim();
  }
  if (typeof overrides.embeddingBaseUrl === "string") {
    config.embeddingBaseUrl = overrides.embeddingBaseUrl.trim();
  }
  if (typeof overrides.embeddingModel === "string") {
    config.embeddingModel = overrides.embeddingModel.trim();
  }
}

function loadEnvFile(filePath: string) {
  if (!fs.existsSync(filePath)) {
    return;
  }

  const raw = fs.readFileSync(filePath, "utf8");
  const lines = raw.split(/\r?\n/);

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) {
      continue;
    }

    const separatorIndex = trimmed.indexOf("=");
    if (separatorIndex <= 0) {
      continue;
    }

    const key = trimmed.slice(0, separatorIndex).trim();
    const value = trimmed.slice(separatorIndex + 1).trim().replace(/^['"]|['"]$/g, "");

    if (!(key in process.env)) {
      process.env[key] = value;
    }
  }
}

function safeJsonParse<T>(input: string, fallback: T): T {
  try {
    return JSON.parse(input) as T;
  } catch {
    return fallback;
  }
}
