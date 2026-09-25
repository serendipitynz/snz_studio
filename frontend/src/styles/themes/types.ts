// Theme token model for SNZ Studio.
//
// `ThemeTokens` is the flat set of values every styled component consumes via
// Emotion's `({ theme }) => theme.X`. To avoid authoring ~55 values per theme by
// hand, each palette provides a compact `ThemeSpec` (solid anchor colors + a few
// rgb triplets + the opaque "elevation" surfaces) and `buildTokens` derives the
// full token set. The translucent washes (lines, accent/warm/danger tints,
// shadows) are derived from each theme's own hue triplets at fixed alphas, so they
// stay faithful to the original Solarized look while adapting per theme. The
// white-overlay surfaces that would turn muddy on a dark base are authored
// explicitly in the spec instead of derived.

export type ThemeVariant = "light" | "dark";

export type ThemeFamily = "standard" | "solarized" | "catppuccin" | "rosePine" | "tokyoNight" | "github" | "one";

// rgb triplet, e.g. "42, 161, 152"
type Rgb = string;

const rgba = (rgb: Rgb, alpha: number): string => `rgba(${rgb}, ${alpha})`;

// ThemeSpec is the compact per-theme input. Alpha washes are derived from the
// triplets; only the colors that must be concrete per theme are listed here.
export interface ThemeSpec {
  scheme: ThemeVariant;

  // solid colors
  ink: string;
  muted: string;
  accent: string;
  warm: string;
  danger: string;
  dangerText: string;
  fieldBorder: string;

  // page canvas
  bg: string;
  bgGradientFrom: string;
  bgGradientTo: string;

  // rgb triplets used to derive translucent washes
  accentRgb: Rgb;
  warmRgb: Rgb;
  dangerRgb: Rgb;
  lineRgb: Rgb; // the hairline / muted-overlay hue
  shadowRgb: Rgb;

  // explicit surfaces (the white-overlay replacements — must be concrete per theme)
  surfacePane: string;
  surfaceCard: string;
  surfaceElevate: string;
  surfaceCardFaint: string;
  surfaceField: string;
  surfaceButton: string;
  surfaceDropzone: string;
  paneHeaderFrom: string;
  paneHeaderTo: string;
  composerFrom: string;
  composerTo: string;
  floatBtnBg: string;
  modalScrim: string;

  // multiplies the derived line + wash alphas so hairlines/tints read on dark
  // bases (default 1; dark themes use ~1.5). Solid colors and surfaces are exact.
  overlayScale?: number;

  radius?: string;
  radiusSm?: string;
  font?: string;
  mono?: string;
}

// ThemeTokens is the resolved, component-facing token set.
export interface ThemeTokens {
  scheme: ThemeVariant;

  // canvas
  bg: string;
  bgGradientFrom: string;
  bgGradientTo: string;
  bgRadialAccent: string;
  bgRadialWarm: string;

  // surfaces
  surfacePane: string;
  surfaceCard: string;
  surfaceElevate: string;
  surfaceCardFaint: string;
  surfaceField: string;
  surfaceButton: string;
  surfaceDropzone: string;
  paneHeaderFrom: string;
  paneHeaderTo: string;
  composerFrom: string;
  composerTo: string;
  floatBtnBg: string;
  surfaceHover: string;
  surfacePressed: string;
  // A panel placed inside a panel (the shared "nested panel"), and the rows
  // and cards that sit inside it.
  surfaceNested: string;
  surfaceItem: string;

  // text
  ink: string;
  muted: string;

  // borders / lines
  fieldBorder: string;
  lineSoft: string;
  lineIdle: string;
  line: string;
  lineMedium: string;
  lineStrong: string;
  iconBorder: string;
  dropzoneBorder: string;
  mutedBadgeBg: string;

  // accent
  accent: string;
  accentHover: string;
  accentPressed: string;
  onAccent: string;
  accentSoft: string;
  accentBorder: string;
  accentBubbleFrom: string;
  accentBubbleTo: string;
  accentBubbleBorder: string;
  accentStrong: string;
  accentDropActive: string;
  accentDragBorder: string;
  accentDragBg: string;
  floatBtnBorder: string;
  statusOkGlow: string;

  // warm
  warm: string;
  warmSoft: string;
  warmBorder: string;
  warmBorderStrong: string;
  warmBtnFrom: string;
  warmBtnTo: string;

  // danger / status
  danger: string;
  dangerText: string;
  dangerSoft: string;
  dangerBorder: string;

  // markdown surfaces
  codeBg: string;
  preBg: string;
  tableBorder: string;
  hrBorder: string;

  // chrome
  focus: string;
  modalScrim: string;
  shadow: string;
  shadowPopover: string;

  // typography
  radius: string;
  radiusSm: string;
  font: string;
  mono: string;
}

// The text colour for a filled accent surface. These palettes were authored
// before buttons were filled with the accent, so no spec lists one; take
// whichever candidate reads best on this accent.
function pickOnAccent(accent: string, candidates: string[]): string {
  const luminance = (hex: string): number => {
    const n = Number.parseInt(hex.slice(1), 16);
    const channel = (v: number): number => {
      const c = v / 255;
      return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
    };
    return 0.2126 * channel((n >> 16) & 255) + 0.7152 * channel((n >> 8) & 255) + 0.0722 * channel(n & 255);
  };
  const base = luminance(accent);
  const contrast = (hex: string): number => {
    const other = luminance(hex);
    return (Math.max(base, other) + 0.05) / (Math.min(base, other) + 0.05);
  };
  return candidates.reduce((best, next) => (contrast(next) > contrast(best) ? next : best));
}

const DEFAULT_FONT = "'IBM Plex Sans', 'Avenir Next', sans-serif";
const DEFAULT_MONO = "'IBM Plex Mono', monospace";

export function buildTokens(spec: ThemeSpec): ThemeTokens {
  const k = spec.overlayScale ?? 1;
  // derived alpha helpers: `w` (wash) scales with k and clamps; `s` (solid hue) is fixed.
  const w = (rgb: Rgb, alpha: number): string => rgba(rgb, Math.min(alpha * k, 0.95));

  return {
    scheme: spec.scheme,

    bg: spec.bg,
    bgGradientFrom: spec.bgGradientFrom,
    bgGradientTo: spec.bgGradientTo,
    bgRadialAccent: w(spec.accentRgb, 0.12),
    bgRadialWarm: w(spec.warmRgb, 0.08),

    surfacePane: spec.surfacePane,
    surfaceCard: spec.surfaceCard,
    surfaceElevate: spec.surfaceElevate,
    surfaceCardFaint: spec.surfaceCardFaint,
    surfaceField: spec.surfaceField,
    surfaceButton: spec.surfaceButton,
    surfaceDropzone: spec.surfaceDropzone,
    paneHeaderFrom: spec.paneHeaderFrom,
    paneHeaderTo: spec.paneHeaderTo,
    composerFrom: spec.composerFrom,
    composerTo: spec.composerTo,
    floatBtnBg: spec.floatBtnBg,
    surfaceHover: w(spec.lineRgb, 0.08),
    surfacePressed: w(spec.lineRgb, 0.14),
    // These families keep the look they had before the shared nested-panel
    // step existed: sections on the faint card surface, rows on the elevated one.
    surfaceNested: spec.surfaceCardFaint,
    surfaceItem: spec.surfaceElevate,

    ink: spec.ink,
    muted: spec.muted,

    fieldBorder: spec.fieldBorder,
    lineSoft: w(spec.lineRgb, 0.06),
    lineIdle: w(spec.lineRgb, 0.08),
    line: w(spec.lineRgb, 0.12),
    lineMedium: w(spec.lineRgb, 0.14),
    lineStrong: w(spec.lineRgb, 0.18),
    iconBorder: w(spec.lineRgb, 0.16),
    dropzoneBorder: w(spec.lineRgb, 0.24),
    mutedBadgeBg: w(spec.lineRgb, 0.1),

    accent: spec.accent,
    accentHover: `color-mix(in srgb, ${spec.accent} 86%, ${spec.ink})`,
    accentPressed: `color-mix(in srgb, ${spec.accent} 72%, ${spec.ink})`,
    onAccent: pickOnAccent(spec.accent, [spec.bg, spec.ink, "#ffffff", "#000000"]),
    accentSoft: w(spec.accentRgb, 0.12),
    accentBorder: w(spec.accentRgb, 0.22),
    accentBubbleFrom: w(spec.accentRgb, 0.09),
    accentBubbleTo: w(spec.accentRgb, 0.03),
    accentBubbleBorder: w(spec.accentRgb, 0.2),
    accentStrong: w(spec.accentRgb, 0.45),
    accentDropActive: w(spec.accentRgb, 0.4),
    accentDragBorder: w(spec.accentRgb, 0.34),
    accentDragBg: w(spec.accentRgb, 0.08),
    floatBtnBorder: w(spec.accentRgb, 0.24),
    statusOkGlow: w(spec.accentRgb, 0.16),

    warm: spec.warm,
    warmSoft: w(spec.warmRgb, 0.12),
    warmBorder: w(spec.warmRgb, 0.3),
    warmBorderStrong: w(spec.warmRgb, 0.35),
    warmBtnFrom: w(spec.warmRgb, 0.2),
    warmBtnTo: w(spec.warmRgb, 0.4),

    danger: spec.danger,
    dangerText: spec.dangerText,
    dangerSoft: w(spec.dangerRgb, 0.12),
    dangerBorder: rgba(spec.dangerRgb, 0.33),

    codeBg: w(spec.lineRgb, 0.12),
    preBg: w(spec.lineRgb, 0.08),
    tableBorder: w(spec.lineRgb, 0.18),
    hrBorder: w(spec.lineRgb, 0.2),

    focus: spec.accent,
    modalScrim: spec.modalScrim,
    shadow: `0 20px 40px ${rgba(spec.shadowRgb, 0.14)}`,
    shadowPopover: `0 12px 28px ${rgba(spec.shadowRgb, 0.18)}`,

    radius: spec.radius ?? "18px",
    radiusSm: spec.radiusSm ?? "12px",
    font: spec.font ?? DEFAULT_FONT,
    mono: spec.mono ?? DEFAULT_MONO
  };
}
