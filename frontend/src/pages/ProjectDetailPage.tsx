import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
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
  MultiAgentPresetSelection,
  Project
} from "../api/client";
import { ActionButton } from "../components/ActionButton";
import { Checkbox } from "../components/Checkbox";
import { useConfirm } from "../components/ConfirmDialog";
import { Dialog, DialogTitle } from "../components/Dialog";
import { FailureNotice } from "../components/FailureNotice";
import { DropZoneProgress, FileDropZone } from "../components/FileDropZone";
import {
  CheckIcon,
  FilePlusIcon,
  FolderIcon,
  ImageIcon,
  LockIcon,
  LockOpenIcon,
  MessageSquarePlusIcon,
  PencilIcon,
  PlusIcon,
  SlidersIcon,
  TrashIcon,
  UserShieldIcon,
  UsersIcon
} from "../components/icons";
import { ImageDocumentDialog } from "../components/ImageDocumentDialog";
import { ImageDocumentEditor } from "../components/ImageDocumentEditor";
import { MarkdownPreview } from "../components/MarkdownPreview";
import { PresetChoice, PresetPicker } from "../components/PresetPicker";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import { MessageKey, useLanguage } from "../i18n";
import {
  Badge,
  Card,
  ComposerBox,
  Field,
  Grid,
  IconButton,
  Input,
  InspectorPane,
  Item,
  List,
  MainPane,
  PaneBody,
  PaneHeader,
  Row,
  RouterLink,
  SectionTitle,
  Select,
  Stack,
  SubsectionTitle,
  Subtle,
  Textarea,
  TitleButton,
  WorkspaceShell
} from "../styles/ui";

interface ProjectDetailState {
  project: Project;
  documents: DocumentRecord[];
  memories: MemoryRecord[];
  chats: ChatRecord[];
}

// A failure is told next to what failed (snz-design doc-9 §5.5), so each part of the
// screen that can fail keeps its own message; a dialog's failure stays in the dialog
// instead of landing behind it.
type ErrorArea =
  | "page"
  | "newChat"
  | "documents"
  | "danger"
  | "chats"
  | "memories"
  | "newMemory"
  | "memoryPlan"
  | "title"
  | "systemPrompt"
  | "document";

export function ProjectDetailPage() {
  const { projectId = "" } = useParams();
  const navigate = useNavigate();
  const { t } = useLanguage();
  const confirm = useConfirm();
  const imageButtonRef = useRef<HTMLButtonElement | null>(null);
  const editMemoriesRef = useRef<HTMLButtonElement | null>(null);
  const organizeRef = useRef<HTMLButtonElement | null>(null);
  const [state, setState] = useState<ProjectDetailState | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [errors, setErrors] = useState<Partial<Record<ErrorArea, string>>>({});
  const [loading, setLoading] = useState(true);
  // Each operation is busy on its own, so one save no longer disables every other
  // control on the screen; the ref answers "already running" synchronously.
  const pendingRef = useRef(new Set<string>());
  const [pending, setPending] = useState<ReadonlySet<string>>(() => new Set());
  const [chatTitle, setChatTitle] = useState("");
  const [newChatIsTemporary, setNewChatIsTemporary] = useState(false);
  const [newChatKind, setNewChatKind] = useState<ChatKind>("assistant");
  const [presetChoice, setPresetChoice] = useState<PresetChoice | null>(null);
  const [memoryKind, setMemoryKind] = useState<MemoryKind>("semantic");
  const [memoryContent, setMemoryContent] = useState("");
  const [memoryLocked, setMemoryLocked] = useState(true);
  const [uploadProgress, setUploadProgress] = useState<DropZoneProgress | null>(null);
  const [selectedDocument, setSelectedDocument] = useState<DocumentRecord | null>(null);
  const [documentCategoryDraft, setDocumentCategoryDraft] = useState<DocumentCategory>("misc");
  const [isEditingDocument, setIsEditingDocument] = useState(false);
  const [documentEditorDirty, setDocumentEditorDirty] = useState(false);
  const [savingDocumentContent, setSavingDocumentContent] = useState(false);
  const [isMemoryModalOpen, setIsMemoryModalOpen] = useState(false);
  const [isImageDialogOpen, setIsImageDialogOpen] = useState(false);
  const [isTitleModalOpen, setIsTitleModalOpen] = useState(false);
  const [isSystemPromptModalOpen, setIsSystemPromptModalOpen] = useState(false);
  const [titleDraft, setTitleDraft] = useState("");
  const [systemPromptDraft, setSystemPromptDraft] = useState("");
  const [memoryPlan, setMemoryPlan] = useState<MemoryOrganizationPlan | null>(null);

  function setAreaError(area: ErrorArea, message: string) {
    setErrors((current) => ({ ...current, [area]: message }));
  }

  const isPending = (key: string) => pending.has(key);

  async function run(key: string, area: ErrorArea, fallback: MessageKey, task: () => Promise<void>): Promise<boolean> {
    if (pendingRef.current.has(key)) {
      return false;
    }
    pendingRef.current.add(key);
    setPending(new Set(pendingRef.current));
    setAreaError(area, "");
    try {
      await task();
      return true;
    } catch (nextError) {
      setAreaError(area, nextError instanceof Error ? nextError.message : t(fallback));
      return false;
    } finally {
      pendingRef.current.delete(key);
      setPending(new Set(pendingRef.current));
    }
  }

  // Reloads keep the page mounted: swapping it for the loading card would close any
  // open dialog and drop the focus to the top of the page.
  async function load() {
    try {
      const [projectResponse, projectsResponse] = await Promise.all([api.getProjectDetail(projectId), api.getProjects()]);
      setState(projectResponse);
      setProjects(projectsResponse.projects);
      setAreaError("page", "");
    } catch (nextError) {
      setAreaError("page", nextError instanceof Error ? nextError.message : t("project.loadError"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    setState(null);
    setLoading(true);
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

  useEffect(() => {
    setIsEditingDocument(false);
    setDocumentEditorDirty(false);
  }, [selectedDocument?.id]);

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

  function confirmDelete(heading: MessageKey, message: MessageKey, name: string) {
    return confirm(t(message, { name }), { heading: t(heading), confirmLabel: t("common.delete") });
  }

  // Asked only when there is something unsaved to lose; the default stays with
  // keeping it (snz-design doc-9 §5.7).
  function confirmDiscard() {
    return confirm(t("discard.message"), {
      heading: t("discard.heading"),
      confirmLabel: t("discard.confirm"),
      cancelLabel: t("discard.keepEditing")
    });
  }

  // The preset picker is mounted only while the form is set to a multi-agent
  // chat, so a preset chosen before switching back must not travel with an
  // assistant chat — the server refuses one there.
  function presetInput(): MultiAgentPresetSelection | Record<string, never> {
    if (newChatKind !== "multi_agent" || !presetChoice) {
      return {};
    }
    return presetChoice.selection;
  }

  async function handleCreateChat(event: FormEvent) {
    event.preventDefault();
    await run("createChat", "newChat", "project.createChatError", async () => {
      const response = await api.createChat(projectId, {
        title: chatTitle,
        isTemporary: newChatIsTemporary,
        kind: newChatKind,
        ...presetInput()
      });
      navigate(`/chats/${response.chat.id}`);
    });
  }

  async function handleCreateMemory(event: FormEvent) {
    event.preventDefault();
    await run("createMemory", "newMemory", "project.createMemoryError", async () => {
      await api.createMemory(projectId, { content: memoryContent, kind: memoryKind, locked: memoryLocked });
      setMemoryContent("");
      setMemoryLocked(true);
      setMemoryPlan(null);
      await load();
    });
  }

  async function handleAnalyzeMemories() {
    await run("analyze", "memoryPlan", "project.analyzeError", async () => {
      const response = await api.analyzeMemoryOrganization(projectId);
      setMemoryPlan(response.plan);
    });
  }

  async function handleApplyMemoryPlan() {
    if (!memoryPlan) {
      return;
    }
    await run("apply", "memoryPlan", "project.applyError", async () => {
      const response = await api.applyMemoryOrganization(projectId, memoryPlan);
      setState((current) => (current ? { ...current, memories: response.memories } : current));
      setMemoryPlan(null);
    });
  }

  async function handleToggleMemoryLock(memoryId: string, locked: boolean) {
    await run(`memLock:${memoryId}`, "memories", "project.updateMemoryError", async () => {
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
    });
  }

  async function handleDeleteMemory(memory: MemoryRecord, trigger: HTMLElement) {
    if (!(await confirmDelete("project.deleteMemory", "project.deleteMemoryConfirm", memory.title))) {
      return;
    }
    const next = focusTargetAfterRemoval(trigger, organizeRef.current);
    const deleted = await run(`deleteMem:${memory.id}`, "memories", "project.deleteMemoryError", async () => {
      await api.deleteMemory(memory.id);
      setState((current) =>
        current ? { ...current, memories: current.memories.filter((item) => item.id !== memory.id) } : current
      );
      setMemoryPlan(null);
    });
    if (deleted) {
      focusIfLost(trigger, next);
    }
  }

  async function handleDeleteProject() {
    if (!state) {
      return;
    }

    const confirmed = await confirm(t("project.deleteProjectConfirm", { title: state.project.title }), {
      heading: t("project.deleteProject"),
      confirmLabel: t("common.delete")
    });
    if (!confirmed) {
      return;
    }

    await run("deleteProject", "danger", "project.deleteProjectError", async () => {
      await api.deleteProject(state.project.id);
      navigate("/");
    });
  }

  async function handleUpdateProjectTitle(event: FormEvent) {
    event.preventDefault();
    if (!state || !titleDraft.trim()) {
      return;
    }

    const saved = await run("title", "title", "project.updateTitleError", async () => {
      const response = await api.updateProjectTitle(state.project.id, titleDraft);
      setState((current) => (current ? { ...current, project: response.project } : current));
      setProjects((current) => current.map((project) => (project.id === response.project.id ? response.project : project)));
    });
    if (saved) {
      setIsTitleModalOpen(false);
    }
  }

  async function handleUpdateProjectSystemPrompt(event: FormEvent) {
    event.preventDefault();
    if (!state) {
      return;
    }

    const saved = await run("systemPrompt", "systemPrompt", "project.updateSystemPromptError", async () => {
      const response = await api.updateProjectSystemPrompt(state.project.id, systemPromptDraft);
      setState((current) => (current ? { ...current, project: response.project } : current));
      setProjects((current) => current.map((project) => (project.id === response.project.id ? response.project : project)));
    });
    if (saved) {
      setIsSystemPromptModalOpen(false);
    }
  }

  // Titles are compared with the saved values, so reopening after a discard or a
  // save starts clean. A modal that is saving cannot close: its result would come
  // back to a screen that no longer shows what it was for (doc-9 §6.6).
  async function closeTitleModal() {
    if (pendingRef.current.has("title")) {
      return;
    }
    if (state && titleDraft !== state.project.title && !(await confirmDiscard())) {
      return;
    }
    setTitleDraft(state?.project.title ?? "");
    setAreaError("title", "");
    setIsTitleModalOpen(false);
  }

  async function closeSystemPromptModal() {
    if (pendingRef.current.has("systemPrompt")) {
      return;
    }
    if (state && systemPromptDraft !== state.project.systemPrompt && !(await confirmDiscard())) {
      return;
    }
    setSystemPromptDraft(state?.project.systemPrompt ?? "");
    setAreaError("systemPrompt", "");
    setIsSystemPromptModalOpen(false);
  }

  async function handleFilesChosen(files: File[]) {
    if (!state) {
      return;
    }
    const startingDocuments = state.documents;

    await run("upload", "documents", "project.uploadError", async () => {
      let currentDocuments = [...startingDocuments];
      try {
        for (const [index, file] of files.entries()) {
          const nextType = detectDocumentType(file);
          if (!nextType) {
            continue;
          }

          const existing = currentDocuments.find((document) => document.title === file.name);
          if (
            existing &&
            !(await confirm(t("project.overwritePrompt", { name: file.name }), {
              heading: t("project.overwriteHeading"),
              confirmLabel: t("project.overwriteConfirm")
            }))
          ) {
            continue;
          }

          setUploadProgress({ label: t("project.savingDocument", { name: file.name }), done: index, total: files.length });
          if (existing) {
            await api.deleteDocument(existing.id);
            currentDocuments = currentDocuments.filter((document) => document.id !== existing.id);
          }

          const formData = new FormData();
          formData.set("type", nextType);
          formData.set("title", file.name);
          formData.set("file", file);
          const response = await api.createDocument(projectId, formData);
          currentDocuments = [response.document, ...currentDocuments];
        }
      } finally {
        setUploadProgress(null);
        // Also after a failure: the files saved before it are in the project now.
        await load();
      }
    });
  }

  async function handleDeleteDocument(document: DocumentRecord, trigger: HTMLElement) {
    if (!(await confirmDelete("project.deleteDocument", "project.deleteDocumentConfirm", document.title))) {
      return;
    }
    const next = focusTargetAfterRemoval(trigger, imageButtonRef.current);
    const deleted = await run(`deleteDoc:${document.id}`, "documents", "project.deleteDocumentError", async () => {
      await api.deleteDocument(document.id);
      setSelectedDocument((current) => (current?.id === document.id ? null : current));
      setState((current) =>
        current ? { ...current, documents: current.documents.filter((item) => item.id !== document.id) } : current
      );
    });
    if (deleted) {
      focusIfLost(trigger, next);
    }
  }

  async function handleUpdateDocumentCategory(documentId: string, category: DocumentCategory) {
    await run("category", "document", "project.updateCategoryError", async () => {
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
    });
  }

  // The saved document replaces selectedDocument rather than being patched into
  // it: a misc category may have been inferred again from the new description,
  // and the category draft is reset from selectedDocument, so keeping the old
  // record would leave "misc" in the draft for "Save category" to put back.
  function handleDocumentContentSaved(document: DocumentRecord) {
    setState((current) =>
      current
        ? { ...current, documents: current.documents.map((item) => (item.id === document.id ? document : item)) }
        : current
    );
    setSelectedDocument((current) => (current?.id === document.id ? document : current));
    setIsEditingDocument(false);
    setDocumentEditorDirty(false);
  }

  // The detail dialog stays open while an edit is being saved (the save waits on
  // the embedding sync, which can be slow): its late response would otherwise
  // land on whatever document the dialog shows by then and drop that draft.
  async function closeSelectedDocument() {
    if (savingDocumentContent || !selectedDocument) {
      return;
    }
    const dirty = (isEditingDocument && documentEditorDirty) || documentCategoryDraft !== selectedDocument.category;
    if (dirty && !(await confirmDiscard())) {
      return;
    }
    setAreaError("document", "");
    setSelectedDocument(null);
  }

  // The common project material is the one thing about a document or a memory the
  // human toggles from the list, so it saves on the click rather than through a
  // draft and a save button (design §4.4).
  async function handleUpdateDocumentSharedWithAll(documentId: string, sharedWithAll: boolean, area: ErrorArea) {
    await run(`docShare:${documentId}`, area, "project.updateSharedWithAllError", async () => {
      const response = await api.updateDocumentSharedWithAll(documentId, sharedWithAll);
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
      setSelectedDocument((current) => (current?.id === response.document.id ? response.document : current));
    });
  }

  async function handleUpdateMemorySharedWithAll(memoryId: string, sharedWithAll: boolean) {
    await run(`memShare:${memoryId}`, "memories", "project.updateSharedWithAllError", async () => {
      const response = await api.updateMemorySharedWithAll(memoryId, sharedWithAll);
      setState((current) =>
        current
          ? {
              ...current,
              memories: current.memories.map((memory) => (memory.id === response.memory.id ? response.memory : memory))
            }
          : current
      );
    });
  }

  async function handleDeleteChat(chat: ChatRecord, trigger: HTMLElement) {
    const name = chat.title.trim() || t("sidebar.untitled");
    if (!(await confirmDelete("project.deleteChat", "project.deleteChatConfirm", name))) {
      return;
    }
    const next = focusTargetAfterRemoval(trigger, editMemoriesRef.current);
    const deleted = await run(`deleteChat:${chat.id}`, "chats", "project.deleteChatError", async () => {
      await api.deleteChat(chat.id);
      setState((current) => (current ? { ...current, chats: current.chats.filter((item) => item.id !== chat.id) } : current));
    });
    if (deleted) {
      focusIfLost(trigger, next);
    }
  }

  if (!state) {
    return <Card>{loading ? t("project.loading") : errors.page ? <FailureNotice>{errors.page}</FailureNotice> : t("project.notFound")}</Card>;
  }

  const documentBusy = savingDocumentContent ? t("project.savingClose") : undefined;

  return (
    <>
      <WorkspaceShell>
        <WorkspaceSidebar projects={projects} currentProjectId={state.project.id} chats={state.chats} />

        <MainPane>
          <PaneHeader>
            <Row style={{ alignItems: "center" }}>
              <FolderIcon size={18} />
              <SectionTitle>{state.project.title}</SectionTitle>
            </Row>
            <IconButton
              type="button"
              aria-label={t("project.editTitle")}
              title={t("project.editTitle")}
              onClick={() => setIsTitleModalOpen(true)}
            >
              <PencilIcon />
            </IconButton>
          </PaneHeader>

          <PaneBody>
            <Stack>
              {errors.page ? <FailureNotice>{errors.page}</FailureNotice> : null}

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
                    {newChatKind === "multi_agent" ? <PresetPicker onChange={setPresetChoice} /> : null}
                    <Checkbox checked={newChatIsTemporary} onChange={setNewChatIsTemporary}>
                      {t("project.temporaryChat")}
                    </Checkbox>
                    <Subtle>{t("project.temporaryChatNote")}</Subtle>
                    {errors.newChat ? <FailureNotice>{errors.newChat}</FailureNotice> : null}
                    {/* The screen's one primary action (snz-design doc-8 §6.1). */}
                    <ActionButton
                      type="submit"
                      icon={<MessageSquarePlusIcon />}
                      busy={isPending("createChat")}
                      title={isPending("createChat") ? t("project.creatingChat") : undefined}
                    >
                      {t("project.openChat")}
                    </ActionButton>
                  </ComposerBox>
                </Stack>
              </Card>

              <Card>
                <Stack>
                  <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                    <SectionTitle>{t("project.documents")}</SectionTitle>
                    <IconButton
                      ref={imageButtonRef}
                      type="button"
                      onClick={() => setIsImageDialogOpen(true)}
                      aria-label={t("project.addImage")}
                      title={t("project.addImage")}
                    >
                      <ImageIcon />
                    </IconButton>
                  </Row>

                  <FileDropZone
                    label={t("project.dropLabel")}
                    acceptWords={t("project.dropAccept")}
                    accepts={(file) => detectDocumentType(file) !== null}
                    acceptsType={isDocumentMime}
                    inputAccept=".md,.markdown,.txt,image/*"
                    multiple
                    chooseLabel={t("project.chooseFiles")}
                    chooseIcon={<FilePlusIcon />}
                    progress={uploadProgress}
                    onFilesChosen={(files) => void handleFilesChosen(files)}
                  />

                  {errors.documents ? <FailureNotice>{errors.documents}</FailureNotice> : null}

                  <List>
                    {state.documents.length === 0 ? <Item>{t("project.noDocuments")}</Item> : null}
                    {state.documents.map((document) => (
                      <Item
                        key={document.id}
                        data-row=""
                        style={{ cursor: "pointer" }}
                        onClick={() => setSelectedDocument(document)}
                      >
                        <Row style={{ justifyContent: "space-between", alignItems: "flex-start" }}>
                          <div style={{ minWidth: 0, flex: 1 }}>
                            <Row style={{ alignItems: "center" }}>
                              {/* The row answers the pointer; this button is the same
                                  action for the keyboard. */}
                              <TitleButton
                                type="button"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  setSelectedDocument(document);
                                }}
                              >
                                {document.title}
                              </TitleButton>
                              <Badge tone={categoryTone(document.category)}>{t(`category.${document.category}`)}</Badge>
                              <Badge tone={document.type === "image" ? "warm" : "accent"}>{document.type}</Badge>
                              {document.sharedWithAll ? <Badge tone="accent">{t("project.sharedWithAll")}</Badge> : null}
                            </Row>
                            <Subtle>{describeDocument(t, document)}</Subtle>
                          </div>

                          <Row style={{ alignItems: "center", flexWrap: "nowrap" }} onClick={(event) => event.stopPropagation()}>
                            <ActionButton
                              iconOnly
                              type="button"
                              busy={isPending(`docShare:${document.id}`)}
                              aria-label={document.sharedWithAll ? t("project.unshareWithAll") : t("project.shareWithAll")}
                              title={document.sharedWithAll ? t("project.unshareWithAll") : t("project.shareWithAll")}
                              onClick={() =>
                                void handleUpdateDocumentSharedWithAll(document.id, !document.sharedWithAll, "documents")
                              }
                            >
                              {document.sharedWithAll ? <UsersIcon /> : <UserShieldIcon />}
                            </ActionButton>
                            <ActionButton
                              iconOnly
                              type="button"
                              data-delete=""
                              busy={isPending(`deleteDoc:${document.id}`)}
                              aria-label={t("project.deleteDocument")}
                              title={t("project.deleteDocument")}
                              onClick={(event) => void handleDeleteDocument(document, event.currentTarget)}
                            >
                              <TrashIcon />
                            </ActionButton>
                          </Row>
                        </Row>
                      </Item>
                    ))}
                  </List>
                </Stack>
              </Card>

              <Card>
                <Stack>
                  <SectionTitle>{t("project.dangerZone")}</SectionTitle>
                  <Subtle>{t("project.dangerDesc")}</Subtle>
                  {errors.danger ? <FailureNotice>{errors.danger}</FailureNotice> : null}
                  <div>
                    <ActionButton
                      type="button"
                      variant="danger"
                      icon={<TrashIcon />}
                      busy={isPending("deleteProject")}
                      title={isPending("deleteProject") ? t("project.deletingProject") : undefined}
                      onClick={() => void handleDeleteProject()}
                    >
                      {t("project.deleteProject")}
                    </ActionButton>
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
                <SubsectionTitle>{t("project.systemPrompt")}</SubsectionTitle>
                <IconButton
                  type="button"
                  aria-label={t("project.editSystemPrompt")}
                  title={t("project.editSystemPrompt")}
                  onClick={() => setIsSystemPromptModalOpen(true)}
                >
                  <PencilIcon />
                </IconButton>
              </Row>
              <Subtle>{state.project.systemPrompt || t("project.noSystemPrompt")}</Subtle>
            </Stack>
          </Card>

          <Card>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <SubsectionTitle>{t("project.memories")}</SubsectionTitle>
                <IconButton
                  ref={editMemoriesRef}
                  type="button"
                  aria-label={t("project.editMemories")}
                  title={t("project.editMemories")}
                  onClick={() => setIsMemoryModalOpen(true)}
                >
                  <PencilIcon />
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
              <SubsectionTitle>{t("project.chats")}</SubsectionTitle>
              {errors.chats ? <FailureNotice>{errors.chats}</FailureNotice> : null}
              <List>
                {state.chats.length === 0 ? <Subtle>{t("project.noChats")}</Subtle> : null}
                {state.chats.slice(0, 8).map((chat) => (
                  <Item key={chat.id} data-row="">
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

                      <ActionButton
                        iconOnly
                        type="button"
                        data-delete=""
                        busy={isPending(`deleteChat:${chat.id}`)}
                        aria-label={t("project.deleteChat")}
                        title={t("project.deleteChat")}
                        onClick={(event) => void handleDeleteChat(chat, event.currentTarget)}
                      >
                        <TrashIcon />
                      </ActionButton>
                    </Row>
                  </Item>
                ))}
              </List>
            </Stack>
          </Card>
        </InspectorPane>
      </WorkspaceShell>

      {selectedDocument ? (
        <Dialog onClose={() => void closeSelectedDocument()}>
          <Stack>
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <div>
                <DialogTitle>{selectedDocument.title}</DialogTitle>
                <Subtle>
                  {selectedDocument.type} · {t(`category.${selectedDocument.category}`)}
                  {selectedDocument.sharedWithAll ? ` · ${t("project.sharedWithAll")}` : ""}
                </Subtle>
              </div>
              <ActionButton
                type="button"
                variant="normal"
                disabledReason={documentBusy}
                onClick={() => void closeSelectedDocument()}
              >
                {t("common.close")}
              </ActionButton>
            </Row>

            {errors.document ? <FailureNotice>{errors.document}</FailureNotice> : null}

            {isEditingDocument ? (
              <Card>
                <ImageDocumentEditor
                  document={selectedDocument}
                  onSaved={handleDocumentContentSaved}
                  onCancel={() => {
                    setIsEditingDocument(false);
                    setDocumentEditorDirty(false);
                  }}
                  onSavingChange={setSavingDocumentContent}
                  onDirtyChange={setDocumentEditorDirty}
                />
              </Card>
            ) : (
              <>
                {selectedDocument.tags.length ? (
                  <Subtle>{t("project.tags", { tags: selectedDocument.tags.join(", ") })}</Subtle>
                ) : null}
                {selectedDocument.note ? <Subtle>{selectedDocument.note}</Subtle> : null}
                {selectedDocument.type === "image" ? (
                  <div>
                    <ActionButton type="button" variant="normal" icon={<PencilIcon />} onClick={() => setIsEditingDocument(true)}>
                      {t("documentEditor.edit")}
                    </ActionButton>
                  </div>
                ) : null}
              </>
            )}

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
                  {/* Normal rather than primary: the editor's save is this dialog's
                      primary action while it is open (doc-8 §6.1). */}
                  <ActionButton
                    type="button"
                    variant="normal"
                    icon={<CheckIcon />}
                    busy={isPending("category")}
                    title={isPending("category") ? t("project.saving") : undefined}
                    disabledReason={
                      documentCategoryDraft === selectedDocument.category ? t("project.categoryUnchanged") : undefined
                    }
                    onClick={() => void handleUpdateDocumentCategory(selectedDocument.id, documentCategoryDraft)}
                  >
                    {t("project.saveCategory")}
                  </ActionButton>
                </div>

                <Stack style={{ gap: 6 }}>
                  <Checkbox
                    checked={selectedDocument.sharedWithAll}
                    onChange={(checked) => void handleUpdateDocumentSharedWithAll(selectedDocument.id, checked, "document")}
                  >
                    {t("project.shareWithAll")}
                  </Checkbox>
                  <Subtle>{t("project.shareWithAllHint")}</Subtle>
                </Stack>
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

              {selectedDocument.type === "image" && selectedDocument.derivedText && !isEditingDocument ? (
                <div style={{ marginTop: 14 }}>
                  <SubsectionTitle>{t("project.derivedText")}</SubsectionTitle>
                  <div style={{ marginTop: 10, whiteSpace: "pre-wrap", lineHeight: 1.7 }}>{selectedDocument.derivedText}</div>
                </div>
              ) : null}
            </Card>
          </Stack>
        </Dialog>
      ) : null}

      {isTitleModalOpen ? (
        <Dialog onClose={() => void closeTitleModal()}>
          <Stack>
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <DialogTitle>{t("project.editTitleModal")}</DialogTitle>
              <ActionButton
                type="button"
                variant="normal"
                disabledReason={isPending("title") ? t("project.savingClose") : undefined}
                onClick={() => void closeTitleModal()}
              >
                {t("common.close")}
              </ActionButton>
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
                {errors.title ? <FailureNotice>{errors.title}</FailureNotice> : null}
                <div>
                  <ActionButton
                    type="submit"
                    icon={<CheckIcon />}
                    busy={isPending("title")}
                    title={isPending("title") ? t("project.saving") : undefined}
                    disabledReason={titleDraft.trim() ? undefined : t("project.titleRequired")}
                  >
                    {t("project.saveTitle")}
                  </ActionButton>
                </div>
              </Stack>
            </Card>
          </Stack>
        </Dialog>
      ) : null}

      {isSystemPromptModalOpen ? (
        <Dialog onClose={() => void closeSystemPromptModal()}>
          <Stack>
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <DialogTitle>{t("project.editSystemPromptModal")}</DialogTitle>
              <ActionButton
                type="button"
                variant="normal"
                disabledReason={isPending("systemPrompt") ? t("project.savingClose") : undefined}
                onClick={() => void closeSystemPromptModal()}
              >
                {t("common.close")}
              </ActionButton>
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
                {errors.systemPrompt ? <FailureNotice>{errors.systemPrompt}</FailureNotice> : null}
                <div>
                  <ActionButton
                    type="submit"
                    icon={<CheckIcon />}
                    busy={isPending("systemPrompt")}
                    title={isPending("systemPrompt") ? t("project.saving") : undefined}
                  >
                    {t("project.saveSystemPrompt")}
                  </ActionButton>
                </div>
              </Stack>
            </Card>
          </Stack>
        </Dialog>
      ) : null}

      {isImageDialogOpen ? (
        <ImageDocumentDialog
          projectId={state.project.id}
          documents={state.documents}
          onClose={() => setIsImageDialogOpen(false)}
          onCreated={() => {
            setIsImageDialogOpen(false);
            void load();
          }}
        />
      ) : null}

      {/* The memory being written stays in the page state when this closes, so
          closing loses nothing and asks nothing (doc-9 §5.7). */}
      {isMemoryModalOpen ? (
        <Dialog onClose={() => setIsMemoryModalOpen(false)}>
          <Stack>
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <DialogTitle>{t("project.projectMemories")}</DialogTitle>
              <Row style={{ alignItems: "center", flexWrap: "nowrap" }}>
                <ActionButton
                  ref={organizeRef}
                  type="button"
                  variant="normal"
                  icon={<SlidersIcon />}
                  busy={isPending("analyze")}
                  title={isPending("analyze") ? t("project.organizing") : undefined}
                  onClick={() => void handleAnalyzeMemories()}
                >
                  {t("project.organize")}
                </ActionButton>
                <ActionButton type="button" variant="normal" onClick={() => setIsMemoryModalOpen(false)}>
                  {t("common.close")}
                </ActionButton>
              </Row>
            </Row>

            {errors.memoryPlan ? <FailureNotice>{errors.memoryPlan}</FailureNotice> : null}

            {memoryPlan ? (
              <Card>
                <Stack>
                  <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                    <SectionTitle>{t("project.organizationPlan")}</SectionTitle>
                    <ActionButton
                      type="button"
                      variant="normal"
                      icon={<CheckIcon />}
                      busy={isPending("apply")}
                      title={isPending("apply") ? t("project.saving") : undefined}
                      disabledReason={memoryPlan.changes.length === 0 ? t("project.applyNothing") : undefined}
                      onClick={() => void handleApplyMemoryPlan()}
                    >
                      {t("project.apply")}
                    </ActionButton>
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
                  <Checkbox checked={memoryLocked} onChange={setMemoryLocked}>
                    {t("project.lockHint")}
                  </Checkbox>
                  {errors.newMemory ? <FailureNotice>{errors.newMemory}</FailureNotice> : null}
                  <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                    {/* The dialog's one primary action; applying a plan is normal. */}
                    <ActionButton
                      type="submit"
                      icon={<PlusIcon />}
                      busy={isPending("createMemory")}
                      title={isPending("createMemory") ? t("project.saving") : undefined}
                    >
                      {t("project.saveMemory")}
                    </ActionButton>
                  </Row>
                </ComposerBox>
              </Stack>
            </Card>

            {errors.memories ? <FailureNotice>{errors.memories}</FailureNotice> : null}

            <Grid columns="1fr 1fr 1fr">
              {(["procedural", "semantic", "episodic"] as const).map((kind) => (
                <Card key={kind}>
                  <Stack>
                    <SubsectionTitle>{t(`memoryKind.${kind}`)}</SubsectionTitle>
                    {groupedMemories[kind].length === 0 ? <Subtle>{t("project.noKindMemoryYet", { kind: t(`memoryKind.${kind}`) })}</Subtle> : null}
                    <List>
                      {groupedMemories[kind].map((memory) => (
                        <Item key={memory.id} data-row="">
                          {/* Title, actions and badges each get their own line. Sharing the
                              first line between the title and three icon buttons left the
                              title a few characters wide in the memory pane's narrow column
                              (observed on a memory whose title is a long sentence). */}
                          <strong style={{ display: "block", overflowWrap: "anywhere" }}>{memory.title}</strong>
                          <Row style={{ justifyContent: "flex-end", alignItems: "center", flexWrap: "nowrap" }}>
                            <ActionButton
                              iconOnly
                              type="button"
                              busy={isPending(`memShare:${memory.id}`)}
                              aria-label={memory.sharedWithAll ? t("project.unshareWithAll") : t("project.shareWithAll")}
                              title={memory.sharedWithAll ? t("project.unshareWithAll") : t("project.shareWithAll")}
                              onClick={() => void handleUpdateMemorySharedWithAll(memory.id, !memory.sharedWithAll)}
                            >
                              {memory.sharedWithAll ? <UsersIcon /> : <UserShieldIcon />}
                            </ActionButton>
                            <ActionButton
                              iconOnly
                              type="button"
                              busy={isPending(`memLock:${memory.id}`)}
                              aria-label={memory.locked ? t("project.unlockMemory") : t("project.lockMemory")}
                              title={memory.locked ? t("project.unlockMemory") : t("project.lockMemory")}
                              onClick={() => void handleToggleMemoryLock(memory.id, !memory.locked)}
                            >
                              {memory.locked ? <LockOpenIcon /> : <LockIcon />}
                            </ActionButton>
                            <ActionButton
                              iconOnly
                              type="button"
                              data-delete=""
                              busy={isPending(`deleteMem:${memory.id}`)}
                              aria-label={t("project.deleteMemory")}
                              title={t("project.deleteMemory")}
                              onClick={(event) => void handleDeleteMemory(memory, event.currentTarget)}
                            >
                              <TrashIcon />
                            </ActionButton>
                          </Row>
                          <Row style={{ alignItems: "center" }}>
                            <Badge tone="muted">{memory.source}</Badge>
                            {memory.locked ? <Badge tone="warm">{t("project.locked")}</Badge> : null}
                            {memory.sharedWithAll ? <Badge tone="accent">{t("project.sharedWithAll")}</Badge> : null}
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
        </Dialog>
      ) : null}
    </>
  );
}

// A deleted row takes its delete button with it, and focus would fall to the top of
// the page. It goes to the same button on a neighbouring row instead, or to the
// nearest control of the region when the row was the last (snz-design doc-9 §5.1).
// Found before the delete, while the row is still there to look around from.
function focusTargetAfterRemoval(trigger: HTMLElement, fallback: HTMLElement | null): HTMLElement | null {
  const row = trigger.closest("[data-row]");
  for (const sibling of [row?.nextElementSibling, row?.previousElementSibling]) {
    if (sibling instanceof HTMLElement && sibling.matches("[data-row]")) {
      const control = sibling.querySelector<HTMLElement>("[data-delete]");
      if (control) {
        return control;
      }
    }
  }
  return fallback;
}

// Waits for the row to leave the DOM: the render that removes it is not always done
// by the next frame (WebKit ran the frame first). Gives up after a few frames.
function focusIfLost(trigger: HTMLElement, target: HTMLElement | null) {
  let frames = 0;
  const step = () => {
    if (trigger.isConnected && frames < 10) {
      frames += 1;
      requestAnimationFrame(step);
      return;
    }
    const active = document.activeElement;
    if ((!active || active === document.body) && target?.isConnected) {
      target.focus();
    }
  };
  requestAnimationFrame(step);
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

// Only a MIME type seen while dragging, before any name is known: a Markdown file
// often has none, which the drop zone reads as "not known yet" rather than refused.
function isDocumentMime(mime: string) {
  return mime.startsWith("image/") || mime === "text/plain" || mime === "text/markdown" || mime === "text/x-markdown";
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
