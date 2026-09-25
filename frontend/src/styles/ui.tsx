import { css } from "@emotion/react";
import type { Theme } from "@emotion/react";
import styled from "@emotion/styled";
import { Link } from "react-router-dom";
import { snzTokens } from "./themes/snz-tokens";

// Focus is drawn outside the control rather than by recolouring its border:
// a recoloured border is measured against the border it replaces and cannot
// reach 3:1, an outer ring is measured against the surface (snz-design
// doc-8 §5.1). :focus-visible keeps it off pointer presses.
export const focusRing = ({ theme }: { theme: Theme }) => css`
  &:focus-visible {
    outline: ${snzTokens.border.focus} solid ${theme.focus};
    outline-offset: ${snzTokens.border.focusOffset};
  }
`;

// Disabled reads from shape, not only colour, and keeps the default cursor
// (snz-design doc-8 §5.4). aria-disabled is styled alike so a control can stay
// focusable while it explains why it is disabled.
const disabledLook = css`
  &:disabled,
  &[aria-disabled="true"] {
    border-style: ${snzTokens.border.disabledStyle};
    opacity: ${snzTokens.opacity.disabled};
    cursor: default;
  }
`;

// A busy control keeps its focus but takes no press, so it does not answer the
// pointer either (snz-design doc-8 §6.1).
const ENABLED = ':not(:disabled):not([aria-disabled="true"]):not([aria-busy="true"])';

// How far the focus ring reaches outside a control. A scrolling container
// clips at its padding edge, so one holding focusable rows pads by this much
// (cancelling it sideways with a negative margin) to keep the ring whole.
export const FOCUS_RING_REACH = `calc(${snzTokens.border.focus} + ${snzTokens.border.focusOffset})`;

// Below this width the workspace is one column and the sidebar folds into its
// collapsed entry (snz-design doc-9 §6.8).
export const NARROW_MEDIA = "(max-width: 900px)";

export const Page = styled.div`
  min-height: 100vh;
  background:
    radial-gradient(circle at top left, ${({ theme }) => theme.bgRadialAccent} 0%, transparent 30%),
    radial-gradient(circle at bottom right, ${({ theme }) => theme.bgRadialWarm} 0%, transparent 24%),
    linear-gradient(180deg, ${({ theme }) => theme.bgGradientFrom} 0%, ${({ theme }) => theme.bgGradientTo} 100%);
  color: ${({ theme }) => theme.ink};
`;

// The panes sit flat on the canvas, separated by a narrow gap rather than by
// shadows, so the gutter comes from the shared spacing scale.
export const Container = styled.div`
  min-height: 100vh;
  padding: ${snzTokens.space.sm};
`;

// Below this width the side region (the inspector, the participant column) has
// no column of its own; a screen with a trigger for it lays it over the
// conversation instead.
export const SIDE_REGION_MEDIA = "(max-width: 1180px)";

// $side: whether the side region's column is there on a wide screen. Below
// SIDE_REGION_MEDIA there is never a third column.
export const WorkspaceShell = styled.div<{ $side?: boolean }>`
  height: calc(100vh - 2 * ${snzTokens.space.sm});
  display: grid;
  grid-template-columns: ${({ $side }) => ($side === false ? "280px minmax(0, 1fr)" : "280px minmax(0, 1fr) 340px")};
  gap: ${snzTokens.space.sm};

  @media ${SIDE_REGION_MEDIA} {
    grid-template-columns: 250px minmax(0, 1fr);
  }

  @media ${NARROW_MEDIA} {
    height: auto;
    min-height: calc(100vh - 2 * ${snzTokens.space.sm});
    grid-template-columns: 1fr;
    /* The collapsed entry keeps its own height; the page body takes the rest. */
    grid-template-rows: auto 1fr;
  }
`;

export const SidebarPane = styled.aside`
  background: ${({ theme }) => theme.surfacePane};
  border: 1px solid ${({ theme }) => theme.lineMedium};
  border-radius: ${({ theme }) => theme.radius};
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: 0;
  overflow: hidden;
`;

export const MainPane = styled.main`
  min-width: 0;
  min-height: 0;
  height: 100%;
  position: relative;
  background: ${({ theme }) => theme.surfacePane};
  border: 1px solid ${({ theme }) => theme.lineMedium};
  border-radius: ${({ theme }) => theme.radius};
  overflow: hidden;
  display: flex;
  flex-direction: column;
`;

// A screen that shows and hides the pane itself puts the hidden attribute on it,
// and $overlay while it lies over the conversation below SIDE_REGION_MEDIA; one
// without a trigger leaves the pane to drop out below that width.
export const InspectorPane = styled.aside<{ $toggled?: boolean; $overlay?: boolean }>`
  background: ${({ theme }) => theme.surfacePane};
  border: 1px solid ${({ theme }) => theme.lineMedium};
  border-radius: ${({ theme }) => theme.radius};
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
  min-height: 0;
  overflow: auto;

  &[hidden] {
    display: none;
  }

  ${({ $toggled }) =>
    $toggled
      ? ""
      : css`
          @media ${SIDE_REGION_MEDIA} {
            display: none;
          }
        `}

  ${({ $overlay, theme }) =>
    $overlay
      ? css`
          position: fixed;
          inset-block: ${snzTokens.space.sm};
          inset-inline-end: ${snzTokens.space.sm};
          inline-size: min(360px, calc(100vw - 2 * ${snzTokens.space.sm}));
          z-index: 15;
          background: ${theme.surfaceCard};
          box-shadow: ${theme.shadowPopover};
        `
      : ""}
`;


export const Brand = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

export const Eyebrow = styled.div`
  font-size: 11px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: ${({ theme }) => theme.warm};
`;

export const Heading = styled.h1`
  margin: 0;
  font-size: 26px;
  line-height: 1;
`;

export const SectionTitle = styled.h2`
  margin: 0;
  font-size: 18px;
  line-height: 1.2;
`;

// A heading inside a section or a dialog, one level below SectionTitle. A section
// is named by a heading, not by a badge (snz-design doc-9 §6.3).
export const SubsectionTitle = styled.h3`
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  line-height: 1.3;
  color: ${({ theme }) => theme.inkStrong};
`;

// A document title that opens the document: a real button, so the keyboard can
// reach what the pointer reaches by pressing the row.
export const TitleButton = styled.button`
  padding: 0;
  border: none;
  background: none;
  color: inherit;
  font: inherit;
  font-weight: 600;
  text-align: start;
  cursor: pointer;
  overflow-wrap: anywhere;

  ${focusRing}
`;

// The disclosure line of a <details> in the conversation. The native element keeps
// its keyboard and its expanded state; this gives it the shared focus ring.
export const Summary = styled.summary`
  inline-size: fit-content;
  border-radius: ${({ theme }) => theme.radiusSm};
  cursor: pointer;

  ${focusRing}
`;

export const Subtle = styled.p`
  margin: 0;
  color: ${({ theme }) => theme.muted};
  line-height: 1.5;
`;

export const ErrorText = styled.p`
  margin: 0;
  color: ${({ theme }) => theme.dangerText};
  line-height: 1.5;
`;

export const Divider = styled.hr`
  border: none;
  border-top: 1px solid ${({ theme }) => theme.lineMedium};
  margin: 0;
`;

export const Stack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
`;

export const Row = styled.div`
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  min-width: 0;
`;

export const Grid = styled.div<{ columns?: string }>`
  display: grid;
  grid-template-columns: ${({ columns }) => columns ?? "1fr"};
  gap: 14px;

  @media (max-width: 900px) {
    grid-template-columns: 1fr;
  }
`;

export const Card = styled.section`
  background: ${({ theme }) => theme.surfaceNested};
  border: 1px solid ${({ theme }) => theme.lineMedium};
  border-radius: ${({ theme }) => theme.radius};
  padding: 16px;
  min-width: 0;
`;

export const PaneHeader = styled.div`
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 18px 20px;
  border-bottom: 1px solid ${({ theme }) => theme.line};
  background: linear-gradient(180deg, ${({ theme }) => theme.paneHeaderFrom}, ${({ theme }) => theme.paneHeaderTo});
  flex-shrink: 0;
`;

export const PaneBody = styled.div`
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
  padding: 18px 20px;
`;

export const MessageArea = styled.div`
  position: relative;
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
`;

export const ScrollColumn = styled.div`
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow: auto;
  min-height: 0;
`;

export const SidebarSection = styled.div`
  display: flex;
  flex-direction: column;
  gap: 8px;
`;

export const SidebarSectionLabel = styled.div`
  font-size: 11px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: ${({ theme }) => theme.muted};
`;

// The current location is drawn from aria-current, so what the screen shows and
// what a screen reader hears cannot drift apart. The face alone is too close to
// the unselected row to read, so the frame and the left band carry it as well
// (snz-design doc-8 §5.2, doc-9 §6.8).
export const SidebarLink = styled(Link)`
  display: block;
  text-decoration: none;
  color: inherit;
  padding: 11px 12px;
  border-radius: ${({ theme }) => theme.radiusSm};
  background: transparent;
  border: 1px solid ${({ theme }) => theme.lineIdle};

  &:not([aria-current]):hover {
    background: ${({ theme }) => theme.surfaceHover};
  }

  &[aria-current] {
    background: ${({ theme }) => theme.surfaceSelected};
    border-color: ${({ theme }) => theme.selected};
    box-shadow: inset ${snzTokens.border.band} 0 0 ${({ theme }) => theme.selected};
    color: ${({ theme }) => theme.inkStrong};
  }

  /* An untitled chat's muted placeholder would keep its own colour and drop
     the strong words from the current row. */
  &[aria-current] > p {
    color: inherit;
  }

  ${focusRing}
`;

export const SidebarButton = styled.button`
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  text-align: left;
  text-decoration: none;
  color: inherit;
  padding: 11px 12px;
  border-radius: ${({ theme }) => theme.radiusSm};
  background: transparent;
  border: 1px solid ${({ theme }) => theme.lineIdle};
  font: inherit;
  cursor: pointer;

  &${ENABLED}:hover {
    background: ${({ theme }) => theme.surfaceHover};
  }

  &${ENABLED}:active {
    background: ${({ theme }) => theme.surfacePressed};
  }

  ${focusRing}
  ${disabledLook}
`;

export const SidebarMeta = styled.div`
  font-size: 12px;
  color: ${({ theme }) => theme.muted};
  margin-top: 4px;
`;

export const Input = styled.input`
  width: 100%;
  border: 1px solid ${({ theme }) => theme.fieldBorder};
  background: ${({ theme }) => theme.surfaceField};
  color: ${({ theme }) => theme.ink};
  border-radius: ${({ theme }) => theme.radiusSm};
  padding: 12px 14px;
  font: inherit;

  ${focusRing}
  ${disabledLook}

  &::placeholder {
    color: ${({ theme }) => theme.muted};
  }
`;

export const Textarea = styled.textarea`
  width: 100%;
  min-height: 120px;
  border: 1px solid ${({ theme }) => theme.fieldBorder};
  background: ${({ theme }) => theme.surfaceField};
  color: ${({ theme }) => theme.ink};
  border-radius: ${({ theme }) => theme.radiusSm};
  padding: 12px 14px;
  font: inherit;

  ${focusRing}
  ${disabledLook}
  resize: vertical;

  &::placeholder {
    color: ${({ theme }) => theme.muted};
  }
`;

// The select draws its own chevron in the figure colour: the native one is about
// 10px, too small to read as the control's affordance (snz-design doc-8 §6.3).
const selectChevron = (colour: string) =>
  `url("data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='${colour}' stroke-width='2.5' stroke-linecap='round' stroke-linejoin='round'><path d='M5 9l7 7 7-7'/></svg>`
  )}")`;

export const Select = styled.select`
  width: 100%;
  appearance: none;
  border: 1px solid ${({ theme }) => theme.fieldBorder};
  background-color: ${({ theme }) => theme.surfaceField};
  background-image: ${({ theme }) => selectChevron(theme.figure)};
  background-repeat: no-repeat;
  background-position: right ${snzTokens.space.sm} center;
  background-size: ${snzTokens.icon.sizeMd};
  color: ${({ theme }) => theme.ink};
  border-radius: ${({ theme }) => theme.radiusSm};
  padding: 12px calc(${snzTokens.icon.sizeMd} + 2 * ${snzTokens.space.sm}) 12px 14px;
  font: inherit;
  cursor: pointer;

  &${ENABLED}:hover {
    background-color: ${({ theme }) => theme.surfaceHover};
  }

  ${focusRing}
  ${disabledLook}
`;

export const Field = styled.label`
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: ${({ theme }) => theme.muted};
`;

export const FieldHeader = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
`;

export const StatusDot = styled.span<{ $connected: boolean }>`
  width: 10px;
  height: 10px;
  border-radius: 999px;
  flex-shrink: 0;
  background: ${({ $connected, theme }) => ($connected ? theme.accent : theme.danger)};
  box-shadow: 0 0 0 3px ${({ $connected, theme }) => ($connected ? theme.statusOkGlow : theme.dangerSoft)};
`;

// The variants are snz-design doc-8 §6.1's primary / normal / destructive.
// The default stays primary because that is how the unmarked buttons looked
// before; keeping one primary per screen is each screen's call.
type ButtonVariant = "primary" | "normal" | "danger";

const buttonFace = (variant: ButtonVariant, theme: Theme) => {
  if (variant === "primary") {
    return css`
      border-color: ${theme.accent};
      background: ${theme.accent};
      color: ${theme.onAccent};

      &${ENABLED}:hover {
        border-color: ${theme.accentHover};
        background: ${theme.accentHover};
      }

      &${ENABLED}:active {
        border-color: ${theme.accentPressed};
        background: ${theme.accentPressed};
      }
    `;
  }
  // The destructive button keeps the plain surface: filling it with the
  // danger colour would make it look like the error notices (doc-8 §6.1).
  return css`
    border-color: ${variant === "danger" ? theme.danger : theme.fieldBorder};
    background: ${theme.surfaceButton};
    color: ${variant === "danger" ? theme.dangerText : theme.ink};

    &${ENABLED}:hover {
      background: ${theme.surfaceHover};
    }

    &${ENABLED}:active {
      background: ${theme.surfacePressed};
    }
  `;
};

export const Button = styled.button<{ variant?: ButtonVariant }>`
  border: 1px solid;
  ${({ variant, theme }) => buttonFace(variant ?? "primary", theme)}
  border-radius: ${({ theme }) => theme.radius};
  padding: 11px 16px;
  font: inherit;
  font-weight: 600;
  cursor: pointer;

  &[aria-busy="true"] {
    cursor: default;
  }

  ${focusRing}
  ${disabledLook}
`;

export const List = styled.div`
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
`;

export const Item = styled.article`
  border: 1px solid ${({ theme }) => theme.line};
  border-radius: ${({ theme }) => theme.radius};
  padding: 14px;
  background: ${({ theme }) => theme.surfaceItem};
  min-width: 0;
`;

export const Badge = styled.span<{ tone?: "accent" | "warm" | "muted" }>`
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 11px;
  background: ${({ tone, theme }) =>
    tone === "warm" ? theme.warmSoft : tone === "muted" ? theme.mutedBadgeBg : theme.accentSoft};
  color: ${({ tone, theme }) => (tone === "warm" ? theme.warm : tone === "muted" ? theme.muted : theme.accent)};
`;

// Read out but not drawn: words that sit beside a figure which already says
// the same thing on screen.
export const VisuallyHidden = styled.span`
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
  white-space: nowrap;
`;

export const RouterLink = styled(Link)`
  color: inherit;
  text-decoration: none;

  ${focusRing}
`;

export const MessageScroller = styled.div`
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
  padding: 18px 20px 28px;

  @media (max-width: 900px) {
    flex: 0 1 auto;
  }
`;

// The assistant's face is the accent's soft surface, where the muted words fall
// below 4.5:1 (Solarized Dark 4.42), so the meta words there take the words role
// of that surface (snz-design doc-5 §3.2).
export const MessageBubble = styled.article<{ $role: "user" | "assistant" | "system" }>`
  ${({ $role, theme }) => ($role === "assistant" ? `--meta-ink: ${theme.onAccentSoft};` : "")}
  max-width: min(860px, 100%);
  margin-left: ${({ $role }) => ($role === "user" ? "auto" : "0")};
  padding: 16px 18px;
  border-radius: ${({ theme }) => theme.radius};
  border: 1px solid
    ${({ $role, theme }) => ($role === "assistant" ? theme.accentBubbleBorder : theme.line)};
  background: ${({ $role, theme }) =>
    $role === "assistant"
      ? `linear-gradient(180deg, ${theme.accentBubbleFrom}, ${theme.accentBubbleTo})`
      : theme.surfaceElevate};
`;

export const Composer = styled.form`
  flex-shrink: 0;
  padding: 14px 20px 20px;
  border-top: 1px solid ${({ theme }) => theme.line};
  background: linear-gradient(180deg, ${({ theme }) => theme.composerFrom}, ${({ theme }) => theme.composerTo});
`;

export const ComposerBox = styled.div`
  display: flex;
  flex-direction: column;
  gap: 12px;
  border: 1px solid ${({ theme }) => theme.lineMedium};
  border-radius: ${({ theme }) => theme.radius};
  padding: 14px;
  background: ${({ theme }) => theme.surfaceElevate};
`;

export const MetaText = styled.div`
  font-size: 12px;
  color: var(--meta-ink, ${({ theme }) => theme.muted});
`;

export const FloatingScrollButton = styled.button`
  position: absolute;
  left: 50%;
  bottom: 12px;
  transform: translateX(-50%);
  z-index: 3;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-radius: 999px;
  border: 1px solid ${({ theme }) => theme.floatBtnBorder};
  background: ${({ theme }) => theme.floatBtnBg};
  box-shadow: ${({ theme }) => theme.shadowPopover};
  color: ${({ theme }) => theme.ink};
  cursor: pointer;

  &:hover {
    background: ${({ theme }) => theme.surfaceHover};
  }

  &:active {
    background: ${({ theme }) => theme.surfacePressed};
  }

  ${focusRing}

  @media (max-width: 900px) {
    bottom: 108px;
  }
`;

// A button without its words (snz-design doc-8 §6.2): the control's size and the
// button's corner radius, not a circle.
export const IconButton = styled.button`
  width: ${snzTokens.size.control};
  height: ${snzTokens.size.control};
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: ${({ theme }) => theme.radius};
  border: 1px solid ${({ theme }) => theme.iconBorder};
  background: ${({ theme }) => theme.surfaceButton};
  color: ${({ theme }) => theme.ink};
  cursor: pointer;

  &[aria-busy="true"] {
    cursor: default;
  }

  &${ENABLED}:hover {
    background: ${({ theme }) => theme.surfaceHover};
  }

  &${ENABLED}:active {
    background: ${({ theme }) => theme.surfacePressed};
  }

  ${focusRing}
  ${disabledLook}
`;

// The overlay's own way out, at its top right corner: the trigger in the header
// lies under the overlay while it is open. Tucked into the corner so it clears the
// first section below the heading, with room left for its focus ring.
export const RegionCloseButton = styled(IconButton)`
  position: absolute;
  inset-block-start: ${snzTokens.space.xs};
  inset-inline-end: ${snzTokens.space.xs};
`;

// The trigger that shows and hides a side region (snz-design doc-9 §6.3.1). Shown,
// it takes the selected face and outline as well as aria-expanded and its figure,
// so the state is not told by colour alone.
export const RegionToggleButton = styled(IconButton)`
  &[aria-expanded="true"] {
    background: ${({ theme }) => theme.surfaceSelected};
    border-color: ${({ theme }) => theme.selected};
  }
`;

export const ModalOverlay = styled.div`
  position: fixed;
  inset: 0;
  background: ${({ theme }) => theme.modalScrim};
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  z-index: 20;
`;

export const ModalCard = styled.div`
  width: min(860px, 100%);
  max-height: calc(100vh - 48px);
  overflow: auto;
  background: ${({ theme }) => theme.surfaceCard};
  border: 1px solid ${({ theme }) => theme.lineMedium};
  border-radius: ${({ theme }) => theme.radius};
  box-shadow: ${({ theme }) => theme.shadow};
  padding: 20px;
`;
