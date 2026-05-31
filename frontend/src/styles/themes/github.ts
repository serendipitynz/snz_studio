import { buildTokens, ThemeTokens } from "./types";
import { LIGHT_SURFACES } from "./presets";

// GitHub Light — Primer color anchors. The canvas is the subtle grey so white
// cards (LIGHT_SURFACES) read as the familiar GitHub card-on-grey look.
const light: ThemeTokens = buildTokens({
  scheme: "light",
  ink: "#1f2328", // fg.default
  muted: "#656d76", // fg.muted
  accent: "#0969da", // accent.fg
  warm: "#9a6700", // attention.fg
  danger: "#cf222e", // danger.fg
  dangerText: "#cf222e",
  fieldBorder: "#d0d7de", // border.default
  bg: "#f6f8fa", // canvas.subtle
  bgGradientFrom: "#f6f8fa",
  bgGradientTo: "#eef1f4",
  accentRgb: "9, 105, 218",
  warmRgb: "154, 103, 0",
  dangerRgb: "207, 34, 46",
  lineRgb: "175, 184, 193",
  shadowRgb: "140, 149, 159",
  ...LIGHT_SURFACES
});

// GitHub Dark — canvas/border/fg ramp.
const dark: ThemeTokens = buildTokens({
  scheme: "dark",
  ink: "#e6edf3", // fg.default
  muted: "#8b949e", // fg.muted
  accent: "#2f81f7", // accent.fg
  warm: "#d29922", // attention.fg
  danger: "#f85149", // danger.fg
  dangerText: "#f85149",
  fieldBorder: "#30363d", // border.default
  bg: "#0d1117", // canvas.default
  bgGradientFrom: "#0d1117",
  bgGradientTo: "#161b22", // canvas.subtle
  accentRgb: "47, 129, 247",
  warmRgb: "210, 153, 34",
  dangerRgb: "248, 81, 73",
  lineRgb: "139, 148, 158",
  shadowRgb: "0, 0, 0",
  surfacePane: "#161b22", // canvas.subtle
  surfaceCard: "#1c2128",
  surfaceElevate: "#21262d",
  surfaceCardFaint: "#21262d",
  surfaceField: "#0d1117",
  surfaceButton: "#21262d",
  surfaceDropzone: "#161b22",
  paneHeaderFrom: "#21262d",
  paneHeaderTo: "#161b22",
  composerFrom: "rgba(13, 17, 23, 0.2)",
  composerTo: "rgba(22, 27, 34, 0.86)",
  floatBtnBg: "#1c2128",
  modalScrim: "rgba(1, 4, 9, 0.7)",
  overlayScale: 1.5
});

export const github = { light, dark };
