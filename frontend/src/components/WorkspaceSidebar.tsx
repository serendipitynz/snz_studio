import { useEffect, useId, useState } from "react";
import { useNavigate } from "react-router-dom";
import styled from "@emotion/styled";
import { api, ChatKind, ChatRecord, Project } from "../api/client";
import { MessageKey, useLanguage } from "../i18n";
import { SettingsModal } from "./SettingsModal";
import { usePopupMenu } from "./usePopupMenu";
import {
  Button,
  Divider,
  FOCUS_RING_REACH,
  focusRing,
  IconButton,
  NARROW_MEDIA,
  RouterLink,
  Row,
  SidebarButton,
  SidebarLink,
  SidebarPane,
  SidebarSection,
  SidebarSectionLabel,
  Stack,
  Subtle
} from "../styles/ui";
import { snzTokens } from "../styles/themes/snz-tokens";

interface WorkspaceSidebarProps {
  projects: Project[];
  currentProjectId?: string;
  chats?: ChatRecord[];
  activeChatId?: string;
}

// Presets are deliberately absent: picking one needs the project form's fields,
// so this quick menu creates an empty roster and leaves the line-up to the
// organisation panel, where the preset can still be applied until the
// conversation's first message.
const CHAT_KIND_CHOICES: { kind: ChatKind; label: MessageKey }[] = [
  { kind: "assistant", label: "multiAgent.kindAssistant" },
  { kind: "multi_agent", label: "multiAgent.kindMultiAgent" }
];

export function WorkspaceSidebar(props: WorkspaceSidebarProps) {
  const navigate = useNavigate();
  const { t } = useLanguage();
  const isNarrow = useMediaQuery(NARROW_MEDIA);
  const [creatingChat, setCreatingChat] = useState(false);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const kindMenu = usePopupMenu<HTMLButtonElement>();
  const entry = usePopupMenu<HTMLButtonElement>();
  const entryPanelId = useId();
  const { close: closeKindMenu } = kindMenu;
  const { close: closeEntry } = entry;

  // The two variants render different triggers, so a popup left open across
  // the switch would have nothing to return focus to.
  useEffect(() => {
    closeKindMenu(false);
    closeEntry(false);
  }, [isNarrow, closeKindMenu, closeEntry]);

  // Picking a destination replaces the page, so the collapsed entry closes and
  // hands focus back to its trigger (snz-design doc-9 §6.8).
  function leaveEntry() {
    if (isNarrow) {
      closeEntry(true);
    }
  }

  async function handleCreateChat(kind: ChatKind) {
    if (!props.currentProjectId || creatingChat) {
      return;
    }

    // Focus goes back to the trigger first, so the busy state is on the
    // control the user just used (snz-design doc-9 §6.11).
    closeKindMenu(true);
    setCreatingChat(true);

    try {
      const response = await api.createChat(props.currentProjectId, { title: "", kind });
      leaveEntry();
      navigate(`/chats/${response.chat.id}`);
    } finally {
      setCreatingChat(false);
    }
  }

  function openSettings() {
    leaveEntry();
    setIsSettingsOpen(true);
  }

  // A chat's page is inside its project, so the project row says so without
  // claiming to be the page itself.
  const projectCurrent = props.activeChatId ? "true" : "page";

  const destinations = (
    <Stack
      as="nav"
      aria-label={t("sidebar.navigation")}
      style={{
        flex: "1 1 auto",
        minHeight: 0,
        overflow: "auto",
        padding: FOCUS_RING_REACH,
        // Only sideways: a vertical negative margin would also push the
        // scroll clip edge into the gap under the divider, leaving a sliver
        // of the scrolled-away row painted there.
        margin: `0 calc(-1 * ${FOCUS_RING_REACH})`
      }}
    >
      <SidebarSection>
        <SidebarSectionLabel>{t("sidebar.projects")}</SidebarSectionLabel>
        {props.projects.map((project) => (
          <SidebarLink
            key={project.id}
            to={`/projects/${project.id}`}
            aria-current={project.id === props.currentProjectId ? projectCurrent : undefined}
            onClick={leaveEntry}
          >
            <Row style={{ alignItems: "center", gap: 8, flexWrap: "nowrap" }}>
              <strong style={{ minWidth: 0, overflowWrap: "anywhere" }}>{project.title}</strong>
              <Subtle style={{ flexShrink: 0 }}>({project.chatCount})</Subtle>
            </Row>
          </SidebarLink>
        ))}
      </SidebarSection>

      {props.currentProjectId ? (
        <SidebarSection>
          <Row style={{ justifyContent: "space-between", alignItems: "center", flexWrap: "nowrap" }}>
            <SidebarSectionLabel>{t("sidebar.chats")}</SidebarSectionLabel>
            <MenuAnchor ref={kindMenu.anchorRef}>
              {/* While a chat is being created the button stays focusable and
                  keeps its name and size; it shows the busy figure in place of
                  the plus and ignores presses (snz-design doc-8 §6.1). */}
              <IconButton
                ref={kindMenu.triggerRef}
                type="button"
                aria-label={t("sidebar.createChat")}
                aria-haspopup="menu"
                aria-expanded={kindMenu.isOpen}
                aria-busy={creatingChat || undefined}
                onClick={creatingChat ? undefined : kindMenu.toggle}
                onKeyDown={creatingChat ? undefined : kindMenu.triggerProps.onKeyDown}
                title={t(creatingChat ? "sidebar.creatingChat" : "sidebar.createChat")}
              >
                {creatingChat ? <SpinnerIcon /> : <PlusIcon />}
              </IconButton>
              {kindMenu.isOpen ? (
                <KindMenu
                  ref={kindMenu.popupRef}
                  role="menu"
                  aria-label={t("multiAgent.chatType")}
                  {...kindMenu.popupProps}
                >
                  {CHAT_KIND_CHOICES.map((choice) => (
                    <KindMenuItem
                      key={choice.kind}
                      type="button"
                      role="menuitem"
                      // Arrow keys move between the items; Tab leaves the menu.
                      tabIndex={-1}
                      onClick={() => void handleCreateChat(choice.kind)}
                    >
                      {t(choice.label)}
                    </KindMenuItem>
                  ))}
                </KindMenu>
              ) : null}
            </MenuAnchor>
          </Row>
          {props.chats?.length ? (
            props.chats.map((chat) => (
              <SidebarLink
                key={chat.id}
                to={`/chats/${chat.id}`}
                aria-current={chat.id === props.activeChatId ? "page" : undefined}
                onClick={leaveEntry}
              >
                {renderChatTitle(t, chat)}
              </SidebarLink>
            ))
          ) : (
            <Subtle>{t("sidebar.noChats")}</Subtle>
          )}
        </SidebarSection>
      ) : null}
    </Stack>
  );

  const settingsButton = (
    <SidebarButton type="button" onClick={openSettings} style={{ flexShrink: 0 }}>
      <GearIcon />
      <span>{t("sidebar.settings")}</span>
    </SidebarButton>
  );

  const settingsModal = isSettingsOpen ? <SettingsModal onClose={() => setIsSettingsOpen(false)} /> : null;

  if (isNarrow) {
    return (
      <CollapsedEntry ref={entry.anchorRef}>
        <Brand />
        <Button
          ref={entry.triggerRef}
          type="button"
          variant="normal"
          aria-controls={entry.isOpen ? entryPanelId : undefined}
          {...entry.triggerProps}
          onClick={entry.toggle}
          style={{ display: "inline-flex", alignItems: "center", gap: 8 }}
        >
          <MenuIcon />
          <span>{t("sidebar.menu")}</span>
        </Button>
        {entry.isOpen ? (
          <CollapsedPanel id={entryPanelId} ref={entry.popupRef} {...entry.popupProps}>
            {destinations}
            <Divider />
            {settingsButton}
          </CollapsedPanel>
        ) : null}
        {settingsModal}
      </CollapsedEntry>
    );
  }

  return (
    <SidebarPane>
      <Row style={{ justifyContent: "space-between", alignItems: "center", flexShrink: 0 }}>
        <Brand />
        <RouterLink to="/" aria-label={t("sidebar.home")}>
          <HomeIcon />
        </RouterLink>
      </Row>

      <Divider />
      {destinations}
      <Divider />
      {settingsButton}
      {settingsModal}
    </SidebarPane>
  );
}

function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => window.matchMedia(query).matches);

  useEffect(() => {
    const list = window.matchMedia(query);
    const update = () => setMatches(list.matches);
    update();
    list.addEventListener("change", update);
    return () => list.removeEventListener("change", update);
  }, [query]);

  return matches;
}

function Brand() {
  return (
    <RouterLink to="/" style={{ fontWeight: 700, letterSpacing: "0.08em" }}>
      SNZ STUDIO
    </RouterLink>
  );
}

// Sticks to the top of the page so the trigger stays in reach while the body
// scrolls under it; a menu whose trigger scrolls away would have to close
// (snz-design doc-9 §5.3).
const CollapsedEntry = styled.div`
  position: sticky;
  top: 0;
  z-index: 15;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 14px;
  background: ${({ theme }) => theme.surfacePane};
  border: 1px solid ${({ theme }) => theme.lineMedium};
  border-radius: ${({ theme }) => theme.radius};
`;

// Lies over the page body rather than pushing it down (snz-design doc-9 §6.8).
const CollapsedPanel = styled.div`
  position: absolute;
  top: calc(100% + ${snzTokens.space.xs});
  left: 0;
  right: 0;
  max-height: 75vh;
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 18px;
  background: ${({ theme }) => theme.surfaceCard};
  border: 1px solid ${({ theme }) => theme.fieldBorder};
  border-radius: ${({ theme }) => theme.radius};
  box-shadow: ${({ theme }) => theme.shadowPopover};
`;

const MenuAnchor = styled.div`
  position: relative;
  flex-shrink: 0;
  display: inline-flex;
`;

const KindMenu = styled.div`
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  z-index: 10;
  min-width: 190px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px;
  background: ${({ theme }) => theme.surfaceCard};
  /* It lies over the page, so its edge needs the control line (snz-design doc-9 §6.11). */
  border: 1px solid ${({ theme }) => theme.fieldBorder};
  border-radius: ${({ theme }) => theme.radius};
  box-shadow: ${({ theme }) => theme.shadowPopover};
`;

const KindMenuItem = styled.button`
  padding: 8px 10px;
  border: 0;
  border-radius: ${({ theme }) => theme.radiusSm};
  background: transparent;
  color: ${({ theme }) => theme.ink};
  font: inherit;
  text-align: left;
  white-space: nowrap;
  cursor: pointer;

  &:hover,
  &:focus-visible {
    background: ${({ theme }) => theme.surfaceHover};
  }

  ${focusRing}
`;

function renderChatTitle(t: (key: MessageKey) => string, chat: ChatRecord) {
  // The two markers are independent facts about the chat, so both can show.
  const marker = `${chat.kind === "multi_agent" ? "👥" : ""}${chat.isTemporary ? "⏱️" : ""}`;
  const prefix = marker ? `${marker} ` : "";

  if (chat.title.trim()) {
    return <strong>{`${prefix}${chat.title}`}</strong>;
  }

  return <Subtle>{`${prefix}${t("sidebar.untitled")}`}</Subtle>;
}

function HomeIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" fill="none" aria-hidden="true" style={{ display: "block" }}>
      <path
        d="M3 7.75 9 3l6 4.75V15h-4.25v-4H7.25v4H3V7.75Z"
        stroke="currentColor"
        strokeWidth="1.35"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function MenuIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M2.5 4h11M2.5 8h11M2.5 12h11" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  );
}

function SpinnerIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <circle cx="8" cy="8" r="5.5" stroke="currentColor" strokeOpacity="0.22" strokeWidth="1.6" />
      <path d="M13.5 8A5.5 5.5 0 0 0 8 2.5" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round">
        <animateTransform
          attributeName="transform"
          attributeType="XML"
          type="rotate"
          from="0 8 8"
          to="360 8 8"
          dur="0.8s"
          repeatCount="indefinite"
        />
      </path>
    </svg>
  );
}

function PlusIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M8 3v10M3 8h10" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  );
}

function GearIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden="true" style={{ display: "block", flexShrink: 0 }}>
      <circle cx="12" cy="12" r="3" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
      <path
        d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z"
        stroke="currentColor"
        strokeWidth="1.7"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
