// The text frames of a streamed answer. The server re-derives the visible text
// from the whole raw response on every chunk, so it does not only grow: markup
// that streamed as text is removed once the rest of the tag arrives. A "delta"
// appends to what is shown; a "replace" stands in for everything shown so far.
export type StreamTextEvent = "delta" | "replace";

// The model's reasoning streams beside the answer in frames of its own, with the
// same append / replace meaning.
export type StreamReasoningEvent = "reasoning" | "reasoning-replace";

export function isStreamTextEvent(event: string): event is StreamTextEvent {
  return event === "delta" || event === "replace";
}

export function isStreamReasoningEvent(event: string): event is StreamReasoningEvent {
  return event === "reasoning" || event === "reasoning-replace";
}

export function applyStreamText(current: string, event: StreamTextEvent | StreamReasoningEvent, content: unknown): string {
  const text = typeof content === "string" ? content : "";
  return event === "replace" || event === "reasoning-replace" ? text : `${current}${text}`;
}
