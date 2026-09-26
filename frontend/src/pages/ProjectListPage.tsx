import { useEffect, useState } from "react";
import { api, Project } from "../api/client";
import { ActionButton } from "../components/ActionButton";
import { ConnectionStatusCard } from "../components/ConnectionStatusCard";
import { CreateProjectDialog } from "../components/CreateProjectDialog";
import { FailureNotice } from "../components/FailureNotice";
import { PlusIcon, SpinnerIcon } from "../components/icons";
import { RecentChatsCard } from "../components/RecentChatsCard";
import { ReorderList } from "../components/ReorderList";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import { useLanguage } from "../i18n";
import {
  Badge,
  Card,
  Grid,
  Item,
  MainPane,
  PaneBody,
  PaneHeader,
  RouterLink,
  Row,
  SectionTitle,
  Stack,
  Subtle,
  VisuallyHidden,
  WorkspaceShell
} from "../styles/ui";

export function ProjectListPage() {
  const { t } = useLanguage();
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [savingOrder, setSavingOrder] = useState(false);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  // A failure is told next to what failed (snz-design doc-9 §5.5): loading and
  // reordering fail on the list, creating fails in its dialog.
  const [listError, setListError] = useState("");
  const [loadFailed, setLoadFailed] = useState(false);

  async function load() {
    setLoading(true);
    try {
      const projectsResponse = await api.getProjects();
      setProjects(projectsResponse.projects);
      setListError("");
      setLoadFailed(false);
    } catch (nextError) {
      // The rows already on screen stay, so the failure does not read as an empty list.
      setListError(nextError instanceof Error ? nextError.message : t("dashboard.loadError"));
      setLoadFailed(true);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function handleReordered(projectId: string, toIndex: number) {
    const previousProjects = projects;
    const nextProjects = moveProject(previousProjects, projectId, toIndex);
    setProjects(nextProjects);
    setSavingOrder(true);
    setListError("");

    try {
      const response = await api.reorderProjects(nextProjects.map((project) => project.id));
      setProjects(response.projects);
    } catch (nextError) {
      setProjects(previousProjects);
      setListError(nextError instanceof Error ? nextError.message : t("dashboard.reorderError"));
    } finally {
      setSavingOrder(false);
    }
  }

  return (
    <WorkspaceShell $side={false}>
      <WorkspaceSidebar projects={projects} />

      <MainPane>
        <PaneHeader>
          <SectionTitle>{t("dashboard.title")}</SectionTitle>
        </PaneHeader>

        <PaneBody>
          {/* The owner's arrangement (TASK-49): recent chats, then the
              projects, then the connections, top to bottom and left to right. */}
          <Stack style={{ gap: 14 }}>
            <Grid columns="minmax(0, 1fr) minmax(0, 1fr)">
              <RecentChatsCard />

              <Card>
                <Stack>
                  <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                    {/* The count sits beside the heading it counts (owner, 2026-09-26). */}
                    <Row style={{ alignItems: "center", gap: 8 }}>
                      <SectionTitle>{t("dashboard.projects")}</SectionTitle>
                      <Badge tone="accent">{t("dashboard.projectsCount", { count: projects.length })}</Badge>
                      {/* The one busy display for a save of the order (doc-9 §5.6):
                          a figure beside the count, so the list keeps its size
                          and nothing under the pointer moves. */}
                      <span role="status" style={{ display: "inline-flex" }}>
                        {savingOrder ? (
                          <>
                            <SpinnerIcon />
                            <VisuallyHidden>{t("reorder.saving")}</VisuallyHidden>
                          </>
                        ) : null}
                      </span>
                    </Row>
                    <ActionButton type="button" icon={<PlusIcon />} onClick={() => setIsCreateOpen(true)}>
                      {t("dashboard.newProject")}
                    </ActionButton>
                  </Row>
                  {listError ? <FailureNotice>{listError}</FailureNotice> : null}
                  {!loading && !loadFailed && projects.length === 0 ? <Item>{t("dashboard.noProjects")}</Item> : null}
                  <ReorderList
                    items={projects}
                    label={t("dashboard.projectOrder")}
                    nameOf={(project) => project.title}
                    busy={savingOrder}
                    onReordered={(projectId, toIndex) => void handleReordered(projectId, toIndex)}
                    renderItem={(project) => (
                      <RouterLink
                        to={`/projects/${project.id}`}
                        style={{ display: "flex", flexDirection: "column", gap: 2, padding: "10px 8px", minWidth: 0 }}
                      >
                        <strong style={{ overflowWrap: "anywhere" }}>{project.title}</strong>
                        <Subtle>
                          {t("dashboard.projectMeta", {
                            count: project.chatCount,
                            time: new Date(project.lastActivityAt).toLocaleString()
                          })}
                        </Subtle>
                        {project.systemPrompt.trim() ? (
                          <Subtle style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                            {firstLine(project.systemPrompt)}
                          </Subtle>
                        ) : null}
                      </RouterLink>
                    )}
                  />
                </Stack>
              </Card>
            </Grid>

            <ConnectionStatusCard />
          </Stack>
        </PaneBody>
      </MainPane>

      {isCreateOpen ? (
        <CreateProjectDialog
          onClose={() => setIsCreateOpen(false)}
          onCreated={() => {
            setIsCreateOpen(false);
            void load();
          }}
        />
      ) : null}
    </WorkspaceShell>
  );
}

function moveProject(projects: Project[], projectId: string, toIndex: number) {
  const next = [...projects];
  const fromIndex = next.findIndex((project) => project.id === projectId);
  if (fromIndex < 0) {
    return next;
  }
  const [moved] = next.splice(fromIndex, 1);
  next.splice(toIndex, 0, moved);
  return next;
}

function firstLine(text: string) {
  return text.trim().split(/\r?\n/, 1)[0];
}
