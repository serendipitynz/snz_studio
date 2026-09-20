import { DragEvent, FormEvent, KeyboardEvent, UIEvent, useEffect, useMemo, useRef, useState } from "react";
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
import { useConfirm } from "../components/ConfirmDialog";
import { ExportChatButton } from "../components/ExportChatButton";
import { MarkdownPreview } from "../components/MarkdownPreview";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import { useLanguage } from "../i18n";
import {
  Badge,
  Button,
  Card,
  Composer,
  ComposerBox,
  DropZone,
  ErrorText,
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
  ModalCard,
  ModalOverlay,
  PaneHeader,
  Row,
  Select,
  SectionTitle,
  Stack,
  Subtle,
  Textarea,
  WorkspaceShell
} from "../styles/ui";
import { useParams } from "react-router-dom";

interface ChatState {
  project: Project;
  chat: ChatRecord;
  summary: ChatSummary | null;
  messages: MessageRecord[];
}

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

export function ChatPage() {
  const { chatId = "" } = useParams();
  const { t } = useLanguage();
  const confirm = useConfirm();
  const messageScrollerRef = useRef<HTMLDivElement | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const [state, setState] = useState<ChatState | null>(null);
  const [projects, setProjects] = useState<ProjectRecord[]>([]);
  const [projectChats, setProjectChats] = useState<ChatRecord[]>([]);
  const [projectDocuments, setProjectDocuments] = useState<DocumentRecord[]>([]);
  const [draft, setDraft] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [uploadingDocuments, setUploadingDocuments] = useState(false);
  const [uploadStatus, setUploadStatus] = useState("");
  const [error, setError] = useState("");
  const [showScrollToBottom, setShowScrollToBottom] = useState(false);
  const [isTitleModalOpen, setIsTitleModalOpen] = useState(false);
  const [isDocumentModalOpen, setIsDocumentModalOpen] = useState(false);
  const [isInspectorCollapsed, setIsInspectorCollapsed] = useState(() => {
    if (typeof window === "undefined") {
      return false;
    }

    return window.localStorage.getItem(INSPECTOR_STORAGE_KEY) === "true";
  });
  const [titleDraft, setTitleDraft] = useState("");
  const [isTemporaryDraft, setIsTemporaryDraft] = useState(false);
  const [isComposing, setIsComposing] = useState(false);
  const [documentPickerValue, setDocumentPickerValue] = useState("");
  const [dragActive, setDragActive] = useState(false);
  const [copiedMessageId, setCopiedMessageId] = useState<string | null>(null);
  const [reviewingMessageId, setReviewingMessageId] = useState<string | null>(null);
  const [reviewTargetMessageId, setReviewTargetMessageId] = useState<string | null>(null);
  const [reviewContent, setReviewContent] = useState("");
  const [reviewReferences, setReviewReferences] = useState<ReviewReference[]>([]);
  const [reviewLoading, setReviewLoading] = useState(false);

  async function load() {
    setLoading(true);
    setError("");
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
      setError(nextError instanceof Error ? nextError.message : t("chat.loadError"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [chatId]);

  useEffect(() => {
    setTitleDraft(state?.chat.title ?? "");
    setIsTemporaryDraft(state?.chat.isTemporary ?? false);
  }, [state?.chat.title, state?.chat.isTemporary]);

  useEffect(() => {
    window.localStorage.setItem(INSPECTOR_STORAGE_KEY, String(isInspectorCollapsed));
  }, [isInspectorCollapsed]);

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
    if (!draft.trim()) {
      return;
    }

    const content = draft;
    setSending(true);
    setError("");
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
      setError(nextError instanceof Error ? nextError.message : t("chat.sendError"));
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
    if (!state) {
      return;
    }

    setSending(true);
    setError("");

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
      setError(nextError instanceof Error ? nextError.message : t("chat.updateError"));
    } finally {
      setSending(false);
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

  async function handleUploadFiles(fileList: FileList | File[]) {
    if (!state) {
      return;
    }

    const files = Array.from(fileList);
    if (!files.length) {
      return;
    }

    setUploadingDocuments(true);
    setError("");
    setUploadStatus("");

    try {
      let currentDocuments = [...projectDocuments];

      for (const file of files) {
        const nextType = detectDocumentType(file);
        if (!nextType) {
          throw new Error(t("chat.unsupportedFile", { name: file.name }));
        }

        const existing = currentDocuments.find((document) => document.title === file.name);
        if (existing) {
          const overwrite = await confirm(t("chat.overwritePrompt", { name: file.name }));
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
        setUploadStatus(t("chat.savingDocument", { name: file.name }));
        const response = await api.createDocument(state.project.id, formData);
        currentDocuments = [response.document, ...currentDocuments];
      }

      setProjectDocuments(currentDocuments);
      setIsDocumentModalOpen(false);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("chat.uploadError"));
    } finally {
      setUploadingDocuments(false);
      setUploadStatus("");
      setDragActive(false);
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
      }
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

  async function handleCopyMessage(messageId: string, content: string) {
    try {
      await navigator.clipboard.writeText(content);
      setCopiedMessageId(messageId);
      window.setTimeout(() => {
        setCopiedMessageId((current) => (current === messageId ? null : current));
      }, 1400);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("chat.copyMessageError"));
    }
  }

  async function handleReviewMessage(messageId: string) {
    setReviewTargetMessageId(messageId);
    setReviewContent("");
    setReviewReferences([]);
    setReviewingMessageId(messageId);
    setReviewLoading(true);
    setError("");

    try {
      await streamReviewMessage(messageId);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("chat.reviewError"));
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
    try {
      await navigator.clipboard.writeText(reviewContent);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("chat.copyReviewError"));
    }
  }

  if (loading) {
    return <Card>{t("chat.loading")}</Card>;
  }

  if (!state) {
    return <Card>{error || t("chat.notFound")}</Card>;
  }

  return (
    <WorkspaceShell style={isInspectorCollapsed ? { gridTemplateColumns: "280px minmax(0, 1fr)" } : undefined}>
      <WorkspaceSidebar
        projects={projects}
        currentProjectId={state.project.id}
        chats={projectChats}
        activeChatId={state.chat.id}
      />

      <MainPane>
        <PaneHeader>
          <Row style={{ alignItems: "center" }}>
            <ChatIcon />
            {state.chat.title.trim() ? (
              <SectionTitle>{state.chat.isTemporary ? `⏱️ ${state.chat.title}` : state.chat.title}</SectionTitle>
            ) : (
              <Subtle style={{ opacity: 0.78 }}>
                {state.chat.isTemporary ? `⏱️ ${t("sidebar.untitled")}` : t("sidebar.untitled")}
              </Subtle>
            )}
            <Badge tone="accent">{state.project.title}</Badge>
          </Row>
          <Row style={{ alignItems: "center", flexWrap: "nowrap" }}>
            <ExportChatButton chatId={state.chat.id} chatTitle={state.chat.title} onError={setError} />
            <IconButton type="button" aria-label={t("chat.editTitle")} onClick={() => setIsTitleModalOpen(true)}>
              <EditIcon />
            </IconButton>
            <IconButton
              type="button"
              aria-label={isInspectorCollapsed ? t("chat.showInspector") : t("chat.hideInspector")}
              onClick={() => setIsInspectorCollapsed((current) => !current)}
            >
              {isInspectorCollapsed ? <PanelOpenIcon /> : <PanelCloseIcon />}
            </IconButton>
          </Row>
        </PaneHeader>

        <MessageArea>
          <MessageScroller ref={messageScrollerRef} onScroll={handleMessageScroll}>
            <Stack>
              {error ? <ErrorText>{error}</ErrorText> : null}
              {state.messages.map((message) => (
                <MessageBubble key={message.id} $role={message.role}>
                  <Stack>
                    {message.role === "assistant" ? (
                      message.content ? (
                        <MarkdownPreview source={message.content} />
                      ) : (
                        <Row style={{ alignItems: "center", gap: 10 }}>
                          <SpinnerIcon />
                          <MetaText>{t("chat.generating")}</MetaText>
                        </Row>
                      )
                    ) : (
                      <div style={{ whiteSpace: "pre-wrap", lineHeight: 1.65 }}>{message.content}</div>
                    )}
                    {message.references.length || message.role === "assistant" ? (
                      <Row style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 12 }}>
                        <div style={{ minWidth: 0, flex: 1 }}>
                          {message.references.length ? (
                            <details>
                              <summary>{t("chat.referencesUsed", { count: message.references.length })}</summary>
                              <List style={{ marginTop: 10 }}>
                                {message.references.map((reference) => (
                                  <Item key={reference.id}>
                                    <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                                      <strong>{reference.label}</strong>
                                      <Badge tone={reference.sourceType === "document" ? "warm" : "accent"}>{reference.sourceType}</Badge>
                                    </Row>
                                    <Subtle>{reference.excerpt || t("chat.noExcerpt")}</Subtle>
                                  </Item>
                                ))}
                              </List>
                            </details>
                          ) : null}
                        </div>
                        {message.role === "assistant" ? (
                          <Stack style={{ gap: 2, alignItems: "flex-end" }}>
                            <MetaText style={{ whiteSpace: "nowrap", textAlign: "right", opacity: 0.78 }}>
                              {formatAssistantMetrics(message)}
                            </MetaText>
                            {formatAssistantModel(message) ? (
                              <MetaText style={{ whiteSpace: "nowrap", textAlign: "right", opacity: 0.62 }}>
                                {formatAssistantModel(message)}
                              </MetaText>
                            ) : null}
                          </Stack>
                        ) : null}
                      </Row>
                    ) : null}
                    <Row style={{ justifyContent: "flex-end", alignItems: "center", gap: 10, flexWrap: "nowrap" }}>
                      {message.role === "assistant" ? (
                        <IconButton
                          type="button"
                          aria-label={t("chat.reviewMessage")}
                          onClick={() => void handleReviewMessage(message.id)}
                          disabled={reviewingMessageId === message.id}
                          title={reviewingMessageId === message.id ? t("chat.reviewing") : t("chat.review")}
                          style={{
                            width: 24,
                            height: 24,
                            border: "none",
                            background: "transparent",
                            padding: 0,
                            opacity: reviewingMessageId === message.id ? 0.55 : 0.82
                          }}
                        >
                          <ReviewIcon />
                        </IconButton>
                      ) : null}
                      <IconButton
                        type="button"
                        aria-label={t("chat.copyMessage")}
                        onClick={() => void handleCopyMessage(message.id, message.content)}
                        title={copiedMessageId === message.id ? t("chat.copied") : t("chat.copy")}
                        style={{
                          width: 24,
                          height: 24,
                          border: "none",
                          background: "transparent",
                          padding: 0,
                          opacity: copiedMessageId === message.id ? 1 : 0.82
                        }}
                      >
                        <CopyIcon />
                      </IconButton>
                      <MetaText style={{ whiteSpace: "nowrap", opacity: 0.78 }}>
                        {new Date(message.createdAt).toLocaleTimeString()}
                      </MetaText>
                    </Row>
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
            <Textarea
              ref={textareaRef}
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={handleComposerKeyDown}
              onCompositionStart={() => setIsComposing(true)}
              onCompositionEnd={() => setIsComposing(false)}
              placeholder={t("chat.composerPlaceholder")}
              style={{ minHeight: 110, maxHeight: 460, resize: "none" }}
            />
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <Row style={{ alignItems: "center", flex: 1, minWidth: 0 }}>
                <Select
                  value={documentPickerValue}
                  disabled={projectDocuments.length === 0 || uploadingDocuments}
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
                </Select>
                <IconButton type="button" aria-label={t("chat.addDocument")} onClick={() => setIsDocumentModalOpen(true)} disabled={uploadingDocuments}>
                  <PlusIcon />
                </IconButton>
              </Row>
              <Button type="submit" disabled={sending}>
                {sending ? (
                  <Row style={{ alignItems: "center", gap: 8, flexWrap: "nowrap" }}>
                    <SpinnerIcon />
                    <span>{t("chat.sending")}</span>
                  </Row>
                ) : (
                  t("chat.send")
                )}
              </Button>
            </Row>
          </ComposerBox>
        </Composer>
      </MainPane>

      {!isInspectorCollapsed ? (
        <InspectorPane>
          <SectionTitle>{t("chat.contextInspector")}</SectionTitle>

          <Card>
            <Stack>
              <Badge tone="accent">{t("chat.chatSummary")}</Badge>
              <Subtle>{state.summary?.summary || t("chat.noSummary")}</Subtle>
            </Stack>
          </Card>

          <Card>
            <Stack>
              <Badge tone="warm">{t("chat.latestReferences")}</Badge>
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
              <Badge tone="muted">{t("chat.projectPrompt")}</Badge>
              <Subtle>{state.project.systemPrompt || t("chat.noProjectPrompt")}</Subtle>
            </Stack>
          </Card>
        </InspectorPane>
      ) : null}

      {isTitleModalOpen ? (
        <ModalOverlay onClick={() => setIsTitleModalOpen(false)}>
          <ModalCard onClick={(event) => event.stopPropagation()}>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <SectionTitle>{t("chat.editTitleModal")}</SectionTitle>
                <Button type="button" variant="ghost" onClick={() => setIsTitleModalOpen(false)}>
                  {t("common.close")}
                </Button>
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
                  <label style={{ display: "flex", alignItems: "center", gap: 10 }}>
                    <input
                      type="checkbox"
                      checked={isTemporaryDraft}
                      onChange={(event) => setIsTemporaryDraft(event.target.checked)}
                    />
                    <span>{t("chat.temporaryChat")}</span>
                  </label>
                  <Subtle>{t("chat.temporaryNote")}</Subtle>
                  <div>
                    <Button type="submit" disabled={sending}>
                      {t("chat.saveSettings")}
                    </Button>
                  </div>
                </Stack>
              </Card>
            </Stack>
          </ModalCard>
        </ModalOverlay>
      ) : null}

      {isDocumentModalOpen ? (
        <ModalOverlay onClick={() => setIsDocumentModalOpen(false)}>
          <ModalCard onClick={(event) => event.stopPropagation()}>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <SectionTitle>{t("chat.addDocumentModal")}</SectionTitle>
                <Button type="button" variant="ghost" onClick={() => setIsDocumentModalOpen(false)}>
                  {t("common.close")}
                </Button>
              </Row>

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
                  <Subtle>{t("chat.dropHint")}</Subtle>
                  <Subtle>{t("chat.overwriteHint")}</Subtle>
                  {uploadStatus ? (
                    <Row style={{ alignItems: "center", gap: 10 }}>
                      <SpinnerIcon />
                      <Subtle>{uploadStatus}</Subtle>
                    </Row>
                  ) : null}
                  <div>
                    <Button type="button" onClick={() => fileInputRef.current?.click()} disabled={uploadingDocuments}>
                      {uploadingDocuments ? t("chat.processing") : t("chat.chooseFiles")}
                    </Button>
                  </div>
                </Stack>
              </DropZone>
            </Stack>
          </ModalCard>
        </ModalOverlay>
      ) : null}

      {reviewTargetMessageId ? (
        <ModalOverlay
          onClick={() => {
            setReviewTargetMessageId(null);
            setReviewContent("");
            setReviewReferences([]);
            setReviewLoading(false);
          }}
        >
          <ModalCard onClick={(event) => event.stopPropagation()}>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <SectionTitle>{t("chat.editorialReview")}</SectionTitle>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => {
                    setReviewTargetMessageId(null);
                    setReviewContent("");
                    setReviewReferences([]);
                    setReviewLoading(false);
                  }}
                >
                  {t("common.close")}
                </Button>
              </Row>
              <Card style={{ position: "relative" }}>
                <IconButton
                  type="button"
                  aria-label={t("chat.copyReviewText")}
                  onClick={() => void handleCopyReview()}
                  title={t("chat.copyReview")}
                  style={{
                    position: "absolute",
                    top: 14,
                    right: 14,
                    width: 24,
                    height: 24,
                    border: "none",
                    background: "transparent",
                    padding: 0,
                    opacity: 0.82
                  }}
                >
                  <CopyIcon />
                </IconButton>
                {reviewContent ? (
                  <MarkdownPreview source={reviewContent} />
                ) : reviewLoading ? (
                  <Row style={{ alignItems: "center", gap: 10 }}>
                    <SpinnerIcon />
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
                    style={{ width: 24, height: 24, border: "none", background: "transparent", padding: 0, opacity: 0.82 }}
                  >
                    <CopyIcon />
                  </IconButton>
                </Row>
              </Card>
              <Card>
                <Stack>
                  <Badge tone="warm">{t("chat.reviewReferences")}</Badge>
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
          </ModalCard>
        </ModalOverlay>
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

function PlusIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M8 3v10M3 8h10" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  );
}

function PanelCloseIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M2.75 3.25h10.5v9.5H2.75z" stroke="currentColor" strokeWidth="1.2" />
      <path d="M10.25 3.25v9.5" stroke="currentColor" strokeWidth="1.2" />
      <path d="M8.25 8 5.75 10.25V5.75L8.25 8Z" fill="currentColor" />
    </svg>
  );
}

function PanelOpenIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M2.75 3.25h10.5v9.5H2.75z" stroke="currentColor" strokeWidth="1.2" />
      <path d="M10.25 3.25v9.5" stroke="currentColor" strokeWidth="1.2" />
      <path d="M7.75 8 10.25 5.75v4.5L7.75 8Z" fill="currentColor" />
    </svg>
  );
}

function CopyIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M5.25 5V3.75c0-.55.45-1 1-1h5a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H10"
        stroke="currentColor"
        strokeWidth="1.2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <rect x="3" y="5.25" width="7.75" height="8" rx="1" stroke="currentColor" strokeWidth="1.2" />
    </svg>
  );
}

function ReviewIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M3.25 3.5h9.5v6.75h-5l-2.75 2v-2h-1.75V3.5Z"
        stroke="currentColor"
        strokeWidth="1.2"
        strokeLinejoin="round"
      />
      <path d="M5.5 6.25h5M5.5 8h3.25" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
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

function ScrollDownIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M4 6.5 8 10.5l4-4" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
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

function ChatIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" fill="none" aria-hidden="true" style={{ flexShrink: 0 }}>
      <path
        d="M5.25 13.5H3.5a1 1 0 0 1-1-1V4.75a1 1 0 0 1 1-1h11a1 1 0 0 1 1 1v7.75a1 1 0 0 1-1 1H8.75l-3.5 2.25V13.5Z"
        stroke="currentColor"
        strokeWidth="1.35"
        strokeLinejoin="round"
      />
    </svg>
  );
}
