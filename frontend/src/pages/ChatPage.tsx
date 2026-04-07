import { DragEvent, FormEvent, KeyboardEvent, UIEvent, useEffect, useMemo, useRef, useState } from "react";
import { api, ChatRecord, ChatSummary, DocumentRecord, MessageRecord, Project, Project as ProjectRecord } from "../api/client";
import { MarkdownPreview } from "../components/MarkdownPreview";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import {
  Badge,
  Button,
  Card,
  Composer,
  ComposerBox,
  DropZone,
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

const INSPECTOR_STORAGE_KEY = "snz.chat.inspectorCollapsed";

export function ChatPage() {
  const { chatId = "" } = useParams();
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
  const [isComposing, setIsComposing] = useState(false);
  const [documentPickerValue, setDocumentPickerValue] = useState("");
  const [dragActive, setDragActive] = useState(false);
  const [copiedMessageId, setCopiedMessageId] = useState<string | null>(null);

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
      setError(nextError instanceof Error ? nextError.message : "Failed to load chat");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [chatId]);

  useEffect(() => {
    setTitleDraft(state?.chat.title ?? "");
  }, [state?.chat.title]);

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
      setError(nextError instanceof Error ? nextError.message : "Failed to send message");
      void load();
    } finally {
      setSending(false);
    }
  }

  async function streamMessage(content: string, assistantMessageId: string) {
    const response = await fetch(`/api/chats/${chatId}/messages/stream`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content })
    });

    if (!response.ok) {
      const data = (await response.json().catch(() => null)) as { error?: string } | null;
      throw new Error(data?.error ?? `Request failed with ${response.status}`);
    }

    if (!response.body) {
      throw new Error("Stream response did not include a body");
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
        | { messages?: MessageRecord[]; summary?: ChatSummary | null };

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
                messages: payload.messages ?? current.messages,
                summary: payload.summary ?? current.summary
              }
            : current
        );
        return;
      }

      if (eventName === "error") {
        const message = "message" in payload ? payload.message ?? "Stream failed" : "Stream failed";
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
    if (!state || !titleDraft.trim()) {
      return;
    }

    setSending(true);
    setError("");

    try {
      const response = await api.updateChatTitle(state.chat.id, titleDraft);
      setState((current) => (current ? { ...current, chat: response.chat } : current));
      setProjectChats((current) => current.map((chat) => (chat.id === response.chat.id ? response.chat : chat)));
      setIsTitleModalOpen(false);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to update chat title");
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

    setSending(true);
    setError("");

    try {
      let currentDocuments = [...projectDocuments];

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
        const response = await api.createDocument(state.project.id, formData);
        currentDocuments = [response.document, ...currentDocuments];
      }

      setProjectDocuments(currentDocuments);
      setIsDocumentModalOpen(false);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to upload document");
    } finally {
      setSending(false);
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
      setError(nextError instanceof Error ? nextError.message : "Failed to copy message");
    }
  }

  if (loading) {
    return <Card>Loading chat…</Card>;
  }

  if (!state) {
    return <Card>{error || "Chat not found"}</Card>;
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
            <SectionTitle>{state.chat.title}</SectionTitle>
            <Badge tone="accent">{state.project.title}</Badge>
          </Row>
          <Row style={{ alignItems: "center", flexWrap: "nowrap" }}>
            <IconButton type="button" aria-label="Edit chat title" onClick={() => setIsTitleModalOpen(true)}>
              <EditIcon />
            </IconButton>
            <IconButton
              type="button"
              aria-label={isInspectorCollapsed ? "Show context inspector" : "Hide context inspector"}
              onClick={() => setIsInspectorCollapsed((current) => !current)}
            >
              {isInspectorCollapsed ? <PanelOpenIcon /> : <PanelCloseIcon />}
            </IconButton>
          </Row>
        </PaneHeader>

        <MessageArea>
          <MessageScroller ref={messageScrollerRef} onScroll={handleMessageScroll}>
            <Stack>
              {error ? <Subtle style={{ color: "#ff7a6c" }}>{error}</Subtle> : null}
              {state.messages.map((message) => (
                <MessageBubble key={message.id} $role={message.role}>
                  <Stack>
                    <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                      <Row style={{ alignItems: "center" }}>
                        <Badge tone={message.role === "assistant" ? "accent" : "muted"}>{message.role}</Badge>
                        <IconButton
                          type="button"
                          aria-label="Copy raw message text"
                          onClick={() => void handleCopyMessage(message.id, message.content)}
                        >
                          <CopyIcon />
                        </IconButton>
                        {copiedMessageId === message.id ? <MetaText>Copied</MetaText> : null}
                      </Row>
                      <MetaText>{new Date(message.createdAt).toLocaleTimeString()}</MetaText>
                    </Row>
                    {message.role === "assistant" ? (
                      <MarkdownPreview source={message.content} />
                    ) : (
                      <div style={{ whiteSpace: "pre-wrap", lineHeight: 1.65 }}>{message.content}</div>
                    )}
                    {message.references.length || message.role === "assistant" ? (
                      <Row style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 12 }}>
                        <div style={{ minWidth: 0, flex: 1 }}>
                          {message.references.length ? (
                            <details>
                              <summary>References used ({message.references.length})</summary>
                              <List style={{ marginTop: 10 }}>
                                {message.references.map((reference) => (
                                  <Item key={reference.id}>
                                    <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                                      <strong>{reference.label}</strong>
                                      <Badge tone={reference.sourceType === "document" ? "warm" : "accent"}>{reference.sourceType}</Badge>
                                    </Row>
                                    <Subtle>{reference.excerpt || "No excerpt stored"}</Subtle>
                                  </Item>
                                ))}
                              </List>
                            </details>
                          ) : null}
                        </div>
                        {message.role === "assistant" ? (
                          <MetaText style={{ whiteSpace: "nowrap", textAlign: "right", opacity: 0.78 }}>
                            {formatAssistantMetrics(message)}
                          </MetaText>
                        ) : null}
                      </Row>
                    ) : null}
                  </Stack>
                </MessageBubble>
              ))}
            </Stack>
          </MessageScroller>

          {showScrollToBottom ? (
            <FloatingScrollButton type="button" onClick={scrollToBottom}>
              <span>Scroll to latest</span>
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
              placeholder="Message this project workspace..."
              style={{ minHeight: 110, maxHeight: 460, resize: "none" }}
            />
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <Row style={{ alignItems: "center", flex: 1, minWidth: 0 }}>
                <Select
                  value={documentPickerValue}
                  disabled={projectDocuments.length === 0}
                  onChange={(event) => handleDocumentSelect(event.target.value)}
                  style={{ minWidth: 230, maxWidth: 360 }}
                >
                  {projectDocuments.length === 0 ? (
                    <option value="">(no document)</option>
                  ) : (
                    <>
                      <option value="">select to insert:</option>
                      {projectDocuments.map((document) => (
                        <option key={document.id} value={document.title}>
                          {document.title}
                        </option>
                      ))}
                    </>
                  )}
                </Select>
                <IconButton type="button" aria-label="Add document" onClick={() => setIsDocumentModalOpen(true)}>
                  <PlusIcon />
                </IconButton>
              </Row>
              <Button type="submit" disabled={sending}>
                {sending ? (
                  <Row style={{ alignItems: "center", gap: 8, flexWrap: "nowrap" }}>
                    <SpinnerIcon />
                    <span>Sending...</span>
                  </Row>
                ) : (
                  "Send"
                )}
              </Button>
            </Row>
          </ComposerBox>
        </Composer>
      </MainPane>

      {!isInspectorCollapsed ? (
        <InspectorPane>
          <SectionTitle>Context Inspector</SectionTitle>

          <Card>
            <Stack>
              <Badge tone="accent">Chat summary</Badge>
              <Subtle>{state.summary?.summary || "No summary yet."}</Subtle>
            </Stack>
          </Card>

          <Card>
            <Stack>
              <Badge tone="warm">Latest references</Badge>
              {latestAssistantMessage?.references.length ? (
                <List>
                  {latestAssistantMessage.references.map((reference) => (
                    <Item key={reference.id}>
                      <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                        <strong>{reference.label}</strong>
                        <Badge tone={reference.sourceType === "document" ? "warm" : "accent"}>{reference.sourceType}</Badge>
                      </Row>
                      <Subtle>{reference.excerpt || "No excerpt stored"}</Subtle>
                    </Item>
                  ))}
                </List>
              ) : (
                <Subtle>No assistant references yet.</Subtle>
              )}
            </Stack>
          </Card>

          <Card>
            <Stack>
              <Badge tone="muted">Project prompt</Badge>
              <Subtle>{state.project.systemPrompt || "No project system prompt configured."}</Subtle>
            </Stack>
          </Card>
        </InspectorPane>
      ) : null}

      {isTitleModalOpen ? (
        <ModalOverlay onClick={() => setIsTitleModalOpen(false)}>
          <ModalCard onClick={(event) => event.stopPropagation()}>
            <Stack>
              <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                <SectionTitle>Edit Chat Title</SectionTitle>
                <Button type="button" variant="ghost" onClick={() => setIsTitleModalOpen(false)}>
                  Close
                </Button>
              </Row>
              <Card as="form" onSubmit={handleUpdateChatTitle}>
                <Stack>
                  <Field>
                    Title
                    <Input value={titleDraft} onChange={(event) => setTitleDraft(event.target.value)} placeholder="Chat title" />
                  </Field>
                  <div>
                    <Button type="submit" disabled={sending || !titleDraft.trim()}>
                      Save title
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
                <SectionTitle>Add Document</SectionTitle>
                <Button type="button" variant="ghost" onClick={() => setIsDocumentModalOpen(false)}>
                  Close
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
                  <Subtle>Drop markdown, text, or image files here.</Subtle>
                  <Subtle>If the same file name already exists, you will be asked whether to overwrite it.</Subtle>
                  <div>
                    <Button type="button" onClick={() => fileInputRef.current?.click()} disabled={sending}>
                      {sending ? "Uploading..." : "Choose files"}
                    </Button>
                  </div>
                </Stack>
              </DropZone>
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
