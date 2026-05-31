import type { ThemeSpec } from "./types";

// Surface fields shared by the LIGHT variants (except Solarized Light, which keeps
// its exact original warm-white values for zero visual regression). On a light base
// these translucent-white overlays read as frosted panels — the same "elevation"
// effect the original design used. Dark variants must NOT use these (white-at-alpha
// turns muddy on a dark base); each dark theme authors concrete surfaces instead.
type SurfaceKeys =
  | "surfacePane"
  | "surfaceCard"
  | "surfaceElevate"
  | "surfaceCardFaint"
  | "surfaceField"
  | "surfaceButton"
  | "surfaceDropzone"
  | "paneHeaderFrom"
  | "paneHeaderTo"
  | "composerFrom"
  | "composerTo"
  | "floatBtnBg"
  | "modalScrim";

export const LIGHT_SURFACES: Pick<ThemeSpec, SurfaceKeys> = {
  surfacePane: "rgba(255, 255, 255, 0.66)",
  surfaceCard: "rgba(255, 255, 255, 0.94)",
  surfaceElevate: "rgba(255, 255, 255, 0.5)",
  surfaceCardFaint: "rgba(255, 255, 255, 0.55)",
  surfaceField: "rgba(255, 255, 255, 0.8)",
  surfaceButton: "rgba(255, 255, 255, 0.6)",
  surfaceDropzone: "rgba(255, 255, 255, 0.42)",
  paneHeaderFrom: "rgba(255, 255, 255, 0.62)",
  paneHeaderTo: "rgba(255, 255, 255, 0.3)",
  composerFrom: "rgba(255, 255, 255, 0.18)",
  composerTo: "rgba(255, 255, 255, 0.72)",
  floatBtnBg: "rgba(255, 255, 255, 0.92)",
  modalScrim: "rgba(20, 22, 30, 0.34)"
};
