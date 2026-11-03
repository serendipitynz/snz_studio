import assert from "node:assert/strict";
import test from "node:test";
import type { MessageRecord } from "../src/api/client.ts";
import { createPinLedger } from "../src/components/pinLedger.ts";

function message(id: string, pinned: boolean): MessageRecord {
  return { id, pinned } as MessageRecord;
}

const pins = (messages: MessageRecord[]) => messages.map((entry) => [entry.id, entry.pinned]);

test("a reload read before a pin confirmed during it keeps the pin", () => {
  const ledger = createPinLedger();
  const mark = ledger.mark();
  const read = [message("a", false), message("b", false)];
  ledger.confirm("a", true);

  assert.deepEqual(pins(ledger.applySince(mark, read)), [["a", true], ["b", false]]);
});

test("a pin confirmed before the request is left to what the request read", () => {
  const ledger = createPinLedger();
  ledger.confirm("a", true);
  const mark = ledger.mark();
  // Another window took the pin off after this one confirmed it.
  const read = [message("a", false)];

  assert.deepEqual(pins(ledger.applySince(mark, read)), [["a", false]]);
});

test("the latest confirmation of a message wins", () => {
  const ledger = createPinLedger();
  const mark = ledger.mark();
  ledger.confirm("a", true);
  ledger.confirm("a", false);

  assert.deepEqual(pins(ledger.applySince(mark, [message("a", true)])), [["a", false]]);
});

test("messages the ledger never saw pass through unchanged", () => {
  const ledger = createPinLedger();
  const mark = ledger.mark();
  ledger.confirm("a", true);
  const read = [message("c", true)];

  assert.equal(ledger.applySince(mark, read)[0], read[0]);
});
