import { buildTokens, ThemeTokens } from "./types";

// Solarized Light is ported 1:1 from the original single theme object + ui.tsx
// literals so the current look is preserved pixel-for-pixel.
const light: ThemeTokens = buildTokens({
  scheme: "light",
  ink: "#2f3b42",
  muted: "#657b83",
  accent: "#2aa198",
  warm: "#b58900",
  danger: "#dc322f",
  dangerText: "#dc322f",
  fieldBorder: "#d9d2bf",
  bg: "#fdf6e3",
  bgGradientFrom: "#fdf6e3",
  bgGradientTo: "#f7f0da",
  accentRgb: "42, 161, 152",
  warmRgb: "181, 137, 0",
  dangerRgb: "220, 50, 47",
  lineRgb: "101, 123, 131",
  shadowRgb: "88, 110, 117",
  surfacePane: "rgba(255, 251, 240, 0.92)",
  surfaceCard: "#fffaf0",
  surfaceElevate: "rgba(255, 255, 255, 0.42)",
  surfaceCardFaint: "rgba(255, 255, 255, 0.46)",
  surfaceField: "rgba(255, 255, 255, 0.72)",
  surfaceButton: "rgba(255, 255, 255, 0.52)",
  surfaceDropzone: "rgba(255, 255, 255, 0.34)",
  paneHeaderFrom: "rgba(255, 255, 255, 0.55)",
  paneHeaderTo: "rgba(255, 250, 240, 0.35)",
  composerFrom: "rgba(253, 246, 227, 0.2)",
  composerTo: "rgba(247, 240, 218, 0.86)",
  floatBtnBg: "rgba(255, 250, 240, 0.94)",
  modalScrim: "rgba(47, 59, 66, 0.34)"
});

// Solarized Dark uses the canonical base03/02/01/00 ramp with the same accents.
const dark: ThemeTokens = buildTokens({
  scheme: "dark",
  ink: "#eee8d5",
  muted: "#93a1a1",
  accent: "#2aa198",
  warm: "#b58900",
  danger: "#dc322f",
  dangerText: "#dc322f",
  fieldBorder: "#586e75",
  bg: "#002b36",
  bgGradientFrom: "#002b36",
  bgGradientTo: "#073642",
  accentRgb: "42, 161, 152",
  warmRgb: "181, 137, 0",
  dangerRgb: "220, 50, 47",
  lineRgb: "131, 148, 150",
  shadowRgb: "0, 0, 0",
  surfacePane: "#073642",
  surfaceCard: "#08404f",
  surfaceElevate: "#0a4150",
  surfaceCardFaint: "#0a4150",
  surfaceField: "#00252e",
  surfaceButton: "#0a4150",
  surfaceDropzone: "#04303b",
  paneHeaderFrom: "#0a4150",
  paneHeaderTo: "#073642",
  composerFrom: "rgba(0, 43, 54, 0.2)",
  composerTo: "rgba(7, 54, 66, 0.86)",
  floatBtnBg: "#08404f",
  modalScrim: "rgba(0, 0, 0, 0.55)",
  overlayScale: 1.5
});

export const solarized = { light, dark };
