import { buildTokens, ThemeTokens } from "./types";
import { LIGHT_SURFACES } from "./presets";

// Tokyo Night Day (light) — the "day" variant anchors.
const light: ThemeTokens = buildTokens({
  scheme: "light",
  ink: "#343b58",
  muted: "#6c75a3",
  accent: "#118c74", // teal
  warm: "#8c6c3e", // yellow
  danger: "#f52a65", // red
  dangerText: "#f52a65",
  fieldBorder: "#c4c8da",
  bg: "#e1e2e7",
  bgGradientFrom: "#e1e2e7",
  bgGradientTo: "#d5d9e6",
  accentRgb: "17, 140, 116",
  warmRgb: "140, 108, 62",
  dangerRgb: "245, 42, 101",
  lineRgb: "132, 140, 181", // comment
  shadowRgb: "160, 166, 196",
  ...LIGHT_SURFACES
});

// Tokyo Night (dark) — the classic "night" variant.
const dark: ThemeTokens = buildTokens({
  scheme: "dark",
  ink: "#c0caf5", // fg
  muted: "#7982a9",
  accent: "#73daca", // teal
  warm: "#e0af68", // yellow
  danger: "#f7768e", // red
  dangerText: "#f7768e",
  fieldBorder: "#2f3549",
  bg: "#1a1b26", // bg
  bgGradientFrom: "#1a1b26",
  bgGradientTo: "#16161e", // bg_dark
  accentRgb: "115, 218, 202",
  warmRgb: "224, 175, 104",
  dangerRgb: "247, 118, 142",
  lineRgb: "86, 95, 137", // comment
  shadowRgb: "0, 0, 0",
  surfacePane: "#16161e", // bg_dark
  surfaceCard: "#1f2030",
  surfaceElevate: "#292e42", // bg_highlight
  surfaceCardFaint: "#292e42",
  surfaceField: "#16161e",
  surfaceButton: "#292e42",
  surfaceDropzone: "#1a1b26",
  paneHeaderFrom: "#292e42",
  paneHeaderTo: "#1a1b26",
  composerFrom: "rgba(26, 27, 38, 0.2)",
  composerTo: "rgba(22, 22, 30, 0.86)",
  floatBtnBg: "#1f2030",
  modalScrim: "rgba(0, 0, 0, 0.62)",
  overlayScale: 1.5
});

export const tokyoNight = { light, dark };
