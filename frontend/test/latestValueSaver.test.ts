import assert from "node:assert/strict";
import test from "node:test";
import { createLatestValueSaver } from "../src/components/latestValueSaver.ts";

// Settles the pending write and the callbacks that follow it.
const flush = () => new Promise<void>((resolve) => setImmediate(resolve));

function harness() {
  const writes: { value: string; resolve: () => void; reject: () => void }[] = [];
  const saved: string[] = [];
  const settled: boolean[] = [];
  const choose = createLatestValueSaver<string>(
    (value) =>
      new Promise<void>((resolve, reject) => {
        writes.push({ value, resolve, reject: () => reject(new Error("write failed")) });
      }),
    (value) => saved.push(value),
    (lastFailed) => settled.push(lastFailed)
  );
  return { choose, writes, saved, settled };
}

test("a choice made during a write waits for it, so the two cannot finish out of order", async () => {
  const { choose, writes, saved, settled } = harness();
  void choose("en");
  void choose("ja");
  await flush();
  assert.deepEqual(writes.map((write) => write.value), ["en"]);

  writes[0].resolve();
  await flush();
  assert.deepEqual(writes.map((write) => write.value), ["en", "ja"]);
  assert.deepEqual(settled, []);

  writes[1].resolve();
  await flush();
  assert.deepEqual(saved, ["en", "ja"]);
  assert.deepEqual(settled, [false]);
});

test("only the newest of the choices made during a write is sent", async () => {
  const { choose, writes } = harness();
  void choose("en");
  void choose("ja");
  void choose("en");
  writes[0].resolve();
  await flush();
  assert.deepEqual(writes.map((write) => write.value), ["en", "en"]);
});

test("when the last write fails, the earlier success is what was saved", async () => {
  const { choose, writes, saved, settled } = harness();
  void choose("en");
  void choose("ja");
  writes[0].resolve();
  await flush();
  writes[1].reject();
  await flush();
  assert.deepEqual(saved, ["en"]);
  assert.deepEqual(settled, [true]);
});

test("a failed write followed by a successful one settles as saved", async () => {
  const { choose, writes, saved, settled } = harness();
  void choose("en");
  void choose("ja");
  writes[0].reject();
  await flush();
  writes[1].resolve();
  await flush();
  assert.deepEqual(saved, ["ja"]);
  assert.deepEqual(settled, [false]);
});

test("a choice after the writes settle starts a new write", async () => {
  const { choose, writes, settled } = harness();
  void choose("en");
  writes[0].reject();
  await flush();
  void choose("ja");
  writes[1].resolve();
  await flush();
  assert.deepEqual(writes.map((write) => write.value), ["en", "ja"]);
  assert.deepEqual(settled, [true, false]);
});
