import assert from "node:assert/strict";
import test from "node:test";
import type { AvailableUpdate, UpdateCheck } from "../src/components/updateCheck.ts";
import { AUTO_CHECK_DELAY_MS, manualReasonKey, scheduleAutoCheck } from "../src/components/updateCheck.ts";

const flush = () => new Promise<void>((resolve) => setImmediate(resolve));

const available: UpdateCheck = {
  status: "available",
  currentVersion: "0.1.0",
  version: "0.1.1",
  releaseUrl: "https://example.test/releases/tag/v0.1.1"
};

function setup(t: test.TestContext, options: { enabled?: boolean | Error; result?: UpdateCheck | Error }) {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const calls = { isEnabled: 0, check: 0 };
  const offered: AvailableUpdate[] = [];
  const cancel = scheduleAutoCheck({
    isEnabled: async () => {
      calls.isEnabled += 1;
      if (options.enabled instanceof Error) {
        throw options.enabled;
      }
      return options.enabled ?? true;
    },
    check: async () => {
      calls.check += 1;
      if (options.result instanceof Error) {
        throw options.result;
      }
      return options.result ?? available;
    },
    onAvailable: (update) => offered.push(update)
  });
  return { calls, offered, cancel };
}

test("the startup check waits for the delay before asking anything", async (t) => {
  const { calls, offered } = setup(t, {});
  await flush();
  assert.equal(calls.isEnabled, 0);
  t.mock.timers.tick(AUTO_CHECK_DELAY_MS - 1);
  await flush();
  assert.equal(calls.isEnabled, 0);
  t.mock.timers.tick(1);
  await flush();
  assert.deepEqual(calls, { isEnabled: 1, check: 1 });
  assert.deepEqual(offered, [
    { version: "0.1.1", currentVersion: "0.1.0", releaseUrl: "https://example.test/releases/tag/v0.1.1" }
  ]);
});

test("a disabled startup check does not reach the network", async (t) => {
  const { calls, offered } = setup(t, { enabled: false });
  t.mock.timers.tick(AUTO_CHECK_DELAY_MS);
  await flush();
  assert.deepEqual(calls, { isEnabled: 1, check: 0 });
  assert.equal(offered.length, 0);
});

test("an offline or failed check offers nothing and throws nothing", async (t) => {
  for (const result of [
    { status: "failed", currentVersion: "0.1.0", releaseUrl: "" },
    new Error("binding rejected")
  ]) {
    const { calls, offered } = setup(t, { result });
    t.mock.timers.tick(AUTO_CHECK_DELAY_MS);
    await flush();
    assert.equal(calls.check, 1);
    assert.equal(offered.length, 0);
    t.mock.timers.reset();
  }
  const { offered } = setup(t, { enabled: new Error("binding missing") });
  t.mock.timers.tick(AUTO_CHECK_DELAY_MS);
  await flush();
  assert.equal(offered.length, 0);
});

test("an up-to-date result offers nothing", async (t) => {
  const { offered } = setup(t, { result: { status: "upToDate", currentVersion: "0.1.1", releaseUrl: "" } });
  t.mock.timers.tick(AUTO_CHECK_DELAY_MS);
  await flush();
  assert.equal(offered.length, 0);
});

test("a cancelled check never runs", async (t) => {
  const { calls, cancel } = setup(t, {});
  cancel();
  t.mock.timers.tick(AUTO_CHECK_DELAY_MS);
  await flush();
  assert.deepEqual(calls, { isEnabled: 0, check: 0 });
});

test("each reason the Go side returns has its own sentence; an unknown one has none", () => {
  for (const reason of ["translocated", "readOnlyVolume", "notWritable", "notInstalled", "unsupportedPlatform", "devBuild"]) {
    assert.ok(manualReasonKey(reason)?.startsWith("update.manual"), reason);
  }
  assert.equal(manualReasonKey("somethingNew"), null);
  assert.equal(manualReasonKey("toString"), null);
  assert.equal(manualReasonKey(undefined), null);
});
