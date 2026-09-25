import { useEffect, useId, useRef, useState } from "react";
import { useLanguage } from "../i18n";
import { SIDE_REGION_MEDIA } from "../styles/ui";
import { announce } from "./announce";
import { useMediaQuery } from "./useMediaQuery";

function readHidden(storageKey: string): boolean {
  try {
    return window.localStorage.getItem(storageKey) === "true";
  } catch {
    return false;
  }
}

function writeHidden(storageKey: string, hidden: boolean) {
  try {
    window.localStorage.setItem(storageKey, String(hidden));
  } catch {
    // Unsaved, the choice still holds on this screen.
  }
}

// A side region shown and hidden by a trigger outside it (snz-design doc-9 §6.3.1).
// On a wide screen it is a column beside the conversation, the user's choice is
// stored under storageKey and focus stays on the trigger. Below SIDE_REGION_MEDIA
// it starts hidden without writing that choice, and showing it lays it over the
// conversation's right edge (the owner's call, 2026-09-26). That overlay is not
// modal: focus moves into it and goes back to the trigger when it closes (by its
// close button, Escape or the trigger), but it is not confined, so the
// conversation stays usable beside it. Neither is the overlay stored.
// name is the region's name in the words read out and on the close button.
export function useSideRegion(storageKey: string, name: string) {
  const { t } = useLanguage();
  const regionId = useId();
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const regionRef = useRef<HTMLElement | null>(null);
  const closeRef = useRef<HTMLButtonElement | null>(null);
  const [hiddenWide, setHiddenWide] = useState(() => readHidden(storageKey));
  const [shownNarrow, setShownNarrow] = useState(false);
  const narrow = useMediaQuery(SIDE_REGION_MEDIA);
  const shown = narrow ? shownNarrow : !hiddenWide;
  const overlay = narrow && shown;
  const hiddenWideRef = useRef(hiddenWide);
  hiddenWideRef.current = hiddenWide;

  // Crossing the width hides the region on its own (a narrow screen always starts
  // with it hidden; a wide one goes back to the stored choice). The region is
  // still on screen while this runs, so focus inside it can be moved to the
  // trigger before the region takes it away.
  useEffect(() => {
    const list = window.matchMedia(SIDE_REGION_MEDIA);
    const handleChange = () => {
      const willShow = list.matches ? false : !hiddenWideRef.current;
      if (!willShow && regionRef.current?.contains(document.activeElement)) {
        triggerRef.current?.focus();
      }
      setShownNarrow(false);
    };
    list.addEventListener("change", handleChange);
    return () => list.removeEventListener("change", handleChange);
  }, []);

  function closeOverlay() {
    setShownNarrow(false);
    triggerRef.current?.focus();
    announce(t("region.hidden", { name }));
  }

  useEffect(() => {
    if (!overlay) {
      return;
    }
    // Opening lays the region over the trigger, so focus goes into it.
    closeRef.current?.focus();

    // Escape closes the overlay, unless something inside it (a hint) or a dialog
    // on top has already taken this Escape: only the innermost closes (doc-9 §5.2).
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented || event.isComposing || event.keyCode === 229) {
        return;
      }
      if (document.querySelector('[role="dialog"][aria-modal="true"]')) {
        return;
      }
      closeOverlay();
    };
    // Focus moving onto a control the overlay covers entirely closes it, so the
    // focused control is never hidden behind it (WCAG 2.4.11). Focus on a control
    // still partly in view (the composer) leaves it open.
    const handleFocusIn = (event: FocusEvent) => {
      const region = regionRef.current;
      const target = event.target;
      if (!region || !(target instanceof HTMLElement) || region.contains(target)) {
        return;
      }
      const a = region.getBoundingClientRect();
      const b = target.getBoundingClientRect();
      if (b.left >= a.left && b.right <= a.right && b.top >= a.top && b.bottom <= a.bottom) {
        setShownNarrow(false);
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    document.addEventListener("focusin", handleFocusIn);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      document.removeEventListener("focusin", handleFocusIn);
    };
  }, [overlay]);

  function toggle() {
    if (narrow) {
      if (shownNarrow) {
        closeOverlay();
      } else {
        setShownNarrow(true);
      }
      return;
    }
    const next = hiddenWide;
    if (!next && regionRef.current?.contains(document.activeElement)) {
      triggerRef.current?.focus();
    }
    setHiddenWide(!next);
    writeHidden(storageKey, !next);
    // The column comes after the whole conversation in reading order, so where it
    // appeared is said rather than left to be found.
    announce(next ? t("region.shownBeside", { name }) : t("region.hidden", { name }));
  }

  return {
    shown,
    overlay,
    triggerProps: {
      ref: triggerRef,
      "aria-expanded": shown,
      "aria-controls": regionId,
      onClick: toggle
    },
    regionProps: {
      ref: regionRef,
      id: regionId,
      hidden: !shown,
      $overlay: overlay
    },
    closeProps: {
      ref: closeRef,
      "aria-label": t("region.close", { name }),
      title: t("region.close", { name }),
      onClick: closeOverlay
    }
  };
}
