import { authHeaders } from "./client";

// streamSSE posts to a local SSE route and hands each frame to onEvent as a
// parsed payload. The loopback API always answers a stream with `event:` +
// `data:` frames separated by a blank line, so the parsing is the small subset of
// the SSE grammar the server actually emits rather than a general client.
//
// An `error` frame is thrown rather than delivered, so a caller's try/catch sees
// a failed turn the same way it sees a failed request.
export async function streamSSE(
  path: string,
  init: RequestInit,
  onEvent: (event: string, payload: Record<string, unknown>) => void
): Promise<void> {
  const response = await fetch(`${window.__API_BASE__ ?? ""}${path}`, {
    ...init,
    headers: authHeaders(init.headers)
  });

  if (!response.ok) {
    const data = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new Error(data?.error ?? `Request failed with ${response.status}`);
  }
  if (!response.body) {
    throw new Error("Stream response did not include a body");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  const handleFrame = (rawFrame: string) => {
    const lines = rawFrame.split(/\r?\n/);
    const event = lines.find((line) => line.startsWith("event:"))?.slice(6).trim() ?? "message";
    const dataText = lines
      .filter((line) => line.startsWith("data:"))
      .map((line) => line.slice(5).trimStart())
      .join("\n");

    if (!dataText) {
      return;
    }

    const payload = JSON.parse(dataText) as Record<string, unknown>;
    if (event === "error") {
      throw new Error(typeof payload.message === "string" ? payload.message : "Stream failed");
    }
    onEvent(event, payload);
  };

  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }

    buffer += decoder.decode(value, { stream: true });
    let separatorIndex = buffer.indexOf("\n\n");
    while (separatorIndex >= 0) {
      const rawFrame = buffer.slice(0, separatorIndex).trim();
      buffer = buffer.slice(separatorIndex + 2);
      if (rawFrame) {
        handleFrame(rawFrame);
      }
      separatorIndex = buffer.indexOf("\n\n");
    }
  }

  buffer += decoder.decode();
  if (buffer.trim()) {
    handleFrame(buffer.trim());
  }
}
