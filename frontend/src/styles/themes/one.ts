import { buildTokens, ThemeTokens } from "./types";
import { LIGHT_SURFACES } from "./presets";

// Atom One Light — official syntax anchors.
const light: ThemeTokens = buildTokens({
  scheme: "light",
  ink: "#383a42", // fg
  muted: "#a0a1a7", // comment
  accent: "#0184bc", // cyan
  warm: "#c18401", // orange/yellow
  danger: "#e45649", // red
  dangerText: "#e45649",
  fieldBorder: "#dbdbdc",
  bg: "#fafafa",
  bgGradientFrom: "#fafafa",
  bgGradientTo: "#f0f0f1",
  accentRgb: "1, 132, 188",
  warmRgb: "193, 132, 1",
  dangerRgb: "228, 86, 73",
  lineRgb: "160, 161, 167",
  shadowRgb: "160, 161, 167",
  ...LIGHT_SURFACES
});

// Atom One Dark — official background/fg/comment ramp.
const dark: ThemeTokens = buildTokens({
  scheme: "dark",
  ink: "#abb2bf", // fg
  muted: "#8b919e",
  accent: "#56b6c2", // cyan
  warm: "#e5c07b", // yellow
  danger: "#e06c75", // red
  dangerText: "#e06c75",
  fieldBorder: "#3e4451", // visual
  bg: "#282c34", // bg
  bgGradientFrom: "#282c34",
  bgGradientTo: "#21252b",
  accentRgb: "86, 182, 194",
  warmRgb: "229, 192, 123",
  dangerRgb: "224, 108, 117",
  lineRgb: "92, 99, 112", // comment
  shadowRgb: "0, 0, 0",
  surfacePane: "#21252b",
  surfaceCard: "#2c313a",
  surfaceElevate: "#2f343d",
  surfaceCardFaint: "#2f343d",
  surfaceField: "#21252b",
  surfaceButton: "#2f343d",
  surfaceDropzone: "#282c34",
  paneHeaderFrom: "#2f343d",
  paneHeaderTo: "#282c34",
  composerFrom: "rgba(40, 44, 52, 0.2)",
  composerTo: "rgba(33, 37, 43, 0.86)",
  floatBtnBg: "#2c313a",
  modalScrim: "rgba(0, 0, 0, 0.6)",
  overlayScale: 1.5
});

export const one = { light, dark };
