import type { EmbeddingStatus } from "../api/client";

export function isEmbeddingPreparing(status: EmbeddingStatus | null): boolean {
  return status?.state === "downloading" || status?.state === "starting";
}

// Re-reads the internal embedding status every intervalMs while the sidecar is
// still downloading or starting, and stops once it is ready, failed or disabled.
// The first read waits one interval because the caller already holds a status it
// just read. A failed read is retried at the same pace, so one dropped request
// does not leave "preparing" up after the sidecar is ready. Returns a function
// that stops the polling.
export function pollEmbeddingStatus(
  read: () => Promise<EmbeddingStatus>,
  onStatus: (status: EmbeddingStatus) => void,
  intervalMs = 2000
): () => void {
  let active = true;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const tick = async () => {
    let status: EmbeddingStatus;
    try {
      status = await read();
    } catch {
      if (active) {
        timer = setTimeout(tick, intervalMs);
      }
      return;
    }
    if (!active) {
      return;
    }
    onStatus(status);
    if (isEmbeddingPreparing(status)) {
      timer = setTimeout(tick, intervalMs);
    }
  };
  timer = setTimeout(tick, intervalMs);
  return () => {
    active = false;
    clearTimeout(timer);
  };
}
