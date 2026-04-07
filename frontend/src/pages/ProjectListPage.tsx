import { FormEvent, useEffect, useState } from "react";
import { api, Project } from "../api/client";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import {
  Badge,
  Button,
  Card,
  ComposerBox,
  Field,
  Grid,
  Input,
  InspectorPane,
  Item,
  List,
  MainPane,
  PaneBody,
  PaneHeader,
  RouterLink,
  SectionTitle,
  Stack,
  Subtle,
  Textarea,
  WorkspaceShell
} from "../styles/ui";

export function ProjectListPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [title, setTitle] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      const response = await api.getProjects();
      setProjects(response.projects);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to load projects");
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
      setError(nextError instanceof Error ? nextError.message : "Failed to create project");
    } finally {
      setSubmitting(false);
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
                    <Item key={project.id}>
                      <RouterLink to={`/projects/${project.id}`}>
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
        <SectionTitle>Workspace Notes</SectionTitle>
        <Card>
          <Stack>
            <Badge tone="warm">Shared context</Badge>
            <Subtle>Each project owns its documents, memories, and chats.</Subtle>
          </Stack>
        </Card>
        <Card>
          <Stack>
            <Badge tone="accent">Retrieval</Badge>
            <Subtle>Chats use summary, procedural memory, and FTS-based document retrieval instead of replaying full history.</Subtle>
          </Stack>
        </Card>
        <Card>
          <Stack>
            <Badge tone="muted">Why this layout</Badge>
            <Subtle>The shell mirrors desktop LLM apps: navigation on the left, working surface in the center, context inspector on the right.</Subtle>
          </Stack>
        </Card>
      </InspectorPane>
    </WorkspaceShell>
  );
}
