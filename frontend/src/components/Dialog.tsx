import { createContext, CSSProperties, ReactNode, useContext, useEffect, useId, useRef, useState } from "react";
import { ModalCard, ModalOverlay, SectionTitle } from "../styles/ui";

// Innermost last. A confirm dialog raised from inside another dialog must take Escape and
// Tab alone: both listeners sit on document, so without this one Escape would close both.
const openDialogs: object[] = [];

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

const DialogTitleIdContext = createContext<string | undefined>(undefined);

interface DialogProps {
  onClose: () => void;
  children: ReactNode;
  // Overrides the DialogTitle id, for a dialog whose label is not a heading.
  labelledBy?: string;
  describedBy?: string;
  // For a dialog opened after an await: by then focus may no longer be on the control the
  // user activated, so the default — whatever was focused when the dialog first rendered —
  // would miss it.
  returnFocusTo?: HTMLElement | null;
  style?: CSSProperties;
}

export function Dialog({ onClose, children, labelledBy, describedBy, returnFocusTo, style }: DialogProps) {
  const titleId = useId();
  const cardRef = useRef<HTMLDivElement | null>(null);
  // Read during the first render, not in an effect: React applies autoFocus during commit,
  // so by the time any effect runs the focus has already moved into the dialog.
  const [opener] = useState(() => (document.activeElement instanceof HTMLElement ? document.activeElement : null));
  const onCloseRef = useRef(onClose);
  const returnFocusRef = useRef(returnFocusTo);

  useEffect(() => {
    onCloseRef.current = onClose;
    returnFocusRef.current = returnFocusTo;
  });

  useEffect(() => {
    const card = cardRef.current;
    if (!card) {
      return;
    }

    const token = {};
    openDialogs.push(token);
    // An autoFocus child already holds focus; otherwise the card takes it so the first Tab
    // starts inside the dialog rather than in the page behind it.
    if (!card.contains(document.activeElement)) {
      card.focus();
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (openDialogs[openDialogs.length - 1] !== token) {
        return;
      }
      // Escape during IME conversion cancels the conversion; it must not also close the
      // dialog and drop what was typed. Same check as the chat composer's Enter.
      if (event.key === "Escape" && !event.isComposing && event.keyCode !== 229) {
        onCloseRef.current();
        return;
      }
      if (event.key === "Tab") {
        keepTabInside(event, card);
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      openDialogs.splice(openDialogs.indexOf(token), 1);
      const target = returnFocusRef.current ?? opener;
      // Deferred and gated on the card being gone: StrictMode runs this cleanup once on a
      // dialog that stays mounted, and focusing the opener then would pull focus off the
      // dialog's autoFocus control when the effect re-runs.
      queueMicrotask(() => {
        if (!card.isConnected && target?.isConnected) {
          target.focus();
        }
      });
    };
  }, [opener]);

  return (
    <ModalOverlay>
      <ModalCard
        ref={cardRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy ?? titleId}
        aria-describedby={describedBy}
        tabIndex={-1}
        style={{ outline: "none", ...style }}
      >
        <DialogTitleIdContext.Provider value={titleId}>{children}</DialogTitleIdContext.Provider>
      </ModalCard>
    </ModalOverlay>
  );
}

export function DialogTitle({ children, id }: { children: ReactNode; id?: string }) {
  const contextId = useContext(DialogTitleIdContext);
  return <SectionTitle id={id ?? contextId}>{children}</SectionTitle>;
}

// The overlay hides the page's controls without taking them out of the tab order, so Tab
// has to be wrapped by hand.
function keepTabInside(event: KeyboardEvent, card: HTMLElement) {
  // getClientRects drops display:none controls, such as the hidden file input in the
  // document upload dialog, which focus() would silently ignore.
  const focusable = Array.from(card.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
    (element) => element.getClientRects().length > 0
  );
  if (focusable.length === 0) {
    event.preventDefault();
    card.focus();
    return;
  }

  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  const active = document.activeElement;
  const outside = !card.contains(active);
  if (event.shiftKey && (active === first || active === card || outside)) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && (active === last || outside)) {
    event.preventDefault();
    first.focus();
  }
}
