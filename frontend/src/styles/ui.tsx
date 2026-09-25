import { css } from "@emotion/react";
import type { Theme } from "@emotion/react";
import styled from "@emotion/styled";
import { Link } from "react-router-dom";
import { snzTokens } from "./themes/snz-tokens";

// Focus is drawn outside the control rather than by recolouring its border:
// a recoloured border is measured against the border it replaces and cannot
// reach 3:1, an outer ring is measured against the surface (snz-design
// doc-8 §5.1). :focus-visible keeps it off pointer presses.
const focusRing = ({ theme }: { theme: Theme }) => css`
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

const ENABLED = ':not(:disabled):not([aria-disabled="true"])';

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

export const WorkspaceShell = styled.div<{ $columns?: string }>`
  height: calc(100vh - 2 * ${snzTokens.space.sm});
  display: grid;
  grid-template-columns: ${({ $columns }) => $columns ?? "280px minmax(0, 1fr) 340px"};
  gap: ${snzTokens.space.sm};

  @media (max-width: 1180px) {
    grid-template-columns: 250px minmax(0, 1fr);
  }

  @media (max-width: 900px) {
    height: auto;
    min-height: calc(100vh - 2 * ${snzTokens.space.sm});
    grid-template-columns: 1fr;
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

export const InspectorPane = styled.aside`
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

  @media (max-width: 1180px) {
    display: none;
  }
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

export const SidebarLink = styled(Link, {
  shouldForwardProp: (prop) => prop !== "$active"
})<{ $active?: boolean }>`
  display: block;
  text-decoration: none;
  color: inherit;
  padding: 11px 12px;
  border-radius: ${({ theme }) => theme.radiusSm};
  background: ${({ $active, theme }) => ($active ? theme.accentSoft : "transparent")};
  border: 1px solid ${({ $active, theme }) => ($active ? theme.accentBorder : theme.lineIdle)};

  &:hover {
    background: ${({ theme }) => theme.surfaceHover};
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

export const Select = styled.select`
  width: 100%;
  border: 1px solid ${({ theme }) => theme.fieldBorder};
  background: ${({ theme }) => theme.surfaceField};
  color: ${({ theme }) => theme.ink};
  border-radius: ${({ theme }) => theme.radiusSm};
  padding: 12px 14px;
  font: inherit;

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

export const MessageBubble = styled.article<{ $role: "user" | "assistant" | "system" }>`
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
  color: ${({ theme }) => theme.muted};
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

export const IconButton = styled.button`
  width: 34px;
  height: 34px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  border: 1px solid ${({ theme }) => theme.iconBorder};
  background: ${({ theme }) => theme.surfaceButton};
  color: ${({ theme }) => theme.ink};
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

export const DropZone = styled.div<{ $active?: boolean }>`
  border: 1px dashed ${({ $active, theme }) => ($active ? theme.accentDropActive : theme.dropzoneBorder)};
  background: ${({ $active, theme }) => ($active ? theme.accentDragBg : theme.surfaceDropzone)};
  border-radius: ${({ theme }) => theme.radius};
  padding: 18px;
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
