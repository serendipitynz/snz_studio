import { FormEvent, UIEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { api, ApiError, ChatRecord, ChatSummary, MemoryKind, MessageRecord, Participant, Project, TurnRule } from "../api/client";
import { streamSSE } from "../api/sse";
import { CopyMessageButton } from "../components/CopyMessageButton";
import { Dialog, DialogTitle } from "../components/Dialog";
import { ExportChatButton } from "../components/ExportChatButton";
import { MarkdownPreview } from "../components/MarkdownPreview";
import { MessageReferences } from "../components/MessageReferences";
import { ParticipantPanel } from "../components/ParticipantPanel";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import { MessageKey, useLanguage } from "../i18n";
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
// What one turn needs to send: the rule, and the nomination the manual rule
// requires. Who speaks under the other rules is the server's to decide, and it
// says so in the `speaker` frame.
interface TurnInput {
  turnRule: TurnRule;
  nomineeId: string;
}

// The `speaker` frame: who the engine picked, before generation starts
// (design §4.6.6). weights is the calculation behind the pick for a rule that
// weighs the roster, and empty for the rules that go by position.
interface TurnSpeaker {
  participant: Participant;
  modelName: string;
  weights: SpeakerWeight[];
}

interface SpeakerWeight {
  participantId: string;
  displayName?: string;
  weight: number;
  factors: { name: string; value: number }[];
}

interface TurnDonePayload {
  chat?: ChatRecord;
  message?: MessageRecord;
  messages?: MessageRecord[];
  participants?: Participant[];
}

// The save dialog's contents: where the draft came from and what the user has
// made of it so far (design §4.4). A conclusion draft saves through the last
// utterance of its range, since the save route only needs the message's chat.
interface MemoryDraft {
  origin: { type: "message"; message: MessageRecord } | { type: "conclusion"; messageCount: number };
  anchorMessageId: string;
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
  const factorLabel = (name: string) => (name in factorLabelKeys ? t(factorLabelKeys[name]) : name);
  const messageScrollerRef = useRef<HTMLDivElement | null>(null);
  const [state, setState] = useState<MultiAgentState | null>(null);
  const [participants, setParticipants] = useState<Participant[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectChats, setProjectChats] = useState<ChatRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [turnError, setTurnError] = useState("");
  // A turn is in flight from the request until its stream ends, but its speaker
  // is known only once the `speaker` frame arrives, so the two are kept apart.
  const [turnRunning, setTurnRunning] = useState(false);
  const [runningSpeaker, setRunningSpeaker] = useState<TurnSpeaker | null>(null);
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
  const [concluding, setConcluding] = useState(false);
  const [conclusionError, setConclusionError] = useState("");
  // The button that opened the save dialog. The dialog opens only after the
  // server's draft arrives, so by then focus is no longer reliably on it.
  const memoryOpenerRef = useRef<HTMLButtonElement | null>(null);
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
  // The rule is the chat's standing setting, so a removed facilitator leaves it
  // selected with nobody to interleave. The engine falls back to roster order
  // rather than refusing the turn (design §4.2 step 1), which is invisible
  // unless the view says so.
  const facilitatorMissing =
    state?.chat.turnRule === "facilitator_alternating" &&
    !roster.some((participant) => participant.id === state?.chat.facilitatorId);
  // A multi-agent chat is one with two or more participants (design §2), and the
  // controls hold that line rather than assuming it: on a roster of one,
  // round_robin re-selects the same speaker every turn — (0 + 1) % 1 — so
  // auto-advance would be one model answering itself until someone stops it.
  const turnBlocked = roster.length < 2 || (manualRule && !nomineeId);

  const currentTurnInput = (): TurnInput => ({
    turnRule: state?.chat.turnRule ?? "round_robin",
    nomineeId
  });

  // runTurn takes what the turn needs and hands back what the next turn needs,
  // instead of reading either from the component's scope. The auto-advance loop
  // awaits every turn inside one closure, so a turn reading the scope would see
  // the values of the render the loop started in — a rule changed mid-run would
  // not reach the next request. Threading the `done` frame's own chat through
  // also keeps the loop on what the server actually stored rather than on
  // whether React has re-rendered yet.
  async function runTurn(input: TurnInput): Promise<TurnInput | null> {
    if (turnInFlightRef.current) {
      return null;
    }

    turnInFlightRef.current = true;
    setTurnError("");
    setStreamedContent("");
    setRunningSpeaker(null);
    setTurnRunning(true);

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
          if (event === "speaker") {
            const speaker = payload as unknown as TurnSpeaker;
            // A frame missing weights or factors must drop them, not break the render.
            setRunningSpeaker({
              ...speaker,
              weights: (speaker.weights ?? []).map((entry) => ({ ...entry, factors: entry.factors ?? [] }))
            });
            return;
          }

          if (event === "delta") {
            const delta = typeof payload.content === "string" ? payload.content : "";
            if (delta) {
              setStreamedContent((current) => `${current}${delta}`);
            }
            return;
          }

          if (event === "done") {
            const done = payload as TurnDonePayload;
            next = {
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
      setTurnRunning(false);
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
  async function handleOpenMemoryDialog(message: MessageRecord, opener: HTMLButtonElement) {
    memoryOpenerRef.current = opener;
    setMemoryPreparing(true);
    setMemoryError("");
    try {
      const response = await api.getMessageMemoryDraft(message.id);
      openMemoryDraft({
        origin: { type: "message", message },
        anchorMessageId: message.id,
        content: response.draft.content,
        kind: response.draft.kind,
        locked: true
      });
    } catch (nextError) {
      setMemoryError(nextError instanceof Error ? nextError.message : t("multiAgent.saveMemoryDraftError"));
    } finally {
      setMemoryPreparing(false);
    }
  }

  // Both draft flows await the server before opening the one dialog, so a
  // response arriving after the other flow already opened it must not replace
  // what the user is editing there. The buttons also exclude each other while
  // a draft is being prepared; this covers what slips past that.
  function openMemoryDraft(next: MemoryDraft) {
    setMemoryDraft((current) => current ?? next);
  }

  // A conclusion draft opens the same save dialog, so it is generated even in a
  // temporary chat — only the save stays closed there (design §4.4). A range
  // over the limit is refused by the server rather than clipped, and the view
  // turns that into asking for a later starting utterance.
  async function handleDraftConclusion(opener: HTMLButtonElement, fromMessageId?: string) {
    memoryOpenerRef.current = opener;
    setConcluding(true);
    setConclusionError("");
    try {
      const response = await api.draftConclusion(chatId, fromMessageId);
      setMemoryError("");
      openMemoryDraft({
        origin: { type: "conclusion", messageCount: response.messageCount },
        anchorMessageId: response.anchorMessageId,
        content: response.draft.content,
        kind: response.draft.kind,
        locked: true
      });
    } catch (nextError) {
      if (nextError instanceof ApiError && nextError.status === 422 && nextError.body) {
        setConclusionError(
          t("multiAgent.concludeTooLong", { chars: Number(nextError.body.chars), limit: Number(nextError.body.limit) })
        );
      } else {
        setConclusionError(nextError instanceof Error ? nextError.message : t("multiAgent.concludeError"));
      }
    } finally {
      setConcluding(false);
    }
  }

  // Escape follows the cancel button, which is disabled while the save is in
  // flight: closing then would hide an error the save might still report.
  function closeMemoryDialog() {
    if (!memorySaving) {
      setMemoryDraft(null);
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
      const response = await api.saveMessageMemory(memoryDraft.anchorMessageId, {
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

  const stopPending = !autoRunning && turnRunning;
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
            <Button
              type="button"
              variant="ghost"
              title={t("multiAgent.concludeTitle")}
              disabled={concluding || memoryPreparing || memorySaving || state.messages.length === 0}
              onClick={(event) => void handleDraftConclusion(event.currentTarget)}
            >
              {t("multiAgent.conclude")}
            </Button>
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
              {state.messages.length === 0 && !turnRunning ? <Subtle>{t("multiAgent.spectatorEmpty")}</Subtle> : null}

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
                    <Row style={{ justifyContent: "flex-end", alignItems: "center", gap: 10, flexWrap: "nowrap" }}>
                      <CopyMessageButton content={message.content} onError={setError} />
                      <IconButton
                        type="button"
                        aria-label={t("multiAgent.concludeFromHere")}
                        title={t("multiAgent.concludeFromHere")}
                        disabled={concluding || memoryPreparing || memorySaving}
                        onClick={(event) => void handleDraftConclusion(event.currentTarget, message.id)}
                        style={{ width: 24, height: 24, border: "none", background: "transparent", padding: 0, opacity: 0.82 }}
                      >
                        <ConcludeFromHereIcon />
                      </IconButton>
                      {/* Same bare 24px icon button as the single-assistant page's review and
                          copy actions, so the per-message actions read alike across chat kinds. */}
                      <IconButton
                        type="button"
                        aria-label={t("multiAgent.saveMemory")}
                        title={memorySaveBlocked ? t("multiAgent.temporaryNoSave") : t("multiAgent.saveMemoryTitle")}
                        disabled={memorySaveBlocked || memoryPreparing || memorySaving || concluding}
                        onClick={(event) => void handleOpenMemoryDialog(message, event.currentTarget)}
                        style={{
                          width: 24,
                          height: 24,
                          border: "none",
                          background: "transparent",
                          padding: 0,
                          opacity: memorySaveBlocked ? 0.4 : 0.82
                        }}
                      >
                        <MemoryStickIcon />
                      </IconButton>
                      <MetaText style={{ whiteSpace: "nowrap", opacity: 0.68 }}>
                        {new Date(message.createdAt).toLocaleTimeString()}
                      </MetaText>
                    </Row>
                  </Stack>
                </MessageBubble>
              ))}

              {turnRunning ? (
                <MessageBubble $role="assistant">
                  <Stack>
                    <Row style={{ justifyContent: "space-between", alignItems: "baseline", gap: 12 }}>
                      <strong style={{ overflowWrap: "anywhere" }}>
                        {runningSpeaker?.participant.displayName ?? t("multiAgent.runningTurnUnknown")}
                      </strong>
                      <MetaText style={{ whiteSpace: "nowrap", opacity: 0.68 }}>
                        {runningSpeaker?.modelName ?? ""}
                      </MetaText>
                    </Row>
                    {runningSpeaker && runningSpeaker.weights.length > 0 ? (
                      <details>
                        <summary>
                          <MetaText as="span">{t("multiAgent.speakerWeights")}</MetaText>
                        </summary>
                        <Stack style={{ gap: 2, marginTop: 4 }}>
                          {runningSpeaker.weights.map((entry) => (
                            <MetaText key={entry.participantId}>
                              {speakerById.get(entry.participantId)?.displayName ?? entry.displayName ?? entry.participantId}:{" "}
                              {formatWeight(entry.weight)}
                              {entry.factors.length > 0
                                ? ` = ${entry.factors.map((factor) => `${factorLabel(factor.name)} ×${formatWeight(factor.value)}`).join(" · ")}`
                                : ""}
                            </MetaText>
                          ))}
                        </Stack>
                      </details>
                    ) : null}
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
                <Button type="button" onClick={() => void handleAdvanceTurn()} disabled={autoRunning || turnRunning || turnBlocked}>
                  {t("multiAgent.advanceTurn")}
                </Button>
                <Button
                  type="button"
                  variant={autoRunning ? "warm" : "solid"}
                  onClick={() => void handleToggleAutoRun()}
                  disabled={manualRule || (!autoRunning && (turnRunning || turnBlocked))}
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
                {turnRunning ? (
                  <Row style={{ alignItems: "center", gap: 8 }}>
                    <SpinnerIcon />
                    <MetaText>
                      {runningSpeaker
                        ? t("multiAgent.runningTurn", { name: runningSpeaker.participant.displayName })
                        : t("multiAgent.runningTurnUnknown")}
                    </MetaText>
                  </Row>
                ) : null}
                {autoRunning ? <MetaText>{t("multiAgent.autoRunning")}</MetaText> : null}
                {stopPending ? <MetaText>{t("multiAgent.stopPending")}</MetaText> : null}
                {concluding ? (
                  <Row style={{ alignItems: "center", gap: 8 }}>
                    <SpinnerIcon />
                    <MetaText>{t("multiAgent.concluding")}</MetaText>
                  </Row>
                ) : null}
                {conclusionError ? <ErrorText>{conclusionError}</ErrorText> : null}
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
                {facilitatorMissing ? (
                  <MetaText style={{ opacity: 0.68 }}>{t("multiAgent.facilitatorMissing")}</MetaText>
                ) : null}
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
          disabled={autoRunning || turnRunning}
        />
      </InspectorPane>

      {memoryDraft ? (
        <Dialog
          onClose={closeMemoryDialog}
          returnFocusTo={memoryOpenerRef.current}
          style={{ width: "min(640px, 100%)" }}
        >
          <form onSubmit={handleSaveMemory}>
            <Stack>
              {memoryDraft.origin.type === "message" ? (
                <>
                  <DialogTitle>{t("multiAgent.saveMemoryTitle")}</DialogTitle>
                  <Subtle>{t("multiAgent.saveMemoryFrom", { name: speakerLabel(memoryDraft.origin.message) })}</Subtle>
                </>
              ) : (
                <>
                  <DialogTitle>{t("multiAgent.concludeDialogTitle")}</DialogTitle>
                  <Subtle>{t("multiAgent.concludeFrom", { count: memoryDraft.origin.messageCount })}</Subtle>
                </>
              )}
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
                  autoFocus
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
              {memorySaveBlocked ? <MetaText>{t("multiAgent.temporaryNoSave")}</MetaText> : null}
              {memoryError ? <ErrorText>{memoryError}</ErrorText> : null}
              <Row style={{ justifyContent: "flex-end" }}>
                <Button type="button" variant="ghost" onClick={closeMemoryDialog} disabled={memorySaving}>
                  {t("common.cancel")}
                </Button>
                <Button type="submit" disabled={memorySaveBlocked || memorySaving || !memoryDraft.content.trim()}>
                  {memorySaving ? t("multiAgent.saveMemorySaving") : t("multiAgent.saveMemoryConfirm")}
                </Button>
              </Row>
            </Stack>
          </form>
        </Dialog>
      ) : null}
    </WorkspaceShell>
  );
}

// The weighted rule's factor names (design §4.6.1) as the server sends them. A
// name this list does not know is shown as sent rather than dropped.
const factorLabelKeys: Record<string, MessageKey> = {
  consecutive: "multiAgent.factorConsecutive",
  recent: "multiAgent.factorRecent",
  call: "multiAgent.factorCall"
};

function formatWeight(value: number): string {
  return String(Number(value.toFixed(3)));
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

// Lucide "memory-stick" (ISC, see THIRD_PARTY_NOTICES.md), sized to the 16px
// grid the other per-message icons use.
function MemoryStickIcon() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M12 12v-2" />
      <path d="M12 18v-2" />
      <path d="M16 12v-2" />
      <path d="M16 18v-2" />
      <path d="M2 11h1.5" />
      <path d="M20 18v-2" />
      <path d="M20.5 11H22" />
      <path d="M4 18v-2" />
      <path d="M8 12v-2" />
      <path d="M8 18v-2" />
      <rect x="2" y="6" width="20" height="10" rx="2" />
    </svg>
  );
}

// Three lines ending in a check: a conclusion drawn from this message onward.
function ConcludeFromHereIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M2.75 4h10.5M2.75 8h6.5M2.75 12h4" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
      <path d="m9.5 11.5 1.75 1.75 3-3.25" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
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
