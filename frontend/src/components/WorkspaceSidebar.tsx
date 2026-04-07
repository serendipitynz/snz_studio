import { ChatRecord, Project } from "../api/client";
import {
  Divider,
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
  return (
    <SidebarPane>
      <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
        <RouterLink to="/" style={{ fontWeight: 700, letterSpacing: "0.08em" }}>
          SNZ STUDIO
        </RouterLink>
        <RouterLink to="/" aria-label="Home">
          <HomeIcon />
        </RouterLink>
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
            <SidebarSectionLabel>Chats</SidebarSectionLabel>
            {props.chats?.length ? (
              props.chats.map((chat) => (
                <SidebarLink key={chat.id} to={`/chats/${chat.id}`} $active={chat.id === props.activeChatId}>
                  {renderChatTitle(chat.title)}
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

function renderChatTitle(title: string) {
  if (title.trim()) {
    return <strong>{title}</strong>;
  }

  return <Subtle style={{ opacity: 0.78 }}>(undefined)</Subtle>;
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
