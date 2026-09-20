import { createContext, ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import styled from "@emotion/styled";
import { useLanguage } from "../i18n";
import { Button, ModalCard, ModalOverlay, Row, Stack, Subtle } from "../styles/ui";

// window.confirm / alert / prompt must not be used anywhere in this app: Wails' macOS
// WebView declares WKUIDelegate without implementing the panel callbacks, so WKWebView
// resolves confirm() to false without ever showing a dialog and swallows the other two.
// This provider is the replacement. Wails' own runtime MessageDialog was rejected because
// it is unavailable when the SPA runs under `vite` during development.
interface ConfirmContextValue {
  confirm: (message: string) => Promise<boolean>;
}

const ConfirmContext = createContext<ConfirmContextValue | null>(null);

const ConfirmCard = styled(ModalCard)`
  width: min(420px, 100%);
`;

// Only one dialog is ever mounted, so a fixed id cannot collide.
const MESSAGE_ID = "confirm-dialog-message";

interface PendingConfirm {
  message: string;
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
    (message: string) =>
      new Promise<boolean>((resolve) => {
        // Only one dialog can be on screen, so a second request has to decline the first
        // rather than replace it: an unsettled promise would leave its caller — the document
        // upload loop, which awaits per file — waiting forever with its busy flag stuck on.
        pendingRef.current?.settle(false);
        const next = { message, settle: resolve };
        pendingRef.current = next;
        setPending(next);
      }),
    []
  );

  // Escape cancels. window.confirm answered the key natively and this replaces it, so
  // without the listener the dialog would be the one thing on screen the keyboard cannot
  // dismiss; it resolves false, which is the safe direction for every current caller.
  useEffect(() => {
    if (!pending) {
      return;
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        close(false);
      }
    }

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [pending, close]);

  const value = useMemo(() => ({ confirm }), [confirm]);

  return (
    <ConfirmContext.Provider value={value}>
      {children}
      {pending ? (
        <ModalOverlay>
          <ConfirmCard role="dialog" aria-modal="true" aria-labelledby={MESSAGE_ID}>
            <Stack>
              <Subtle id={MESSAGE_ID}>{pending.message}</Subtle>
              <Row style={{ justifyContent: "flex-end" }}>
                {/* Cancel takes the initial focus: every current caller asks about a destructive
                    action, so a stray Enter right after the click must not confirm one. */}
                <Button type="button" variant="ghost" autoFocus onClick={() => close(false)}>
                  {t("common.cancel")}
                </Button>
                <Button type="button" variant="warm" onClick={() => close(true)}>
                  {t("common.ok")}
                </Button>
              </Row>
            </Stack>
          </ConfirmCard>
        </ModalOverlay>
      ) : null}
    </ConfirmContext.Provider>
  );
}

export function useConfirm(): (message: string) => Promise<boolean> {
  const context = useContext(ConfirmContext);
  if (!context) {
    throw new Error("useConfirm must be used inside ConfirmProvider");
  }
  return context.confirm;
}
