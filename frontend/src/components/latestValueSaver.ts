// Sends one write at a time and, of the values chosen meanwhile, only the newest.
// Overlapping writes could finish in any order, leaving the server on one value and
// the screen on another. Holding the control disabled during a write would prevent
// that too, but drops keyboard focus: arrow keys on a closed select change the value
// on every press.
export function createLatestValueSaver<T>(
  save: (value: T) => Promise<unknown>,
  onSaved: (value: T) => void,
  onSettled: (lastFailed: boolean) => void
): (value: T) => Promise<void> {
  let saving = false;
  let queued: { value: T } | null = null;

  return async (value) => {
    if (saving) {
      queued = { value };
      return;
    }
    saving = true;
    let current = value;
    for (;;) {
      let failed = false;
      try {
        await save(current);
        onSaved(current);
      } catch {
        failed = true;
      }
      // The callback below may choose again, so the write is over before it runs.
      if (queued === null) {
        saving = false;
        onSettled(failed);
        return;
      }
      current = queued.value;
      queued = null;
    }
  };
}
