import { useEffect, useId, useRef, useState } from "react";
import { useLanguage } from "../i18n";
import { NARROW_MEDIA, SIDE_REGION_MEDIA } from "../styles/ui";
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
// On a wide screen the user's choice is stored under storageKey. Below
// SIDE_REGION_MEDIA the region is hidden on its own, without writing that choice,
// and the trigger stays, so the region can still be shown there; that is not stored
// either, and a wide screen goes back to the stored choice.
// name is the region's name in the words read out when it is shown or hidden.
export function useSideRegion(storageKey: string, name: string) {
  const { t } = useLanguage();
  const regionId = useId();
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const regionRef = useRef<HTMLElement | null>(null);
  const [hiddenWide, setHiddenWide] = useState(() => readHidden(storageKey));
  const [shownNarrow, setShownNarrow] = useState(false);
  const narrow = useMediaQuery(SIDE_REGION_MEDIA);
  const stacked = useMediaQuery(NARROW_MEDIA);
  const shown = narrow ? shownNarrow : !hiddenWide;
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

  function toggle() {
    const next = !shown;
    if (!next && regionRef.current?.contains(document.activeElement)) {
      triggerRef.current?.focus();
    }
    if (narrow) {
      setShownNarrow(next);
    } else {
      setHiddenWide(!next);
      writeHidden(storageKey, !next);
    }
    // The region comes after the whole conversation in reading order, so where it
    // appeared is said rather than left to be found.
    announce(next ? t(stacked ? "region.shownBelow" : "region.shownBeside", { name }) : t("region.hidden", { name }));
  }

  return {
    shown,
    triggerProps: {
      ref: triggerRef,
      "aria-expanded": shown,
      "aria-controls": regionId,
      onClick: toggle
    },
    regionProps: {
      ref: regionRef,
      id: regionId,
      hidden: !shown
    }
  };
}
