import { FormEvent, UIEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { api, ApiError, ChatRecord, ChatSummary, MemoryKind, MessageRecord, Participant, Project, TurnRule } from "../api/client";
import { streamSSE } from "../api/sse";
import { ActionButton } from "../components/ActionButton";
import { Checkbox } from "../components/Checkbox";
import { CopyMessageButton } from "../components/CopyMessageButton";
import { DiceRollChips, messageCopyText } from "../components/DiceRollChips";
import { Dialog, DialogActions, DialogBody, DialogForm, DialogHeader, DialogTitle } from "../components/Dialog";
import { ExportChatButton } from "../components/ExportChatButton";
import { ComposerTextarea } from "../components/ComposerTextarea";
import { FailureNotice } from "../components/FailureNotice";
import { GuardedSelect } from "../components/GuardedSelect";
import { Hint } from "../components/Hint";
import {
  CheckIcon,
  MemoryStickIcon,
  PanelRightCloseIcon,
  PanelRightOpenIcon,
  PlayIcon,
  RotateCwFadingClockIcon,
  RotateCwIcon,
  SendIcon,
  SpinnerIcon,
  SquareIcon,
  StepForwardIcon,
  SummaryIcon,
  UserGroupIcon
} from "../components/icons";
import { MarkdownPreview } from "../components/MarkdownPreview";
import { MessageReferences } from "../components/MessageReferences";
import { ParticipantPanel } from "../components/ParticipantPanel";
import { useSideRegion } from "../components/useSideRegion";
import { WorkspaceSidebar } from "../components/WorkspaceSidebar";
import { MessageKey, useLanguage } from "../i18n";
import { snzTokens } from "../styles/themes/snz-tokens";
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
  RegionCloseButton,
  RegionToggleButton,
  Row,
  SectionTitle,
  Select,
  Stack,
  StateBadge,
  Subtle,
  Summary,
  Textarea,
  VisuallyHidden,
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
  // Who follows this turn, for the rules that go by position; null otherwise.
  next: Participant | null;
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

// The row's buttons keep their size and take a narrower inline padding than the
// shared button, so the row holds one line about as far as the single chat's.
// nowrap because CJK words break between any two characters: the group's
// automatic minimum would otherwise count "進める" as one character wide and let
// the row lay the group out narrower than its buttons draw.
const ROW_BUTTON_STYLE = { flexShrink: 0, paddingInline: 12, whiteSpace: "nowrap" } as const;

// While the busy figure shows, the select's end padding makes room for it left of
// the chevron (the Select's own padding covers the chevron alone). The select's
// width comes from the row, so the padding changing does not move anything.
const SPEAKER_SELECT_BUSY_END_PADDING = `calc(${snzTokens.icon.sizeMd} + 2 * ${snzTokens.space.sm} + 14px + ${snzTokens.space.xs})`;
const SPEAKER_SPINNER_STYLE = {
  position: "absolute",
  insetInlineEnd: `calc(${snzTokens.icon.sizeMd} + 2 * ${snzTokens.space.sm})`,
  top: "50%",
  transform: "translateY(-50%)",
  display: "inline-flex",
  pointerEvents: "none"
} as const;

// The one slash command the composer suggests (design §4.8.3 item 3).
const ROLL_KEYWORD = "/roll";

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
  const [stopRequested, setStopRequested] = useState(false);
  // The engine's pick for the next turn and the number of past messages a turn
  // reads, both from the server so the view keeps no copy of the rule.
  const [nextSpeaker, setNextSpeaker] = useState<Participant | null>(null);
  const [historyLimit, setHistoryLimit] = useState<number | null>(null);
  const nextRequestRef = useRef(0);
  // The running turn's announced pick, kept in a ref because the intervention
  // that voids it happens outside the turn's own closure.
  const announcedNextRef = useRef<Participant | null>(null);
  // The number the running turn will have once stored. Fixed when the turn
  // starts: counting stored turns plus one while running would briefly show one
  // too many between the done frame's transcript and the turn's end.
  const [runningTurnNumber, setRunningTurnNumber] = useState(0);
  const completedTurnsRef = useRef(0);
  const [nomineeId, setNomineeId] = useState("");
  const [draft, setDraft] = useState("");
  const composerRef = useRef<HTMLTextAreaElement | null>(null);
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
  const rosterRegion = useSideRegion(ROSTER_STORAGE_KEY, t("multiAgent.inspector"));

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

  const completedTurns = state?.messages.filter((message) => message.participantId).length ?? 0;
  completedTurnsRef.current = completedTurns;
  const turnNumber = turnRunning ? runningTurnNumber : completedTurns;

  // Re-read whenever something the pick depends on has changed, and not while a
  // turn runs: the speaker frame reports the pick then. A failed read leaves the
  // select on its prompt; the turn itself still picks and reports its speaker.
  const loaded = state !== null;
  const turnRule = state?.chat.turnRule;
  const facilitatorId = state?.chat.facilitatorId;
  const messageCount = state?.messages.length ?? 0;
  const rosterKey = roster.map((participant) => participant.id).join(",");
  useEffect(() => {
    if (!loaded || turnRunning) {
      return;
    }
    const request = ++nextRequestRef.current;
    api
      .getNextSpeaker(chatId)
      .then((response) => {
        if (request === nextRequestRef.current) {
          setNextSpeaker(response.participant);
          setHistoryLimit(response.historyLimit);
        }
      })
      .catch(() => {
        if (request === nextRequestRef.current) {
          setNextSpeaker(null);
        }
      });
  }, [chatId, loaded, turnRunning, turnRule, facilitatorId, messageCount, rosterKey]);

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
    setRunningTurnNumber(completedTurnsRef.current + 1);
    announcedNextRef.current = null;
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
            announcedNextRef.current = speaker.next ?? null;
            // A frame missing weights or factors must drop them, not break the render.
            setRunningSpeaker({
              ...speaker,
              next: speaker.next ?? null,
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
            // Until the re-read lands, the pick the turn announced stands in for
            // it. An auto-advance loop starts the next turn before then, and the
            // pick from before this turn would name the wrong speaker meanwhile.
            nextRequestRef.current += 1;
            setNextSpeaker(announcedNextRef.current);
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
      setStopRequested(true);
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
      setStopRequested(false);
    }
  }

  async function handleIntervene(event: FormEvent) {
    event.preventDefault();
    const content = draft.trim();
    if (!content || posting) {
      return;
    }

    setPosting(true);
    setInterveneError("");
    try {
      const response = await api.sendMessage(chatId, content);
      setState((current) => (current ? { ...current, chat: response.chat, messages: response.messages } : current));
      setDraft("");
      // An intervention can change who follows the running turn (the facilitator
      // answers it), so the pick that turn announced no longer holds.
      announcedNextRef.current = null;
      setRunningSpeaker((current) => (current ? { ...current, next: null } : current));
    } catch (nextError) {
      setInterveneError(nextError instanceof Error ? nextError.message : t("multiAgent.interveneError"));
    } finally {
      setPosting(false);
    }
  }

  // A candidate replaces the command being typed on the last line, and the action
  // word is selected so what is typed next replaces it.
  function insertRollCandidate(candidate: string, action: string) {
    const next = draft.slice(0, draft.lastIndexOf("\n") + 1) + candidate;
    setDraft(next);
    window.requestAnimationFrame(() => {
      const node = composerRef.current;
      if (!node) {
        return;
      }
      node.focus();
      node.setSelectionRange(next.length - action.length, next.length);
    });
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

  // Stopping clears the loop's flag at once, but the turn in flight runs on, so
  // the button keeps saying "stop" and shows it is busy until that turn ends.
  const stopPending = stopRequested && turnRunning;
  const autoShowsStop = autoRunning || stopPending;
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
  const advanceReason = autoRunning
    ? t("multiAgent.autoRunning")
    : stopPending
      ? t("multiAgent.turnRunningReason")
      : turnRunning
        ? undefined
        : turnBlockedReason;
  const autoReason = autoShowsStop
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

  // The speaker select is a choice only while manual waits for a nomination.
  // Otherwise it reports the engine's own pick (design §6): who speaks next, or,
  // while a turn runs and the next one cannot be known yet (weighted reads the
  // utterance, manual waits for the user), who is speaking now.
  const speakerSelectEnabled = manualRule && !turnRunning && !autoRunning;
  const speakerReason = speakerSelectEnabled
    ? undefined
    : autoRunning
      ? t("multiAgent.lockedAuto")
      : turnRunning
        ? t("multiAgent.lockedTurn")
        : t("multiAgent.speakerByRule");
  // The picks arrive as participant rows captured when they were read, so the
  // name is taken from the current roster by id: a rename saved in the panel
  // since then must show without waiting for the next read.
  const current = (participant: Participant | null | undefined) =>
    participant ? speakerById.get(participant.id) ?? participant : null;
  const speakingNow = turnRunning
    ? current(runningSpeaker?.participant ?? (manualRule ? speakerById.get(nomineeId) : nextSpeaker))
    : null;
  const shownNext = current(turnRunning ? runningSpeaker?.next : manualRule ? null : nextSpeaker);
  const speakerIsCurrent = turnRunning && !shownNext;
  // The composer suggests the command while its name is being typed on the last
  // line, and keeps the format in view while the arguments are (design §4.8.3
  // item 3). There is no dice button: the command is the one way in, for the
  // human and the models alike. A chat that has not enabled /roll reads the line
  // as text (design §4.8.7), so nothing is suggested there.
  const roll = state.chat.commands.roll;
  const rollTarget = roll?.target ?? 0;
  const commandLine = draft.slice(draft.lastIndexOf("\n") + 1).trimStart();
  const typingCommand =
    roll !== undefined && commandLine.startsWith("/") && !/\s/.test(commandLine) && ROLL_KEYWORD.startsWith(commandLine);
  const writingRoll =
    roll !== undefined && (commandLine.startsWith(`${ROLL_KEYWORD} `) || commandLine.startsWith(`${ROLL_KEYWORD}\u3000`));
  const rollAction = t("multiAgent.rollAction");
  const rollCandidates = [
    `${ROLL_KEYWORD} 1d20+0 ${rollAction}`,
    `${ROLL_KEYWORD} 1d20+0 ${t("multiAgent.rollTargetKeyword")}${rollTarget || 12} ${rollAction}`
  ];
  const speakerText = shownNext
    ? t("multiAgent.speakerNext", { name: shownNext.displayName })
    : speakingNow
      ? t("multiAgent.speakerNow", { name: speakingNow.displayName })
      : turnRunning
        ? t("multiAgent.runningTurnUnknown")
        : t("multiAgent.speakerPrompt");

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
            <UserGroupIcon size={18} />
            {state.chat.isTemporary ? (
              <>
                <RotateCwFadingClockIcon size={18} />
                <VisuallyHidden>{t("chat.temporaryChat")}</VisuallyHidden>
              </>
            ) : null}
            <SectionTitle>{chatTitle}</SectionTitle>
            <Badge tone="warm">{t("multiAgent.badge")}</Badge>
            <Badge tone="accent">{state.project.title}</Badge>
          </Row>
          <Row style={{ alignItems: "center", flexWrap: "nowrap", gap: 8 }}>
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
              aria-label={t("multiAgent.inspector")}
              title={rosterRegion.shown ? t("multiAgent.hideInspector") : t("multiAgent.showInspector")}
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
                    {/* A roll-only message has no body to draw; its chip stands alone. */}
                    {!message.content ? null : message.role === "assistant" ? (
                      <MarkdownPreview source={message.content} />
                    ) : (
                      <div style={{ whiteSpace: "pre-wrap", lineHeight: 1.65 }}>{message.content}</div>
                    )}
                    <DiceRollChips rolls={message.diceRolls ?? []} />
                    <MessageReferences references={message.references} />
                    <Row style={{ justifyContent: "flex-end", alignItems: "center", gap: 10, flexWrap: "nowrap" }}>
                      <CopyMessageButton
                        content={messageCopyText(message, t)}
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
            {turnError ? <FailureNotice>{turnError}</FailureNotice> : null}
            {conclusionError ? <FailureNotice>{conclusionError}</FailureNotice> : null}
            {interveneError ? <FailureNotice>{interveneError}</FailureNotice> : null}
            {facilitatorMissing ? <MetaText>{t("multiAgent.facilitatorMissing")}</MetaText> : null}
            {memorySavedTitle ? <MetaText>{t("multiAgent.saveMemorySaved", { title: memorySavedTitle })}</MetaText> : null}
            {typingCommand ? (
              <Row role="group" aria-label={t("multiAgent.commandSuggestions")} style={{ gap: 4 }}>
                {rollCandidates.map((candidate) => (
                  <ActionButton
                    key={candidate}
                    type="button"
                    variant="normal"
                    onClick={() => insertRollCandidate(candidate, rollAction)}
                    style={ROW_BUTTON_STYLE}
                  >
                    <code>{candidate}</code>
                  </ActionButton>
                ))}
              </Row>
            ) : null}
            {typingCommand || writingRoll ? (
              <MetaText>
                {t("multiAgent.rollFormat")}
                {rollTarget > 0 ? t("multiAgent.rollFormatDefault", { target: rollTarget }) : ""}
              </MetaText>
            ) : null}
            <ComposerTextarea
              ref={composerRef}
              value={draft}
              onChange={setDraft}
              placeholder={t("multiAgent.intervenePlaceholder")}
              aria-label={t("multiAgent.interveneLabel")}
            />
            {/* One flat row, positioned so the hint's note spans it (Hint). It keeps to
                one line about as far as the single chat's does: short labels, 4px gaps,
                and a speaker select that line-breaks at its 130px minimum and grows to
                220px. Below that the controls wrap one by one, since a group that could
                not break would be clipped by the pane (overflow: hidden). */}
            <Row style={{ alignItems: "center", gap: 4, rowGap: 8, position: "relative" }}>
              {/* The busy figure sits inside the select, left of its chevron: beside
                  the select it would take a slot that stands empty whenever no turn
                  runs. */}
              <span style={{ position: "relative", display: "inline-flex", flex: "1 1 130px", minWidth: 130, maxWidth: 220 }}>
                <GuardedSelect
                  value={speakerSelectEnabled ? nomineeId : ""}
                  aria-label={t("multiAgent.speaker")}
                  disabledReason={speakerReason}
                  onChange={(event) => setNomineeId(event.target.value)}
                  style={{
                    width: "100%",
                    minWidth: 0,
                    ...(speakerIsCurrent ? { paddingInlineEnd: SPEAKER_SELECT_BUSY_END_PADDING } : {})
                  }}
                >
                  {speakerSelectEnabled ? (
                    <>
                      <option value="">{t("multiAgent.speakerPrompt")}</option>
                      {roster.map((participant) => (
                        <option key={participant.id} value={participant.id}>
                          {participant.displayName}
                        </option>
                      ))}
                    </>
                  ) : (
                    <option value="">{speakerText}</option>
                  )}
                </GuardedSelect>
                {speakerIsCurrent ? (
                  <span style={SPEAKER_SPINNER_STYLE}>
                    <SpinnerIcon size={14} />
                  </span>
                ) : null}
              </span>
              <ActionButton
                type="button"
                icon={<StepForwardIcon />}
                busy={turnRunning && !autoRunning && !stopPending}
                disabledReason={advanceReason}
                onClick={() => void handleAdvanceTurn()}
                style={ROW_BUTTON_STYLE}
              >
                {t("multiAgent.advanceTurn")}
              </ActionButton>
              <ActionButton
                type="button"
                variant="normal"
                icon={autoShowsStop ? <SquareIcon /> : <PlayIcon />}
                busy={stopPending}
                disabledReason={autoReason}
                onClick={() => void handleToggleAutoRun()}
                style={ROW_BUTTON_STYLE}
              >
                {autoShowsStop ? t("multiAgent.autoStop") : t("multiAgent.autoStart")}
              </ActionButton>
              <Hint
                name={t("multiAgent.progressHintName")}
                body={t("multiAgent.progressHint", { limit: historyLimit ?? "…" })}
              />
              {/* The auto margin takes the free space, so the badge and the send stay
                  together at the end of whichever line they land on. The pair is small
                  enough to fit the narrowest pane, so it need not break. */}
              <Row style={{ alignItems: "center", gap: 4, flexWrap: "nowrap", marginInlineStart: "auto" }}>
                {turnNumber > 0 ? (
                  <StateBadge tone="neutral" style={{ whiteSpace: "nowrap" }}>
                    {t("multiAgent.turnCount", { count: turnNumber })}
                  </StateBadge>
                ) : null}
                <ActionButton
                  type="submit"
                  variant="normal"
                  icon={<SendIcon />}
                  busy={posting}
                  disabledReason={posting || draft.trim() ? undefined : t("chat.messageRequired")}
                  style={ROW_BUTTON_STYLE}
                >
                  {t("multiAgent.intervene")}
                </ActionButton>
              </Row>
            </Row>
          </ComposerBox>
        </Composer>
      </MainPane>

      {/* Hidden rather than unmounted: the panel holds unsaved edits (display name,
          role prompt, endpoint, model, scene), and hiding it must not throw away
          an edit in progress. Hidden also takes it out of the grid, so the
          transcript gets the full width; on a narrow screen it lies over the
          transcript instead. */}
      <InspectorPane $toggled {...rosterRegion.regionProps} aria-label={t("multiAgent.inspector")}>
        {rosterRegion.overlay ? (
          <RegionCloseButton type="button" {...rosterRegion.closeProps}>
            <PanelRightCloseIcon />
          </RegionCloseButton>
        ) : null}
        <ParticipantPanel
          chat={state.chat}
          participants={participants}
          conversationStarted={state.messages.length > 0}
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
          <DialogHeader>
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
          </DialogHeader>
          <DialogForm onSubmit={handleSaveMemory}>
            <DialogBody>
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
            </DialogBody>
            <DialogActions notice={memoryError ? <FailureNotice>{memoryError}</FailureNotice> : null}>
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
            </DialogActions>
          </DialogForm>
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
