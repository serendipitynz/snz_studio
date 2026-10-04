import assert from "node:assert/strict";
import test from "node:test";
import { applyStreamText, isStreamTextEvent } from "../src/api/streamText.ts";

function replay(frames: [string, unknown][]): string {
  let shown = "";
  for (const [event, content] of frames) {
    if (isStreamTextEvent(event)) {
      shown = applyStreamText(shown, event, content);
    }
  }
  return shown;
}

test("deltas append to what is shown", () => {
  assert.equal(replay([["delta", "Hello"], ["delta", ", world"]]), "Hello, world");
});

test("a replace withdraws a tag fragment that streamed as text", () => {
  assert.equal(
    replay([["delta", "答えは<|en"], ["replace", "答えはここ"], ["delta", "です"]]),
    "答えはここです"
  );
});

test("a replace with empty content clears what is shown", () => {
  assert.equal(replay([["delta", "Draft"], ["replace", ""]]), "");
});

test("frames that do not carry text leave the text alone", () => {
  assert.equal(replay([["delta", "a"], ["speaker", { id: "p1" }], ["done", null], ["delta", undefined]]), "a");
});
