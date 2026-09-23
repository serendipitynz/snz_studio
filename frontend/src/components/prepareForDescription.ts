import { ImageDescriptionAvailability } from "../api/client";
import { MessageKey } from "../i18n";

// LM Studio with Gemma 4 answered a 1024x1024 or 2048x512 image but refused 1280x1280
// and 1600x900 outright ({"error":"terminated"}) rather than resizing them, so anything
// above one megapixel is downscaled here before it is sent. Done in the browser because
// the WebView already decodes PNG, JPEG and WebP, where the Go side would need a new
// dependency for WebP and resampling.
const MAX_DESCRIPTION_PIXELS = 1024 * 1024;

// prepareForDescription returns the image to send for description. A JPEG within
// MAX_DESCRIPTION_PIXELS goes as it is; anything larger is downscaled. A PNG or WebP
// is always redrawn onto white, because it can carry transparency and the model
// runtime drops the alpha channel: LM Studio with Gemma 4 described a transparent
// WebP with black text as an all-black image even when the PNG sent kept its alpha.
// White is what a person sees behind a transparent image in this app and in most
// viewers; white line art on a transparent background is the case this gives up.
// The stored document keeps the original file either way.
export async function prepareForDescription(file: Blob): Promise<Blob> {
  const url = URL.createObjectURL(file);
  try {
    const image = new Image();
    image.src = url;
    await image.decode();
    const pixels = image.naturalWidth * image.naturalHeight;
    const isJpeg = file.type === "image/jpeg";
    if (isJpeg && pixels <= MAX_DESCRIPTION_PIXELS) {
      return file;
    }
    const scale = Math.min(1, Math.sqrt(MAX_DESCRIPTION_PIXELS / pixels));
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.floor(image.naturalWidth * scale));
    canvas.height = Math.max(1, Math.floor(image.naturalHeight * scale));
    const context = canvas.getContext("2d");
    if (!context) {
      throw new Error("canvas 2d context is unavailable");
    }
    if (!isJpeg) {
      context.fillStyle = "#ffffff";
      context.fillRect(0, 0, canvas.width, canvas.height);
    }
    context.drawImage(image, 0, 0, canvas.width, canvas.height);
    const type = isJpeg ? "image/jpeg" : "image/png";
    return await new Promise<Blob>((resolve, reject) =>
      canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error("image could not be re-encoded"))), type, 0.9)
    );
  } finally {
    URL.revokeObjectURL(url);
  }
}

export type PreparedImage = { blob: Blob } | { error: string } | null;

// generateBlockerReason says why "Generate description" is unavailable for an
// image, or returns "" when it can run. It is guidance only: the server sniffs
// the bytes it receives and is what actually refuses.
export function generateBlockerReason(
  t: (key: MessageKey, vars?: Record<string, string | number>) => string,
  availability: ImageDescriptionAvailability,
  sourceType: string,
  prepared: PreparedImage
): string {
  if (!availability.enabled) {
    return t("imageDialog.disabledNoModel");
  }
  if (!availability.formats.includes(sourceType)) {
    return t("imageDialog.disabledFormat", { type: sourceType });
  }
  if (prepared && "error" in prepared) {
    return t("imageDialog.disabledDecode", { detail: prepared.error });
  }
  if (prepared && prepared.blob.size > availability.maxBytes) {
    return t("imageDialog.disabledSize", { mb: Math.floor(availability.maxBytes / (1024 * 1024)) });
  }
  return "";
}
