import assert from "node:assert/strict";
import test from "node:test";
import { describeUploadFailure, detectDocumentType, isDocumentMime } from "../src/components/documentFiles.ts";

test("the four stored image formats and the text kinds are accepted", () => {
  assert.equal(detectDocumentType({ name: "a.png", type: "image/png" }), "image");
  assert.equal(detectDocumentType({ name: "a.jpg", type: "image/jpeg" }), "image");
  assert.equal(detectDocumentType({ name: "a.gif", type: "image/gif" }), "image");
  assert.equal(detectDocumentType({ name: "a.webp", type: "image/webp" }), "image");
  assert.equal(detectDocumentType({ name: "Notes.MD", type: "" }), "markdown");
  assert.equal(detectDocumentType({ name: "a.markdown", type: "text/markdown" }), "markdown");
  assert.equal(detectDocumentType({ name: "a.txt", type: "text/plain" }), "text");
});

test("other images are refused before they are sent", () => {
  assert.equal(detectDocumentType({ name: "icon.svg", type: "image/svg+xml" }), null);
  assert.equal(detectDocumentType({ name: "photo.heic", type: "image/heic" }), null);
  assert.equal(detectDocumentType({ name: "scan.bmp", type: "image/bmp" }), null);
  assert.equal(detectDocumentType({ name: "page.html", type: "text/html" }), null);
});

test("a drag shows refusal only for a MIME type certainly not accepted", () => {
  for (const mime of ["image/png", "image/jpeg", "image/gif", "image/webp", "text/plain", "text/markdown", "text/x-markdown"]) {
    assert.equal(isDocumentMime(mime), true, mime);
  }
  for (const mime of ["image/svg+xml", "image/heic", "application/pdf"]) {
    assert.equal(isDocumentMime(mime), false, mime);
  }
});

const t = (key: string, vars?: Record<string, string | number>) => `${key} ${JSON.stringify(vars)}`;

function refusal(status: number, body: Record<string, unknown> | null) {
  return Object.assign(new Error("server words"), { status, body });
}

test("an unsupported image and an oversize upload are worded with the file name", () => {
  const unsupported = describeUploadFailure(t, refusal(400, { error: "x", code: "unsupported_image" }), "photo.png");
  assert.equal((unsupported as Error).message, 'documentUpload.unsupportedImage {"name":"photo.png"}');

  const tooLarge = describeUploadFailure(t, refusal(413, { error: "x", limit: 20 << 20 }), "big.png");
  assert.equal((tooLarge as Error).message, 'documentUpload.tooLarge {"name":"big.png","limit":20}');
});

test("any other failure is left as it is", () => {
  const cases = [
    refusal(400, { error: "invalid document type" }),
    refusal(413, { error: "upload is too large" }),
    refusal(500, null),
    new Error("network down"),
    "not an error"
  ];
  for (const error of cases) {
    assert.equal(describeUploadFailure(t, error, "a.png"), error);
  }
});
