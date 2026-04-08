import styled from "@emotion/styled";
import { Link } from "react-router-dom";
import { theme } from "./theme";

export const Page = styled.div`
  min-height: 100vh;
  background:
    radial-gradient(circle at top left, rgba(42, 161, 152, 0.12) 0%, transparent 30%),
    radial-gradient(circle at bottom right, rgba(181, 137, 0, 0.08) 0%, transparent 24%),
    linear-gradient(180deg, #fdf6e3 0%, #f7f0da 100%);
  color: ${theme.colors.ink};
`;

export const Container = styled.div`
  min-height: 100vh;
  padding: 14px;
`;

export const WorkspaceShell = styled.div`
  height: calc(100vh - 28px);
  display: grid;
  grid-template-columns: 280px minmax(0, 1fr) 340px;
  gap: 14px;

  @media (max-width: 1180px) {
    grid-template-columns: 250px minmax(0, 1fr);
  }

  @media (max-width: 900px) {
    height: auto;
    min-height: calc(100vh - 28px);
    grid-template-columns: 1fr;
  }
`;

export const SidebarPane = styled.aside`
  background: rgba(255, 251, 240, 0.92);
  border: 1px solid rgba(101, 123, 131, 0.14);
  border-radius: 24px;
  padding: 18px;
  box-shadow: ${theme.shadow};
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
  background: rgba(255, 251, 240, 0.9);
  border: 1px solid rgba(101, 123, 131, 0.14);
  border-radius: 24px;
  box-shadow: ${theme.shadow};
  overflow: hidden;
  display: flex;
  flex-direction: column;
`;

export const InspectorPane = styled.aside`
  background: rgba(255, 251, 240, 0.92);
  border: 1px solid rgba(101, 123, 131, 0.14);
  border-radius: 24px;
  padding: 18px;
  box-shadow: ${theme.shadow};
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
  color: ${theme.colors.warm};
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
  color: ${theme.colors.muted};
  line-height: 1.5;
`;

export const Divider = styled.hr`
  border: none;
  border-top: 1px solid rgba(101, 123, 131, 0.14);
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
  background: rgba(255, 255, 255, 0.46);
  border: 1px solid rgba(101, 123, 131, 0.14);
  border-radius: ${theme.radius};
  padding: 16px;
  min-width: 0;
`;

export const PaneHeader = styled.div`
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 18px 20px;
  border-bottom: 1px solid rgba(101, 123, 131, 0.12);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.55), rgba(255, 250, 240, 0.35));
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
  color: ${theme.colors.muted};
`;

export const SidebarLink = styled(Link)<{ $active?: boolean }>`
  display: block;
  text-decoration: none;
  color: inherit;
  padding: 11px 12px;
  border-radius: 14px;
  background: ${({ $active }) => ($active ? "rgba(42, 161, 152, 0.12)" : "transparent")};
  border: 1px solid ${({ $active }) => ($active ? "rgba(42, 161, 152, 0.22)" : "rgba(101, 123, 131, 0.08)")};

  &:hover {
    background: rgba(101, 123, 131, 0.06);
  }
`;

export const SidebarMeta = styled.div`
  font-size: 12px;
  color: ${theme.colors.muted};
  margin-top: 4px;
`;

export const Input = styled.input`
  width: 100%;
  border: 1px solid ${theme.colors.line};
  background: rgba(255, 255, 255, 0.72);
  color: ${theme.colors.ink};
  border-radius: 14px;
  padding: 12px 14px;
  font: inherit;

  &::placeholder {
    color: ${theme.colors.muted};
  }
`;

export const Textarea = styled.textarea`
  width: 100%;
  min-height: 120px;
  border: 1px solid ${theme.colors.line};
  background: rgba(255, 255, 255, 0.72);
  color: ${theme.colors.ink};
  border-radius: 14px;
  padding: 12px 14px;
  font: inherit;
  resize: vertical;

  &::placeholder {
    color: ${theme.colors.muted};
  }
`;

export const Select = styled.select`
  width: 100%;
  border: 1px solid ${theme.colors.line};
  background: rgba(255, 255, 255, 0.72);
  color: ${theme.colors.ink};
  border-radius: 14px;
  padding: 12px 14px;
  font: inherit;
`;

export const Field = styled.label`
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: ${theme.colors.muted};
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
  background: ${({ $connected }) => ($connected ? "#2aa198" : "#dc322f")};
  box-shadow: 0 0 0 3px ${({ $connected }) => ($connected ? "rgba(42, 161, 152, 0.16)" : "rgba(220, 50, 47, 0.12)")};
`;

export const Button = styled.button<{ variant?: "solid" | "ghost" | "warm" }>`
  border: 1px solid
    ${({ variant }) =>
      variant === "ghost"
        ? theme.colors.line
        : variant === "warm"
          ? "rgba(181, 137, 0, 0.35)"
          : "rgba(181, 137, 0, 0.3)"};
  background: ${({ variant }) =>
    variant === "ghost"
      ? "transparent"
      : variant === "warm"
        ? "rgba(181, 137, 0, 0.12)"
        : "linear-gradient(180deg, rgba(181, 137, 0, 0.2) 0%, rgba(181, 137, 0, 0.4) 100%)"};
  color: ${theme.colors.ink};
  border-radius: 12px;
  padding: 11px 16px;
  font: inherit;
  font-weight: 600;
  cursor: pointer;
`;

export const List = styled.div`
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
`;

export const Item = styled.article`
  border: 1px solid rgba(101, 123, 131, 0.12);
  border-radius: 16px;
  padding: 14px;
  background: rgba(255, 255, 255, 0.42);
  min-width: 0;
`;

export const Badge = styled.span<{ tone?: "accent" | "warm" | "muted" }>`
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 11px;
  background: ${({ tone }) =>
    tone === "warm" ? theme.colors.warmSoft : tone === "muted" ? "rgba(101, 123, 131, 0.1)" : theme.colors.accentSoft};
  color: ${({ tone }) => (tone === "warm" ? theme.colors.warm : tone === "muted" ? theme.colors.muted : theme.colors.accent)};
`;

export const RouterLink = styled(Link)`
  color: inherit;
  text-decoration: none;
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
  border-radius: 20px;
  border: 1px solid
    ${({ $role }) => ($role === "assistant" ? "rgba(42, 161, 152, 0.2)" : "rgba(101, 123, 131, 0.12)")};
  background: ${({ $role }) =>
    $role === "assistant"
      ? "linear-gradient(180deg, rgba(42, 161, 152, 0.09), rgba(42, 161, 152, 0.03))"
      : "rgba(255, 255, 255, 0.42)"};
`;

export const Composer = styled.form`
  flex-shrink: 0;
  padding: 14px 20px 20px;
  border-top: 1px solid rgba(101, 123, 131, 0.12);
  background: linear-gradient(180deg, rgba(253, 246, 227, 0.2), rgba(247, 240, 218, 0.86));
`;

export const ComposerBox = styled.div`
  display: flex;
  flex-direction: column;
  gap: 12px;
  border: 1px solid rgba(101, 123, 131, 0.14);
  border-radius: 18px;
  padding: 14px;
  background: rgba(255, 255, 255, 0.42);
`;

export const MetaText = styled.div`
  font-size: 12px;
  color: ${theme.colors.muted};
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
  border: 1px solid rgba(42, 161, 152, 0.24);
  background: rgba(255, 250, 240, 0.94);
  box-shadow: 0 12px 28px rgba(88, 110, 117, 0.18);
  color: ${theme.colors.ink};
  cursor: pointer;

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
  border: 1px solid rgba(101, 123, 131, 0.16);
  background: rgba(255, 255, 255, 0.52);
  color: ${theme.colors.ink};
  cursor: pointer;
`;

export const DropZone = styled.div<{ $active?: boolean }>`
  border: 1px dashed ${({ $active }) => ($active ? "rgba(42, 161, 152, 0.4)" : "rgba(101, 123, 131, 0.24)")};
  background: ${({ $active }) => ($active ? "rgba(42, 161, 152, 0.08)" : "rgba(255, 255, 255, 0.34)")};
  border-radius: 18px;
  padding: 18px;
`;

export const ModalOverlay = styled.div`
  position: fixed;
  inset: 0;
  background: rgba(47, 59, 66, 0.34);
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
  background: #fffaf0;
  border: 1px solid rgba(101, 123, 131, 0.14);
  border-radius: 24px;
  box-shadow: ${theme.shadow};
  padding: 20px;
`;
