import assert from "node:assert/strict";
import test from "node:test";
import type { EmbeddingStatus } from "../src/api/client.ts";
import { pollEmbeddingStatus } from "../src/components/embeddingStatusPoll.ts";

// Settles the pending read and the status callback that follows it.
const flush = () => new Promise<void>((resolve) => setImmediate(resolve));

function status(state: EmbeddingStatus["state"]): EmbeddingStatus {
  return { state, modelId: "ruri-v3-30m", dim: 256, downloaded: 0, total: 0 };
}

function scripted(replies: (EmbeddingStatus | Error)[]) {
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

async function advance(t: test.TestContext, times: number) {
  for (let i = 0; i < times; i += 1) {
    t.mock.timers.tick(2000);
    await flush();
  }
}

test("waits one interval, then polls while preparing and stops once ready", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const { read, calls } = scripted([status("downloading"), status("starting"), status("ready")]);
  const seen: EmbeddingStatus["state"][] = [];
  pollEmbeddingStatus(read, (next) => seen.push(next.state), 2000);

  await flush();
  assert.equal(calls(), 0);
  await advance(t, 5);
  assert.deepEqual(seen, ["downloading", "starting", "ready"]);
  assert.equal(calls(), 3);
});

test("stops once the sidecar has failed", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const { read, calls } = scripted([status("starting"), status("error")]);
  const seen: EmbeddingStatus["state"][] = [];
  pollEmbeddingStatus(read, (next) => seen.push(next.state), 2000);

  await advance(t, 5);
  assert.deepEqual(seen, ["starting", "error"]);
  assert.equal(calls(), 2);
});

test("a failed read is retried, so the ready state still shows", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const { read } = scripted([new Error("connection refused"), status("ready")]);
  const seen: EmbeddingStatus["state"][] = [];
  pollEmbeddingStatus(read, (next) => seen.push(next.state), 2000);

  await advance(t, 1);
  assert.deepEqual(seen, []);
  await advance(t, 1);
  assert.deepEqual(seen, ["ready"]);
});

test("stopping cancels the next read and drops a reply still in flight", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  let release: (value: EmbeddingStatus) => void = () => {};
  let calls = 0;
  const read = () => {
    calls += 1;
    return new Promise<EmbeddingStatus>((resolve) => {
      release = resolve;
    });
  };
  const seen: EmbeddingStatus["state"][] = [];
  const stop = pollEmbeddingStatus(read, (next) => seen.push(next.state), 2000);

  t.mock.timers.tick(2000);
  stop();
  release(status("starting"));
  await flush();
  await advance(t, 5);
  assert.deepEqual(seen, []);
  assert.equal(calls, 1);
});
