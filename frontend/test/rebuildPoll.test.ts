import assert from "node:assert/strict";
import test from "node:test";
import type { EmbeddingRebuildState } from "../src/api/client.ts";
import { pollRebuildState } from "../src/components/rebuildPoll.ts";

// Settles the pending read and the state callback that follows it.
const flush = () => new Promise<void>((resolve) => setImmediate(resolve));

function scripted(replies: (EmbeddingRebuildState | Error)[]) {
  let calls = 0;
  const read = async () => {
    const reply = replies[Math.min(calls, replies.length - 1)];
    calls += 1;
    if (reply instanceof Error) {
      throw reply;
    }
    return reply;
  };
  return { read, calls: () => calls };
}

test("polls while running and stops once the rebuild has ended", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const { read, calls } = scripted(["running", "running", "done"]);
  const seen: EmbeddingRebuildState[] = [];
  pollRebuildState(read, (state) => seen.push(state), 2000);

  await flush();
  for (let i = 0; i < 3; i += 1) {
    t.mock.timers.tick(2000);
    await flush();
  }
  assert.deepEqual(seen, ["running", "running", "done"]);
  assert.equal(calls(), 3);
});

test("a failed read while running is retried, so the end still shows", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const { read } = scripted(["running", new Error("connection refused"), "done"]);
  const seen: EmbeddingRebuildState[] = [];
  pollRebuildState(read, (state) => seen.push(state), 2000);

  await flush();
  t.mock.timers.tick(2000);
  await flush();
  assert.deepEqual(seen, ["running"]);
  t.mock.timers.tick(2000);
  await flush();
  assert.deepEqual(seen, ["running", "done"]);
});

test("stopping cancels the next read and drops a reply still in flight", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const { read, calls } = scripted(["running"]);
  const seen: EmbeddingRebuildState[] = [];
  const stop = pollRebuildState(read, (state) => seen.push(state), 2000);
  stop();

  await flush();
  t.mock.timers.tick(10_000);
  await flush();
  assert.deepEqual(seen, []);
  assert.equal(calls(), 1);
});
