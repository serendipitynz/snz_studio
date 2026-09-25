import { ThemeProvider } from "@emotion/react";
import { createContext, ReactNode, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { isThemeFamily, resolveTokens, THEME_FAMILIES } from "./themes";
import type { ThemeFamily, ThemeVariant } from "./themes";

export type ThemeMode = "light" | "dark" | "auto";

const FAMILY_KEY = "snz.theme.family";
const MODE_KEY = "snz.theme.mode";
const DARK_QUERY = "(prefers-color-scheme: dark)";

interface ThemeControllerValue {
  family: ThemeFamily;
  mode: ThemeMode;
  variant: ThemeVariant; // resolved (auto -> light/dark)
  families: typeof THEME_FAMILIES;
  setFamily: (family: ThemeFamily) => void;
  setMode: (mode: ThemeMode) => void;
  // The stored choice names a family or a mode this build does not have, so the
  // shared default is drawn in its place (snz-design doc-7 §5.1). It stays true
  // until a choice made here is saved over it.
  storedChoiceUnknown: boolean;
  // The last choice applies for this run but could not be stored (doc-7 §5.3).
  saveFailed: boolean;
}

const ThemeControllerContext = createContext<ThemeControllerValue | null>(null);

interface StoredChoice {
  family: ThemeFamily;
  mode: ThemeMode;
  unknown?: boolean;
}

function readKey(key: string): string | null {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

// The two keys are written independently, so a user who changed only one of
// them has the other missing. Only when both are missing does the shared
// default (standard + auto) apply; a missing half otherwise keeps the default
// this build had before (solarized / light), so the axis the user never
// touched does not change under them (snz-design doc-7 §6.2). A value this
// build does not recognise draws the shared default and is not rewritten
// (doc-7 §5.1).
const SHARED_DEFAULT: StoredChoice = { family: "standard", mode: "auto" };

function isThemeMode(value: string): value is ThemeMode {
  return value === "light" || value === "dark" || value === "auto";
}

function readStoredChoice(): StoredChoice {
  if (typeof window === "undefined") {
    return SHARED_DEFAULT;
  }
  const storedFamily = readKey(FAMILY_KEY);
  const storedMode = readKey(MODE_KEY);
  if (storedFamily === null && storedMode === null) {
    return SHARED_DEFAULT;
  }
  if ((storedFamily !== null && !isThemeFamily(storedFamily)) || (storedMode !== null && !isThemeMode(storedMode))) {
    return { ...SHARED_DEFAULT, unknown: true };
  }
  return { family: storedFamily ?? "solarized", mode: storedMode ?? "light" };
}

// A failed write still applies the choice for this run (snz-design doc-7
// §5.3); the settings modal tells the user it was not saved.
function writeChoice(choice: StoredChoice): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  try {
    window.localStorage.setItem(FAMILY_KEY, choice.family);
    window.localStorage.setItem(MODE_KEY, choice.mode);
    return true;
  } catch {
    return false;
  }
}

function prefersDark(): boolean {
  return typeof window !== "undefined" && typeof window.matchMedia === "function" && window.matchMedia(DARK_QUERY).matches;
}

export function ThemeController({ children }: { children: ReactNode }) {
  const [initial] = useState<StoredChoice>(readStoredChoice);
  const [family, setFamilyState] = useState<ThemeFamily>(initial.family);
  const [mode, setModeState] = useState<ThemeMode>(initial.mode);
  const [systemDark, setSystemDark] = useState<boolean>(prefersDark);
  const [storedChoiceUnknown, setStoredChoiceUnknown] = useState(Boolean(initial.unknown));
  const [saveFailed, setSaveFailed] = useState(false);

  // Track the OS appearance only matters when mode === "auto", but the listener is
  // cheap so keep it always attached and resolve lazily below.
  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }
    const query = window.matchMedia(DARK_QUERY);
    const onChange = (event: MediaQueryListEvent) => setSystemDark(event.matches);
    query.addEventListener("change", onChange);
    return () => query.removeEventListener("change", onChange);
  }, []);

  // Every choice stores both axes. Writing only the changed key would leave
  // the other one missing, and on the next launch the missing half falls back
  // to this build's old default (readStoredChoice) rather than to what was on
  // screen: a new user who picked only Dark would come back to Solarized Dark.
  // An unknown stored value is replaced only here, by a choice the user made.
  const store = useCallback((choice: StoredChoice) => {
    const saved = writeChoice(choice);
    setSaveFailed(!saved);
    if (saved) {
      setStoredChoiceUnknown(false);
    }
  }, []);

  const setFamily = useCallback(
    (next: ThemeFamily) => {
      setFamilyState(next);
      store({ family: next, mode });
    },
    [mode, store]
  );

  const setMode = useCallback(
    (next: ThemeMode) => {
      setModeState(next);
      store({ family, mode: next });
    },
    [family, store]
  );

  const variant: ThemeVariant = mode === "auto" ? (systemDark ? "dark" : "light") : mode;
  const tokens = useMemo(() => resolveTokens(family, variant), [family, variant]);

  const value = useMemo<ThemeControllerValue>(
    () => ({
      family,
      mode,
      variant,
      families: THEME_FAMILIES,
      setFamily,
      setMode,
      storedChoiceUnknown,
      saveFailed
    }),
    [family, mode, variant, setFamily, setMode, storedChoiceUnknown, saveFailed]
  );

  return (
    <ThemeControllerContext.Provider value={value}>
      <ThemeProvider theme={tokens}>{children}</ThemeProvider>
    </ThemeControllerContext.Provider>
  );
}

export function useThemeController(): ThemeControllerValue {
  const value = useContext(ThemeControllerContext);
  if (!value) {
    throw new Error("useThemeController must be used within a ThemeController");
  }
  return value;
}
