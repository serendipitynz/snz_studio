import { buildTokens, ThemeTokens } from "./types";
import { LIGHT_SURFACES } from "./presets";

// Rosé Pine Dawn (light) — official palette.
const light: ThemeTokens = buildTokens({
  scheme: "light",
  ink: "#575279", // text
  muted: "#797593", // subtle
  accent: "#56949f", // foam
  warm: "#ea9d34", // gold
  danger: "#b4637a", // love
  dangerText: "#b4637a",
  fieldBorder: "#dfdad9", // highlightMed
  bg: "#faf4ed", // base
  bgGradientFrom: "#faf4ed",
  bgGradientTo: "#f2e9e1", // overlay
  accentRgb: "86, 148, 159",
  warmRgb: "234, 157, 52",
  dangerRgb: "180, 99, 122",
  lineRgb: "152, 147, 165", // muted
  shadowRgb: "152, 147, 165",
  ...LIGHT_SURFACES
});

// Rosé Pine Moon (dark) — official base/surface/overlay/highlight ramp.
const dark: ThemeTokens = buildTokens({
  scheme: "dark",
  ink: "#e0def4", // text
  muted: "#908caa", // subtle
  accent: "#9ccfd8", // foam
  warm: "#f6c177", // gold
  danger: "#eb6f92", // love
  dangerText: "#eb6f92",
  fieldBorder: "#44415a", // highlightMed
  bg: "#232136", // base
  bgGradientFrom: "#232136",
  bgGradientTo: "#2a273f", // surface
  accentRgb: "156, 207, 216",
  warmRgb: "246, 193, 119",
  dangerRgb: "235, 111, 146",
  lineRgb: "144, 140, 170",
  shadowRgb: "0, 0, 0",
  surfacePane: "#2a273f", // surface
  surfaceCard: "#393552", // overlay
  surfaceElevate: "#393552",
  surfaceCardFaint: "#393552",
  surfaceField: "#1c1a2e",
  surfaceButton: "#393552",
  surfaceDropzone: "#2a273f",
  paneHeaderFrom: "#393552",
  paneHeaderTo: "#2a273f",
  composerFrom: "rgba(42, 39, 63, 0.2)",
  composerTo: "rgba(35, 33, 54, 0.86)",
  floatBtnBg: "#393552",
  modalScrim: "rgba(0, 0, 0, 0.6)",
  overlayScale: 1.5
});

export const rosePine = { light, dark };
