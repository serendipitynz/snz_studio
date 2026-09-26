import { FormEvent, KeyboardEvent, UIEvent, useEffect, useMemo, useRef, useState } from "react";
import {
  api,
  authHeaders,
  ChatRecord,
  ChatSummary,
  DocumentRecord,
  MessageRecord,
  Project,
  Project as ProjectRecord
} from "../api/client";
import type { ReviewReference } from "../api/client";
import { ActionButton } from "../components/ActionButton";
import { Checkbox } from "../components/Checkbox";
import { useConfirm } from "../components/ConfirmDialog";
import { CopyMessageButton } from "../components/CopyMessageButton";
import { Dialog, DialogTitle } from "../components/Dialog";
import { ExportChatButton } from "../components/ExportChatButton";
import { FailureNotice } from "../components/FailureNotice";
import { DropZoneProgress, FileDropZone } from "../components/FileDropZone";
import { GuardedSelect } from "../components/GuardedSelect";
import {
  CheckIcon,
  ClipboardIcon,
  FilePlusIcon,
  MessageSquareCodeIcon,
  MessagesSquareIcon,
  PanelRightCloseIcon,
  PanelRightOpenIcon,
  PencilIcon,
  RotateCwFadingClockIcon,
  SendIcon,
  SpinnerIcon
} from "../components/icons";
import { MarkdownPreview } from "../components/MarkdownPreview";
import { MessageReferences } from "../components/MessageReferences";
import { useSideRegion } from "../components/useSideRegion";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import { useLanguage } from "../i18n";
import {
  Badge,
  Button,
  Card,
  Composer,
  ComposerBox,
  Field,
  FloatingScrollButton,
  IconButton,
  Input,
  InspectorPane,
  Item,
  List,
  MainPane,
  MessageArea,
  MessageBubble,
  MessageScroller,
  MetaText,
  PaneHeader,
  RegionCloseButton,
  RegionToggleButton,
  Row,
  SectionTitle,
  Stack,
  Subtle,
  SubsectionTitle,
  Textarea,
  VisuallyHidden,
  WorkspaceShell
} from "../styles/ui";
import { useParams } from "react-router-dom";

interface ChatState {
  project: Project;
  chat: ChatRecord;
  summary: ChatSummary | null;
  messages: MessageRecord[];
}

// Where a failure is told: next to what failed rather than at the top of the page
// (snz-design doc-9 §5.5).
type ErrorArea = "load" | "header" | "composer" | "title" | "upload" | "review";

function createOptimisticMessage(chatId: string, role: "user" | "assistant", content: string): MessageRecord {
  return {
    id: `temp-${role}-${crypto.randomUUID()}`,
    chatId,
    role,
    content,
    createdAt: new Date().toISOString(),
    responseMs: null,
    outputTokens: null,
    tokensPerSecond: null,
    modelName: null,
    participantId: null,
    addressedParticipantIds: [],
    references: []
  };
}

function formatAssistantMetrics(message: MessageRecord) {
  if (message.role !== "assistant") {
    return "";
  }

  const parts = [];
  if (message.responseMs != null) {
    parts.push(`${(message.responseMs / 1000).toFixed(1)}s`);
  }
  if (message.outputTokens != null) {
    parts.push(`${message.outputTokens} tok`);
  }
  if (message.tokensPerSecond != null) {
    parts.push(`${message.tokensPerSecond.toFixed(1)} tok/s`);
  }
  return parts.join(" · ");
}

function formatAssistantModel(message: MessageRecord) {
  if (message.role !== "assistant") {
    return "";
  }

  return message.modelName ?? "";
}

const INSPECTOR_STORAGE_KEY = "snz.chat.inspectorCollapsed";

// The 24px bare icon buttons of a message's action row, shared with the copy button.
const MESSAGE_ACTION_STYLE = { width: 24, height: 24, border: "none", background: "transparent", padding: 0 } as const;

export function ChatPage() {
  const { chatId = "" } = useParams();
  const { t } = useLanguage();
  const confirm = useConfirm();
  const messageScrollerRef = useRef<HTMLDivElement | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const [state, setState] = useState<ChatState | null>(null);
  const [projects, setProjects] = useState<ProjectRecord[]>([]);
  const [projectChats, setProjectChats] = useState<ChatRecord[]>([]);
  const [projectDocuments, setProjectDocuments] = useState<DocumentRecord[]>([]);
  const [draft, setDraft] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [savingTitle, setSavingTitle] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<DropZoneProgress | null>(null);
  const [errors, setErrors] = useState<Partial<Record<ErrorArea, string>>>({});
  const [messageErrors, setMessageErrors] = useState<Record<string, string>>({});
  const [showScrollToBottom, setShowScrollToBottom] = useState(false);
  const [isTitleModalOpen, setIsTitleModalOpen] = useState(false);
  const [isDocumentModalOpen, setIsDocumentModalOpen] = useState(false);
  const inspector = useSideRegion(INSPECTOR_STORAGE_KEY, t("chat.contextInspector"));
  const [titleDraft, setTitleDraft] = useState("");
  const [isTemporaryDraft, setIsTemporaryDraft] = useState(false);
  const [isComposing, setIsComposing] = useState(false);
  const [documentPickerValue, setDocumentPickerValue] = useState("");
  const [reviewingMessageId, setReviewingMessageId] = useState<string | null>(null);
  const [reviewTargetMessageId, setReviewTargetMessageId] = useState<string | null>(null);
  const [reviewContent, setReviewContent] = useState("");
  const [reviewReferences, setReviewReferences] = useState<ReviewReference[]>([]);
  const [reviewLoading, setReviewLoading] = useState(false);

  function setError(area: ErrorArea, message: string) {
    setErrors((current) => ({ ...current, [area]: message }));
  }

  function errorMessage(error: unknown, fallback: Parameters<typeof t>[0]) {
    return error instanceof Error ? error.message : t(fallback);
  }

  // Only the first load swaps the page for the loading card. A reload after a
  // failed send keeps the page, so focus and the failure beside the composer stay.
  async function load(initial = false) {
    if (initial) {
      setLoading(true);
    }
    setError("load", "");
    try {
      const chatResponse = await api.getChatDetail(chatId);
      const [projectResponse, projectsResponse] = await Promise.all([
        api.getProjectDetail(chatResponse.project.id),
        api.getProjects()
      ]);

      setState(chatResponse);
      setProjectChats(projectResponse.chats);
      setProjectDocuments(projectResponse.documents);
      setProjects(projectsResponse.projects);
    } catch (nextError) {
      setError("load", errorMessage(nextError, "chat.loadError"));
    } finally {
      if (initial) {
        setLoading(false);
      }
    }
  }

  useEffect(() => {
    setErrors({});
    setMessageErrors({});
    void load(true);
  }, [chatId]);

  useEffect(() => {
    setTitleDraft(state?.chat.title ?? "");
    setIsTemporaryDraft(state?.chat.isTemporary ?? false);
  }, [state?.chat.title, state?.chat.isTemporary]);

  useEffect(() => {
    const node = textareaRef.current;
    if (!node) {
      return;
    }

    const computedStyle = window.getComputedStyle(node);
    const lineHeight = Number.parseFloat(computedStyle.lineHeight) || 22;
    const padding =
      Number.parseFloat(computedStyle.paddingTop || "0") + Number.parseFloat(computedStyle.paddingBottom || "0");
    const border =
      Number.parseFloat(computedStyle.borderTopWidth || "0") + Number.parseFloat(computedStyle.borderBottomWidth || "0");
    const minHeight = 110;
    const maxHeight = Math.round(lineHeight * 20 + padding + border);

    node.style.height = "auto";
    const nextHeight = draft.trim() ? Math.min(Math.max(node.scrollHeight, minHeight), maxHeight) : minHeight;
    node.style.height = `${nextHeight}px`;
    node.style.overflowY = node.scrollHeight > maxHeight ? "auto" : "hidden";
  }, [draft]);

  useEffect(() => {
    const node = messageScrollerRef.current;
    if (!node) {
      return;
    }

    const updateScrollIndicator = () => {
      const remaining = node.scrollHeight - node.scrollTop - node.clientHeight;
      setShowScrollToBottom(remaining > 24);
    };

    updateScrollIndicator();
    window.addEventListener("resize", updateScrollIndicator);
    return () => window.removeEventListener("resize", updateScrollIndicator);
  }, [state?.messages.length, loading]);

  useEffect(() => {
    if (loading) {
      return;
    }

    const node = messageScrollerRef.current;
    if (!node) {
      return;
    }

    const frame = window.requestAnimationFrame(() => {
      node.scrollTop = node.scrollHeight;
    });

    return () => window.cancelAnimationFrame(frame);
  }, [chatId, loading, state?.messages.length]);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (!draft.trim() || sending) {
      return;
    }

    const content = draft;
    setSending(true);
    setError("composer", "");
    setDraft("");

    const optimisticUserMessage = createOptimisticMessage(chatId, "user", content);
    const optimisticAssistantMessage = createOptimisticMessage(chatId, "assistant", "");

    setState((current) =>
      current
        ? {
            ...current,
            messages: [...current.messages, optimisticUserMessage, optimisticAssistantMessage]
          }
        : current
    );

    try {
      await streamMessage(content, optimisticAssistantMessage.id);
    } catch (nextError) {
      setError("composer", errorMessage(nextError, "chat.sendError"));
      void load();
    } finally {
      setSending(false);
    }
  }

  async function streamMessage(content: string, assistantMessageId: string) {
    const response = await fetch(`${window.__API_BASE__ ?? ""}/api/chats/${chatId}/messages/stream`, {
      method: "POST",
      headers: authHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ content })
    });

    if (!response.ok) {
      const data = (await response.json().catch(() => null)) as { error?: string } | null;
      throw new Error(data?.error ?? t("chat.requestFailed", { status: response.status }));
    }

    if (!response.body) {
      throw new Error(t("chat.streamNoBody"));
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    const handleEvent = (rawChunk: string) => {
      const lines = rawChunk.split(/\r?\n/);
      const eventName = lines.find((line) => line.startsWith("event:"))?.slice(6).trim() ?? "message";
      const dataText = lines
        .filter((line) => line.startsWith("data:"))
        .map((line) => line.slice(5).trimStart())
        .join("\n");

      if (!dataText) {
        return;
      }

      const payload = JSON.parse(dataText) as
        | { content?: string }
        | { message?: string }
        | { chat?: ChatRecord; messages?: MessageRecord[]; summary?: ChatSummary | null };

      if (eventName === "delta") {
        const delta = "content" in payload ? payload.content ?? "" : "";
        if (!delta) {
          return;
        }

        setState((current) =>
          current
            ? {
                ...current,
                messages: current.messages.map((message) =>
                  message.id === assistantMessageId ? { ...message, content: `${message.content}${delta}` } : message
                )
              }
            : current
        );
        return;
      }

      if (eventName === "done" && "messages" in payload) {
        setState((current) =>
          current
            ? {
                ...current,
                chat: payload.chat ?? current.chat,
                messages: payload.messages ?? current.messages,
                summary: payload.summary ?? current.summary
              }
            : current
        );
        if (payload.chat) {
          setProjectChats((current) => current.map((chat) => (chat.id === payload.chat?.id ? payload.chat : chat)));
        }
        return;
      }

      if (eventName === "error") {
        const message = "message" in payload ? payload.message ?? t("chat.streamFailed") : t("chat.streamFailed");
        throw new Error(message);
      }
    };

    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }

      buffer += decoder.decode(value, { stream: true });
      let separatorIndex = buffer.indexOf("\n\n");

      while (separatorIndex >= 0) {
        const rawChunk = buffer.slice(0, separatorIndex).trim();
        buffer = buffer.slice(separatorIndex + 2);
        if (rawChunk) {
          handleEvent(rawChunk);
        }
        separatorIndex = buffer.indexOf("\n\n");
      }
    }

    buffer += decoder.decode();
    if (buffer.trim()) {
      handleEvent(buffer.trim());
    }
  }

  async function handleUpdateChatTitle(event: FormEvent) {
    event.preventDefault();
    if (!state || savingTitle) {
      return;
    }

    setSavingTitle(true);
    setError("title", "");

    try {
      let nextChat = state.chat;

      if (titleDraft !== state.chat.title) {
        const response = await api.updateChatTitle(state.chat.id, titleDraft);
        nextChat = response.chat;
      }

      if (isTemporaryDraft !== nextChat.isTemporary) {
        const response = await api.updateChatTemporary(state.chat.id, isTemporaryDraft);
        nextChat = response.chat;
      }

      setState((current) => (current ? { ...current, chat: nextChat } : current));
      setProjectChats((current) => current.map((chat) => (chat.id === nextChat.id ? nextChat : chat)));
      setIsTitleModalOpen(false);
    } catch (nextError) {
      setError("title", errorMessage(nextError, "chat.updateError"));
    } finally {
      setSavingTitle(false);
    }
  }

  // A save still in flight would report into a dialog no longer on screen.
  function closeTitleModal() {
    if (!savingTitle) {
      setIsTitleModalOpen(false);
      setError("title", "");
    }
  }

  function insertDocumentTitle(title: string) {
    const textarea = textareaRef.current;
    if (!textarea) {
      setDraft((current) => `${current}${title}`);
      return;
    }

    const start = textarea.selectionStart ?? draft.length;
    const end = textarea.selectionEnd ?? draft.length;
    const nextDraft = `${draft.slice(0, start)}${title}${draft.slice(end)}`;
    const nextCursor = start + title.length;

    setDraft(nextDraft);
    window.requestAnimationFrame(() => {
      textarea.focus();
      textarea.setSelectionRange(nextCursor, nextCursor);
    });
  }

  function handleDocumentSelect(value: string) {
    setDocumentPickerValue("");
    if (!value) {
      return;
    }

    insertDocumentTitle(value);
  }

  // The drop zone has already refused the kinds it does not take, so every file
  // here is imported, after an overwrite is confirmed where one would happen.
  async function handleFilesChosen(files: File[]) {
    if (!state || uploadProgress) {
      return;
    }

    setError("upload", "");
    let currentDocuments = [...projectDocuments];
    let failed = false;

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
        const response = await api.createDocument(state.project.id, formData);
        currentDocuments = [response.document, ...currentDocuments];
      }
    } catch (nextError) {
      failed = true;
      setError("upload", errorMessage(nextError, "chat.uploadError"));
    } finally {
      // Also after a failure: the files saved before it are in the project now.
      setProjectDocuments(currentDocuments);
      setUploadProgress(null);
    }

    if (!failed) {
      setIsDocumentModalOpen(false);
    }
  }

  // An import still running would report its failure into a dialog no longer on
  // screen, so the dialog stays until the import has finished.
  function closeDocumentModal() {
    if (uploadProgress) {
      return;
    }
    setIsDocumentModalOpen(false);
    setError("upload", "");
  }

  function handleComposerKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (isComposing || event.nativeEvent.isComposing || event.keyCode === 229) {
      return;
    }

    if (event.key !== "Enter" || event.shiftKey) {
      return;
    }

    event.preventDefault();
    if (!draft.trim() || sending) {
      return;
    }

    void handleSubmit(event);
  }

  const latestAssistantMessage = useMemo(() => {
    return [...(state?.messages ?? [])].reverse().find((message) => message.role === "assistant") ?? null;
  }, [state]);

  function handleMessageScroll(event: UIEvent<HTMLDivElement>) {
    const node = event.currentTarget;
    const remaining = node.scrollHeight - node.scrollTop - node.clientHeight;
    setShowScrollToBottom(remaining > 24);
  }

  function scrollToBottom() {
    const node = messageScrollerRef.current;
    if (!node) {
      return;
    }

    node.scrollTo({
      top: node.scrollHeight,
      behavior: "smooth"
    });
  }

  function closeReview() {
    setReviewTargetMessageId(null);
    setReviewContent("");
    setReviewReferences([]);
    setReviewLoading(false);
    setError("review", "");
  }

  async function handleReviewMessage(messageId: string) {
    setReviewTargetMessageId(messageId);
    setReviewContent("");
    setReviewReferences([]);
    setReviewingMessageId(messageId);
    setReviewLoading(true);
    setError("review", "");

    try {
      await streamReviewMessage(messageId);
    } catch (nextError) {
      setError("review", errorMessage(nextError, "chat.reviewError"));
    } finally {
      setReviewingMessageId(null);
      setReviewLoading(false);
    }
  }

  async function streamReviewMessage(messageId: string) {
    const response = await fetch(`${window.__API_BASE__ ?? ""}/api/messages/${messageId}/review/stream`, {
      method: "POST",
      headers: authHeaders()
    });

    if (!response.ok) {
      const data = (await response.json().catch(() => null)) as { error?: string } | null;
      throw new Error(data?.error ?? t("chat.requestFailed", { status: response.status }));
    }

    if (!response.body) {
      throw new Error(t("chat.reviewStreamNoBody"));
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    const handleEvent = (rawChunk: string) => {
      const lines = rawChunk.split(/\r?\n/);
      const eventName = lines.find((line) => line.startsWith("event:"))?.slice(6).trim() ?? "message";
      const dataText = lines
        .filter((line) => line.startsWith("data:"))
        .map((line) => line.slice(5).trimStart())
        .join("\n");

      if (!dataText) {
        return;
      }

      const payload = JSON.parse(dataText) as {
        content?: string;
        message?: string;
        review?: string;
        references?: ReviewReference[];
      };

      if (eventName === "delta" && payload.content) {
        setReviewContent((current) => `${current}${payload.content}`);
        return;
      }

      if (eventName === "done") {
        setReviewContent(payload.review ?? "");
        setReviewReferences(payload.references ?? []);
        return;
      }

      if (eventName === "error") {
        throw new Error(payload.message ?? t("chat.reviewStreamFailed"));
      }
    };

    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }

      buffer += decoder.decode(value, { stream: true });
      let separatorIndex = buffer.indexOf("\n\n");

      while (separatorIndex >= 0) {
        const rawChunk = buffer.slice(0, separatorIndex).trim();
        buffer = buffer.slice(separatorIndex + 2);
        if (rawChunk) {
          handleEvent(rawChunk);
        }
        separatorIndex = buffer.indexOf("\n\n");
      }
    }

    buffer += decoder.decode();
    if (buffer.trim()) {
      handleEvent(buffer.trim());
    }
  }

  async function handleCopyReview() {
    setError("review", "");
    try {
      await navigator.clipboard.writeText(reviewContent);
    } catch (nextError) {
      setError("review", errorMessage(nextError, "chat.copyReviewError"));
    }
  }

  if (loading) {
    return <Card>{t("chat.loading")}</Card>;
  }

  if (!state) {
    return <Card>{errors.load ? <FailureNotice>{errors.load}</FailureNotice> : t("chat.notFound")}</Card>;
  }

  const sendReason = sending || draft.trim() ? undefined : t("chat.messageRequired");
  const documentPickerReason = uploadProgress
    ? t("chat.uploadingReason")
    : projectDocuments.length === 0
      ? t("chat.noDocumentsReason")
      : undefined;

  return (
    <WorkspaceShell $side={inspector.shown}>
      <WorkspaceSidebar
        projects={projects}
        currentProjectId={state.project.id}
        chats={projectChats}
        activeChatId={state.chat.id}
      />

      <MainPane>
        <PaneHeader>
          <Row style={{ alignItems: "center" }}>
            <MessagesSquareIcon size={18} />
            {state.chat.isTemporary ? (
              <>
                <RotateCwFadingClockIcon size={18} />
                <VisuallyHidden>{t("chat.temporaryChat")}</VisuallyHidden>
              </>
            ) : null}
            {state.chat.title.trim() ? (
              <SectionTitle>{state.chat.title}</SectionTitle>
            ) : (
              <Subtle style={{ opacity: 0.78 }}>{t("sidebar.untitled")}</Subtle>
            )}
            <Badge tone="accent">{state.project.title}</Badge>
          </Row>
          <Row style={{ alignItems: "center", flexWrap: "nowrap", gap: 8 }}>
            <ExportChatButton
              chatId={state.chat.id}
              chatTitle={state.chat.title}
              onError={(message) => setError("header", message)}
            />
            <IconButton type="button" aria-label={t("chat.editTitle")} onClick={() => setIsTitleModalOpen(true)}>
              <PencilIcon />
            </IconButton>
            <RegionToggleButton
              type="button"
              aria-label={t("chat.contextInspector")}
              title={inspector.shown ? t("chat.hideInspector") : t("chat.showInspector")}
              {...inspector.triggerProps}
            >
              {inspector.shown ? <PanelRightCloseIcon /> : <PanelRightOpenIcon />}
            </RegionToggleButton>
          </Row>
        </PaneHeader>

        {errors.header ? (
          <div style={{ padding: "12px 20px 0" }}>
            <FailureNotice>{errors.header}</FailureNotice>
          </div>
        ) : null}

        <MessageArea>
          <MessageScroller ref={messageScrollerRef} onScroll={handleMessageScroll}>
            <Stack>
              {errors.load ? <FailureNotice>{errors.load}</FailureNotice> : null}
              {state.messages.map((message) => (
                <MessageBubble key={message.id} $role={message.role}>
                  <Stack>
                    {message.role === "assistant" ? (
                      message.content ? (
                        <MarkdownPreview source={message.content} />
                      ) : (
                        <Row style={{ alignItems: "center", gap: 10 }}>
                          <SpinnerIcon size={14} />
                          <MetaText>{t("chat.generating")}</MetaText>
                        </Row>
                      )
                    ) : (
                      <div style={{ whiteSpace: "pre-wrap", lineHeight: 1.65 }}>{message.content}</div>
                    )}
                    {message.references.length || message.role === "assistant" ? (
                      <Row style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 12 }}>
                        <div style={{ minWidth: 0, flex: 1 }}>
                          <MessageReferences references={message.references} />
                        </div>
                        {message.role === "assistant" ? (
                          <Stack style={{ gap: 2, alignItems: "flex-end" }}>
                            <MetaText style={{ whiteSpace: "nowrap", textAlign: "right" }}>
                              {formatAssistantMetrics(message)}
                            </MetaText>
                            {formatAssistantModel(message) ? (
                              <MetaText style={{ whiteSpace: "nowrap", textAlign: "right" }}>
                                {formatAssistantModel(message)}
                              </MetaText>
                            ) : null}
                          </Stack>
                        ) : null}
                      </Row>
                    ) : null}
                    <Row style={{ justifyContent: "flex-end", alignItems: "center", gap: 10, flexWrap: "nowrap" }}>
                      {message.role === "assistant" ? (
                        <ActionButton
                          type="button"
                          iconOnly
                          aria-label={t("chat.reviewMessage")}
                          title={t("chat.review")}
                          busy={reviewingMessageId === message.id}
                          onClick={() => void handleReviewMessage(message.id)}
                          style={MESSAGE_ACTION_STYLE}
                        >
                          <MessageSquareCodeIcon />
                        </ActionButton>
                      ) : null}
                      <CopyMessageButton
                        content={message.content}
                        onError={(next) => setMessageErrors((current) => ({ ...current, [message.id]: next }))}
                      />
                      <MetaText style={{ whiteSpace: "nowrap" }}>
                        {new Date(message.createdAt).toLocaleTimeString()}
                      </MetaText>
                    </Row>
                    {messageErrors[message.id] ? <FailureNotice>{messageErrors[message.id]}</FailureNotice> : null}
                  </Stack>
                </MessageBubble>
              ))}
            </Stack>
          </MessageScroller>

          {showScrollToBottom ? (
            <FloatingScrollButton type="button" onClick={scrollToBottom}>
              <span>{t("chat.scrollToLatest")}</span>
              <ScrollDownIcon />
            </FloatingScrollButton>
          ) : null}
        </MessageArea>

        <Composer onSubmit={handleSubmit}>
          <ComposerBox>
            {errors.composer ? <FailureNotice>{errors.composer}</FailureNotice> : null}
            <Textarea
              ref={textareaRef}
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={handleComposerKeyDown}
              onCompositionStart={() => setIsComposing(true)}
              onCompositionEnd={() => setIsComposing(false)}
              placeholder={t("chat.composerPlaceholder")}
              aria-label={t("chat.composerLabel")}
              style={{ minHeight: 110, maxHeight: 460, resize: "none" }}
            />
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <Row style={{ alignItems: "center", flex: 1, minWidth: 0 }}>
                <GuardedSelect
                  value={documentPickerValue}
                  aria-label={t("chat.insertDocument")}
                  disabledReason={documentPickerReason}
                  onChange={(event) => handleDocumentSelect(event.target.value)}
                  style={{ minWidth: 230, maxWidth: 360 }}
                >
                  {projectDocuments.length === 0 ? (
                    <option value="">{t("chat.noDocumentOption")}</option>
                  ) : (
                    <>
                      <option value="">{t("chat.insertDocument")}</option>
                      {projectDocuments.map((document) => (
                        <option key={document.id} value={document.title}>
                          {document.title}
                        </option>
                      ))}
                    </>
                  )}
                </GuardedSelect>
                <ActionButton
                  type="button"
                  iconOnly
                  aria-label={t("chat.addDocument")}
                  title={t("chat.addDocument")}
                  busy={Boolean(uploadProgress)}
                  onClick={() => setIsDocumentModalOpen(true)}
                >
                  <FilePlusIcon />
                </ActionButton>
              </Row>
              <ActionButton type="submit" icon={<SendIcon />} busy={sending} disabledReason={sendReason}>
                {t("chat.send")}
              </ActionButton>
            </Row>
          </ComposerBox>
        </Composer>
      </MainPane>

      <InspectorPane $toggled {...inspector.regionProps} aria-labelledby={`${inspector.regionProps.id}-heading`}>
        {inspector.overlay ? (
          <RegionCloseButton type="button" {...inspector.closeProps}>
            <PanelRightCloseIcon />
          </RegionCloseButton>
        ) : null}
        <SectionTitle id={`${inspector.regionProps.id}-heading`}>{t("chat.contextInspector")}</SectionTitle>

        <Card>
          <Stack>
            <SubsectionTitle>{t("chat.chatSummary")}</SubsectionTitle>
            <Subtle>{state.summary?.summary || t("chat.noSummary")}</Subtle>
          </Stack>
        </Card>

        <Card>
          <Stack>
            <SubsectionTitle>{t("chat.latestReferences")}</SubsectionTitle>
            {latestAssistantMessage?.references.length ? (
              <List>
                {latestAssistantMessage.references.map((reference) => (
                  <Item key={reference.id}>
                    <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                      <strong>{reference.label}</strong>
                      <Badge tone={reference.sourceType === "document" ? "warm" : "accent"}>{reference.sourceType}</Badge>
                    </Row>
                    <Subtle>{reference.excerpt || t("chat.noExcerpt")}</Subtle>
                  </Item>
                ))}
              </List>
            ) : (
              <Subtle>{t("chat.noAssistantReferences")}</Subtle>
            )}
          </Stack>
        </Card>

        <Card>
          <Stack>
            <SubsectionTitle>{t("chat.projectPrompt")}</SubsectionTitle>
            <Subtle>{state.project.systemPrompt || t("chat.noProjectPrompt")}</Subtle>
          </Stack>
        </Card>
      </InspectorPane>

      {isTitleModalOpen ? (
        <Dialog onClose={closeTitleModal}>
          <Stack>
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <DialogTitle>{t("chat.editTitleModal")}</DialogTitle>
              <ActionButton
                type="button"
                variant="normal"
                disabledReason={savingTitle ? t("project.savingClose") : undefined}
                onClick={closeTitleModal}
              >
                {t("common.close")}
              </ActionButton>
            </Row>
            <Card as="form" onSubmit={handleUpdateChatTitle}>
              <Stack>
                <Field>
                  {t("chat.titleField")}
                  <Input
                    value={titleDraft}
                    onChange={(event) => setTitleDraft(event.target.value)}
                    placeholder={t("chat.titlePlaceholder")}
                  />
                </Field>
                <Checkbox checked={isTemporaryDraft} onChange={setIsTemporaryDraft}>
                  {t("chat.temporaryChat")}
                </Checkbox>
                <Subtle>{t("chat.temporaryNote")}</Subtle>
                <div>
                  <ActionButton type="submit" icon={<CheckIcon />} busy={savingTitle}>
                    {t("chat.saveSettings")}
                  </ActionButton>
                </div>
                {errors.title ? <FailureNotice>{errors.title}</FailureNotice> : null}
              </Stack>
            </Card>
          </Stack>
        </Dialog>
      ) : null}

      {isDocumentModalOpen ? (
        <Dialog onClose={closeDocumentModal}>
          <Stack>
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <DialogTitle>{t("chat.addDocumentModal")}</DialogTitle>
              <ActionButton
                type="button"
                variant="normal"
                disabledReason={uploadProgress ? t("chat.uploadingClose") : undefined}
                onClick={closeDocumentModal}
              >
                {t("common.close")}
              </ActionButton>
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
              autoFocusChoose
              progress={uploadProgress}
              onFilesChosen={(files) => void handleFilesChosen(files)}
            />

            {errors.upload ? <FailureNotice>{errors.upload}</FailureNotice> : null}
          </Stack>
        </Dialog>
      ) : null}

      {reviewTargetMessageId ? (
        <Dialog onClose={closeReview}>
          <Stack>
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <DialogTitle>{t("chat.editorialReview")}</DialogTitle>
              <Button type="button" variant="normal" onClick={closeReview}>
                {t("common.close")}
              </Button>
            </Row>
            <Card style={{ position: "relative" }}>
              <IconButton
                type="button"
                aria-label={t("chat.copyReviewText")}
                onClick={() => void handleCopyReview()}
                title={t("chat.copyReview")}
                style={{ ...MESSAGE_ACTION_STYLE, position: "absolute", top: 14, right: 14 }}
              >
                <ClipboardIcon />
              </IconButton>
              {reviewContent ? (
                <MarkdownPreview source={reviewContent} />
              ) : reviewLoading ? (
                <Row style={{ alignItems: "center", gap: 10 }}>
                  <SpinnerIcon size={14} />
                  <MetaText>{t("chat.reviewing")}</MetaText>
                </Row>
              ) : (
                <Subtle>{t("chat.noReviewContent")}</Subtle>
              )}
              <Row style={{ justifyContent: "flex-end", alignItems: "center", flexWrap: "nowrap", marginTop: 12 }}>
                <IconButton
                  type="button"
                  aria-label={t("chat.copyReviewText")}
                  onClick={() => void handleCopyReview()}
                  title={t("chat.copyReview")}
                  style={MESSAGE_ACTION_STYLE}
                >
                  <ClipboardIcon />
                </IconButton>
              </Row>
            </Card>
            {errors.review ? <FailureNotice>{errors.review}</FailureNotice> : null}
            <Card>
              <Stack>
                <SubsectionTitle>{t("chat.reviewReferences")}</SubsectionTitle>
                {reviewReferences.length ? (
                  <List>
                    {reviewReferences.map((reference) => (
                      <Item key={`${reference.sourceType}-${reference.sourceId}-${reference.label}`}>
                        <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                          <strong>{reference.label}</strong>
                          <Badge tone={reference.sourceType === "document" ? "warm" : "accent"}>{reference.sourceType}</Badge>
                        </Row>
                        <Subtle>{reference.excerpt || t("chat.noExcerpt")}</Subtle>
                      </Item>
                    ))}
                  </List>
                ) : (
                  <Subtle>{t("chat.noReviewReferences")}</Subtle>
                )}
              </Stack>
            </Card>
          </Stack>
        </Dialog>
      ) : null}
    </WorkspaceShell>
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

function isDocumentMime(mime: string) {
  return mime.startsWith("image/") || mime === "text/plain" || mime === "text/markdown" || mime === "text/x-markdown";
}

function ScrollDownIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M4 6.5 8 10.5l4-4" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
