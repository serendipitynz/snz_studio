import { KeyboardEvent as ReactKeyboardEvent, useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import styled from "@emotion/styled";
import { api, ChatKind, ChatRecord, Project } from "../api/client";
import { MessageKey, useLanguage } from "../i18n";
import { SettingsModal } from "./SettingsModal";
import {
  Divider,
  FOCUS_RING_REACH,
  IconButton,
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
  const [creatingChat, setCreatingChat] = useState(false);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [isKindMenuOpen, setIsKindMenuOpen] = useState(false);
  const anchorRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);

  const closeKindMenu = useCallback((restoreFocus: boolean) => {
    setIsKindMenuOpen(false);
    if (restoreFocus) {
      triggerRef.current?.focus();
    }
  }, []);

  useEffect(() => {
    if (!isKindMenuOpen) {
      return;
    }

    itemRefs.current[0]?.focus();

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        closeKindMenu(true);
      }
    }

    // A pointer outside the menu has already chosen where focus should land, so
    // this path closes without pulling focus back to the trigger.
    function handlePointerDown(event: PointerEvent) {
      if (!anchorRef.current?.contains(event.target as Node)) {
        setIsKindMenuOpen(false);
      }
    }

    document.addEventListener("keydown", handleKeyDown);
    document.addEventListener("pointerdown", handlePointerDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      document.removeEventListener("pointerdown", handlePointerDown);
    };
  }, [isKindMenuOpen, closeKindMenu]);

  // role="menu" promises arrow-key navigation; Tab alone would not honour it.
  function handleKindMenuKeyDown(event: ReactKeyboardEvent<HTMLDivElement>) {
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") {
      return;
    }

    event.preventDefault();
    const items = itemRefs.current.filter((item): item is HTMLButtonElement => item !== null);
    if (!items.length) {
      return;
    }

    const step = event.key === "ArrowDown" ? 1 : -1;
    const current = items.indexOf(document.activeElement as HTMLButtonElement);
    items[(current + step + items.length) % items.length]?.focus();
  }

  async function handleCreateChat(kind: ChatKind) {
    if (!props.currentProjectId || creatingChat) {
      return;
    }

    closeKindMenu(false);
    setCreatingChat(true);

    try {
      const response = await api.createChat(props.currentProjectId, { title: "", kind });
      navigate(`/chats/${response.chat.id}`);
    } finally {
      setCreatingChat(false);
    }
  }

  return (
    <SidebarPane>
      <Row style={{ justifyContent: "space-between", alignItems: "center", flexShrink: 0 }}>
        <RouterLink to="/" style={{ fontWeight: 700, letterSpacing: "0.08em" }}>
          SNZ STUDIO
        </RouterLink>
        <RouterLink to="/" aria-label={t("sidebar.home")}>
          <HomeIcon />
        </RouterLink>
      </Row>

      <Divider />

      <Stack
        style={{
          flex: "1 1 auto",
          minHeight: 0,
          overflow: "auto",
          padding: FOCUS_RING_REACH,
          margin: `calc(-1 * ${FOCUS_RING_REACH})`
        }}
      >
        <SidebarSection>
          <SidebarSectionLabel>{t("sidebar.projects")}</SidebarSectionLabel>
          {props.projects.map((project) => (
            <SidebarLink key={project.id} to={`/projects/${project.id}`} $active={project.id === props.currentProjectId}>
              <Row style={{ alignItems: "center", gap: 8, flexWrap: "nowrap" }}>
                <strong style={{ minWidth: 0, overflowWrap: "anywhere" }}>{project.title}</strong>
                <Subtle style={{ opacity: 0.72, flexShrink: 0 }}>({project.chatCount})</Subtle>
              </Row>
            </SidebarLink>
          ))}
        </SidebarSection>

        {props.currentProjectId ? (
          <SidebarSection>
            <Row style={{ justifyContent: "space-between", alignItems: "center", flexWrap: "nowrap" }}>
              <SidebarSectionLabel>{t("sidebar.chats")}</SidebarSectionLabel>
              <MenuAnchor ref={anchorRef}>
                <IconButton
                  ref={triggerRef}
                  type="button"
                  aria-label={t("sidebar.createChat")}
                  aria-haspopup="menu"
                  aria-expanded={isKindMenuOpen}
                  onClick={() => setIsKindMenuOpen((open) => !open)}
                  disabled={creatingChat}
                  title={t("sidebar.createChat")}
                >
                  <PlusIcon />
                </IconButton>
                {isKindMenuOpen ? (
                  <KindMenu role="menu" aria-label={t("multiAgent.chatType")} onKeyDown={handleKindMenuKeyDown}>
                    {CHAT_KIND_CHOICES.map((choice, index) => (
                      <KindMenuItem
                        key={choice.kind}
                        ref={(element) => {
                          itemRefs.current[index] = element;
                        }}
                        type="button"
                        role="menuitem"
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
                <SidebarLink key={chat.id} to={`/chats/${chat.id}`} $active={chat.id === props.activeChatId}>
                  {renderChatTitle(t, chat)}
                </SidebarLink>
              ))
            ) : (
              <Subtle>{t("sidebar.noChats")}</Subtle>
            )}
          </SidebarSection>
        ) : null}
      </Stack>

      <Divider />

      <SidebarButton type="button" onClick={() => setIsSettingsOpen(true)} style={{ flexShrink: 0 }}>
        <GearIcon />
        <span>{t("sidebar.settings")}</span>
      </SidebarButton>

      {isSettingsOpen ? <SettingsModal onClose={() => setIsSettingsOpen(false)} /> : null}
    </SidebarPane>
  );
}

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
  border: 1px solid ${({ theme }) => theme.lineMedium};
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
`;

function renderChatTitle(t: (key: MessageKey) => string, chat: ChatRecord) {
  // The two markers are independent facts about the chat, so both can show.
  const marker = `${chat.kind === "multi_agent" ? "👥" : ""}${chat.isTemporary ? "⏱️" : ""}`;
  const prefix = marker ? `${marker} ` : "";

  if (chat.title.trim()) {
    return <strong>{`${prefix}${chat.title}`}</strong>;
  }

  return <Subtle style={{ opacity: 0.78 }}>{`${prefix}${t("sidebar.untitled")}`}</Subtle>;
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
