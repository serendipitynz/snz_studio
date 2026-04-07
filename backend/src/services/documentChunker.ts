import { truncate } from "../lib/utils.js";

const CHUNK_SIZE = 1000;
const CHUNK_OVERLAP = 150;

export function chunkDocumentText(text: string) {
  const normalized = text.replace(/\r\n/g, "\n").trim();
  if (!normalized) {
    return [];
  }

  const chunks: string[] = [];
  let offset = 0;

  while (offset < normalized.length) {
    const nextChunk = normalized.slice(offset, offset + CHUNK_SIZE).trim();
    if (nextChunk) {
      chunks.push(truncate(nextChunk, CHUNK_SIZE));
    }
    offset += CHUNK_SIZE - CHUNK_OVERLAP;
  }

  return chunks;
}
