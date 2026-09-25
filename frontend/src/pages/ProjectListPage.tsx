import { FormEvent, useEffect, useState } from "react";
import { api, Project } from "../api/client";
import { FailureNotice } from "../components/FailureNotice";
import { ReorderList } from "../components/ReorderList";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import { useLanguage } from "../i18n";
import {
  Badge,
  Button,
  Card,
  ComposerBox,
  Field,
  Grid,
  Input,
  Item,
  MainPane,
  PaneBody,
  PaneHeader,
  RouterLink,
  SectionTitle,
  Stack,
  Textarea,
  VisuallyHidden,
  WorkspaceShell
} from "../styles/ui";

export function ProjectListPage() {
  const { t } = useLanguage();
  const [projects, setProjects] = useState<Project[]>([]);
  const [title, setTitle] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [savingOrder, setSavingOrder] = useState(false);
  // A failure is told next to what failed (snz-design doc-9 §5.5): loading and
  // reordering fail on the list, creating fails on the form.
  const [listError, setListError] = useState("");
  const [loadFailed, setLoadFailed] = useState(false);
  const [createError, setCreateError] = useState("");

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

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (submitting) {
      return;
    }
    setSubmitting(true);
    setCreateError("");

    try {
      await api.createProject({ title, description: "", systemPrompt });
      setTitle("");
      setSystemPrompt("");
      await load();
    } catch (nextError) {
      setCreateError(nextError instanceof Error ? nextError.message : t("dashboard.createError"));
    } finally {
      setSubmitting(false);
    }
  }

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
                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
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
                    <Badge tone="accent">{t("dashboard.projectsCount", { count: projects.length })}</Badge>
                  </div>
                </div>
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
                      style={{ display: "block", padding: "12px 8px", overflowWrap: "anywhere" }}
                    >
                      <strong>{project.title}</strong>
                    </RouterLink>
                  )}
                />
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
                  {createError ? <FailureNotice>{createError}</FailureNotice> : null}
                  {/* While the project is being created the button keeps its
                      focus, name and width: the busy figure takes the plus's
                      place and presses are ignored (snz-design doc-8 §6.1). */}
                  <Button
                    type="submit"
                    aria-busy={submitting || undefined}
                    title={submitting ? t("dashboard.creating") : undefined}
                    style={{ display: "inline-flex", alignItems: "center", justifyContent: "center", gap: 8 }}
                  >
                    {submitting ? <SpinnerIcon /> : <PlusIcon />}
                    {t("dashboard.createButton")}
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

function PlusIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M8 3v10M3 8h10" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
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
