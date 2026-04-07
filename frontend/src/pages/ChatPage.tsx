import { FormEvent, KeyboardEvent, UIEvent, useEffect, useMemo, useRef, useState } from "react";
import { api, ChatRecord, ChatSummary, MessageRecord, Project, Project as ProjectRecord } from "../api/client";
import { MarkdownPreview } from "../components/MarkdownPreview";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
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
  ModalCard,
  ModalOverlay,
  PaneHeader,
  Row,
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

export function ChatPage() {
  const { chatId = "" } = useParams();
  const messageScrollerRef = useRef<HTMLDivElement | null>(null);
  const [state, setState] = useState<ChatState | null>(null);
  const [projects, setProjects] = useState<ProjectRecord[]>([]);
  const [projectChats, setProjectChats] = useState<ChatRecord[]>([]);
  const [draft, setDraft] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const [showScrollToBottom, setShowScrollToBottom] = useState(false);
  const [isTitleModalOpen, setIsTitleModalOpen] = useState(false);
  const [titleDraft, setTitleDraft] = useState("");
  const [isComposing, setIsComposing] = useState(false);

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

    setSending(true);
    setError("");

    try {
      const response = await api.sendMessage(chatId, draft);
      setDraft("");
      setState((current) => (current ? { ...current, messages: response.messages, summary: response.summary } : current));
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to send message");
    } finally {
      setSending(false);
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

  if (loading) {
    return <Card>Loading chat…</Card>;
  }

  if (!state) {
    return <Card>{error || "Chat not found"}</Card>;
  }

  return (
    <WorkspaceShell>
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
          <IconButton type="button" aria-label="Edit chat title" onClick={() => setIsTitleModalOpen(true)}>
            <EditIcon />
          </IconButton>
        </PaneHeader>

        <MessageArea>
          <MessageScroller ref={messageScrollerRef} onScroll={handleMessageScroll}>
            <Stack>
              {error ? <Subtle style={{ color: "#ff7a6c" }}>{error}</Subtle> : null}
              {state.messages.map((message) => (
                <MessageBubble key={message.id} $role={message.role}>
                  <Stack>
                    <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
                      <Badge tone={message.role === "assistant" ? "accent" : "muted"}>{message.role}</Badge>
                      <MetaText>{new Date(message.createdAt).toLocaleTimeString()}</MetaText>
                    </Row>
                    {message.role === "assistant" ? (
                      <MarkdownPreview source={message.content} />
                    ) : (
                      <div style={{ whiteSpace: "pre-wrap", lineHeight: 1.65 }}>{message.content}</div>
                    )}
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
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={handleComposerKeyDown}
              onCompositionStart={() => setIsComposing(true)}
              onCompositionEnd={() => setIsComposing(false)}
              placeholder="Message this project workspace..."
              style={{ minHeight: 110 }}
            />
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <MetaText>Context comes from project settings, chat summary, memories, and retrieved documents.</MetaText>
              <Button type="submit" disabled={sending}>
                {sending ? "Sending..." : "Send"}
              </Button>
            </Row>
          </ComposerBox>
        </Composer>
      </MainPane>

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
    </WorkspaceShell>
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
