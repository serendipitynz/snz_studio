import { FormEvent, useEffect, useState } from "react";
import { api, Project, WorkspaceConfiguration } from "../api/client";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import {
  Badge,
  Button,
  Card,
  ComposerBox,
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
  SectionTitle,
  Stack,
  StatusDot,
  Subtle,
  Textarea,
  WorkspaceShell
} from "../styles/ui";

export function ProjectListPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [configuration, setConfiguration] = useState<WorkspaceConfiguration | null>(null);
  const [configDraft, setConfigDraft] = useState({
    llmBaseUrl: "",
    llmModel: "",
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
  const [embeddingModelOptions, setEmbeddingModelOptions] = useState<string[]>([]);
  const [loadingLlmModels, setLoadingLlmModels] = useState(false);
  const [loadingEmbeddingModels, setLoadingEmbeddingModels] = useState(false);
  const [error, setError] = useState("");

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
                  <SectionTitle>Available Projects</SectionTitle>
                  <Badge tone="accent">{projects.length} projects</Badge>
                </div>
                {error ? <Subtle style={{ color: "#ff7a6c" }}>{error}</Subtle> : null}
                <List>
                  {!loading && projects.length === 0 ? <Item>No projects yet.</Item> : null}
                  {projects.map((project) => (
                    <Item key={project.id} style={{ padding: 0, overflow: "hidden" }}>
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
