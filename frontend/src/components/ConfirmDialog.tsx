import { createContext, ReactNode, useCallback, useContext, useMemo, useRef, useState } from "react";
import { useLanguage } from "../i18n";
import { Button, Row, Stack, Subtle } from "../styles/ui";
import { Dialog, DialogTitle } from "./Dialog";

// window.confirm / alert / prompt must not be used anywhere in this app: Wails' macOS
// WebView declares WKUIDelegate without implementing the panel callbacks, so WKWebView
// resolves confirm() to false without ever showing a dialog and swallows the other two.
// This provider is the replacement. Wails' own runtime MessageDialog was rejected because
// it is unavailable when the SPA runs under `vite` during development.
// The words are per call because a destructive confirm names the operation on its
// button and a discard confirm offers to keep editing (snz-design doc-9 §5.7). Callers
// that pass none still get Cancel / OK.
export interface ConfirmOptions {
  heading?: string;
  confirmLabel?: string;
  cancelLabel?: string;
}

type Confirm = (message: string, options?: ConfirmOptions) => Promise<boolean>;

interface ConfirmContextValue {
  confirm: Confirm;
}

const ConfirmContext = createContext<ConfirmContextValue | null>(null);

// Only one dialog is ever mounted, so fixed ids cannot collide.
const MESSAGE_ID = "confirm-dialog-message";
const HEADING_ID = "confirm-dialog-heading";

interface PendingConfirm {
  message: string;
  options: ConfirmOptions;
  settle: (accepted: boolean) => void;
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const { t } = useLanguage();
  const [pending, setPending] = useState<PendingConfirm | null>(null);
  const pendingRef = useRef<PendingConfirm | null>(null);

  const close = useCallback((accepted: boolean) => {
    pendingRef.current?.settle(accepted);
    pendingRef.current = null;
    setPending(null);
  }, []);

  const confirm = useCallback(
    (message: string, options: ConfirmOptions = {}) =>
      new Promise<boolean>((resolve) => {
        // Only one dialog can be on screen, so a second request has to decline the first
        // rather than replace it: an unsettled promise would leave its caller — the document
        // upload loop, which awaits per file — waiting forever with its busy flag stuck on.
        pendingRef.current?.settle(false);
        const next = { message, options, settle: resolve };
        pendingRef.current = next;
        setPending(next);
      }),
    []
  );

  const value = useMemo(() => ({ confirm }), [confirm]);

  return (
    <ConfirmContext.Provider value={value}>
      {children}
      {pending ? (
        // Escape cancels, as window.confirm did natively. It resolves false, which is the
        // safe direction for every current caller.
        <Dialog
          onClose={() => close(false)}
          labelledBy={pending.options.heading ? HEADING_ID : MESSAGE_ID}
          describedBy={pending.options.heading ? MESSAGE_ID : undefined}
          style={{ width: "min(420px, 100%)" }}
        >
          <Stack>
            {pending.options.heading ? <DialogTitle id={HEADING_ID}>{pending.options.heading}</DialogTitle> : null}
            <Subtle id={MESSAGE_ID}>{pending.message}</Subtle>
            <Row style={{ justifyContent: "flex-end" }}>
              {/* Cancel takes the initial focus: every current caller asks about a destructive
                  action, so a stray Enter right after the click must not confirm one. */}
              <Button type="button" variant="normal" autoFocus onClick={() => close(false)}>
                {pending.options.cancelLabel ?? t("common.cancel")}
              </Button>
              <Button type="button" variant="danger" onClick={() => close(true)}>
                {pending.options.confirmLabel ?? t("common.ok")}
              </Button>
            </Row>
          </Stack>
        </Dialog>
      ) : null}
    </ConfirmContext.Provider>
  );
}

export function useConfirm(): Confirm {
  const context = useContext(ConfirmContext);
  if (!context) {
    throw new Error("useConfirm must be used inside ConfirmProvider");
  }
  return context.confirm;
}
