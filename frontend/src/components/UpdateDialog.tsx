import styled from "@emotion/styled";
import { createContext, MouseEvent, ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useLanguage } from "../i18n";
import { focusRing, Button, Subtle } from "../styles/ui";
import { CheckForUpdate, GetAutoCheckUpdates, InstallUpdate } from "../wailsjs/go/main/App";
import { BrowserOpenURL, EventsOn } from "../wailsjs/runtime/runtime";
import { ActionButton } from "./ActionButton";
import { announce } from "./announce";
import { Dialog, DialogActions, DialogBody, DialogHeader, DialogTitle } from "./Dialog";
import { FailureNotice } from "./FailureNotice";
import { DownloadIcon } from "./icons";
import { Progress } from "./Progress";
import { AvailableUpdate, manualReasonKey, scheduleAutoCheck } from "./updateCheck";

interface UpdateContextValue {
  offer: (update: AvailableUpdate) => void;
}

const UpdateContext = createContext<UpdateContextValue | null>(null);

// Holds the update dialog for the whole app, so the startup check and the settings'
// manual check open the same one, and runs the startup check once.
export function UpdateProvider({ children }: { children: ReactNode }) {
  const [offered, setOffered] = useState<AvailableUpdate | null>(null);

  useEffect(
    () => scheduleAutoCheck({ isEnabled: GetAutoCheckUpdates, check: CheckForUpdate, onAvailable: setOffered }),
    []
  );

  const offer = useCallback((update: AvailableUpdate) => setOffered(update), []);
  const value = useMemo(() => ({ offer }), [offer]);

  return (
    <UpdateContext.Provider value={value}>
      {children}
      {offered ? <UpdateDialog key={offered.version} update={offered} onClose={() => setOffered(null)} /> : null}
    </UpdateContext.Provider>
  );
}

export function useUpdateOffer(): (update: AvailableUpdate) => void {
  const context = useContext(UpdateContext);
  if (!context) {
    throw new Error("useUpdateOffer must be used inside UpdateProvider");
  }
  return context.offer;
}

// The browser opens outside the app: a plain link would navigate the app's own
// WebView away from the app.
export function openExternal(url: string) {
  try {
    BrowserOpenURL(url);
  } catch {
    window.open(url, "_blank", "noreferrer");
  }
}

type Phase =
  | { kind: "offer"; failed: boolean }
  | { kind: "installing" }
  | { kind: "restarting" }
  | { kind: "manual"; reason?: string; releaseUrl: string };

interface Downloaded {
  downloaded: number;
  total: number;
}

function megabytes(bytes: number): string {
  return (bytes / 1_000_000).toFixed(1);
}

// The modal of snz-design doc-9 §6.6. Nothing is downloaded until Update is pressed.
// Once it is, the dialog cannot be closed: InstallUpdate has no way to stop, and a
// dialog that closed would leave the app quitting with no word of why.
function UpdateDialog({ update, onClose }: { update: AvailableUpdate; onClose: () => void }) {
  const { t } = useLanguage();
  const [phase, setPhase] = useState<Phase>({ kind: "offer", failed: false });
  const [progress, setProgress] = useState<Downloaded | null>(null);
  const installingRef = useRef(false);

  useEffect(
    () =>
      EventsOn("update:progress", (next: Downloaded) => {
        if (installingRef.current) {
          setProgress(next);
        }
      }),
    []
  );

  const busy = phase.kind === "installing" || phase.kind === "restarting";

  function requestClose() {
    if (busy) {
      announce(t("update.busyClose"));
      return;
    }
    onClose();
  }

  async function install() {
    installingRef.current = true;
    setProgress(null);
    setPhase({ kind: "installing" });
    announce(t("update.downloading"));
    let result: { status: string; reason?: string; releaseUrl: string };
    try {
      result = await InstallUpdate(update.version);
    } catch {
      result = { status: "failed", releaseUrl: update.releaseUrl };
    }
    installingRef.current = false;
    // Branches on the status and the reason code only, never on error text: every
    // failure after approval leaves the installed app as it was, so the user is told
    // that and nothing more (the reason is in the log).
    if (result.status === "restarting") {
      setPhase({ kind: "restarting" });
      announce(t("update.restarting"));
    } else if (result.status === "manual") {
      setPhase({ kind: "manual", reason: result.reason, releaseUrl: result.releaseUrl || update.releaseUrl });
    } else {
      setPhase({ kind: "offer", failed: true });
    }
  }

  if (phase.kind === "manual") {
    const reasonKey = manualReasonKey(phase.reason);
    return (
      <Dialog onClose={onClose} style={{ width: "min(480px, 100%)" }}>
        <DialogHeader>
          <DialogTitle>{t("update.manualHeading")}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          {reasonKey ? <Words>{t(reasonKey)}</Words> : null}
          <Words>{t("update.manualLead", { version: update.version })}</Words>
        </DialogBody>
        <DialogActions>
          <Button type="button" variant="normal" onClick={onClose}>
            {t("common.close")}
          </Button>
          <Button type="button" variant="primary" onClick={() => openExternal(phase.releaseUrl)}>
            {t("update.openReleases")}
          </Button>
        </DialogActions>
      </Dialog>
    );
  }

  return (
    <Dialog onClose={requestClose} style={{ width: "min(480px, 100%)" }}>
      <DialogHeader>
        <DialogTitle>{t("update.heading")}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <Words>{t("update.versions", { version: update.version, current: update.currentVersion })}</Words>
        <Words>{t("update.howItWorks")}</Words>
        <Subtle>{t("update.systemMayAsk")}</Subtle>
        <p style={{ margin: 0 }}>
          <ExternalLink
            href={update.releaseUrl}
            onClick={(event: MouseEvent<HTMLAnchorElement>) => {
              event.preventDefault();
              openExternal(update.releaseUrl);
            }}
          >
            {t("update.releaseNotes")}
          </ExternalLink>
        </p>
        {busy ? <InstallProgress phase={phase.kind} progress={progress} /> : null}
      </DialogBody>
      <DialogActions
        notice={phase.kind === "offer" && phase.failed ? <FailureNotice>{t("update.failed")}</FailureNotice> : undefined}
      >
        <ActionButton
          type="button"
          variant="normal"
          disabledReason={busy ? t("update.busyClose") : undefined}
          onClick={requestClose}
        >
          {t("update.later")}
        </ActionButton>
        <ActionButton type="button" variant="primary" icon={<DownloadIcon />} busy={busy} onClick={() => void install()}>
          {t("update.install")}
        </ActionButton>
      </DialogActions>
    </Dialog>
  );
}

// No event follows the last downloaded bytes: verifying and replacing report nothing,
// so a full band switches to the words for that step rather than sitting at 100%.
function InstallProgress({ phase, progress }: { phase: "installing" | "restarting"; progress: Downloaded | null }) {
  const { t } = useLanguage();
  if (phase === "restarting") {
    return <Progress label={t("update.restarting")} fullWidth />;
  }
  const known = progress !== null && progress.total > 0;
  if (known && progress.downloaded >= progress.total) {
    return <Progress label={t("update.installing")} fullWidth />;
  }
  return (
    <Progress
      label={t("update.downloading")}
      total={known ? progress.total : undefined}
      done={progress?.downloaded ?? 0}
      readout={
        known
          ? t("update.downloadAmount", {
              done: megabytes(progress.downloaded),
              total: megabytes(progress.total),
              pct: Math.floor((progress.downloaded / progress.total) * 100)
            })
          : undefined
      }
      fullWidth
    />
  );
}

// What the dialog asks about takes the body ink, as in the confirm dialog (doc-9 §6.6).
const Words = styled.p`
  margin: 0;
  color: ${({ theme }) => theme.ink};
  line-height: 1.5;
  overflow-wrap: anywhere;
`;

const ExternalLink = styled.a`
  color: ${({ theme }) => theme.accent};
  border-radius: ${({ theme }) => theme.radiusSm};
  overflow-wrap: anywhere;

  ${focusRing}
`;
