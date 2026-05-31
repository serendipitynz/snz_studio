import { useTheme } from "@emotion/react";
import { DragEvent, FormEvent, useEffect, useState } from "react";
import { api, Project, WorkspaceConfiguration } from "../api/client";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import { useThemeController, ThemeMode } from "../styles/ThemeController";
import type { ThemeFamily } from "../styles/themes";
import {
  Badge,
  Button,
  Card,
  ComposerBox,
  ErrorText,
  Field,
  FieldHeader,
  Grid,
  IconButton,
  Input,
  InspectorPane,
  Item,
  List,
  MainPane,
  ModalCard,
  ModalOverlay,
  PaneBody,
  PaneHeader,
  RouterLink,
  Select,
  SectionTitle,
  Stack,
  StatusDot,
  Subtle,
  Textarea,
  WorkspaceShell
} from "../styles/ui";

export function ProjectListPage() {
  const theme = useTheme();
  const { family, mode, families, setFamily, setMode } = useThemeController();
  const [projects, setProjects] = useState<Project[]>([]);
  const [configuration, setConfiguration] = useState<WorkspaceConfiguration | null>(null);
  const [configDraft, setConfigDraft] = useState({
    llmBaseUrl: "",
    llmModel: "",
    llmResponseFormat: "standard" as "standard" | "llm_jp_thinking",
    reviewBaseUrl: "",
    reviewModel: "",
    embeddingBaseUrl: "",
    embeddingModel: ""
  });
  const [title, setTitle] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [savingConfig, setSavingConfig] = useState(false);
  const [isConfigModalOpen, setIsConfigModalOpen] = useState(false);
  const [llmModelOptions, setLlmModelOptions] = useState<string[]>([]);
  const [reviewModelOptions, setReviewModelOptions] = useState<string[]>([]);
  const [embeddingModelOptions, setEmbeddingModelOptions] = useState<string[]>([]);
  const [loadingLlmModels, setLoadingLlmModels] = useState(false);
  const [loadingReviewModels, setLoadingReviewModels] = useState(false);
  const [loadingEmbeddingModels, setLoadingEmbeddingModels] = useState(false);
  const [error, setError] = useState("");
  const [draggedProjectId, setDraggedProjectId] = useState<string | null>(null);
  const [dropTargetProjectId, setDropTargetProjectId] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    setError("");
    try {
      const [projectsResponse, configurationResponse] = await Promise.all([api.getProjects(), api.getConfiguration()]);
      setProjects(projectsResponse.projects);
      setConfiguration(configurationResponse.configuration);
      setConfigDraft({
        llmBaseUrl: configurationResponse.configuration.llmBaseUrl,
        llmModel: configurationResponse.configuration.llmModel,
        llmResponseFormat: configurationResponse.configuration.llmResponseFormat,
        reviewBaseUrl: configurationResponse.configuration.reviewBaseUrl,
        reviewModel: configurationResponse.configuration.reviewModel,
        embeddingBaseUrl: configurationResponse.configuration.embeddingBaseUrl,
        embeddingModel: configurationResponse.configuration.embeddingModel
      });
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to load projects");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    if (!isConfigModalOpen || !configDraft.llmBaseUrl.trim()) {
      return;
    }

    const timeout = window.setTimeout(() => {
      setLoadingLlmModels(true);
      api
        .listConfigurationModels({ kind: "llm", baseUrl: configDraft.llmBaseUrl })
        .then((response) => setLlmModelOptions(response.models))
        .catch(() => setLlmModelOptions([]))
        .finally(() => setLoadingLlmModels(false));
    }, 250);

    return () => window.clearTimeout(timeout);
  }, [configDraft.llmBaseUrl, isConfigModalOpen]);

  useEffect(() => {
    if (!isConfigModalOpen || !configDraft.reviewBaseUrl.trim()) {
      return;
    }

    const timeout = window.setTimeout(() => {
      setLoadingReviewModels(true);
      api
        .listConfigurationModels({ kind: "llm", baseUrl: configDraft.reviewBaseUrl })
        .then((response) => setReviewModelOptions(response.models))
        .catch(() => setReviewModelOptions([]))
        .finally(() => setLoadingReviewModels(false));
    }, 250);

    return () => window.clearTimeout(timeout);
  }, [configDraft.reviewBaseUrl, isConfigModalOpen]);

  useEffect(() => {
    if (!isConfigModalOpen || !configDraft.embeddingBaseUrl.trim()) {
      return;
    }

    const timeout = window.setTimeout(() => {
      setLoadingEmbeddingModels(true);
      api
        .listConfigurationModels({ kind: "embedding", baseUrl: configDraft.embeddingBaseUrl })
        .then((response) => setEmbeddingModelOptions(response.models))
        .catch(() => setEmbeddingModelOptions([]))
        .finally(() => setLoadingEmbeddingModels(false));
    }, 250);

    return () => window.clearTimeout(timeout);
  }, [configDraft.embeddingBaseUrl, isConfigModalOpen]);

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
      setError(nextError instanceof Error ? nextError.message : "Failed to create project");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleConfigurationSubmit(event: FormEvent) {
    event.preventDefault();
    setSavingConfig(true);
    setError("");

    try {
      const response = await api.updateConfiguration(configDraft);
      setConfiguration(response.configuration);
      setConfigDraft({
        llmBaseUrl: response.configuration.llmBaseUrl,
        llmModel: response.configuration.llmModel,
        llmResponseFormat: response.configuration.llmResponseFormat,
        reviewBaseUrl: response.configuration.reviewBaseUrl,
        reviewModel: response.configuration.reviewModel,
        embeddingBaseUrl: response.configuration.embeddingBaseUrl,
        embeddingModel: response.configuration.embeddingModel
      });
      setIsConfigModalOpen(false);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to save configuration");
    } finally {
      setSavingConfig(false);
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
      setError(nextError instanceof Error ? nextError.message : "Failed to reorder projects");
    }
  }

  return (
    <WorkspaceShell>
      <WorkspaceSidebar projects={projects} />

      <MainPane>
        <PaneHeader>
          <SectionTitle>Dashboard</SectionTitle>
        </PaneHeader>

        <PaneBody>
          <Grid columns="1.05fr 0.95fr">
            <Card>
              <Stack>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <SectionTitle>Projects</SectionTitle>
                  <Badge tone="accent">{projects.length} projects</Badge>
                </div>
                {error ? <ErrorText>{error}</ErrorText> : null}
                <List>
                  {!loading && projects.length === 0 ? <Item>No projects yet.</Item> : null}
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
                              : theme.surfaceElevate
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
                <SectionTitle>Create Project</SectionTitle>
                <ComposerBox>
                  <Field>
                    Title
                    <Input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="Local research workspace" />
                  </Field>
                  <Field>
                    System Prompt
                    <Textarea
                      value={systemPrompt}
                      onChange={(event) => setSystemPrompt(event.target.value)}
                      placeholder="Default assistant behavior for this project"
                    />
                  </Field>
                  <Button type="submit" disabled={submitting}>
                    {submitting ? "Creating..." : "Create project"}
                  </Button>
                </ComposerBox>
              </Stack>
            </Card>
          </Grid>
        </PaneBody>
      </MainPane>

      <InspectorPane>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
          <SectionTitle>Configuration</SectionTitle>
          <IconButton type="button" aria-label="Edit configuration" onClick={() => setIsConfigModalOpen(true)}>
            ✎
          </IconButton>
        </div>
        <Card>
          <Stack>
            <Badge tone="accent">App config</Badge>
            <Field>
              <FieldHeader>
                <span>LLM Endpoint</span>
                <StatusDot $connected={Boolean(configuration?.llmConnected)} />
              </FieldHeader>
              <Subtle>{configuration?.llmBaseUrl || "Not configured"}</Subtle>
            </Field>
            <Field>
              <FieldHeader>
                <span>LLM Model</span>
                <StatusDot $connected={Boolean(configuration?.llmConnected)} />
              </FieldHeader>
              <Subtle>{configuration?.llmModel || "Not configured"}</Subtle>
            </Field>
            <Field>
              <FieldHeader>
                <span>LLM Response Format</span>
                <StatusDot $connected={Boolean(configuration?.llmConnected)} />
              </FieldHeader>
              <Subtle>{configuration?.llmResponseFormat === "llm_jp_thinking" ? "LLM-jp Thinking" : "Standard"}</Subtle>
            </Field>
            <Field>
              <FieldHeader>
                <span>Review Endpoint</span>
                <StatusDot $connected={Boolean(configuration?.reviewConnected)} />
              </FieldHeader>
              <Subtle>{configuration?.reviewBaseUrl || "Not configured"}</Subtle>
            </Field>
            <Field>
              <FieldHeader>
                <span>Review Model</span>
                <StatusDot $connected={Boolean(configuration?.reviewConnected)} />
              </FieldHeader>
              <Subtle>{configuration?.reviewModel || "Not configured"}</Subtle>
            </Field>
            <Field>
              <FieldHeader>
                <span>Embedding Endpoint</span>
                <StatusDot $connected={Boolean(configuration?.embeddingConnected)} />
              </FieldHeader>
              <Subtle>{configuration?.embeddingBaseUrl || "Not configured"}</Subtle>
            </Field>
            <Field>
              <FieldHeader>
                <span>Embedding Model</span>
                <StatusDot $connected={Boolean(configuration?.embeddingConnected)} />
              </FieldHeader>
              <Subtle>{configuration?.embeddingModel || "Not configured"}</Subtle>
            </Field>
          </Stack>
        </Card>

        <Card>
          <Stack>
            <Badge tone="accent">Appearance</Badge>
            <Field>
              Theme
              <Select value={family} onChange={(event) => setFamily(event.target.value as ThemeFamily)}>
                {families.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.label}
                  </option>
                ))}
              </Select>
            </Field>
            <Field>
              Mode
              <Select value={mode} onChange={(event) => setMode(event.target.value as ThemeMode)}>
                <option value="light">Light</option>
                <option value="dark">Dark</option>
                <option value="auto">Auto (follow OS)</option>
              </Select>
            </Field>
          </Stack>
        </Card>
      </InspectorPane>

      {isConfigModalOpen ? (
        <ModalOverlay onClick={() => setIsConfigModalOpen(false)}>
          <ModalCard onClick={(event) => event.stopPropagation()}>
            <Stack as="form" onSubmit={handleConfigurationSubmit}>
              <SectionTitle>Edit Configuration</SectionTitle>
              <Field>
                LLM Endpoint
                <Input
                  value={configDraft.llmBaseUrl}
                  onChange={(event) => setConfigDraft((current) => ({ ...current, llmBaseUrl: event.target.value }))}
                  placeholder="http://127.0.0.1:1234/v1"
                />
              </Field>
              <Field>
                LLM Model
                <Input
                  list="llm-model-options"
                  value={configDraft.llmModel}
                  onChange={(event) => setConfigDraft((current) => ({ ...current, llmModel: event.target.value }))}
                  placeholder="openai/gpt-oss-20b"
                />
                <datalist id="llm-model-options">
                  {llmModelOptions.map((model) => (
                    <option key={model} value={model} />
                  ))}
                </datalist>
                <Subtle>{loadingLlmModels ? "Loading model candidates..." : llmModelOptions.length ? `${llmModelOptions.length} candidates found` : "No model candidates available"}</Subtle>
              </Field>
              <Field>
                LLM Response Format
                <Select
                  value={configDraft.llmResponseFormat}
                  onChange={(event) =>
                    setConfigDraft((current) => ({
                      ...current,
                      llmResponseFormat: event.target.value as "standard" | "llm_jp_thinking"
                    }))
                  }
                >
                  <option value="standard">Standard</option>
                  <option value="llm_jp_thinking">LLM-jp Thinking</option>
                </Select>
              </Field>
              <Field>
                Review Endpoint
                <Input
                  value={configDraft.reviewBaseUrl}
                  onChange={(event) => setConfigDraft((current) => ({ ...current, reviewBaseUrl: event.target.value }))}
                  placeholder="http://127.0.0.1:1234/v1"
                />
              </Field>
              <Field>
                Review Model
                <Input
                  list="review-model-options"
                  value={configDraft.reviewModel}
                  onChange={(event) => setConfigDraft((current) => ({ ...current, reviewModel: event.target.value }))}
                  placeholder="review model id"
                />
                <datalist id="review-model-options">
                  {reviewModelOptions.map((model) => (
                    <option key={model} value={model} />
                  ))}
                </datalist>
                <Subtle>
                  {loadingReviewModels
                    ? "Loading model candidates..."
                    : reviewModelOptions.length
                      ? `${reviewModelOptions.length} candidates found`
                      : "No model candidates available"}
                </Subtle>
              </Field>
              <Field>
                Embedding Endpoint
                <Input
                  value={configDraft.embeddingBaseUrl}
                  onChange={(event) => setConfigDraft((current) => ({ ...current, embeddingBaseUrl: event.target.value }))}
                  placeholder="http://127.0.0.1:8080/v1"
                />
              </Field>
              <Field>
                Embedding Model
                <Input
                  list="embedding-model-options"
                  value={configDraft.embeddingModel}
                  onChange={(event) => setConfigDraft((current) => ({ ...current, embeddingModel: event.target.value }))}
                  placeholder="text-embeddings-inference"
                />
                <datalist id="embedding-model-options">
                  {embeddingModelOptions.map((model) => (
                    <option key={model} value={model} />
                  ))}
                </datalist>
                <Subtle>
                  {loadingEmbeddingModels
                    ? "Loading model candidates..."
                    : embeddingModelOptions.length
                      ? `${embeddingModelOptions.length} candidates found`
                      : "No model candidates available"}
                </Subtle>
              </Field>
              <div style={{ display: "flex", justifyContent: "flex-end", gap: 10 }}>
                <Button type="button" variant="ghost" onClick={() => setIsConfigModalOpen(false)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={savingConfig}>
                  {savingConfig ? "Saving..." : "Save configuration"}
                </Button>
              </div>
            </Stack>
          </ModalCard>
        </ModalOverlay>
      ) : null}
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
