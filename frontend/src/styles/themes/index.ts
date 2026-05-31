import { catppuccin } from "./catppuccin";
import { github } from "./github";
import { one } from "./one";
import { rosePine } from "./rosePine";
import { solarized } from "./solarized";
import { tokyoNight } from "./tokyoNight";
import type { ThemeFamily, ThemeTokens, ThemeVariant } from "./types";

export type { ThemeFamily, ThemeTokens, ThemeVariant };

// PALETTES maps every family to its { light, dark } token sets.
export const PALETTES: Record<ThemeFamily, { light: ThemeTokens; dark: ThemeTokens }> = {
  solarized,
  catppuccin,
  rosePine,
  tokyoNight,
  github,
  one
};

// THEME_FAMILIES drives the switcher UI (ordered, with display labels).
export const THEME_FAMILIES: { id: ThemeFamily; label: string }[] = [
  { id: "solarized", label: "Solarized" },
  { id: "catppuccin", label: "Catppuccin" },
  { id: "rosePine", label: "Rosé Pine" },
  { id: "tokyoNight", label: "Tokyo Night" },
  { id: "github", label: "GitHub" },
  { id: "one", label: "One" }
];

export function isThemeFamily(value: string): value is ThemeFamily {
  return value in PALETTES;
}

export function resolveTokens(family: ThemeFamily, variant: ThemeVariant): ThemeTokens {
  return PALETTES[family][variant];
}
