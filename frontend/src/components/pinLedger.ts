import type { MessageRecord } from "../api/client";

// A pin can change while a turn runs or the page reloads (design §4.9.4), so the
// transcript a request read may predate a pin confirmed since. Each confirmed pin
// gets the next number; a response lays the pins numbered after its request's
// mark over what it read. Pins confirmed before the mark are already in that read.
// Trusting the shown pins instead fails when a reload's response is what is shown.
export type PinLedger = {
  mark(): number;
  confirm(messageId: string, pinned: boolean): void;
  applySince(mark: number, messages: MessageRecord[]): MessageRecord[];
};

export function createPinLedger(): PinLedger {
  let last = 0;
  const pins = new Map<string, { pinned: boolean; seq: number }>();

  return {
    mark: () => last,
    confirm(messageId, pinned) {
      last += 1;
      pins.set(messageId, { pinned, seq: last });
    },
    applySince(mark, messages) {
      return messages.map((message) => {
        const pin = pins.get(message.id);
        if (pin === undefined || pin.seq <= mark || pin.pinned === message.pinned) {
          return message;
        }
        return { ...message, pinned: pin.pinned };
      });
    }
  };
}
