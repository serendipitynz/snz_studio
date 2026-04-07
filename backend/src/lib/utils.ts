export function nowIso() {
  return new Date().toISOString();
}

export function createId(prefix: string) {
  return `${prefix}_${crypto.randomUUID()}`;
}

export function parseTags(raw: string | string[] | undefined) {
  if (!raw) {
    return [];
  }

  const value = Array.isArray(raw) ? raw.join(",") : raw;
  return value
    .split(",")
    .map((tag) => tag.trim())
    .filter(Boolean);
}

export function truncate(text: string, maxLength = 280) {
  if (text.length <= maxLength) {
    return text;
  }

  return `${text.slice(0, Math.max(0, maxLength - 1)).trimEnd()}…`;
}

export function safeJsonParse<T>(input: string, fallback: T): T {
  try {
    return JSON.parse(input) as T;
  } catch {
    return fallback;
  }
}
