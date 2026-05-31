import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, ChatRecord, Project } from "../api/client";
import { useThemeController } from "../styles/ThemeController";
import {
  Divider,
  IconButton,
  RouterLink,
  Row,
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

export function WorkspaceSidebar(props: WorkspaceSidebarProps) {
  const navigate = useNavigate();
  const { variant, setMode } = useThemeController();
  const [creatingChat, setCreatingChat] = useState(false);

  async function handleCreateChat() {
    if (!props.currentProjectId || creatingChat) {
      return;
    }

    setCreatingChat(true);

    try {
      const response = await api.createChat(props.currentProjectId, { title: "" });
      navigate(`/chats/${response.chat.id}`);
    } finally {
      setCreatingChat(false);
    }
  }

  return (
    <SidebarPane>
      <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
        <RouterLink to="/" style={{ fontWeight: 700, letterSpacing: "0.08em" }}>
          SNZ STUDIO
        </RouterLink>
        <Row style={{ gap: 8, flexWrap: "nowrap", alignItems: "center" }}>
          <IconButton
            type="button"
            aria-label="Toggle dark mode"
            title="Toggle dark mode"
            onClick={() => setMode(variant === "dark" ? "light" : "dark")}
          >
            {variant === "dark" ? <SunIcon /> : <MoonIcon />}
          </IconButton>
          <RouterLink to="/" aria-label="Home">
            <HomeIcon />
          </RouterLink>
        </Row>
      </Row>

      <Divider />

      <Stack style={{ minHeight: 0, overflow: "auto" }}>
        <SidebarSection>
          <SidebarSectionLabel>Projects</SidebarSectionLabel>
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
              <SidebarSectionLabel>Chats</SidebarSectionLabel>
              <IconButton
                type="button"
                aria-label="Create chat"
                onClick={() => void handleCreateChat()}
                disabled={creatingChat}
                title="Create chat"
              >
                <PlusIcon />
              </IconButton>
            </Row>
            {props.chats?.length ? (
              props.chats.map((chat) => (
                <SidebarLink key={chat.id} to={`/chats/${chat.id}`} $active={chat.id === props.activeChatId}>
                  {renderChatTitle(chat)}
                </SidebarLink>
              ))
            ) : (
              <Subtle>No chats yet.</Subtle>
            )}
          </SidebarSection>
        ) : null}
      </Stack>
    </SidebarPane>
  );
}

function renderChatTitle(chat: ChatRecord) {
  if (chat.title.trim()) {
    return (
      <strong>{chat.isTemporary ? `⏱️ ${chat.title}` : chat.title}</strong>
    );
  }

  return <Subtle style={{ opacity: 0.78 }}>{chat.isTemporary ? "⏱️ (undefined)" : "(undefined)"}</Subtle>;
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

function MoonIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true" style={{ display: "block" }}>
      <path
        d="M13 9.2A5 5 0 1 1 6.8 3a4 4 0 0 0 6.2 6.2Z"
        stroke="currentColor"
        strokeWidth="1.35"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function SunIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true" style={{ display: "block" }}>
      <circle cx="8" cy="8" r="3" stroke="currentColor" strokeWidth="1.35" />
      <path
        d="M8 1.5v1.6M8 12.9v1.6M1.5 8h1.6M12.9 8h1.6M3.4 3.4l1.1 1.1M11.5 11.5l1.1 1.1M12.6 3.4l-1.1 1.1M4.5 11.5l-1.1 1.1"
        stroke="currentColor"
        strokeWidth="1.35"
        strokeLinecap="round"
      />
    </svg>
  );
}
