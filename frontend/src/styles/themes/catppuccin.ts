import { buildTokens, ThemeTokens } from "./types";
import { LIGHT_SURFACES } from "./presets";

// Catppuccin Latte (light) — official palette anchors.
const light: ThemeTokens = buildTokens({
  scheme: "light",
  ink: "#4c4f69",
  muted: "#6c6f85",
  accent: "#179299", // teal
  warm: "#df8e1d", // yellow
  danger: "#d20f39", // red
  dangerText: "#d20f39",
  fieldBorder: "#bcc0cc", // surface1
  bg: "#eff1f5", // base
  bgGradientFrom: "#eff1f5",
  bgGradientTo: "#e6e9ef", // mantle
  accentRgb: "23, 146, 153",
  warmRgb: "223, 142, 29",
  dangerRgb: "210, 15, 57",
  lineRgb: "156, 160, 176", // overlay0
  shadowRgb: "172, 176, 190",
  ...LIGHT_SURFACES
});

// Catppuccin Mocha (dark) — official base/mantle/crust/surface ramp.
const dark: ThemeTokens = buildTokens({
  scheme: "dark",
  ink: "#cdd6f4", // text
  muted: "#a6adc8", // subtext0
  accent: "#94e2d5", // teal
  warm: "#f9e2af", // yellow
  danger: "#f38ba8", // red
  dangerText: "#f38ba8",
  fieldBorder: "#45475a", // surface1
  bg: "#1e1e2e", // base
  bgGradientFrom: "#1e1e2e",
  bgGradientTo: "#181825", // mantle
  accentRgb: "148, 226, 213",
  warmRgb: "249, 226, 175",
  dangerRgb: "243, 139, 168",
  lineRgb: "166, 173, 200",
  shadowRgb: "0, 0, 0",
  surfacePane: "#181825", // mantle
  surfaceCard: "#252537",
  surfaceElevate: "#313244", // surface0
  surfaceCardFaint: "#313244",
  surfaceField: "#11111b", // crust
  surfaceButton: "#313244",
  surfaceDropzone: "#181825",
  paneHeaderFrom: "#313244",
  paneHeaderTo: "#1e1e2e",
  composerFrom: "rgba(49, 50, 68, 0.2)",
  composerTo: "rgba(24, 24, 37, 0.86)",
  floatBtnBg: "#252537",
  modalScrim: "rgba(0, 0, 0, 0.6)",
  overlayScale: 1.5
});

export const catppuccin = { light, dark };
