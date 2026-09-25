import { snzColors, snzTokens } from "./snz-tokens";
import type { SnzColorFamily, SnzColorMode, SnzColors } from "./snz-tokens";
import type { ThemeTokens } from "./types";

// Adapts the snz-design shared tokens (29 colour roles, opaque surfaces) to the
// flat ThemeTokens every styled component reads. The shared design adopted
// opaque surfaces and no gradients (snz-design doc-6 §4), so every *From/*To
// pair collapses to one value, and the translucent washes that buildTokens
// derives from rgb triplets are replaced by the shared soft surfaces.
//
// Radius and fonts are shared dimensions in snz-design but per-family values
// here, so they only change for the families built through this adapter.

function withAlpha(hex: string, alpha: number): string {
  const n = Number.parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
}

function fromColors(c: SnzColors, mode: SnzColorMode): ThemeTokens {
  const t = snzTokens;
  return {
    scheme: mode,

    bg: c.canvas,
    bgGradientFrom: c.canvas,
    bgGradientTo: c.canvas,
    bgRadialAccent: "transparent",
    bgRadialWarm: "transparent",

    surfacePane: c.surface,
    surfaceCard: c.surface,
    // The user's side of a conversation is told apart from the assistant's by
    // this surface (snz-design doc-4 §5.3), so it has to differ from the pane.
    surfaceElevate: c.surfaceAlt,
    surfaceCardFaint: c.surfaceAlt,
    surfaceField: c.surface,
    surfaceButton: c.surface,
    surfaceDropzone: c.surfaceAlt,
    paneHeaderFrom: c.surface,
    paneHeaderTo: c.surface,
    composerFrom: c.surface,
    composerTo: c.surface,
    floatBtnBg: c.surface,
    surfaceHover: c.surfaceHover,
    surfacePressed: c.surfacePressed,
    // Sections take the shared nested-panel face and the rows inside them go
    // back to the plain surface, where the shared hover step still shows
    // (snz-design doc-9 §6.3).
    surfaceNested: c.surfaceNested,
    surfaceItem: c.surface,
    surfaceSelected: c.surfaceSelected,
    selected: c.selected,

    ink: c.fg,
    inkStrong: c.fgStrong,
    muted: c.fgMuted,
    figure: c.figure,

    fieldBorder: c.lineControl,
    lineSoft: c.lineDivider,
    lineIdle: c.lineDivider,
    line: c.lineDivider,
    lineMedium: c.lineDivider,
    lineStrong: c.lineControl,
    iconBorder: c.lineControl,
    dropzoneBorder: c.lineControl,
    mutedBadgeBg: c.surfaceAlt,

    accent: c.accent,
    accentHover: c.accentHover,
    accentPressed: c.accentPressed,
    onAccent: c.onAccent,
    accentSoft: c.accentSoft,
    onAccentSoft: c.onAccentSoft,
    accentBorder: c.accent,
    accentBubbleFrom: c.accentSoft,
    accentBubbleTo: c.accentSoft,
    accentBubbleBorder: c.accent,
    accentStrong: c.accent,
    accentDropActive: c.accent,
    accentDragBorder: c.accent,
    accentDragBg: c.surfaceSelected,
    floatBtnBorder: c.lineControl,
    statusOkGlow: c.successSoft,

    warm: c.warn,
    warmSoft: c.warnSoft,
    warmBorder: c.warn,
    warmBorderStrong: c.warn,
    warmBtnFrom: c.warnSoft,
    warmBtnTo: c.warnSoft,

    danger: c.danger,
    dangerText: c.danger,
    dangerSoft: c.dangerSoft,
    onDangerSoft: c.onDangerSoft,
    dangerBorder: c.danger,

    codeBg: c.surfaceAlt,
    preBg: c.surfaceAlt,
    tableBorder: c.lineDivider,
    hrBorder: c.lineDivider,

    focus: c.focus,
    modalScrim: withAlpha(c.canvas, t.opacity.scrim),
    shadow: t.shadow.modal,
    shadowPopover: t.shadow.raised,

    radius: t.radius.md,
    radiusSm: t.radius.sm,
    font: t.font.family,
    mono: t.font.familyMono
  };
}

export function snzPalette(family: SnzColorFamily): { light: ThemeTokens; dark: ThemeTokens } {
  return {
    light: fromColors(snzColors[family].light, "light"),
    dark: fromColors(snzColors[family].dark, "dark")
  };
}
