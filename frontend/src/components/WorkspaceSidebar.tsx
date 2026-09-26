import { useEffect, useId, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import styled from "@emotion/styled";
import { api, ChatKind, ChatRecord, Project } from "../api/client";
import { MessageKey, useLanguage } from "../i18n";
import { ArrowLeftIcon, MessageSquarePlusIcon, SettingsIcon, SpinnerIcon } from "./icons";
import { SettingsModal } from "./SettingsModal";
import { useMediaQuery } from "./useMediaQuery";
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
  SIDEBAR_PANE_PADDING,
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

  const selectedProject = props.currentProjectId
    ? props.projects.find((project) => project.id === props.currentProjectId)
    : undefined;

  function renderProjectRow(project: Project) {
    return (
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
    );
  }

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
        {props.currentProjectId ? (
          // Inside a project only its own row stays; the back link beside it
          // returns to the dashboard, where the full list shows again, so the
          // sidebar's view follows the route and keeps no state of its own.
          <Row style={{ alignItems: "center", gap: 8, flexWrap: "nowrap" }}>
            <BackLink to="/" aria-label={t("sidebar.backToProjects")} title={t("sidebar.backToProjects")} onClick={leaveEntry}>
              <ArrowLeftIcon />
            </BackLink>
            {selectedProject ? (
              <div style={{ flex: "1 1 auto", minWidth: 0 }}>{renderProjectRow(selectedProject)}</div>
            ) : null}
          </Row>
        ) : (
          props.projects.map(renderProjectRow)
        )}
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
                {creatingChat ? <SpinnerIcon /> : <MessageSquarePlusIcon />}
              </IconButton>
              {kindMenu.isOpen ? (
                <KindMenu
                  ref={kindMenu.popupRef}
                  role="menu"
                  tabIndex={-1}
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

  const settingsFooter = (
    <SettingsFooter>
      <IconButton
        type="button"
        aria-label={t("sidebar.settings")}
        title={t("sidebar.settings")}
        onClick={openSettings}
      >
        <SettingsIcon size={18} />
      </IconButton>
      <Subtle style={{ fontSize: snzTokens.font.sizeSmall }}>{`snz studio v${__APP_VERSION__}`}</Subtle>
    </SettingsFooter>
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
          <CollapsedPanel id={entryPanelId} ref={entry.popupRef} tabIndex={-1} {...entry.popupProps}>
            {destinations}
            <Divider />
            {settingsFooter}
          </CollapsedPanel>
        ) : null}
        {settingsModal}
      </CollapsedEntry>
    );
  }

  return (
    <SidebarPane>
      {destinations}
      <FullBleedDivider />
      <PaneFooterFit>{settingsFooter}</PaneFooterFit>
      {settingsModal}
    </SidebarPane>
  );
}

function Brand() {
  return (
    <RouterLink to="/" style={{ fontWeight: 700, letterSpacing: "0.08em" }}>
      SNZ STUDIO
    </RouterLink>
  );
}

// A press on a popup's own padding would otherwise drop focus to the body,
// where the popup's Escape and Tab handling can no longer hear the keys. With
// tabIndex -1 the popup takes that focus itself; it is never a Tab stop, so it
// draws no ring.
const popupFocusHolder = `
  &:focus {
    outline: none;
  }
`;

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
  ${popupFocusHolder}
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

// A destination, not an action, so it reaches assistive technology as a link
// (snz-design doc-9 §6.8) while keeping the icon button's drawn form.
const BackLink = IconButton.withComponent(Link);

// The rule above the settings footer runs edge to edge of the pane (owner's
// real-window feedback), so it cancels the pane's own padding.
const FullBleedDivider = styled(Divider)`
  margin: 0 calc(-1 * ${SIDEBAR_PANE_PADDING});
`;

const SettingsFooter = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
`;

// The wide pane's 16px flex gap and 12px bottom padding leave the footer taller
// than the owner asked for (8px above, 6px below — real-window feedback), so
// this pulls both back by the difference. Wide pane only: the collapsed entry's
// panel keeps its own 18px padding, whose spacing the feedback did not target.
const PaneFooterFit = styled.div`
  margin-top: -8px;
  margin-bottom: -6px;
`;

const MenuAnchor = styled.div`
  position: relative;
  flex-shrink: 0;
  display: inline-flex;
`;

const KindMenu = styled.div`
  ${popupFocusHolder}
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

function MenuIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M2.5 4h11M2.5 8h11M2.5 12h11" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  );
}



