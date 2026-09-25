import { FormEvent, UIEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { api, ApiError, ChatRecord, ChatSummary, MemoryKind, MessageRecord, Participant, Project, TurnRule } from "../api/client";
import { streamSSE } from "../api/sse";
import { ActionButton } from "../components/ActionButton";
import { Checkbox } from "../components/Checkbox";
import { CopyMessageButton } from "../components/CopyMessageButton";
import { Dialog, DialogTitle } from "../components/Dialog";
import { ExportChatButton } from "../components/ExportChatButton";
import { FailureNotice } from "../components/FailureNotice";
import {
  CheckIcon,
  MemoryStickIcon,
  PanelRightCloseIcon,
  PanelRightOpenIcon,
  PlayIcon,
  RotateCwIcon,
  SendIcon,
  SpinnerIcon,
  SquareIcon,
  StepForwardIcon,
  SummaryIcon
} from "../components/icons";
import { MarkdownPreview } from "../components/MarkdownPreview";
import { MessageReferences } from "../components/MessageReferences";
import { ParticipantPanel } from "../components/ParticipantPanel";
import { useSideRegion } from "../components/useSideRegion";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import { MessageKey, useLanguage } from "../i18n";
import {
  Badge,
  Card,
  Composer,
  ComposerBox,
  Field,
  FloatingScrollButton,
  InspectorPane,
  MainPane,
  MessageArea,
  MessageBubble,
  MessageScroller,
  MetaText,
  PaneHeader,
  RegionToggleButton,
  Row,
  SectionTitle,
  Select,
  Stack,
  Subtle,
  Summary,
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

// The 24px bare icon buttons of a message's action row, shared with the copy button.
const MESSAGE_ACTION_STYLE = { width: 24, height: 24, border: "none", background: "transparent", padding: 0 } as const;

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
  // Each failure is told next to what failed (snz-design doc-9 §5.5): the
  // transcript's load at its top, the export under the header, a turn or an
  // intervention in the composer, a copy or a memory draft in its message.
  const [error, setError] = useState("");
  const [headerError, setHeaderError] = useState("");
  const [turnError, setTurnError] = useState("");
  const [interveneError, setInterveneError] = useState("");
  const [messageErrors, setMessageErrors] = useState<Record<string, string>>({});
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
  // The message whose save-to-memory draft is being prepared.
  const [memoryPreparingId, setMemoryPreparingId] = useState<string | null>(null);
  const [memorySaving, setMemorySaving] = useState(false);
  const [memoryError, setMemoryError] = useState("");
  const [memorySavedTitle, setMemorySavedTitle] = useState("");
  // Where the conclusion draft was asked from: the header, or one message's
  // "from here". Only that button shows the busy figure.
  const [concludingFrom, setConcludingFrom] = useState<string | null>(null);
  const [conclusionError, setConclusionError] = useState("");
  // The button that opened the save dialog. The dialog opens only after the
  // server's draft arrives, so by then focus is no longer reliably on it.
  const memoryOpenerRef = useRef<HTMLButtonElement | null>(null);
  const rosterRegion = useSideRegion(ROSTER_STORAGE_KEY, t("participants.title"));

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
    setInterveneError("");
    try {
      const response = await api.sendMessage(chatId, content);
      setState((current) => (current ? { ...current, chat: response.chat, messages: response.messages } : current));
      setDraft("");
    } catch (nextError) {
      setInterveneError(nextError instanceof Error ? nextError.message : t("multiAgent.interveneError"));
    } finally {
      setPosting(false);
    }
  }

  // The dialog opens on the server's draft rather than on the message alone
  // because the default kind comes from the extraction rules, which live only
  // on the server (design §4.4).
  async function handleOpenMemoryDialog(message: MessageRecord, opener: HTMLButtonElement) {
    memoryOpenerRef.current = opener;
    setMemoryPreparingId(message.id);
    setMemoryError("");
    setMessageErrors((current) => ({ ...current, [message.id]: "" }));
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
      const text = nextError instanceof Error ? nextError.message : t("multiAgent.saveMemoryDraftError");
      setMessageErrors((current) => ({ ...current, [message.id]: text }));
    } finally {
      setMemoryPreparingId(null);
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
    setConcludingFrom(fromMessageId ?? "header");
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
      setConcludingFrom(null);
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
    return <Card>{error ? <FailureNotice>{error}</FailureNotice> : t("multiAgent.notFound")}</Card>;
  }

  const stopPending = !autoRunning && turnRunning;
  const chatTitle = state.chat.title.trim() || t("sidebar.untitled");
  // A temporary multi-agent chat reads project material but never writes it
  // back, so the one write path is closed while the flag is on (design §4.4).
  const memorySaveBlocked = state.chat.isTemporary;
  // One draft is prepared at a time; the button that asked shows it is busy and
  // the others say why they wait.
  const draftBusy = Boolean(memoryPreparingId || concludingFrom || memorySaving);
  const draftBusyReason = draftBusy ? t("multiAgent.draftBusy") : undefined;
  const turnBlockedReason = !turnBlocked
    ? undefined
    : roster.length < 2
      ? t("multiAgent.needTwoParticipants", { count: roster.length })
      : t("multiAgent.nomineeRequired");
  const advanceReason = autoRunning ? t("multiAgent.autoRunning") : turnRunning ? undefined : turnBlockedReason;
  const autoReason = autoRunning
    ? undefined
    : manualRule
      ? t("multiAgent.autoManualNote")
      : turnRunning
        ? t("multiAgent.turnRunningReason")
        : turnBlockedReason;
  const lockedReason = autoRunning
    ? t("multiAgent.lockedAuto")
    : turnRunning
      ? t("multiAgent.lockedTurn")
      : undefined;

  return (
    <WorkspaceShell $side={rosterRegion.shown}>
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
            <ActionButton
              type="button"
              iconOnly
              aria-label={t("multiAgent.conclude")}
              title={t("multiAgent.concludeTitle")}
              busy={concludingFrom === "header"}
              disabledReason={
                concludingFrom === "header"
                  ? undefined
                  : (draftBusyReason ?? (state.messages.length === 0 ? t("multiAgent.concludeEmpty") : undefined))
              }
              onClick={(event) => void handleDraftConclusion(event.currentTarget)}
            >
              <SummaryIcon />
            </ActionButton>
            <ExportChatButton chatId={state.chat.id} chatTitle={state.chat.title} onError={setHeaderError} />
            <ActionButton
              type="button"
              iconOnly
              aria-label={t("multiAgent.reload")}
              title={t("multiAgent.reload")}
              onClick={() => void load()}
            >
              <RotateCwIcon />
            </ActionButton>
            <RegionToggleButton
              type="button"
              aria-label={t("participants.title")}
              title={rosterRegion.shown ? t("multiAgent.hideRoster") : t("multiAgent.showRoster")}
              {...rosterRegion.triggerProps}
            >
              {rosterRegion.shown ? <PanelRightCloseIcon /> : <PanelRightOpenIcon />}
            </RegionToggleButton>
          </Row>
        </PaneHeader>

        {headerError ? (
          <div style={{ padding: "12px 20px 0" }}>
            <FailureNotice>{headerError}</FailureNotice>
          </div>
        ) : null}

        <MessageArea>
          <MessageScroller ref={messageScrollerRef} onScroll={handleMessageScroll}>
            <Stack>
              {error ? <FailureNotice>{error}</FailureNotice> : null}
              {state.messages.length === 0 && !turnRunning ? <Subtle>{t("multiAgent.spectatorEmpty")}</Subtle> : null}

              {state.messages.map((message) => (
                <MessageBubble key={message.id} $role={message.role}>
                  <Stack>
                    <Row style={{ justifyContent: "space-between", alignItems: "baseline", gap: 12 }}>
                      <strong style={{ overflowWrap: "anywhere" }}>{speakerLabel(message)}</strong>
                      <MetaText style={{ whiteSpace: "nowrap" }}>{message.modelName ?? ""}</MetaText>
                    </Row>
                    {message.role === "assistant" ? (
                      <MarkdownPreview source={message.content} />
                    ) : (
                      <div style={{ whiteSpace: "pre-wrap", lineHeight: 1.65 }}>{message.content}</div>
                    )}
                    <MessageReferences references={message.references} />
                    <Row style={{ justifyContent: "flex-end", alignItems: "center", gap: 10, flexWrap: "nowrap" }}>
                      <CopyMessageButton
                        content={message.content}
                        onError={(next) => setMessageErrors((current) => ({ ...current, [message.id]: next }))}
                      />
                      <ActionButton
                        type="button"
                        iconOnly
                        aria-label={t("multiAgent.concludeFromHere")}
                        title={t("multiAgent.concludeFromHere")}
                        busy={concludingFrom === message.id}
                        disabledReason={concludingFrom === message.id ? undefined : draftBusyReason}
                        onClick={(event) => void handleDraftConclusion(event.currentTarget, message.id)}
                        style={MESSAGE_ACTION_STYLE}
                      >
                        <SummaryIcon />
                      </ActionButton>
                      {/* Same bare 24px icon button as the single-assistant page's review and
                          copy actions, so the per-message actions read alike across chat kinds. */}
                      <ActionButton
                        type="button"
                        iconOnly
                        aria-label={t("multiAgent.saveMemory")}
                        title={t("multiAgent.saveMemoryTitle")}
                        busy={memoryPreparingId === message.id}
                        disabledReason={
                          memoryPreparingId === message.id
                            ? undefined
                            : memorySaveBlocked
                              ? t("multiAgent.temporaryNoSave")
                              : draftBusyReason
                        }
                        onClick={(event) => void handleOpenMemoryDialog(message, event.currentTarget)}
                        style={MESSAGE_ACTION_STYLE}
                      >
                        <MemoryStickIcon />
                      </ActionButton>
                      <MetaText style={{ whiteSpace: "nowrap" }}>{new Date(message.createdAt).toLocaleTimeString()}</MetaText>
                    </Row>
                    {messageErrors[message.id] ? <FailureNotice>{messageErrors[message.id]}</FailureNotice> : null}
                  </Stack>
                </MessageBubble>
              ))}

              {/* The utterance being generated is drawn in the same bubble, under its
                  speaker's name, as the ones already stored (snz-design doc-4 §5.3). */}
              {turnRunning ? (
                <MessageBubble $role="assistant" aria-busy="true">
                  <Stack>
                    <Row style={{ justifyContent: "space-between", alignItems: "baseline", gap: 12 }}>
                      <strong style={{ overflowWrap: "anywhere" }}>
                        {runningSpeaker?.participant.displayName ?? t("multiAgent.runningTurnUnknown")}
                      </strong>
                      <MetaText style={{ whiteSpace: "nowrap" }}>{runningSpeaker?.modelName ?? ""}</MetaText>
                    </Row>
                    {runningSpeaker && runningSpeaker.weights.length > 0 ? (
                      <details>
                        <Summary>
                          <MetaText as="span">{t("multiAgent.speakerWeights")}</MetaText>
                        </Summary>
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
                        <SpinnerIcon size={14} />
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
                <ActionButton
                  type="button"
                  icon={<StepForwardIcon />}
                  busy={turnRunning && !autoRunning}
                  disabledReason={advanceReason}
                  onClick={() => void handleAdvanceTurn()}
                >
                  {t("multiAgent.advanceTurn")}
                </ActionButton>
                <ActionButton
                  type="button"
                  variant="normal"
                  icon={autoRunning ? <SquareIcon /> : <PlayIcon />}
                  disabledReason={autoReason}
                  onClick={() => void handleToggleAutoRun()}
                >
                  {autoRunning ? t("multiAgent.autoStop") : t("multiAgent.autoStart")}
                </ActionButton>
                {manualRule ? (
                  <Select
                    value={nomineeId}
                    aria-label={t("multiAgent.nominee")}
                    onChange={(event) => setNomineeId(event.target.value)}
                    style={{ minWidth: 200, width: "auto" }}
                  >
                    <option value="">{t("multiAgent.nominee")}</option>
                    {roster.map((participant) => (
                      <option key={participant.id} value={participant.id}>
                        {participant.displayName}
                      </option>
                    ))}
                  </Select>
                ) : null}
              </Row>

              {turnError ? <FailureNotice>{turnError}</FailureNotice> : null}

              <Stack style={{ gap: 4 }}>
                {turnRunning ? (
                  <Row style={{ alignItems: "center", gap: 8 }}>
                    <SpinnerIcon size={14} />
                    <MetaText>
                      {runningSpeaker
                        ? t("multiAgent.runningTurn", { name: runningSpeaker.participant.displayName })
                        : t("multiAgent.runningTurnUnknown")}
                    </MetaText>
                  </Row>
                ) : null}
                {autoRunning ? <MetaText>{t("multiAgent.autoRunning")}</MetaText> : null}
                {stopPending ? <MetaText>{t("multiAgent.stopPending")}</MetaText> : null}
                {concludingFrom ? (
                  <Row style={{ alignItems: "center", gap: 8 }}>
                    <SpinnerIcon size={14} />
                    <MetaText>{t("multiAgent.concluding")}</MetaText>
                  </Row>
                ) : null}
                {conclusionError ? <FailureNotice>{conclusionError}</FailureNotice> : null}
                {memorySavedTitle ? <MetaText>{t("multiAgent.saveMemorySaved", { title: memorySavedTitle })}</MetaText> : null}
                {memorySaveBlocked ? <MetaText>{t("multiAgent.temporaryNoSave")}</MetaText> : null}
                <MetaText>{t("multiAgent.autoBoundaryNote")}</MetaText>
                {manualRule ? <MetaText>{t("multiAgent.autoManualNote")}</MetaText> : null}
                {roster.length < 2 ? (
                  <MetaText>{t("multiAgent.needTwoParticipants", { count: roster.length })}</MetaText>
                ) : null}
                {manualRule && !nomineeId ? <MetaText>{t("multiAgent.nomineeRequired")}</MetaText> : null}
                {facilitatorMissing ? <MetaText>{t("multiAgent.facilitatorMissing")}</MetaText> : null}
                <MetaText>{t("multiAgent.reloadHint")}</MetaText>
              </Stack>

              <Textarea
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                placeholder={t("multiAgent.intervenePlaceholder")}
                aria-label={t("multiAgent.intervene")}
                style={{ minHeight: 72, resize: "none" }}
              />
              {interveneError ? <FailureNotice>{interveneError}</FailureNotice> : null}
              <Row style={{ justifyContent: "flex-end" }}>
                <ActionButton
                  type="submit"
                  variant="normal"
                  icon={<SendIcon />}
                  busy={posting}
                  disabledReason={posting || draft.trim() ? undefined : t("chat.messageRequired")}
                >
                  {t("multiAgent.intervene")}
                </ActionButton>
              </Row>
            </Stack>
          </ComposerBox>
        </Composer>
      </MainPane>

      {/* Hidden rather than unmounted: the panel holds unsaved edits (display name,
          role prompt, endpoint, model, scene), and hiding it must not throw away
          an edit in progress. Hidden also takes it out of the grid, so the
          transcript gets the full width. */}
      <InspectorPane $toggled {...rosterRegion.regionProps} aria-label={t("participants.title")}>
        <ParticipantPanel
          chat={state.chat}
          participants={participants}
          canApplyPreset={state.messages.length === 0}
          onChatChange={(chat) => setState((current) => (current ? { ...current, chat } : current))}
          onParticipantsChange={setParticipants}
          lockedReason={lockedReason}
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
              <Checkbox
                checked={memoryDraft.locked}
                onChange={(locked) => setMemoryDraft((current) => (current ? { ...current, locked } : current))}
              >
                {t("project.lockHint")}
              </Checkbox>
              <MetaText>{t("multiAgent.saveMemoryNote")}</MetaText>
              {memorySaveBlocked ? <MetaText>{t("multiAgent.temporaryNoSave")}</MetaText> : null}
              {memoryError ? <FailureNotice>{memoryError}</FailureNotice> : null}
              <Row style={{ justifyContent: "flex-end" }}>
                <ActionButton
                  type="button"
                  variant="normal"
                  disabledReason={memorySaving ? t("project.savingClose") : undefined}
                  onClick={closeMemoryDialog}
                >
                  {t("common.cancel")}
                </ActionButton>
                <ActionButton
                  type="submit"
                  icon={<CheckIcon />}
                  busy={memorySaving}
                  disabledReason={
                    memorySaving
                      ? undefined
                      : memorySaveBlocked
                        ? t("multiAgent.temporaryNoSave")
                        : memoryDraft.content.trim()
                          ? undefined
                          : t("multiAgent.memoryContentRequired")
                  }
                >
                  {t("multiAgent.saveMemoryConfirm")}
                </ActionButton>
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
