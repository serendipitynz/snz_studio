import { FormEvent, UIEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { api, ChatRecord, ChatSummary, MemoryKind, MessageRecord, Participant, Project, TurnRule } from "../api/client";
import { streamSSE } from "../api/sse";
import { predictNextSpeaker } from "../api/turnOrder";
import { ExportChatButton } from "../components/ExportChatButton";
import { MarkdownPreview } from "../components/MarkdownPreview";
import { MessageReferences } from "../components/MessageReferences";
import { ParticipantPanel } from "../components/ParticipantPanel";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import { useLanguage } from "../i18n";
import {
  Badge,
  Button,
  Card,
  Composer,
  ComposerBox,
  ErrorText,
  Field,
  FloatingScrollButton,
  IconButton,
  InspectorPane,
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
  Select,
  Stack,
  Subtle,
  Textarea,
  WorkspaceShell
} from "../styles/ui";

interface MultiAgentState {
  project: Project;
  chat: ChatRecord;
  summary: ChatSummary | null;
  messages: MessageRecord[];
}

// A turn's completion arrives as a `done` frame carrying the freshly read chat,
// transcript and roster, which is what lets one turn's result replace the whole
// view without a second round trip.
// What one turn needs to run: the roster to cycle, the transcript the cycle is
// derived from, the rule, and the nomination the manual rule requires.
interface TurnInput {
  roster: Participant[];
  messages: MessageRecord[];
  turnRule: TurnRule;
  nomineeId: string;
}

interface TurnDonePayload {
  chat?: ChatRecord;
  message?: MessageRecord;
  messages?: MessageRecord[];
  participants?: Participant[];
}

// The save dialog's contents: which utterance is being saved and what the user
// has made of the draft so far (design §4.4).
interface MemoryDraft {
  message: MessageRecord;
  content: string;
  kind: MemoryKind;
  locked: boolean;
}

const ROSTER_STORAGE_KEY = "snz.multiAgent.rosterCollapsed";
const MEMORY_SAVED_NOTICE_MS = 5000;

// MultiAgentChatPage is the spectator view and progression control of
// docs/multi-agent-chat-design.md §6. Auto-advance is this loop calling the
// one-turn route repeatedly (§4.1): there is no server-side progression job, so
// stopping means not making the next call, and the turn already in flight runs to
// completion on the server whatever this window does.
export function MultiAgentChatPage() {
  const { chatId = "" } = useParams();
  const { t } = useLanguage();
  const messageScrollerRef = useRef<HTMLDivElement | null>(null);
  const [state, setState] = useState<MultiAgentState | null>(null);
  const [participants, setParticipants] = useState<Participant[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectChats, setProjectChats] = useState<ChatRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [turnError, setTurnError] = useState("");
  const [runningSpeaker, setRunningSpeaker] = useState<Participant | null>(null);
  const [streamedContent, setStreamedContent] = useState("");
  const [autoRunning, setAutoRunning] = useState(false);
  const [nomineeId, setNomineeId] = useState("");
  const [draft, setDraft] = useState("");
  const [posting, setPosting] = useState(false);
  const [showScrollToBottom, setShowScrollToBottom] = useState(false);
  const [memoryDraft, setMemoryDraft] = useState<MemoryDraft | null>(null);
  const [memoryPreparing, setMemoryPreparing] = useState(false);
  const [memorySaving, setMemorySaving] = useState(false);
  const [memoryError, setMemoryError] = useState("");
  const [memorySavedTitle, setMemorySavedTitle] = useState("");
  const [isRosterCollapsed, setIsRosterCollapsed] = useState(() => {
    if (typeof window === "undefined") {
      return false;
    }

    return window.localStorage.getItem(ROSTER_STORAGE_KEY) === "true";
  });

  // The loop reads its stop flag from a ref rather than from state: the flag is
  // set while an awaited turn is in flight, and the loop's closure would keep
  // reading the value state had when the iteration started.
  const autoRunningRef = useRef(false);
  const turnInFlightRef = useRef(false);

  const load = useCallback(async () => {
    setError("");
    try {
      const chatResponse = await api.getChatDetail(chatId);
      const [participantsResponse, projectResponse, projectsResponse] = await Promise.all([
        api.listParticipants(chatId),
        api.getProjectDetail(chatResponse.project.id),
        api.getProjects()
      ]);

      setState(chatResponse);
      setParticipants(participantsResponse.participants);
      setProjectChats(projectResponse.chats);
      setProjects(projectsResponse.projects);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("multiAgent.loadError"));
    }
  }, [chatId, t]);

  useEffect(() => {
    setLoading(true);
    void load().finally(() => setLoading(false));
  }, [load]);

  useEffect(() => {
    window.localStorage.setItem(ROSTER_STORAGE_KEY, String(isRosterCollapsed));
  }, [isRosterCollapsed]);

  // Leaving the page stops the loop. The turn in flight still finishes and is
  // stored server-side, which is the same guarantee a disconnect gets (§4.1).
  useEffect(() => {
    return () => {
      autoRunningRef.current = false;
    };
  }, []);

  // Escape closes the save dialog, as it does the confirm dialog: the overlay
  // is otherwise the one thing on screen the keyboard cannot dismiss.
  useEffect(() => {
    if (!memoryDraft) {
      return;
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setMemoryDraft(null);
      }
    }

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [memoryDraft]);

  useEffect(() => {
    if (!memorySavedTitle) {
      return;
    }

    const timer = window.setTimeout(() => setMemorySavedTitle(""), MEMORY_SAVED_NOTICE_MS);
    return () => window.clearTimeout(timer);
  }, [memorySavedTitle]);

  useEffect(() => {
    const node = messageScrollerRef.current;
    if (!node || loading) {
      return;
    }

    const frame = window.requestAnimationFrame(() => {
      node.scrollTop = node.scrollHeight;
    });
    return () => window.cancelAnimationFrame(frame);
  }, [loading, state?.messages.length, streamedContent]);

  const roster = useMemo(
    () => participants.filter((participant) => participant.deletedAt === null),
    [participants]
  );

  const speakerById = useMemo(() => {
    return new Map(participants.map((participant) => [participant.id, participant]));
  }, [participants]);

  const manualRule = state?.chat.turnRule === "manual";
  // A multi-agent chat is one with two or more participants (design §2), and the
  // controls hold that line rather than assuming it: on a roster of one,
  // round_robin re-selects the same speaker every turn — (0 + 1) % 1 — so
  // auto-advance would be one model answering itself until someone stops it.
  const turnBlocked = roster.length < 2 || (manualRule && !nomineeId);

  const currentTurnInput = (): TurnInput => ({
    roster,
    messages: state?.messages ?? [],
    turnRule: state?.chat.turnRule ?? "round_robin",
    nomineeId
  });

  // runTurn takes what the turn needs and hands back what the next turn needs,
  // instead of reading either from the component's scope. The auto-advance loop
  // awaits every turn inside one closure, so a turn reading the scope would see
  // the values of the render the loop started in — the deltas would all be
  // labelled with the first turn's speaker. Threading the `done` frame's own
  // chat, transcript and roster through also keeps the loop on what the server
  // actually stored rather than on whether React has re-rendered yet.
  async function runTurn(input: TurnInput): Promise<TurnInput | null> {
    if (turnInFlightRef.current) {
      return null;
    }

    turnInFlightRef.current = true;
    setTurnError("");
    setStreamedContent("");
    setRunningSpeaker(predictNextSpeaker(input.roster, input.messages, input.turnRule, input.nomineeId));

    let next: TurnInput | null = null;
    try {
      await streamSSE(
        `/api/chats/${chatId}/turns/stream`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(input.turnRule === "manual" ? { participantId: input.nomineeId } : {})
        },
        (event, payload) => {
          if (event === "delta") {
            const delta = typeof payload.content === "string" ? payload.content : "";
            if (delta) {
              setStreamedContent((current) => `${current}${delta}`);
            }
            return;
          }

          if (event === "done") {
            const done = payload as TurnDonePayload;
            const nextParticipants = done.participants ?? [];
            next = {
              roster: done.participants
                ? nextParticipants.filter((participant) => participant.deletedAt === null)
                : input.roster,
              messages: done.messages ?? input.messages,
              turnRule: done.chat?.turnRule ?? input.turnRule,
              nomineeId: input.nomineeId
            };
            setState((current) =>
              current
                ? { ...current, chat: done.chat ?? current.chat, messages: done.messages ?? current.messages }
                : current
            );
            if (done.participants) {
              setParticipants(done.participants);
            }
          }
        }
      );
      return next;
    } catch (nextError) {
      setTurnError(nextError instanceof Error ? nextError.message : t("multiAgent.turnError"));
      // The turn may well have finished on the server after the stream broke, so
      // the transcript is re-read rather than left showing the pre-turn state
      // (§4.1).
      await load();
      return null;
    } finally {
      turnInFlightRef.current = false;
      setRunningSpeaker(null);
      setStreamedContent("");
    }
  }

  async function handleAdvanceTurn() {
    await runTurn(currentTurnInput());
  }

  async function handleToggleAutoRun() {
    if (autoRunningRef.current) {
      // Only the flag is cleared: the turn in flight is not cancelled, so the loop
      // stops at the next turn boundary (§4.1).
      autoRunningRef.current = false;
      setAutoRunning(false);
      return;
    }

    autoRunningRef.current = true;
    setAutoRunning(true);
    try {
      let input = currentTurnInput();
      while (autoRunningRef.current) {
        const next = await runTurn(input);
        if (!next) {
          break;
        }
        input = next;
      }
    } finally {
      autoRunningRef.current = false;
      setAutoRunning(false);
    }
  }

  async function handleIntervene(event: FormEvent) {
    event.preventDefault();
    const content = draft.trim();
    if (!content) {
      return;
    }

    setPosting(true);
    setTurnError("");
    try {
      const response = await api.sendMessage(chatId, content);
      setState((current) => (current ? { ...current, chat: response.chat, messages: response.messages } : current));
      setDraft("");
    } catch (nextError) {
      setTurnError(nextError instanceof Error ? nextError.message : t("multiAgent.interveneError"));
    } finally {
      setPosting(false);
    }
  }

  // The dialog opens on the server's draft rather than on the message alone
  // because the default kind comes from the extraction rules, which live only
  // on the server (design §4.4).
  async function handleOpenMemoryDialog(message: MessageRecord) {
    setMemoryPreparing(true);
    setMemoryError("");
    try {
      const response = await api.getMessageMemoryDraft(message.id);
      setMemoryDraft({ message, content: response.draft.content, kind: response.draft.kind, locked: true });
    } catch (nextError) {
      setMemoryError(nextError instanceof Error ? nextError.message : t("multiAgent.saveMemoryDraftError"));
    } finally {
      setMemoryPreparing(false);
    }
  }

  async function handleSaveMemory(event: FormEvent) {
    event.preventDefault();
    if (!memoryDraft) {
      return;
    }
    const content = memoryDraft.content.trim();
    if (!content) {
      return;
    }

    setMemorySaving(true);
    setMemoryError("");
    try {
      const response = await api.saveMessageMemory(memoryDraft.message.id, {
        content,
        kind: memoryDraft.kind,
        locked: memoryDraft.locked
      });
      setMemoryDraft(null);
      setMemorySavedTitle(response.memory.title);
    } catch (nextError) {
      setMemoryError(nextError instanceof Error ? nextError.message : t("multiAgent.saveMemoryError"));
    } finally {
      setMemorySaving(false);
    }
  }

  function handleMessageScroll(event: UIEvent<HTMLDivElement>) {
    const node = event.currentTarget;
    setShowScrollToBottom(node.scrollHeight - node.scrollTop - node.clientHeight > 24);
  }

  function scrollToBottom() {
    messageScrollerRef.current?.scrollTo({ top: messageScrollerRef.current.scrollHeight, behavior: "smooth" });
  }

  function speakerLabel(message: MessageRecord): string {
    if (message.role === "user") {
      return t("multiAgent.speakerHuman");
    }
    if (!message.participantId) {
      return t("multiAgent.speakerAssistant");
    }

    const participant = speakerById.get(message.participantId);
    if (!participant) {
      return t("multiAgent.speakerAssistant");
    }
    return participant.deletedAt
      ? t("multiAgent.speakerRemoved", { name: participant.displayName })
      : participant.displayName;
  }

  if (loading) {
    return <Card>{t("multiAgent.loading")}</Card>;
  }

  if (!state) {
    return <Card>{error || t("multiAgent.notFound")}</Card>;
  }

  const stopPending = !autoRunning && runningSpeaker !== null;
  const chatTitle = state.chat.title.trim() || t("sidebar.untitled");
  // A temporary multi-agent chat reads project material but never writes it
  // back, so the one write path is closed while the flag is on (design §4.4).
  const memorySaveBlocked = state.chat.isTemporary;

  return (
    <WorkspaceShell $columns={isRosterCollapsed ? "280px minmax(0, 1fr)" : undefined}>
      <WorkspaceSidebar
        projects={projects}
        currentProjectId={state.project.id}
        chats={projectChats}
        activeChatId={state.chat.id}
      />

      <MainPane>
        <PaneHeader>
          <Row style={{ alignItems: "center" }}>
            <SectionTitle>{state.chat.isTemporary ? `⏱️ ${chatTitle}` : chatTitle}</SectionTitle>
            <Badge tone="warm">{t("multiAgent.badge")}</Badge>
            <Badge tone="accent">{state.project.title}</Badge>
          </Row>
          <Row style={{ alignItems: "center", flexWrap: "nowrap" }}>
            <ExportChatButton chatId={state.chat.id} chatTitle={state.chat.title} onError={setError} />
            <IconButton
              type="button"
              aria-label={t("multiAgent.reload")}
              title={t("multiAgent.reload")}
              onClick={() => void load()}
            >
              <ReloadIcon />
            </IconButton>
            <IconButton
              type="button"
              aria-label={isRosterCollapsed ? t("multiAgent.showRoster") : t("multiAgent.hideRoster")}
              title={isRosterCollapsed ? t("multiAgent.showRoster") : t("multiAgent.hideRoster")}
              onClick={() => setIsRosterCollapsed((current) => !current)}
            >
              {isRosterCollapsed ? <PanelOpenIcon /> : <PanelCloseIcon />}
            </IconButton>
          </Row>
        </PaneHeader>

        <MessageArea>
          <MessageScroller ref={messageScrollerRef} onScroll={handleMessageScroll}>
            <Stack>
              {error ? <ErrorText>{error}</ErrorText> : null}
              {turnError ? <ErrorText>{turnError}</ErrorText> : null}
              {memoryError && !memoryDraft ? <ErrorText>{memoryError}</ErrorText> : null}
              {state.messages.length === 0 && !runningSpeaker ? <Subtle>{t("multiAgent.spectatorEmpty")}</Subtle> : null}

              {state.messages.map((message) => (
                <MessageBubble key={message.id} $role={message.role}>
                  <Stack>
                    <Row style={{ justifyContent: "space-between", alignItems: "baseline", gap: 12 }}>
                      <strong style={{ overflowWrap: "anywhere" }}>{speakerLabel(message)}</strong>
                      <MetaText style={{ whiteSpace: "nowrap", opacity: 0.68 }}>{message.modelName ?? ""}</MetaText>
                    </Row>
                    {message.role === "assistant" ? (
                      <MarkdownPreview source={message.content} />
                    ) : (
                      <div style={{ whiteSpace: "pre-wrap", lineHeight: 1.65 }}>{message.content}</div>
                    )}
                    <MessageReferences references={message.references} />
                    <Row style={{ justifyContent: "space-between", alignItems: "center", gap: 12 }}>
                      <Button
                        type="button"
                        variant="ghost"
                        style={{ padding: "6px 10px", fontSize: "0.85rem" }}
                        title={memorySaveBlocked ? t("multiAgent.temporaryNoSave") : t("multiAgent.saveMemoryTitle")}
                        disabled={memorySaveBlocked || memoryPreparing || memorySaving}
                        onClick={() => void handleOpenMemoryDialog(message)}
                      >
                        {t("multiAgent.saveMemory")}
                      </Button>
                      <MetaText style={{ textAlign: "right", opacity: 0.68 }}>
                        {new Date(message.createdAt).toLocaleTimeString()}
                      </MetaText>
                    </Row>
                  </Stack>
                </MessageBubble>
              ))}

              {runningSpeaker || streamedContent ? (
                <MessageBubble $role="assistant">
                  <Stack>
                    <Row style={{ justifyContent: "space-between", alignItems: "baseline", gap: 12 }}>
                      <strong style={{ overflowWrap: "anywhere" }}>
                        {runningSpeaker?.displayName ?? t("multiAgent.runningTurnUnknown")}
                      </strong>
                      <MetaText style={{ whiteSpace: "nowrap", opacity: 0.68 }}>
                        {runningSpeaker?.modelName ?? ""}
                      </MetaText>
                    </Row>
                    {streamedContent ? (
                      <MarkdownPreview source={streamedContent} />
                    ) : (
                      <Row style={{ alignItems: "center", gap: 10 }}>
                        <SpinnerIcon />
                        <MetaText>{t("multiAgent.speaking")}</MetaText>
                      </Row>
                    )}
                  </Stack>
                </MessageBubble>
              ) : null}
            </Stack>
          </MessageScroller>

          {showScrollToBottom ? (
            <FloatingScrollButton type="button" onClick={scrollToBottom}>
              <span>{t("chat.scrollToLatest")}</span>
            </FloatingScrollButton>
          ) : null}
        </MessageArea>

        <Composer onSubmit={handleIntervene}>
          <ComposerBox>
            <Stack>
              <Row style={{ alignItems: "center" }}>
                <Button type="button" onClick={() => void handleAdvanceTurn()} disabled={autoRunning || runningSpeaker !== null || turnBlocked}>
                  {t("multiAgent.advanceTurn")}
                </Button>
                <Button
                  type="button"
                  variant={autoRunning ? "warm" : "solid"}
                  onClick={() => void handleToggleAutoRun()}
                  disabled={manualRule || (!autoRunning && (runningSpeaker !== null || turnBlocked))}
                >
                  {autoRunning ? t("multiAgent.autoStop") : t("multiAgent.autoStart")}
                </Button>
                {manualRule ? (
                  <Select value={nomineeId} onChange={(event) => setNomineeId(event.target.value)} style={{ minWidth: 200 }}>
                    <option value="">{t("multiAgent.nominee")}</option>
                    {roster.map((participant) => (
                      <option key={participant.id} value={participant.id}>
                        {participant.displayName}
                      </option>
                    ))}
                  </Select>
                ) : null}
              </Row>

              <Stack style={{ gap: 4 }}>
                {runningSpeaker ? (
                  <Row style={{ alignItems: "center", gap: 8 }}>
                    <SpinnerIcon />
                    <MetaText>{t("multiAgent.runningTurn", { name: runningSpeaker.displayName })}</MetaText>
                  </Row>
                ) : null}
                {autoRunning ? <MetaText>{t("multiAgent.autoRunning")}</MetaText> : null}
                {stopPending ? <MetaText>{t("multiAgent.stopPending")}</MetaText> : null}
                {memorySavedTitle ? <MetaText>{t("multiAgent.saveMemorySaved", { title: memorySavedTitle })}</MetaText> : null}
                {memorySaveBlocked ? <MetaText style={{ opacity: 0.68 }}>{t("multiAgent.temporaryNoSave")}</MetaText> : null}
                <MetaText style={{ opacity: 0.68 }}>{t("multiAgent.autoBoundaryNote")}</MetaText>
                {manualRule ? <MetaText style={{ opacity: 0.68 }}>{t("multiAgent.autoManualNote")}</MetaText> : null}
                {roster.length < 2 ? (
                  <MetaText style={{ opacity: 0.68 }}>
                    {t("multiAgent.needTwoParticipants", { count: roster.length })}
                  </MetaText>
                ) : null}
                {manualRule && !nomineeId ? <MetaText style={{ opacity: 0.68 }}>{t("multiAgent.nomineeRequired")}</MetaText> : null}
                <MetaText style={{ opacity: 0.68 }}>{t("multiAgent.reloadHint")}</MetaText>
              </Stack>

              <Textarea
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                placeholder={t("multiAgent.intervenePlaceholder")}
                style={{ minHeight: 72, resize: "none" }}
              />
              <Row style={{ justifyContent: "flex-end" }}>
                <Button type="submit" disabled={posting || !draft.trim()}>
                  {t("multiAgent.intervene")}
                </Button>
              </Row>
            </Stack>
          </ComposerBox>
        </Composer>
      </MainPane>

      {/* Hidden rather than unmounted: the panel holds unsaved edits (display name,
          role prompt, endpoint, model, scene), and collapsing must not throw away
          an edit in progress. display:none also takes it out of the grid, so the
          transcript gets the full width. */}
      <InspectorPane style={isRosterCollapsed ? { display: "none" } : undefined}>
        <ParticipantPanel
          chat={state.chat}
          participants={participants}
          canApplyPreset={state.messages.length === 0}
          onChatChange={(chat) => setState((current) => (current ? { ...current, chat } : current))}
          onParticipantsChange={setParticipants}
          disabled={autoRunning || runningSpeaker !== null}
        />
      </InspectorPane>

      {memoryDraft ? (
        <ModalOverlay>
          <ModalCard as="form" role="dialog" aria-modal="true" onSubmit={handleSaveMemory} style={{ width: "min(640px, 100%)" }}>
            <Stack>
              <SectionTitle>{t("multiAgent.saveMemoryTitle")}</SectionTitle>
              <Subtle>{t("multiAgent.saveMemoryFrom", { name: speakerLabel(memoryDraft.message) })}</Subtle>
              <Field>
                {t("project.kind")}
                <Select
                  value={memoryDraft.kind}
                  onChange={(event) =>
                    setMemoryDraft((current) => (current ? { ...current, kind: event.target.value as MemoryKind } : current))
                  }
                >
                  <option value="semantic">{t("project.kindSemantic")}</option>
                  <option value="procedural">{t("project.kindProcedural")}</option>
                  <option value="episodic">{t("project.kindEpisodic")}</option>
                </Select>
              </Field>
              <Field>
                {t("project.content")}
                <Textarea
                  value={memoryDraft.content}
                  onChange={(event) =>
                    setMemoryDraft((current) => (current ? { ...current, content: event.target.value } : current))
                  }
                  style={{ minHeight: 160 }}
                />
              </Field>
              <label style={{ display: "flex", alignItems: "center", gap: 10 }}>
                <input
                  type="checkbox"
                  checked={memoryDraft.locked}
                  onChange={(event) =>
                    setMemoryDraft((current) => (current ? { ...current, locked: event.target.checked } : current))
                  }
                />
                <span>{t("project.lockHint")}</span>
              </label>
              <MetaText style={{ opacity: 0.68 }}>{t("multiAgent.saveMemoryNote")}</MetaText>
              {memoryError ? <ErrorText>{memoryError}</ErrorText> : null}
              <Row style={{ justifyContent: "flex-end" }}>
                <Button type="button" variant="ghost" onClick={() => setMemoryDraft(null)} disabled={memorySaving}>
                  {t("common.cancel")}
                </Button>
                <Button type="submit" disabled={memorySaving || !memoryDraft.content.trim()}>
                  {memorySaving ? t("multiAgent.saveMemorySaving") : t("multiAgent.saveMemoryConfirm")}
                </Button>
              </Row>
            </Stack>
          </ModalCard>
        </ModalOverlay>
      ) : null}
    </WorkspaceShell>
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

function ReloadIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M13 8a5 5 0 1 1-1.6-3.66M13 3v2.5h-2.5"
        stroke="currentColor"
        strokeWidth="1.4"
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
