import fs from "node:fs";
import path from "node:path";

const cwd = process.cwd();
const envPath = path.resolve(cwd, ".env");

loadEnvFile(envPath);

const dataDir = process.env.DATA_DIR
  ? path.resolve(process.env.DATA_DIR)
  : path.resolve(cwd, "data");

export const config = {
  port: Number(process.env.PORT ?? 8787),
  appOrigin: process.env.APP_ORIGIN ?? "http://127.0.0.1:5173",
  dataDir,
  uploadDir: process.env.UPLOAD_DIR
    ? path.resolve(process.env.UPLOAD_DIR)
    : path.resolve(dataDir, "uploads"),
  sqlitePath: process.env.SQLITE_PATH
    ? path.resolve(process.env.SQLITE_PATH)
    : path.resolve(dataDir, "app.sqlite"),
  llmBaseUrl: process.env.LLM_BASE_URL ?? "http://127.0.0.1:1234/v1",
  llmModel: process.env.LLM_MODEL ?? "local-model",
  llmApiKey: process.env.LLM_API_KEY ?? "",
  llmTimeoutMs: Number(process.env.LLM_TIMEOUT_MS ?? 60000),
  embeddingBaseUrl: process.env.EMBEDDING_BASE_URL ?? process.env.LLM_BASE_URL ?? "http://127.0.0.1:1234/v1",
  embeddingModel: process.env.EMBEDDING_MODEL ?? "",
  embeddingApiKey: process.env.EMBEDDING_API_KEY ?? process.env.LLM_API_KEY ?? "",
  embeddingTimeoutMs: Number(process.env.EMBEDDING_TIMEOUT_MS ?? process.env.LLM_TIMEOUT_MS ?? 60000)
};

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
