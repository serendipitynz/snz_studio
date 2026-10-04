import type { MessageKey } from "../i18n";

// The image formats the server stores (storedImageExtensions in
// internal/httpapi/upload.go). The server judges from the bytes and stays the one
// that refuses; this list only keeps the UI from sending what would be refused,
// so the file is named before the upload rather than failing after it.
const STORED_IMAGE_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"];

export type DocumentFileType = "markdown" | "text" | "image";

export function isStoredImageType(mime: string) {
  return STORED_IMAGE_TYPES.includes(mime);
}

export function detectDocumentType(file: Pick<File, "name" | "type">): DocumentFileType | null {
  const lowerName = file.name.toLowerCase();

  if (isStoredImageType(file.type)) {
    return "image";
  }

  if (lowerName.endsWith(".md") || lowerName.endsWith(".markdown")) {
    return "markdown";
  }

  if (lowerName.endsWith(".txt")) {
    return "text";
  }

  return null;
}

// Only a MIME type seen while dragging, before any name is known: a Markdown file
// often has none, which the drop zone reads as "not known yet" rather than refused.
export function isDocumentMime(mime: string) {
  return isStoredImageType(mime) || mime === "text/plain" || mime === "text/markdown" || mime === "text/x-markdown";
}

export const DOCUMENT_INPUT_ACCEPT = [".md", ".markdown", ".txt", ...STORED_IMAGE_TYPES].join(",");
export const IMAGE_INPUT_ACCEPT = STORED_IMAGE_TYPES.join(",");

type Translate = (key: MessageKey, vars?: Record<string, string | number>) => string;

// Words the two refusals of an upload the user can act on (handleCreateDocument's
// unsupported image and oversize upload) and names the file, since a bulk import
// stops at it. Any other failure is returned as it is.
// The error is read by its shape rather than `instanceof ApiError` so this module
// does not load the API client, which the node test runner cannot resolve.
export function describeUploadFailure(t: Translate, error: unknown, name: string): unknown {
  if (!(error instanceof Error) || !("status" in error) || !("body" in error)) {
    return error;
  }
  const body = error.body as Record<string, unknown> | null;
  if (error.status === 400 && body?.code === "unsupported_image") {
    return new Error(t("documentUpload.unsupportedImage", { name }));
  }
  if (error.status === 413 && typeof body?.limit === "number") {
    return new Error(t("documentUpload.tooLarge", { name, limit: Math.round(body.limit / (1 << 20)) }));
  }
  return error;
}
