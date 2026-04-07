import TinySegmenter from "tiny-segmenter";

const segmenter = new TinySegmenter();

const TOKEN_PATTERN =
  /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}ー]+|[\p{L}\p{N}]+(?:[_-][\p{L}\p{N}]+)*/gu;
const JAPANESE_TOKEN_PATTERN = /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]/u;
const SINGLE_KANA_PATTERN = /^[ぁ-ゖァ-ヺー]$/u;

const ENGLISH_STOPWORDS = new Set([
  "a",
  "an",
  "and",
  "are",
  "for",
  "from",
  "how",
  "into",
  "our",
  "that",
  "the",
  "this",
  "use",
  "with",
  "your"
]);

const JAPANESE_STOPWORDS = new Set([
  "これ",
  "それ",
  "あれ",
  "こと",
  "もの",
  "ため",
  "よう",
  "です",
  "ます",
  "した",
  "して",
  "する",
  "ある",
  "いる",
  "この",
  "その",
  "ですか",
  "ますか",
  "ください",
  "お願いします"
]);

function extractTokenCandidates(input: string) {
  const normalized = input.normalize("NFKC").replace(/\s+/g, " ").trim();
  if (!normalized) {
    return [];
  }

  const tokens: string[] = [];

  for (const segment of segmenter.segment(normalized)) {
    const matches = segment.match(TOKEN_PATTERN);
    if (matches?.length) {
      tokens.push(...matches);
    }
  }

  return tokens;
}

function cleanToken(token: string) {
  const normalized = token.normalize("NFKC").trim().toLowerCase();
  if (!normalized) {
    return null;
  }

  if (JAPANESE_TOKEN_PATTERN.test(normalized)) {
    if (SINGLE_KANA_PATTERN.test(normalized) || JAPANESE_STOPWORDS.has(normalized)) {
      return null;
    }
    return normalized;
  }

  if (normalized.length < 2 || ENGLISH_STOPWORDS.has(normalized)) {
    return null;
  }

  return normalized;
}

export function buildSearchText(...parts: Array<string | null | undefined>) {
  const tokens: string[] = [];

  for (const part of parts) {
    if (!part) {
      continue;
    }

    for (const candidate of extractTokenCandidates(part)) {
      const token = cleanToken(candidate);
      if (token) {
        tokens.push(token);
      }
    }
  }

  return tokens.join(" ");
}

export function tokenizeSearchTerms(input: string, limit = 12) {
  const terms: string[] = [];
  const seen = new Set<string>();

  for (const candidate of extractTokenCandidates(input)) {
    const token = cleanToken(candidate);
    if (!token || seen.has(token)) {
      continue;
    }

    seen.add(token);
    terms.push(token);

    if (terms.length >= limit) {
      break;
    }
  }

  return terms;
}

function escapeFtsToken(token: string) {
  return `"${token.replace(/"/g, "\"\"")}"`;
}

export function toFtsQuery(input: string) {
  const tokens = tokenizeSearchTerms(input);
  if (!tokens.length) {
    return "";
  }

  return tokens.map((token) => `${escapeFtsToken(token)}*`).join(" OR ");
}
