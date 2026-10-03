import type { EmbeddingRebuildState } from "../api/client";

// Reads the rebuild state now, and again every intervalMs while it is running. A
// failed read is retried at the same pace rather than ending the polling: one
// dropped request would otherwise leave the button held and the "running" note up
// long after the rebuild has ended. Returns a function that stops the polling.
export function pollRebuildState(
  read: () => Promise<EmbeddingRebuildState>,
  onState: (state: EmbeddingRebuildState) => void,
  intervalMs = 2000
): () => void {
  let active = true;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const tick = async () => {
    let state: EmbeddingRebuildState;
    try {
      state = await read();
    } catch {
      if (active) {
        timer = setTimeout(tick, intervalMs);
      }
      return;
    }
    if (!active) {
      return;
    }
    onState(state);
    if (state === "running") {
      timer = setTimeout(tick, intervalMs);
    }
  };
  void tick();
  return () => {
    active = false;
    clearTimeout(timer);
  };
}
