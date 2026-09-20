import { useTheme } from "@emotion/react";
import { DragEvent, FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  api,
  ChatKind,
  ChatRecord,
  DocumentCategory,
  DocumentRecord,
  fileSrc,
  MemoryKind,
  MemoryOrganizationPlan,
  MemoryRecord,
  MultiAgentPreset,
  Project
} from "../api/client";
import { useConfirm } from "../components/ConfirmDialog";
import { MarkdownPreview } from "../components/MarkdownPreview";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import { MessageKey, useLanguage } from "../i18n";
import {
  Badge,
  Button,
  Card,
  ComposerBox,
  DropZone,
  ErrorText,
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

// The picker value that stands for the preset read from a file: it lives beside
// the bundled ids in the same select, so it must be a value no bundled id uses.
const IMPORTED_PRESET_CHOICE = "__file__";

interface ProjectDetailState {
  project: Project;
  documents: DocumentRecord[];
  memories: MemoryRecord[];
  chats: ChatRecord[];
}

export function ProjectDetailPage() {
  const { projectId = "" } = useParams();
  const navigate = useNavigate();
  const theme = useTheme();
  const { t } = useLanguage();
  const confirm = useConfirm();
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const [state, setState] = useState<ProjectDetailState | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [chatTitle, setChatTitle] = useState("");
  const [newChatIsTemporary, setNewChatIsTemporary] = useState(false);
  const [newChatKind, setNewChatKind] = useState<ChatKind>("assistant");
  const [presets, setPresets] = useState<MultiAgentPreset[]>([]);
  const [presetChoice, setPresetChoice] = useState("");
  const [importedPreset, setImportedPreset] = useState<MultiAgentPreset | null>(null);
  const [presetError, setPresetError] = useState("");
  const presetsRequestedRef = useRef(false);
  const presetFileRef = useRef<HTMLInputElement | null>(null);
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
      setError(nextError instanceof Error ? nextError.message : t("project.loadError"));
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

  // The bundled presets are only needed once the form is set to a multi-agent
  // chat, and only once: the list is fixed for the life of the process.
  useEffect(() => {
    if (newChatKind !== "multi_agent" || presetsRequestedRef.current) {
      return;
    }
    presetsRequestedRef.current = true;
    api
      .listMultiAgentPresets()
      .then((response) => setPresets(response.presets))
      .catch((nextError) => setPresetError(nextError instanceof Error ? nextError.message : t("preset.loadError")));
  }, [newChatKind, t]);

  const presetGroups = useMemo(() => {
    const order = ["discussion", "drama", "hosted", "pair"];
    const groups = new Map<string, MultiAgentPreset[]>();
    for (const preset of presets) {
      const group = order.includes(preset.group) ? preset.group : "other";
      groups.set(group, [...(groups.get(group) ?? []), preset]);
    }
    return [...order, "other"].filter((group) => groups.has(group)).map((group) => [group, groups.get(group) ?? []] as const);
  }, [presets]);

  const selectedPreset =
    presetChoice === IMPORTED_PRESET_CHOICE ? importedPreset : presets.find((preset) => preset.id === presetChoice) ?? null;

  function presetGroupLabel(group: string): string {
    const key = `preset.group.${group}` as MessageKey;
    return t(key) === key ? t("preset.group.other") : t(key);
  }

  // Only the shape the picker needs is checked here; the server validates the
  // preset in full and its message is shown if it refuses the file.
  async function handleImportPresetFile(file: File | undefined) {
    if (!file) {
      return;
    }
    setPresetError("");
    try {
      const parsed = JSON.parse(await file.text()) as Partial<MultiAgentPreset> | null;
      if (!parsed || typeof parsed.title !== "string" || !Array.isArray(parsed.participants)) {
        throw new Error("not a preset");
      }
      setImportedPreset(parsed as MultiAgentPreset);
      setPresetChoice(IMPORTED_PRESET_CHOICE);
    } catch {
      setPresetError(t("preset.importError"));
    }
  }

  function presetInput(): { presetId?: string; preset?: MultiAgentPreset } {
    if (newChatKind !== "multi_agent" || !presetChoice) {
      return {};
    }
    if (presetChoice === IMPORTED_PRESET_CHOICE) {
      return importedPreset ? { preset: importedPreset } : {};
    }
    return { presetId: presetChoice };
  }

  async function handleCreateChat(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      const response = await api.createChat(projectId, {
        title: chatTitle,
        isTemporary: newChatIsTemporary,
        kind: newChatKind,
        ...presetInput()
      });
      navigate(`/chats/${response.chat.id}`);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("project.createChatError"));
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
      setError(nextError instanceof Error ? nextError.message : t("project.createMemoryError"));
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
      setError(nextError instanceof Error ? nextError.message : t("project.analyzeError"));
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
      setError(nextError instanceof Error ? nextError.message : t("project.applyError"));
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
      setError(nextError instanceof Error ? nextError.message : t("project.updateMemoryError"));
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
      setError(nextError instanceof Error ? nextError.message : t("project.deleteMemoryError"));
    } finally {
      setBusy(false);
    }
  }

  async function handleDeleteProject() {
    if (!state) {
      return;
    }

    const confirmed = await confirm(t("project.deleteProjectConfirm", { title: state.project.title }));
    if (!confirmed) {
      return;
    }

    setBusy(true);
    setError("");

    try {
      await api.deleteProject(state.project.id);
      navigate("/");
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("project.deleteProjectError"));
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
      setError(nextError instanceof Error ? nextError.message : t("project.updateTitleError"));
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
      setError(nextError instanceof Error ? nextError.message : t("project.updateSystemPromptError"));
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
          throw new Error(t("project.unsupportedFile", { name: file.name }));
        }

        const existing = currentDocuments.find((document) => document.title === file.name);
        if (existing) {
          const overwrite = await confirm(t("project.overwritePrompt", { name: file.name }));
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
        setUploadStatus(t("project.savingDocument", { name: file.name }));
        const response = await api.createDocument(projectId, formData);
        currentDocuments = [response.document, ...currentDocuments];
      }

      await load();
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("project.uploadError"));
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
      setError(nextError instanceof Error ? nextError.message : t("project.deleteDocumentError"));
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
      setError(nextError instanceof Error ? nextError.message : t("project.updateCategoryError"));
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
      setError(nextError instanceof Error ? nextError.message : t("project.deleteChatError"));
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
    return <Card>{t("project.loading")}</Card>;
  }

  if (!state) {
    return <Card>{error || t("project.notFound")}</Card>;
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
            <IconButton type="button" aria-label={t("project.editTitle")} onClick={() => setIsTitleModalOpen(true)}>
              <EditIcon />
            </IconButton>
          </PaneHeader>

          <PaneBody>
            <Stack>
              {error ? <ErrorText>{error}</ErrorText> : null}

              <Card as="form" onSubmit={handleCreateChat}>
                <Stack>
                  <SectionTitle>{t("project.newChat")}</SectionTitle>
                  <ComposerBox>
                    <Field>
                      {t("project.chatTitle")}
                      <Input
                        value={chatTitle}
                        onChange={(event) => setChatTitle(event.target.value)}
                        placeholder={t("project.chatTitlePlaceholder")}
                      />
                    </Field>
                    <Field>
                      {t("multiAgent.chatType")}
                      <Select value={newChatKind} onChange={(event) => setNewChatKind(event.target.value as ChatKind)}>
                        <option value="assistant">{t("multiAgent.kindAssistant")}</option>
                        <option value="multi_agent">{t("multiAgent.kindMultiAgent")}</option>
                      </Select>
                    </Field>
                    {newChatKind === "multi_agent" ? (
                      <>
                        <Field>
                          {t("preset.label")}
                          <Select value={presetChoice} onChange={(event) => setPresetChoice(event.target.value)}>
                            <option value="">{t("preset.none")}</option>
                            {importedPreset ? (
                              <option value={IMPORTED_PRESET_CHOICE}>{t("preset.imported", { title: importedPreset.title })}</option>
                            ) : null}
                            {presetGroups.map(([group, items]) => (
                              <optgroup key={group} label={presetGroupLabel(group)}>
                                {items.map((preset) => (
                                  <option key={preset.id} value={preset.id}>
                                    {preset.title}
                                  </option>
                                ))}
                              </optgroup>
                            ))}
                          </Select>
                        </Field>
                        {selectedPreset ? (
                          <Subtle style={{ margin: 0 }}>
                            {selectedPreset.description}
                            {" "}
                            {t("preset.summary", { count: selectedPreset.participants.length, turnRule: selectedPreset.turnRule })}
                          </Subtle>
                        ) : null}
                        <Row style={{ alignItems: "center", gap: 10 }}>
                          <Button type="button" variant="ghost" disabled={busy} onClick={() => presetFileRef.current?.click()}>
                            {t("preset.import")}
                          </Button>
                        </Row>
                        <Subtle style={{ margin: 0 }}>{t("preset.hint")}</Subtle>
                        {presetError ? <ErrorText>{presetError}</ErrorText> : null}
                        <input
                          ref={presetFileRef}
                          type="file"
                          accept=".json,application/json"
                          style={{ display: "none" }}
                          onChange={(event) => {
                            void handleImportPresetFile(event.target.files?.[0]);
                            event.target.value = "";
                          }}
                        />
                      </>
                    ) : null}
                    <label style={{ display: "flex", alignItems: "center", gap: 10 }}>
                      <input
                        type="checkbox"
                        checked={newChatIsTemporary}
                        onChange={(event) => setNewChatIsTemporary(event.target.checked)}
                      />
                      <span>{t("project.temporaryChat")}</span>
                    </label>
                    <Button type="submit" disabled={busy}>
                      {t("project.openChat")}
                    </Button>
                  </ComposerBox>
                </Stack>
              </Card>

              <Card>
                <Stack>
                  <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                    <SectionTitle>{t("project.documents")}</SectionTitle>
                    <IconButton type="button" onClick={() => fileInputRef.current?.click()} aria-label={t("project.addDocument")} disabled={busy}>
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
                      <Subtle>{t("project.dropHint")}</Subtle>
                      <Subtle>{t("project.overwriteHint")}</Subtle>
                      {uploadStatus ? <Subtle>{t("project.embedSyncHint")}</Subtle> : null}
                    </Stack>
                  </DropZone>

                  <List>
                    {state.documents.length === 0 ? <Item>{t("project.noDocuments")}</Item> : null}
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
                              <Badge tone={categoryTone(document.category)}>{t(`category.${document.category}`)}</Badge>
                              <Badge tone={document.type === "image" ? "warm" : "accent"}>{document.type}</Badge>
                            </Row>
                            <Subtle>{describeDocument(t, document)}</Subtle>
                          </div>

                          <div style={{ position: "relative" }}>
                            <IconButton
                              type="button"
                              aria-label={t("project.deleteDocument")}
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
                                  border: `1px solid ${theme.lineStrong}`,
                                  background: theme.surfaceCard,
                                  boxShadow: theme.shadowPopover
                                }}
                              >
                                <Stack>
                                  <Subtle>{t("project.deleteDocumentConfirm")}</Subtle>
                                  <Row>
                                    <Button
                                      type="button"
                                      variant="ghost"
                                      onClick={() => setPendingDeleteDocumentId(null)}
                                    >
                                      {t("common.cancel")}
                                    </Button>
                                    <Button
                                      type="button"
                                      variant="warm"
                                      onClick={() => void handleDeleteDocument(document.id)}
                                    >
                                      {t("common.ok")}
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
                  <Badge tone="muted">{t("project.dangerZone")}</Badge>
                  <Subtle>{t("project.dangerDesc")}</Subtle>
                  <div>
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={handleDeleteProject}
                      disabled={busy}
                      style={{ borderColor: theme.dangerBorder, color: theme.danger }}
                    >
                      {t("project.deleteProject")}
                    </Button>
                  </div>
                </Stack>
              </Card>
            </Stack>
          </PaneBody>
        </MainPane>

        <InspectorPane>
          <SectionTitle>{t("project.assets")}</SectionTitle>

          <Card>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <Badge tone="accent">{t("project.systemPrompt")}</Badge>
                <IconButton type="button" aria-label={t("project.editSystemPrompt")} onClick={() => setIsSystemPromptModalOpen(true)}>
                  <EditIcon />
                </IconButton>
              </Row>
              <Subtle>{state.project.systemPrompt || t("project.noSystemPrompt")}</Subtle>
            </Stack>
          </Card>

          <Card>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <Badge tone="muted">{t("project.memories")}</Badge>
                <IconButton type="button" aria-label={t("project.editMemories")} onClick={() => setIsMemoryModalOpen(true)}>
                  <EditIcon />
                </IconButton>
              </Row>
              {(["procedural", "semantic", "episodic"] as const).map((kind) => (
                <Stack key={kind}>
                  <Subtle>{t(`memoryKind.${kind}`)}</Subtle>
                  {groupedMemories[kind].slice(0, 4).map((memory) => (
                    <Item key={memory.id}>
                      <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                        <strong>{memory.title}</strong>
                        <Row style={{ alignItems: "center", flexWrap: "nowrap" }}>
                          <Badge tone="muted">{memory.source}</Badge>
                          {memory.locked ? <Badge tone="warm">{t("project.locked")}</Badge> : null}
                        </Row>
                      </Row>
                      <Subtle>{memory.content}</Subtle>
                    </Item>
                  ))}
                  {groupedMemories[kind].length === 0 ? <Subtle>{t("project.noKindMemory", { kind: t(`memoryKind.${kind}`) })}</Subtle> : null}
                </Stack>
              ))}
            </Stack>
          </Card>

          <Card>
            <Stack>
              <Badge tone="accent">{t("project.chats")}</Badge>
              <List>
                {state.chats.length === 0 ? <Subtle>{t("project.noChats")}</Subtle> : null}
                {state.chats.slice(0, 8).map((chat) => (
                  <Item key={chat.id} style={{ position: "relative" }}>
                    <Row style={{ justifyContent: "space-between", alignItems: "flex-start" }}>
                      <div style={{ minWidth: 0, flex: 1 }}>
                        <RouterLink to={`/chats/${chat.id}`}>
                          {chat.title.trim() ? (
                            <strong>{chat.isTemporary ? `⏱️ ${chat.title}` : chat.title}</strong>
                          ) : (
                            <Subtle style={{ opacity: 0.78 }}>
                              {chat.isTemporary ? `⏱️ ${t("sidebar.untitled")}` : t("sidebar.untitled")}
                            </Subtle>
                          )}
                        </RouterLink>
                        <Subtle>{new Date(chat.updatedAt).toLocaleString()}</Subtle>
                      </div>

                      <div style={{ position: "relative" }}>
                        <IconButton
                          type="button"
                          aria-label={t("project.deleteChat")}
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
                              border: `1px solid ${theme.lineStrong}`,
                              background: theme.surfaceCard,
                              boxShadow: theme.shadowPopover
                            }}
                          >
                            <Stack>
                              <Subtle>{t("project.deleteChatConfirm")}</Subtle>
                              <Row>
                                <Button type="button" variant="ghost" onClick={() => setPendingDeleteChatId(null)}>
                                  {t("common.cancel")}
                                </Button>
                                <Button type="button" variant="warm" onClick={() => void handleDeleteChat(chat.id)}>
                                  {t("common.ok")}
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
                    {selectedDocument.type} · {t(`category.${selectedDocument.category}`)}
                  </Subtle>
                </div>
                <Button type="button" variant="ghost" onClick={() => setSelectedDocument(null)}>
                  {t("common.close")}
                </Button>
              </Row>

              {selectedDocument.tags.length ? <Subtle>{t("project.tags", { tags: selectedDocument.tags.join(", ") })}</Subtle> : null}
              {selectedDocument.note ? <Subtle>{selectedDocument.note}</Subtle> : null}

              <Card>
                <Stack>
                  <Field>
                    {t("project.category")}
                    <Select
                      value={documentCategoryDraft}
                      onChange={(event) => setDocumentCategoryDraft(event.target.value as DocumentCategory)}
                    >
                      {DOCUMENT_CATEGORIES.map((category) => (
                        <option key={category} value={category}>
                          {t(`category.${category}`)}
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
                      {t("project.saveCategory")}
                    </Button>
                  </div>
                </Stack>
              </Card>

              <Card>
                {selectedDocument.type === "image" && selectedDocument.filePath ? (
                  <img
                    src={fileSrc(selectedDocument.filePath)}
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
                    <Badge tone="warm">{t("project.derivedText")}</Badge>
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
                <SectionTitle>{t("project.editTitleModal")}</SectionTitle>
                <Button type="button" variant="ghost" onClick={() => setIsTitleModalOpen(false)}>
                  {t("common.close")}
                </Button>
              </Row>
              <Card as="form" onSubmit={handleUpdateProjectTitle}>
                <Stack>
                  <Field>
                    {t("project.titleField")}
                    <Input
                      value={titleDraft}
                      onChange={(event) => setTitleDraft(event.target.value)}
                      placeholder={t("project.titlePlaceholder")}
                    />
                  </Field>
                  <div>
                    <Button type="submit" disabled={busy || !titleDraft.trim()}>
                      {t("project.saveTitle")}
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
                <SectionTitle>{t("project.editSystemPromptModal")}</SectionTitle>
                <Button type="button" variant="ghost" onClick={() => setIsSystemPromptModalOpen(false)}>
                  {t("common.close")}
                </Button>
              </Row>
              <Card as="form" onSubmit={handleUpdateProjectSystemPrompt}>
                <Stack>
                  <Field>
                    {t("project.systemPromptField")}
                    <Textarea
                      value={systemPromptDraft}
                      onChange={(event) => setSystemPromptDraft(event.target.value)}
                      placeholder={t("project.systemPromptPlaceholder")}
                    />
                  </Field>
                  <div>
                    <Button type="submit" disabled={busy}>
                      {t("project.saveSystemPrompt")}
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
                <SectionTitle>{t("project.projectMemories")}</SectionTitle>
                <Row style={{ alignItems: "center", flexWrap: "nowrap" }}>
                  <Button type="button" variant="ghost" onClick={() => void handleAnalyzeMemories()} disabled={organizingMemories}>
                    {organizingMemories ? t("project.organizing") : t("project.organize")}
                  </Button>
                  <Button type="button" variant="ghost" onClick={() => setIsMemoryModalOpen(false)}>
                    {t("common.close")}
                  </Button>
                </Row>
              </Row>

              {memoryPlan ? (
                <Card>
                  <Stack>
                    <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                      <SectionTitle>{t("project.organizationPlan")}</SectionTitle>
                      <Button
                        type="button"
                        onClick={() => void handleApplyMemoryPlan()}
                        disabled={organizingMemories || memoryPlan.changes.length === 0}
                      >
                        {t("project.apply")}
                      </Button>
                    </Row>
                    <Subtle>{memoryPlan.summary || t("project.noSummary")}</Subtle>
                    {memoryPlan.changes.length === 0 ? (
                      <Subtle>{t("project.noChanges")}</Subtle>
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
                  <SectionTitle>{t("project.addMemory")}</SectionTitle>
                  <ComposerBox>
                    <Field>
                      {t("project.kind")}
                      <Select value={memoryKind} onChange={(event) => setMemoryKind(event.target.value as MemoryKind)}>
                        <option value="semantic">{t("project.kindSemantic")}</option>
                        <option value="procedural">{t("project.kindProcedural")}</option>
                        <option value="episodic">{t("project.kindEpisodic")}</option>
                      </Select>
                    </Field>
                    <Field>
                      {t("project.content")}
                      <Textarea
                        value={memoryContent}
                        onChange={(event) => setMemoryContent(event.target.value)}
                        placeholder={t("project.contentPlaceholder")}
                      />
                    </Field>
                    <label style={{ display: "flex", alignItems: "center", gap: 10 }}>
                      <input type="checkbox" checked={memoryLocked} onChange={(event) => setMemoryLocked(event.target.checked)} />
                      <span>{t("project.lockHint")}</span>
                    </label>
                    <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                      <Button type="submit" disabled={busy}>
                        {t("project.saveMemory")}
                      </Button>
                    </Row>
                  </ComposerBox>
                </Stack>
              </Card>

              <Grid columns="1fr 1fr 1fr">
                {(["procedural", "semantic", "episodic"] as const).map((kind) => (
                  <Card key={kind}>
                    <Stack>
                      <Badge tone={kind === "procedural" ? "accent" : kind === "semantic" ? "warm" : "muted"}>{t(`memoryKind.${kind}`)}</Badge>
                      {groupedMemories[kind].length === 0 ? <Subtle>{t("project.noKindMemoryYet", { kind: t(`memoryKind.${kind}`) })}</Subtle> : null}
                      <List>
                        {groupedMemories[kind].map((memory) => (
                          <Item key={memory.id} style={{ position: "relative" }}>
                            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                              <div style={{ minWidth: 0, flex: 1 }}>
                                <Row style={{ alignItems: "center" }}>
                                  <strong>{memory.title}</strong>
                                  <Badge tone="muted">{memory.source}</Badge>
                                  {memory.locked ? <Badge tone="warm">{t("project.locked")}</Badge> : null}
                                </Row>
                              </div>
                              <Row style={{ alignItems: "center", flexWrap: "nowrap" }}>
                                <IconButton
                                  type="button"
                                  aria-label={memory.locked ? t("project.unlockMemory") : t("project.lockMemory")}
                                  onClick={() => void handleToggleMemoryLock(memory.id, !memory.locked)}
                                >
                                  {memory.locked ? <UnlockIcon /> : <LockIcon />}
                                </IconButton>
                                <div style={{ position: "relative" }}>
                                  <IconButton
                                    type="button"
                                    aria-label={t("project.deleteMemory")}
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
                                        border: `1px solid ${theme.lineStrong}`,
                                        background: theme.surfaceCard,
                                        boxShadow: theme.shadowPopover
                                      }}
                                    >
                                      <Stack>
                                        <Subtle>{t("project.deleteMemoryConfirm")}</Subtle>
                                        <Row>
                                          <Button type="button" variant="ghost" onClick={() => setPendingDeleteMemoryId(null)}>
                                            {t("common.cancel")}
                                          </Button>
                                          <Button type="button" variant="warm" onClick={() => void handleDeleteMemory(memory.id)}>
                                            {t("common.ok")}
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

function describeDocument(t: (key: MessageKey) => string, document: DocumentRecord) {
  if (document.type === "image") {
    return document.note || document.derivedText || t("project.imageDocument");
  }

  return document.note || document.contentText.slice(0, 140) || t("project.textDocument");
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
