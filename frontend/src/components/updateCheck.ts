// The update check's results as app_update.go returns them. Declared here rather than
// imported from the generated wailsjs models so that node --test can load this file.
export interface UpdateCheck {
  status: string;
  currentVersion: string;
  version?: string;
  releaseUrl: string;
}

export interface AvailableUpdate {
  version: string;
  currentVersion: string;
  releaseUrl: string;
}

export function availableUpdate(check: UpdateCheck): AvailableUpdate | null {
  if (check.status !== "available" || !check.version) {
    return null;
  }
  return { version: check.version, currentVersion: check.currentVersion, releaseUrl: check.releaseUrl };
}

// Long enough for the first render and the startup reads (projects, recent chats) to
// finish before the check's request goes out.
export const AUTO_CHECK_DELAY_MS = 5000;

interface AutoCheckOptions {
  isEnabled: () => Promise<boolean>;
  check: () => Promise<UpdateCheck>;
  onAvailable: (update: AvailableUpdate) => void;
  delayMs?: number;
}

// Runs the startup check once, after a delay. Every failure is dropped without a
// word, a rejected binding included: an app that shows an error at every offline start
// is worse than one that never checks. The returned function cancels it.
export function scheduleAutoCheck({ isEnabled, check, onAvailable, delayMs = AUTO_CHECK_DELAY_MS }: AutoCheckOptions) {
  let cancelled = false;
  const timer = setTimeout(async () => {
    try {
      if (!(await isEnabled()) || cancelled) {
        return;
      }
      const update = availableUpdate(await check());
      if (update && !cancelled) {
        onAvailable(update);
      }
    } catch {
      // Dropped on purpose; see above.
    }
  }, delayMs);
  return () => {
    cancelled = true;
    clearTimeout(timer);
  };
}

// The reasons InstallUpdate gives for an app it cannot replace in place
// (internal/updater and app_update.go), each with the words that say what to do.
const MANUAL_REASON_KEYS = {
  translocated: "update.manualTranslocated",
  readOnlyVolume: "update.manualReadOnlyVolume",
  notWritable: "update.manualNotWritable",
  notInstalled: "update.manualNotInstalled",
  unsupportedPlatform: "update.manualUnsupportedPlatform",
  devBuild: "update.manualDevBuild"
} as const;

export type ManualReasonKey = (typeof MANUAL_REASON_KEYS)[keyof typeof MANUAL_REASON_KEYS];

// A reason this build does not know yet gets no sentence of its own; the dialog still
// sends the user to the Releases page.
export function manualReasonKey(reason: string | undefined): ManualReasonKey | null {
  return reason && Object.hasOwn(MANUAL_REASON_KEYS, reason)
    ? MANUAL_REASON_KEYS[reason as keyof typeof MANUAL_REASON_KEYS]
    : null;
}
