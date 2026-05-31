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
}

const ThemeControllerContext = createContext<ThemeControllerValue | null>(null);

function readFamily(): ThemeFamily {
  if (typeof window === "undefined") {
    return "solarized";
  }
  const stored = window.localStorage.getItem(FAMILY_KEY);
  return stored && isThemeFamily(stored) ? stored : "solarized";
}

function readMode(): ThemeMode {
  if (typeof window === "undefined") {
    return "light";
  }
  const stored = window.localStorage.getItem(MODE_KEY);
  return stored === "light" || stored === "dark" || stored === "auto" ? stored : "light";
}

function prefersDark(): boolean {
  return typeof window !== "undefined" && typeof window.matchMedia === "function" && window.matchMedia(DARK_QUERY).matches;
}

export function ThemeController({ children }: { children: ReactNode }) {
  const [family, setFamilyState] = useState<ThemeFamily>(readFamily);
  const [mode, setModeState] = useState<ThemeMode>(readMode);
  const [systemDark, setSystemDark] = useState<boolean>(prefersDark);

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

  const setFamily = useCallback((next: ThemeFamily) => {
    setFamilyState(next);
    if (typeof window !== "undefined") {
      window.localStorage.setItem(FAMILY_KEY, next);
    }
  }, []);

  const setMode = useCallback((next: ThemeMode) => {
    setModeState(next);
    if (typeof window !== "undefined") {
      window.localStorage.setItem(MODE_KEY, next);
    }
  }, []);

  const variant: ThemeVariant = mode === "auto" ? (systemDark ? "dark" : "light") : mode;
  const tokens = useMemo(() => resolveTokens(family, variant), [family, variant]);

  const value = useMemo<ThemeControllerValue>(
    () => ({ family, mode, variant, families: THEME_FAMILIES, setFamily, setMode }),
    [family, mode, variant, setFamily, setMode]
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
