import { DragEvent, FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  api,
  ChatRecord,
  DocumentCategory,
  DocumentRecord,
  MemoryKind,
  MemoryOrganizationPlan,
  MemoryRecord,
  Project
} from "../api/client";
import { MarkdownPreview } from "../components/MarkdownPreview";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import {
  Badge,
  Button,
  Card,
  ComposerBox,
  DropZone,
  Field,
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
  Row,
  RouterLink,
  SectionTitle,
  Select,
  Stack,
  Subtle,
  Textarea,
  WorkspaceShell
} from "../styles/ui";

interface ProjectDetailState {
  project: Project;
  documents: DocumentRecord[];
  memories: MemoryRecord[];
  chats: ChatRecord[];
}

export function ProjectDetailPage() {
  const { projectId = "" } = useParams();
  const navigate = useNavigate();
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const [state, setState] = useState<ProjectDetailState | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [chatTitle, setChatTitle] = useState("");
  const [memoryKind, setMemoryKind] = useState<MemoryKind>("semantic");
  const [memoryContent, setMemoryContent] = useState("");
  const [memoryLocked, setMemoryLocked] = useState(true);
  const [busy, setBusy] = useState(false);
  const [uploadStatus, setUploadStatus] = useState("");
  const [dragActive, setDragActive] = useState(false);
  const [pendingDeleteDocumentId, setPendingDeleteDocumentId] = useState<string | null>(null);
  const [pendingDeleteChatId, setPendingDeleteChatId] = useState<string | null>(null);
  const [pendingDeleteMemoryId, setPendingDeleteMemoryId] = useState<string | null>(null);
  const [selectedDocument, setSelectedDocument] = useState<DocumentRecord | null>(null);
  const [documentCategoryDraft, setDocumentCategoryDraft] = useState<DocumentCategory>("misc");
  const [isMemoryModalOpen, setIsMemoryModalOpen] = useState(false);
  const [isTitleModalOpen, setIsTitleModalOpen] = useState(false);
  const [isSystemPromptModalOpen, setIsSystemPromptModalOpen] = useState(false);
  const [titleDraft, setTitleDraft] = useState("");
  const [systemPromptDraft, setSystemPromptDraft] = useState("");
  const [memoryPlan, setMemoryPlan] = useState<MemoryOrganizationPlan | null>(null);
  const [organizingMemories, setOrganizingMemories] = useState(false);

  async function load() {
    setLoading(true);
    setError("");
    try {
      const [projectResponse, projectsResponse] = await Promise.all([api.getProjectDetail(projectId), api.getProjects()]);
      setState(projectResponse);
      setProjects(projectsResponse.projects);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to load project");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [projectId]);

  useEffect(() => {
    setTitleDraft(state?.project.title ?? "");
  }, [state?.project.title]);

  useEffect(() => {
    setSystemPromptDraft(state?.project.systemPrompt ?? "");
  }, [state?.project.systemPrompt]);

  useEffect(() => {
    setDocumentCategoryDraft(selectedDocument?.category ?? "misc");
  }, [selectedDocument]);

  const groupedMemories = useMemo(() => {
    const base = {
      semantic: [] as MemoryRecord[],
      procedural: [] as MemoryRecord[],
      episodic: [] as MemoryRecord[]
    };

    state?.memories.forEach((memory) => {
      base[memory.kind].push(memory);
    });

    return base;
  }, [state]);

  async function handleCreateChat(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      const response = await api.createChat(projectId, { title: chatTitle });
      navigate(`/chats/${response.chat.id}`);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to create chat");
    } finally {
      setBusy(false);
    }
  }

  async function handleCreateMemory(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      await api.createMemory(projectId, { content: memoryContent, kind: memoryKind, locked: memoryLocked });
      setMemoryContent("");
      setMemoryLocked(true);
      setMemoryPlan(null);
      await load();
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to create memory");
    } finally {
      setBusy(false);
    }
  }

  async function handleAnalyzeMemories() {
    setIsMemoryModalOpen(true);
    setOrganizingMemories(true);
    setError("");

    try {
      const response = await api.analyzeMemoryOrganization(projectId);
      setMemoryPlan(response.plan);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to analyze memories");
    } finally {
      setOrganizingMemories(false);
    }
  }

  async function handleApplyMemoryPlan() {
    if (!memoryPlan) {
      return;
    }

    setOrganizingMemories(true);
    setError("");

    try {
      const response = await api.applyMemoryOrganization(projectId, memoryPlan);
      setState((current) => (current ? { ...current, memories: response.memories } : current));
      setMemoryPlan(null);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to apply memory organization");
    } finally {
      setOrganizingMemories(false);
    }
  }

  async function handleToggleMemoryLock(memoryId: string, locked: boolean) {
    setBusy(true);
    setError("");

    try {
      const response = await api.updateMemoryLock(memoryId, locked);
      setState((current) =>
        current
          ? {
              ...current,
              memories: current.memories
                .map((memory) => (memory.id === response.memory.id ? response.memory : memory))
                .sort((left, right) => {
                  if (left.locked !== right.locked) {
                    return left.locked ? -1 : 1;
                  }

                  return new Date(right.updatedAt).getTime() - new Date(left.updatedAt).getTime();
                })
            }
          : current
      );
      setMemoryPlan(null);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to update memory");
    } finally {
      setBusy(false);
    }
  }

  async function handleDeleteMemory(memoryId: string) {
    setBusy(true);
    setError("");

    try {
      await api.deleteMemory(memoryId);
      setState((current) =>
        current
          ? {
              ...current,
              memories: current.memories.filter((memory) => memory.id !== memoryId)
            }
          : current
      );
      setPendingDeleteMemoryId(null);
      setMemoryPlan(null);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to delete memory");
    } finally {
      setBusy(false);
    }
  }

  async function handleDeleteProject() {
    if (!state) {
      return;
    }

    const confirmed = window.confirm(`Delete project "${state.project.title}" and all its chats, documents, memories, and summaries?`);
    if (!confirmed) {
      return;
    }

    setBusy(true);
    setError("");

    try {
      await api.deleteProject(state.project.id);
      navigate("/");
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to delete project");
    } finally {
      setBusy(false);
    }
  }

  async function handleUpdateProjectTitle(event: FormEvent) {
    event.preventDefault();
    if (!state || !titleDraft.trim()) {
      return;
    }

    setBusy(true);
    setError("");

    try {
      const response = await api.updateProjectTitle(state.project.id, titleDraft);
      setState((current) => (current ? { ...current, project: response.project } : current));
      setProjects((current) => current.map((project) => (project.id === response.project.id ? response.project : project)));
      setIsTitleModalOpen(false);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to update project title");
    } finally {
      setBusy(false);
    }
  }

  async function handleUpdateProjectSystemPrompt(event: FormEvent) {
    event.preventDefault();
    if (!state) {
      return;
    }

    setBusy(true);
    setError("");

    try {
      const response = await api.updateProjectSystemPrompt(state.project.id, systemPromptDraft);
      setState((current) => (current ? { ...current, project: response.project } : current));
      setProjects((current) => current.map((project) => (project.id === response.project.id ? response.project : project)));
      setIsSystemPromptModalOpen(false);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to update system prompt");
    } finally {
      setBusy(false);
    }
  }

  async function handleUploadFiles(fileList: FileList | File[]) {
    if (!state) {
      return;
    }

    const files = Array.from(fileList);
    if (!files.length) {
      return;
    }

    setBusy(true);
    setError("");
    setUploadStatus("");

    try {
      let currentDocuments = [...state.documents];

      for (const file of files) {
        const nextType = detectDocumentType(file);
        if (!nextType) {
          throw new Error(`Unsupported file type: ${file.name}`);
        }

        const existing = currentDocuments.find((document) => document.title === file.name);
        if (existing) {
          const overwrite = window.confirm(`"${file.name}" already exists. Overwrite the existing document?`);
          if (!overwrite) {
            continue;
          }

          await api.deleteDocument(existing.id);
          currentDocuments = currentDocuments.filter((document) => document.id !== existing.id);
        }

        const formData = new FormData();
        formData.set("type", nextType);
        formData.set("title", file.name);
        formData.set("file", file);
        setUploadStatus(`Saving ${file.name} and generating embeddings...`);
        const response = await api.createDocument(projectId, formData);
        currentDocuments = [response.document, ...currentDocuments];
      }

      await load();
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to upload document");
    } finally {
      setBusy(false);
      setUploadStatus("");
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
      }
    }
  }

  async function handleDeleteDocument(documentId: string) {
    setBusy(true);
    setError("");

    try {
      await api.deleteDocument(documentId);
      setPendingDeleteDocumentId(null);
      setSelectedDocument((current) => (current?.id === documentId ? null : current));
      await load();
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to delete document");
    } finally {
      setBusy(false);
    }
  }

  async function handleUpdateDocumentCategory(documentId: string, category: DocumentCategory) {
    setBusy(true);
    setError("");

    try {
      const response = await api.updateDocumentCategory(documentId, category);
      setState((current) =>
        current
          ? {
              ...current,
              documents: current.documents.map((document) =>
                document.id === response.document.id ? response.document : document
              )
            }
          : current
      );
      setSelectedDocument(response.document);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to update document category");
    } finally {
      setBusy(false);
    }
  }

  async function handleDeleteChat(chatId: string) {
    setBusy(true);
    setError("");

    try {
      await api.deleteChat(chatId);
      setPendingDeleteChatId(null);
      await load();
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to delete chat");
    } finally {
      setBusy(false);
    }
  }

  function handleDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragActive(false);
    void handleUploadFiles(event.dataTransfer.files);
  }

  function handleDragOver(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragActive(true);
  }

  function handleDragLeave(event: DragEvent<HTMLDivElement>) {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
      setDragActive(false);
    }
  }

  if (loading) {
    return <Card>Loading project…</Card>;
  }

  if (!state) {
    return <Card>{error || "Project not found"}</Card>;
  }

  return (
    <>
      <WorkspaceShell>
        <WorkspaceSidebar projects={projects} currentProjectId={state.project.id} chats={state.chats} />

        <MainPane>
          <PaneHeader>
            <Row style={{ alignItems: "center" }}>
              <FolderIcon />
              <SectionTitle>{state.project.title}</SectionTitle>
            </Row>
            <IconButton type="button" aria-label="Edit project title" onClick={() => setIsTitleModalOpen(true)}>
              <EditIcon />
            </IconButton>
          </PaneHeader>

          <PaneBody>
            <Stack>
              {error ? <Subtle style={{ color: "#dc322f" }}>{error}</Subtle> : null}

              <Card as="form" onSubmit={handleCreateChat}>
                <Stack>
                  <SectionTitle>New Chat</SectionTitle>
                  <ComposerBox>
                    <Field>
                      Chat title
                      <Input
                        value={chatTitle}
                        onChange={(event) => setChatTitle(event.target.value)}
                        placeholder="Architecture review"
                      />
                    </Field>
                    <Button type="submit" disabled={busy}>
                      Open chat
                    </Button>
                  </ComposerBox>
                </Stack>
              </Card>

              <Card>
                <Stack>
                  <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                    <SectionTitle>Documents</SectionTitle>
                    <IconButton type="button" onClick={() => fileInputRef.current?.click()} aria-label="Add document" disabled={busy}>
                      <PlusIcon />
                    </IconButton>
                  </Row>
                  {uploadStatus ? (
                    <Row style={{ alignItems: "center", gap: 10 }}>
                      <SpinnerIcon />
                      <Subtle>{uploadStatus}</Subtle>
                    </Row>
                  ) : null}

                  <input
                    ref={fileInputRef}
                    type="file"
                    multiple
                    accept=".md,.markdown,.txt,image/*"
                    style={{ display: "none" }}
                    onChange={(event) => {
                      if (event.target.files) {
                        void handleUploadFiles(event.target.files);
                      }
                    }}
                  />

                  <DropZone $active={dragActive} onDrop={handleDrop} onDragOver={handleDragOver} onDragLeave={handleDragLeave}>
                    <Stack>
                      <Subtle>Drop markdown, text, or image files here, or use the + button to choose files.</Subtle>
                      <Subtle>When a file name already exists, you will be asked whether to overwrite it.</Subtle>
                      {uploadStatus ? <Subtle>Embeddings are created synchronously before the document becomes available.</Subtle> : null}
                    </Stack>
                  </DropZone>

                  <List>
                    {state.documents.length === 0 ? <Item>No documents yet.</Item> : null}
                    {state.documents.map((document) => (
                      <Item
                        key={document.id}
                        style={{ cursor: "pointer", position: "relative" }}
                        onClick={() => setSelectedDocument(document)}
                      >
                        <Row style={{ justifyContent: "space-between", alignItems: "flex-start" }}>
                          <div style={{ minWidth: 0, flex: 1 }}>
                            <Row style={{ alignItems: "center" }}>
                              <strong style={{ overflowWrap: "anywhere" }}>{document.title}</strong>
                              <Badge tone={categoryTone(document.category)}>{document.category}</Badge>
                              <Badge tone={document.type === "image" ? "warm" : "accent"}>{document.type}</Badge>
                            </Row>
                            <Subtle>{describeDocument(document)}</Subtle>
                          </div>

                          <div style={{ position: "relative" }}>
                            <IconButton
                              type="button"
                              aria-label="Delete document"
                              onClick={(event) => {
                                event.stopPropagation();
                                setPendingDeleteDocumentId((current) => (current === document.id ? null : document.id));
                              }}
                            >
                              <TrashIcon />
                            </IconButton>

                            {pendingDeleteDocumentId === document.id ? (
                              <div
                                onClick={(event) => event.stopPropagation()}
                                style={{
                                  position: "absolute",
                                  right: 0,
                                  top: 40,
                                  width: 210,
                                  zIndex: 2,
                                  padding: 12,
                                  borderRadius: 14,
                                  border: "1px solid rgba(101, 123, 131, 0.18)",
                                  background: "#fffaf0",
                                  boxShadow: "0 12px 28px rgba(88, 110, 117, 0.18)"
                                }}
                              >
                                <Stack>
                                  <Subtle>Delete this document?</Subtle>
                                  <Row>
                                    <Button
                                      type="button"
                                      variant="ghost"
                                      onClick={() => setPendingDeleteDocumentId(null)}
                                    >
                                      Cancel
                                    </Button>
                                    <Button
                                      type="button"
                                      variant="warm"
                                      onClick={() => void handleDeleteDocument(document.id)}
                                    >
                                      OK
                                    </Button>
                                  </Row>
                                </Stack>
                              </div>
                            ) : null}
                          </div>
                        </Row>
                      </Item>
                    ))}
                  </List>
                </Stack>
              </Card>

              <Card>
                <Stack>
                  <Badge tone="muted">Danger zone</Badge>
                  <Subtle>Delete the entire project and all associated chats, documents, memories, summaries, and uploaded images.</Subtle>
                  <div>
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={handleDeleteProject}
                      disabled={busy}
                      style={{ borderColor: "#dc322f55", color: "#dc322f" }}
                    >
                      Delete project
                    </Button>
                  </div>
                </Stack>
              </Card>
            </Stack>
          </PaneBody>
        </MainPane>

        <InspectorPane>
          <SectionTitle>Project Assets</SectionTitle>

          <Card>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <Badge tone="accent">System prompt</Badge>
                <IconButton type="button" aria-label="Edit system prompt" onClick={() => setIsSystemPromptModalOpen(true)}>
                  <EditIcon />
                </IconButton>
              </Row>
              <Subtle>{state.project.systemPrompt || "No project system prompt configured."}</Subtle>
            </Stack>
          </Card>

          <Card>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <Badge tone="muted">Memories</Badge>
                <IconButton type="button" aria-label="Edit memories" onClick={() => setIsMemoryModalOpen(true)}>
                  <EditIcon />
                </IconButton>
              </Row>
              {(["procedural", "semantic", "episodic"] as const).map((kind) => (
                <Stack key={kind}>
                  <Subtle>{kind}</Subtle>
                  {groupedMemories[kind].slice(0, 4).map((memory) => (
                    <Item key={memory.id}>
                      <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                        <strong>{memory.title}</strong>
                        <Row style={{ alignItems: "center", flexWrap: "nowrap" }}>
                          <Badge tone="muted">{memory.source}</Badge>
                          {memory.locked ? <Badge tone="warm">locked</Badge> : null}
                        </Row>
                      </Row>
                      <Subtle>{memory.content}</Subtle>
                    </Item>
                  ))}
                  {groupedMemories[kind].length === 0 ? <Subtle>No {kind} memory.</Subtle> : null}
                </Stack>
              ))}
            </Stack>
          </Card>

          <Card>
            <Stack>
              <Badge tone="accent">Chats</Badge>
              <List>
                {state.chats.length === 0 ? <Subtle>No chats yet.</Subtle> : null}
                {state.chats.slice(0, 8).map((chat) => (
                  <Item key={chat.id} style={{ position: "relative" }}>
                    <Row style={{ justifyContent: "space-between", alignItems: "flex-start" }}>
                      <div style={{ minWidth: 0, flex: 1 }}>
                        <RouterLink to={`/chats/${chat.id}`}>
                          {chat.title.trim() ? <strong>{chat.title}</strong> : <Subtle style={{ opacity: 0.78 }}>(undefined)</Subtle>}
                        </RouterLink>
                        <Subtle>{new Date(chat.updatedAt).toLocaleString()}</Subtle>
                      </div>

                      <div style={{ position: "relative" }}>
                        <IconButton
                          type="button"
                          aria-label="Delete chat"
                          onClick={() => setPendingDeleteChatId((current) => (current === chat.id ? null : chat.id))}
                        >
                          <TrashIcon />
                        </IconButton>

                        {pendingDeleteChatId === chat.id ? (
                          <div
                            style={{
                              position: "absolute",
                              right: 0,
                              top: 40,
                              width: 210,
                              zIndex: 2,
                              padding: 12,
                              borderRadius: 14,
                              border: "1px solid rgba(101, 123, 131, 0.18)",
                              background: "#fffaf0",
                              boxShadow: "0 12px 28px rgba(88, 110, 117, 0.18)"
                            }}
                          >
                            <Stack>
                              <Subtle>Delete this chat?</Subtle>
                              <Row>
                                <Button type="button" variant="ghost" onClick={() => setPendingDeleteChatId(null)}>
                                  Cancel
                                </Button>
                                <Button type="button" variant="warm" onClick={() => void handleDeleteChat(chat.id)}>
                                  OK
                                </Button>
                              </Row>
                            </Stack>
                          </div>
                        ) : null}
                      </div>
                    </Row>
                  </Item>
                ))}
              </List>
            </Stack>
          </Card>

        </InspectorPane>
      </WorkspaceShell>

      {selectedDocument ? (
        <ModalOverlay onClick={() => setSelectedDocument(null)}>
          <ModalCard onClick={(event) => event.stopPropagation()}>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <div>
                  <SectionTitle>{selectedDocument.title}</SectionTitle>
                  <Subtle>
                    {selectedDocument.type} · {selectedDocument.category}
                  </Subtle>
                </div>
                <Button type="button" variant="ghost" onClick={() => setSelectedDocument(null)}>
                  Close
                </Button>
              </Row>

              {selectedDocument.tags.length ? <Subtle>Tags: {selectedDocument.tags.join(", ")}</Subtle> : null}
              {selectedDocument.note ? <Subtle>{selectedDocument.note}</Subtle> : null}

              <Card>
                <Stack>
                  <Field>
                    Category
                    <Select
                      value={documentCategoryDraft}
                      onChange={(event) => setDocumentCategoryDraft(event.target.value as DocumentCategory)}
                    >
                      {DOCUMENT_CATEGORIES.map((category) => (
                        <option key={category} value={category}>
                          {category}
                        </option>
                      ))}
                    </Select>
                  </Field>
                  <div>
                    <Button
                      type="button"
                      disabled={busy || documentCategoryDraft === selectedDocument.category}
                      onClick={() => void handleUpdateDocumentCategory(selectedDocument.id, documentCategoryDraft)}
                    >
                      Save category
                    </Button>
                  </div>
                </Stack>
              </Card>

              <Card>
                {selectedDocument.type === "image" && selectedDocument.filePath ? (
                  <img
                    src={selectedDocument.filePath}
                    alt={selectedDocument.title}
                    style={{ width: "100%", borderRadius: 16, display: "block" }}
                  />
                ) : null}

                {selectedDocument.type === "markdown" ? (
                  <MarkdownPreview source={selectedDocument.contentText} />
                ) : null}

                {selectedDocument.type === "text" ? (
                  <pre style={{ margin: 0, whiteSpace: "pre-wrap", lineHeight: 1.7 }}>{selectedDocument.contentText}</pre>
                ) : null}

                {selectedDocument.type === "image" && selectedDocument.derivedText ? (
                  <div style={{ marginTop: 14 }}>
                    <Badge tone="warm">Derived text</Badge>
                    <div style={{ marginTop: 10, whiteSpace: "pre-wrap", lineHeight: 1.7 }}>{selectedDocument.derivedText}</div>
                  </div>
                ) : null}
              </Card>
            </Stack>
          </ModalCard>
        </ModalOverlay>
      ) : null}

      {isTitleModalOpen ? (
        <ModalOverlay onClick={() => setIsTitleModalOpen(false)}>
          <ModalCard onClick={(event) => event.stopPropagation()}>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <SectionTitle>Edit Project Title</SectionTitle>
                <Button type="button" variant="ghost" onClick={() => setIsTitleModalOpen(false)}>
                  Close
                </Button>
              </Row>
              <Card as="form" onSubmit={handleUpdateProjectTitle}>
                <Stack>
                  <Field>
                    Title
                    <Input value={titleDraft} onChange={(event) => setTitleDraft(event.target.value)} placeholder="Project title" />
                  </Field>
                  <div>
                    <Button type="submit" disabled={busy || !titleDraft.trim()}>
                      Save title
                    </Button>
                  </div>
                </Stack>
              </Card>
            </Stack>
          </ModalCard>
        </ModalOverlay>
      ) : null}

      {isSystemPromptModalOpen ? (
        <ModalOverlay onClick={() => setIsSystemPromptModalOpen(false)}>
          <ModalCard onClick={(event) => event.stopPropagation()}>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <SectionTitle>Edit System Prompt</SectionTitle>
                <Button type="button" variant="ghost" onClick={() => setIsSystemPromptModalOpen(false)}>
                  Close
                </Button>
              </Row>
              <Card as="form" onSubmit={handleUpdateProjectSystemPrompt}>
                <Stack>
                  <Field>
                    System Prompt
                    <Textarea
                      value={systemPromptDraft}
                      onChange={(event) => setSystemPromptDraft(event.target.value)}
                      placeholder="Project-wide assistant instructions"
                    />
                  </Field>
                  <div>
                    <Button type="submit" disabled={busy}>
                      Save system prompt
                    </Button>
                  </div>
                </Stack>
              </Card>
            </Stack>
          </ModalCard>
        </ModalOverlay>
      ) : null}

      {isMemoryModalOpen ? (
        <ModalOverlay onClick={() => setIsMemoryModalOpen(false)}>
          <ModalCard onClick={(event) => event.stopPropagation()}>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <SectionTitle>Project Memories</SectionTitle>
                <Row style={{ alignItems: "center", flexWrap: "nowrap" }}>
                  <Button type="button" variant="ghost" onClick={() => void handleAnalyzeMemories()} disabled={organizingMemories}>
                    {organizingMemories ? "Organizing..." : "Organize"}
                  </Button>
                  <Button type="button" variant="ghost" onClick={() => setIsMemoryModalOpen(false)}>
                    Close
                  </Button>
                </Row>
              </Row>

              {memoryPlan ? (
                <Card>
                  <Stack>
                    <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                      <SectionTitle>Organization Plan</SectionTitle>
                      <Button
                        type="button"
                        onClick={() => void handleApplyMemoryPlan()}
                        disabled={organizingMemories || memoryPlan.changes.length === 0}
                      >
                        Apply
                      </Button>
                    </Row>
                    <Subtle>{memoryPlan.summary || "No summary."}</Subtle>
                    {memoryPlan.changes.length === 0 ? (
                      <Subtle>No changes suggested.</Subtle>
                    ) : (
                      <List>
                        {memoryPlan.changes.map((change, index) => (
                          <Item key={`${change.action}-${change.memoryId ?? change.title ?? index}`}>
                            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                              <strong>{change.title || change.memoryId || change.action}</strong>
                              <Badge tone={change.action === "remove" ? "warm" : change.action === "update" ? "accent" : "muted"}>
                                {change.action}
                              </Badge>
                            </Row>
                            {change.kind ? <Subtle>{change.kind}</Subtle> : null}
                            {change.content ? <Subtle>{change.content}</Subtle> : null}
                            <Subtle>{change.reason}</Subtle>
                          </Item>
                        ))}
                      </List>
                    )}
                  </Stack>
                </Card>
              ) : null}

              <Card as="form" onSubmit={handleCreateMemory}>
                <Stack>
                  <SectionTitle>Add Memory</SectionTitle>
                  <ComposerBox>
                    <Field>
                      Kind
                      <Select value={memoryKind} onChange={(event) => setMemoryKind(event.target.value as MemoryKind)}>
                        <option value="semantic">semantic (stable facts)</option>
                        <option value="procedural">procedural (how to work)</option>
                        <option value="episodic">episodic (past decisions or events)</option>
                      </Select>
                    </Field>
                    <Field>
                      Content
                      <Textarea
                        value={memoryContent}
                        onChange={(event) => setMemoryContent(event.target.value)}
                        placeholder="Durable fact worth carrying across chats"
                      />
                    </Field>
                    <label style={{ display: "flex", alignItems: "center", gap: 10 }}>
                      <input type="checkbox" checked={memoryLocked} onChange={(event) => setMemoryLocked(event.target.checked)} />
                      <span>Lock this memory so organizer does not rewrite or remove it</span>
                    </label>
                    <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                      <Button type="submit" disabled={busy}>
                        Save memory
                      </Button>
                    </Row>
                  </ComposerBox>
                </Stack>
              </Card>

              <Grid columns="1fr 1fr 1fr">
                {(["procedural", "semantic", "episodic"] as const).map((kind) => (
                  <Card key={kind}>
                    <Stack>
                      <Badge tone={kind === "procedural" ? "accent" : kind === "semantic" ? "warm" : "muted"}>{kind}</Badge>
                      {groupedMemories[kind].length === 0 ? <Subtle>No {kind} memory yet.</Subtle> : null}
                      <List>
                        {groupedMemories[kind].map((memory) => (
                          <Item key={memory.id} style={{ position: "relative" }}>
                            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                              <div style={{ minWidth: 0, flex: 1 }}>
                                <Row style={{ alignItems: "center" }}>
                                  <strong>{memory.title}</strong>
                                  <Badge tone="muted">{memory.source}</Badge>
                                  {memory.locked ? <Badge tone="warm">locked</Badge> : null}
                                </Row>
                              </div>
                              <Row style={{ alignItems: "center", flexWrap: "nowrap" }}>
                                <IconButton
                                  type="button"
                                  aria-label={memory.locked ? "Unlock memory" : "Lock memory"}
                                  onClick={() => void handleToggleMemoryLock(memory.id, !memory.locked)}
                                >
                                  {memory.locked ? <UnlockIcon /> : <LockIcon />}
                                </IconButton>
                                <div style={{ position: "relative" }}>
                                  <IconButton
                                    type="button"
                                    aria-label="Delete memory"
                                    onClick={() => setPendingDeleteMemoryId((current) => (current === memory.id ? null : memory.id))}
                                  >
                                    <TrashIcon />
                                  </IconButton>

                                  {pendingDeleteMemoryId === memory.id ? (
                                    <div
                                      style={{
                                        position: "absolute",
                                        right: 0,
                                        top: 40,
                                        width: 210,
                                        zIndex: 2,
                                        padding: 12,
                                        borderRadius: 14,
                                        border: "1px solid rgba(101, 123, 131, 0.18)",
                                        background: "#fffaf0",
                                        boxShadow: "0 12px 28px rgba(88, 110, 117, 0.18)"
                                      }}
                                    >
                                      <Stack>
                                        <Subtle>Delete this memory?</Subtle>
                                        <Row>
                                          <Button type="button" variant="ghost" onClick={() => setPendingDeleteMemoryId(null)}>
                                            Cancel
                                          </Button>
                                          <Button type="button" variant="warm" onClick={() => void handleDeleteMemory(memory.id)}>
                                            OK
                                          </Button>
                                        </Row>
                                      </Stack>
                                    </div>
                                  ) : null}
                                </div>
                              </Row>
                            </Row>
                            <Subtle>{memory.content}</Subtle>
                          </Item>
                        ))}
                      </List>
                    </Stack>
                  </Card>
                ))}
              </Grid>
            </Stack>
          </ModalCard>
        </ModalOverlay>
      ) : null}
    </>
  );
}

function detectDocumentType(file: File): "markdown" | "text" | "image" | null {
  const lowerName = file.name.toLowerCase();

  if (file.type.startsWith("image/")) {
    return "image";
  }

  if (lowerName.endsWith(".md") || lowerName.endsWith(".markdown")) {
    return "markdown";
  }

  if (lowerName.endsWith(".txt")) {
    return "text";
  }

  return null;
}

const DOCUMENT_CATEGORIES: DocumentCategory[] = ["world", "character", "rule", "plot", "timeline", "index", "story", "misc"];

function categoryTone(category: DocumentCategory): "accent" | "warm" | "muted" {
  if (category === "rule" || category === "timeline") {
    return "warm";
  }

  if (category === "story" || category === "misc") {
    return "muted";
  }

  return "accent";
}

function describeDocument(document: DocumentRecord) {
  if (document.type === "image") {
    return document.note || document.derivedText || "Image document";
  }

  return document.note || document.contentText.slice(0, 140) || "Text document";
}

function PlusIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M8 3v10M3 8h10" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  );
}

function TrashIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M3.5 4.5h9M6.5 2.75h3M5 4.5v7m3-7v7m3-7v7M4.5 4.5l.5 8.25c.03.52.46.92.98.92h4.04c.52 0 .95-.4.98-.92L11.5 4.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  );
}

function EditIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M3 11.75V13h1.25l7.1-7.1-1.25-1.25L3 11.75ZM12.2 5.05l.75-.75a.88.88 0 0 0 0-1.25l-.95-.95a.88.88 0 0 0-1.25 0l-.75.75 1.25 1.25.95.95Z" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function LockIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M5.75 7V5.75a2.25 2.25 0 1 1 4.5 0V7M4.75 7h6.5a.75.75 0 0 1 .75.75v4.5a.75.75 0 0 1-.75.75h-6.5a.75.75 0 0 1-.75-.75v-4.5A.75.75 0 0 1 4.75 7Z"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function UnlockIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M10.25 7V5.75a2.25 2.25 0 0 0-4.36-.77M4.75 7h6.5a.75.75 0 0 1 .75.75v4.5a.75.75 0 0 1-.75.75h-6.5a.75.75 0 0 1-.75-.75v-4.5A.75.75 0 0 1 4.75 7Z"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function SpinnerIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
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

function FolderIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" fill="none" aria-hidden="true" style={{ flexShrink: 0 }}>
      <path
        d="M2.75 5.25A1.75 1.75 0 0 1 4.5 3.5h3.07c.45 0 .88.18 1.2.5l.73.75c.14.14.34.22.54.22h3.46a1.75 1.75 0 0 1 1.75 1.75v5.78a1.75 1.75 0 0 1-1.75 1.75h-9A1.75 1.75 0 0 1 2.75 12.5V5.25Z"
        stroke="currentColor"
        strokeWidth="1.35"
        strokeLinejoin="round"
      />
    </svg>
  );
}
