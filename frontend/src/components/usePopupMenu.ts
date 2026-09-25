import { KeyboardEvent as ReactKeyboardEvent, useCallback, useEffect, useRef, useState } from "react";

// A popup marks itself with this attribute so that a popup nested inside
// another (the chat kind menu inside the collapsed sidebar) keeps its items out
// of the outer one's arrow-key order.
const POPUP_ATTRIBUTE = "data-popup-menu";
const ITEM_SELECTOR = "a[href], button";

type Edge = "first" | "last";

// The open / close and keyboard rules of snz-design doc-9 §6.11, shared by the
// chat kind menu and the collapsed sidebar (doc-9 §6.8 treats the latter as a
// menu while it is open). Focus is returned to the trigger itself rather than
// to whatever held focus when the popup opened: WebKit does not focus a button
// on click, so the latter can be an unrelated control.
export function usePopupMenu<Trigger extends HTMLElement>() {
  const [isOpen, setIsOpen] = useState(false);
  const [openAt, setOpenAt] = useState<Edge>("first");
  const anchorRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<Trigger>(null);
  const popupRef = useRef<HTMLDivElement>(null);

  const close = useCallback((restoreFocus: boolean) => {
    setIsOpen(false);
    if (restoreFocus) {
      triggerRef.current?.focus();
    }
  }, []);

  const open = useCallback((edge: Edge) => {
    setOpenAt(edge);
    setIsOpen(true);
  }, []);

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    const items = menuItems(popupRef.current);
    (openAt === "last" ? items[items.length - 1] : items[0])?.focus();

    // A pointer outside has already chosen where focus should land, so this
    // path closes without pulling focus back to the trigger.
    function handlePointerDown(event: PointerEvent) {
      if (!anchorRef.current?.contains(event.target as Node)) {
        setIsOpen(false);
      }
    }

    document.addEventListener("pointerdown", handlePointerDown);
    return () => document.removeEventListener("pointerdown", handlePointerDown);
  }, [isOpen, openAt]);

  function handleTriggerKeyDown(event: ReactKeyboardEvent<HTMLElement>) {
    if (isOpen || (event.key !== "ArrowDown" && event.key !== "ArrowUp")) {
      return;
    }
    event.preventDefault();
    // The trigger can sit inside another popup, which would move on as well.
    event.stopPropagation();
    open(event.key === "ArrowDown" ? "first" : "last");
  }

  function handlePopupKeyDown(event: ReactKeyboardEvent<HTMLElement>) {
    if (event.nativeEvent.isComposing) {
      return;
    }

    if (event.key === "Tab") {
      // Tab moves on to its own next stop and is never dragged back to the
      // trigger. The check waits for the browser's own move: closing first
      // would unmount the focused item before focus leaves it. A popup whose
      // items stay in the Tab order (the collapsed sidebar's links) therefore
      // closes only once Tab has left it.
      window.setTimeout(() => {
        if (!popupRef.current?.contains(document.activeElement)) {
          close(false);
        }
      });
      return;
    }

    if (event.key === "Escape") {
      // Only the innermost popup closes (snz-design doc-9 §5.2).
      event.stopPropagation();
      close(true);
      return;
    }

    const items = menuItems(popupRef.current);
    const current = items.indexOf(document.activeElement as HTMLElement);
    let next: number;
    if (event.key === "ArrowDown") {
      next = (current + 1) % items.length;
    } else if (event.key === "ArrowUp") {
      // current is -1 while the popup itself holds focus (after a press on its
      // padding); ArrowUp then goes to the last item, as from the trigger.
      next = current < 0 ? items.length - 1 : (current - 1 + items.length) % items.length;
    } else if (event.key === "Home") {
      next = 0;
    } else if (event.key === "End") {
      next = items.length - 1;
    } else {
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    items[next]?.focus();
  }

  return {
    isOpen,
    open,
    close,
    toggle: () => (isOpen ? close(true) : open("first")),
    anchorRef,
    triggerRef,
    popupRef,
    triggerProps: { "aria-expanded": isOpen, onKeyDown: handleTriggerKeyDown },
    popupProps: { "data-popup-menu": "", onKeyDown: handlePopupKeyDown }
  };
}

function menuItems(popup: HTMLElement | null): HTMLElement[] {
  if (!popup) {
    return [];
  }
  return Array.from(popup.querySelectorAll<HTMLElement>(ITEM_SELECTOR)).filter(
    (item) => item.closest(`[${POPUP_ATTRIBUTE}]`) === popup
  );
}
