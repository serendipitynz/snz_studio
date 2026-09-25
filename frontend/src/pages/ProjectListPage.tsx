import { useTheme } from "@emotion/react";
import { DragEvent, FormEvent, useEffect, useState } from "react";
import { api, Project } from "../api/client";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import { useLanguage } from "../i18n";
import {
  Badge,
  Button,
  Card,
  ComposerBox,
  ErrorText,
  Field,
  Grid,
  Input,
  Item,
  List,
  MainPane,
  PaneBody,
  PaneHeader,
  RouterLink,
  SectionTitle,
  Stack,
  Textarea,
  WorkspaceShell
} from "../styles/ui";

export function ProjectListPage() {
  const theme = useTheme();
  const { t } = useLanguage();
  const [projects, setProjects] = useState<Project[]>([]);
  const [title, setTitle] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [draggedProjectId, setDraggedProjectId] = useState<string | null>(null);
  const [dropTargetProjectId, setDropTargetProjectId] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    setError("");
    try {
      const projectsResponse = await api.getProjects();
      setProjects(projectsResponse.projects);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("dashboard.loadError"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    setError("");

    try {
      await api.createProject({ title, description: "", systemPrompt });
      setTitle("");
      setSystemPrompt("");
      await load();
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("dashboard.createError"));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleProjectDrop(targetProjectId: string) {
    if (!draggedProjectId || draggedProjectId === targetProjectId) {
      setDraggedProjectId(null);
      setDropTargetProjectId(null);
      return;
    }

    const previousProjects = projects;
    const nextProjects = reorderProjects(previousProjects, draggedProjectId, targetProjectId);
    setProjects(nextProjects);
    setDraggedProjectId(null);
    setDropTargetProjectId(null);
    setError("");

    try {
      const response = await api.reorderProjects(nextProjects.map((project) => project.id));
      setProjects(response.projects);
    } catch (nextError) {
      setProjects(previousProjects);
      setError(nextError instanceof Error ? nextError.message : t("dashboard.reorderError"));
    }
  }

  return (
    <WorkspaceShell $columns="280px minmax(0, 1fr)">
      <WorkspaceSidebar projects={projects} />

      <MainPane>
        <PaneHeader>
          <SectionTitle>{t("dashboard.title")}</SectionTitle>
        </PaneHeader>

        <PaneBody>
          <Grid columns="1.05fr 0.95fr">
            <Card>
              <Stack>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <SectionTitle>{t("dashboard.projects")}</SectionTitle>
                  <Badge tone="accent">{t("dashboard.projectsCount", { count: projects.length })}</Badge>
                </div>
                {error ? <ErrorText>{error}</ErrorText> : null}
                <List>
                  {!loading && projects.length === 0 ? <Item>{t("dashboard.noProjects")}</Item> : null}
                  {projects.map((project) => (
                    <Item
                      key={project.id}
                      draggable
                      onDragStart={(event) => {
                        setDraggedProjectId(project.id);
                        event.dataTransfer.effectAllowed = "move";
                        event.dataTransfer.setData("text/plain", project.id);
                      }}
                      onDragOver={(event) => {
                        event.preventDefault();
                        if (draggedProjectId && draggedProjectId !== project.id) {
                          setDropTargetProjectId(project.id);
                        }
                      }}
                      onDragLeave={(event) => {
                        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
                          setDropTargetProjectId((current) => (current === project.id ? null : current));
                        }
                      }}
                      onDrop={(event) => {
                        event.preventDefault();
                        void handleProjectDrop(project.id);
                      }}
                      onDragEnd={() => {
                        setDraggedProjectId(null);
                        setDropTargetProjectId(null);
                      }}
                      style={{
                        padding: 0,
                        overflow: "hidden",
                        cursor: "grab",
                        borderColor: dropTargetProjectId === project.id ? theme.accentDragBorder : theme.line,
                        background:
                          draggedProjectId === project.id
                            ? theme.accentDragBg
                            : dropTargetProjectId === project.id
                              ? theme.accentSoft
                              : theme.surfaceItem
                      }}
                    >
                      <RouterLink
                        to={`/projects/${project.id}`}
                        style={{ display: "block", padding: 14, textDecoration: "none", color: "inherit" }}
                      >
                        <strong>{project.title}</strong>
                      </RouterLink>
                    </Item>
                  ))}
                </List>
              </Stack>
            </Card>

            <Card as="form" onSubmit={handleSubmit}>
              <Stack>
                <SectionTitle>{t("dashboard.createProject")}</SectionTitle>
                <ComposerBox>
                  <Field>
                    {t("dashboard.titleField")}
                    <Input
                      value={title}
                      onChange={(event) => setTitle(event.target.value)}
                      placeholder={t("dashboard.titlePlaceholder")}
                    />
                  </Field>
                  <Field>
                    {t("dashboard.systemPrompt")}
                    <Textarea
                      value={systemPrompt}
                      onChange={(event) => setSystemPrompt(event.target.value)}
                      placeholder={t("dashboard.systemPromptPlaceholder")}
                    />
                  </Field>
                  <Button type="submit" disabled={submitting}>
                    {submitting ? t("dashboard.creating") : t("dashboard.createButton")}
                  </Button>
                </ComposerBox>
              </Stack>
            </Card>
          </Grid>
        </PaneBody>
      </MainPane>
    </WorkspaceShell>
  );
}

function reorderProjects(projects: Project[], draggedProjectId: string, targetProjectId: string) {
  const next = [...projects];
  const draggedIndex = next.findIndex((project) => project.id === draggedProjectId);
  const targetIndex = next.findIndex((project) => project.id === targetProjectId);

  if (draggedIndex < 0 || targetIndex < 0 || draggedIndex === targetIndex) {
    return next;
  }

  const [dragged] = next.splice(draggedIndex, 1);
  next.splice(targetIndex, 0, dragged);
  return next;
}
