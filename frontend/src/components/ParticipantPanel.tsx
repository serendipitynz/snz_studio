import styled from "@emotion/styled";
import { FormEvent, ReactNode, useEffect, useRef, useState } from "react";
import {
  api,
  CHAT_STATE_SHEET_MAX_CHARS,
  ChatRecord,
  Participant,
  PARTICIPANT_STATE_SHEET_MAX_CHARS,
  stateSheetLength,
  TurnRule
} from "../api/client";
import { ActionButton } from "./ActionButton";
import { announce } from "./announce";
import { useConfirm } from "./ConfirmDialog";
import { Checkbox } from "./Checkbox";
import { CollapsibleSection } from "./CollapsibleSection";
import { FailureNotice } from "./FailureNotice";
import { GuardedSelect } from "./GuardedSelect";
import { Hint, HintedField, HintRow } from "./Hint";
import { CheckIcon, MoveDownIcon, MoveUpIcon, SparklesIcon, TrashIcon, UserPlusIcon } from "./icons";
import { PresetChoice, PresetPicker } from "./PresetPicker";
import { useLanguage } from "../i18n";
import {
  Badge,
  Card,
  Divider,
  Field,
  Input,
  MetaText,
  Row,
  SectionTitle,
  Select,
  Stack,
  Subtle,
  SubsectionTitle,
  Textarea
} from "../styles/ui";

interface ParticipantPanelProps {
  chat: ChatRecord;
  participants: Participant[];
  onChatChange: (chat: ChatRecord) => void;
  onParticipantsChange: (participants: Participant[]) => void;
  // Given while a turn or the auto-advance runs: the roster and the rules are
  // held still, and every control that would change them says this instead.
  lockedReason?: string;
  // True once the conversation has a message. It decides which sections start
  // open (before: the line-up wants checking, so settings and cards are open and
  // the state sheets folded; after: the state sheets are what changes, so they
  // open and the rest folds), and it keeps the preset section off screen — a
  // preset replaces the roster, the turn rule and the scene, and a transcript
  // would be left referring to a cast the chat no longer has. The server refuses
  // a late preset either way; this only keeps the control off screen.
  conversationStarted: boolean;
}

// The panel's cards are narrow (a side column), so they take a tighter padding
// than the shared Card to keep the fields inside wide (owner's real-window
// feedback, 2026-09-26).
const PanelCard = styled(Card)`
  padding: 12px;
`;

// The character counter under a state sheet. It turns to the error colour past
// the limit, which is also when the save button says it cannot save.
const StateSheetCounter = styled(MetaText)<{ over: boolean }>`
  color: ${({ theme, over }) => (over ? theme.dangerText : theme.muted)};
`;

// A per-endpoint probe result. Listing an endpoint's models is both the model
// picker's source and the connection check the design asks for (§6): the API has
// no separate check route, and an endpoint that answers with its model list is
// exactly an endpoint a turn can run against.
interface EndpointProbe {
  state: "checking" | "ok" | "failed";
  models: string[];
  error?: string;
}

type MoveDirection = -1 | 1;

// ParticipantPanel is the organisation panel of docs/multi-agent-chat-design.md
// §6: the participant CRUD with its endpoint check and model picker, plus the
// chat-level turn rule, scene prompt and shared state sheet.
// Each failure is told in the section it belongs to (snz-design doc-9 §5.5).
export function ParticipantPanel(props: ParticipantPanelProps) {
  const { t } = useLanguage();
  const confirm = useConfirm();
  const rosterRef = useRef<HTMLDivElement | null>(null);
  const addInputRef = useRef<HTMLInputElement | null>(null);
  const [probes, setProbes] = useState<Record<string, EndpointProbe>>({});
  const [newDisplayName, setNewDisplayName] = useState("");
  const [adding, setAdding] = useState(false);
  const [presetChoice, setPresetChoice] = useState<PresetChoice | null>(null);
  const [applyingPreset, setApplyingPreset] = useState(false);
  const [moving, setMoving] = useState<{ participantId: string; direction: MoveDirection } | null>(null);
  const [removingId, setRemovingId] = useState<string | null>(null);
  const [presetError, setPresetError] = useState("");
  const [settingsError, setSettingsError] = useState("");
  const [addError, setAddError] = useState("");
  const [participantErrors, setParticipantErrors] = useState<Record<string, string>>({});
  // Keyed "chat" for the shared sheet, else by participant id: the state section
  // has its own fields now, so their failures are told there, not in a card that
  // may be folded (snz-design doc-9 §5.5).
  const [stateErrors, setStateErrors] = useState<Record<string, string>>({});
  // Where focus goes once the roster has re-rendered after a move or a removal.
  const pendingFocus = useRef<{ participantId: string; action: string } | "add" | null>(null);

  const started = props.conversationStarted;
  const [stateCollapsed, setStateCollapsed] = useState(!started);
  const [settingsCollapsed, setSettingsCollapsed] = useState(started);
  // Overrides over the phase's default (open before the conversation starts,
  // folded after). Cleared when the conversation starts, so every card takes the
  // folded default at that moment (owner's call, 2026-09-26).
  const [cardCollapsed, setCardCollapsed] = useState<Record<string, boolean>>({});
  const prevStarted = useRef(started);
  useEffect(() => {
    if (started && !prevStarted.current) {
      setStateCollapsed(false);
      setSettingsCollapsed(true);
      setCardCollapsed({});
    }
    prevStarted.current = started;
  }, [started]);

  const roster = props.participants.filter((participant) => participant.deletedAt === null);
  const removed = props.participants.filter((participant) => participant.deletedAt !== null);
  const facilitatorOnRoster = roster.some((participant) => participant.id === props.chat.facilitatorId);

  useEffect(() => {
    const target = pendingFocus.current;
    if (!target) {
      return;
    }
    pendingFocus.current = null;
    if (target === "add") {
      addInputRef.current?.focus();
      return;
    }
    rosterRef.current
      ?.querySelector<HTMLElement>(`[data-participant="${target.participantId}"] [data-action="${target.action}"]`)
      ?.focus();
  }, [props.participants]);

  function setParticipantError(participantId: string, message: string) {
    setParticipantErrors((current) => ({ ...current, [participantId]: message }));
  }

  function errorMessage(error: unknown, fallback: Parameters<typeof t>[0]) {
    return error instanceof Error ? error.message : t(fallback);
  }

  async function probeEndpoint(baseUrl: string) {
    const key = baseUrl.trim();
    setProbes((current) => ({ ...current, [key]: { state: "checking", models: [] } }));
    try {
      const response = await api.listConfigurationModels({ kind: "llm", baseUrl: key });
      setProbes((current) => ({ ...current, [key]: { state: "ok", models: response.models } }));
    } catch (nextError) {
      setProbes((current) => ({
        ...current,
        [key]: { state: "failed", models: [], error: errorMessage(nextError, "participants.connectionFailed") }
      }));
    }
  }

  async function handleAddParticipant(event: FormEvent) {
    event.preventDefault();
    const displayName = newDisplayName.trim();
    if (!displayName || adding) {
      return;
    }

    setAdding(true);
    setAddError("");
    try {
      const response = await api.createParticipant(props.chat.id, { displayName });
      props.onParticipantsChange([...props.participants, response.participant]);
      // A just-added participant opens even mid-conversation: its role prompt and
      // endpoint are still empty, which is what the card is for.
      setCardCollapsed((current) => ({ ...current, [response.participant.id]: false }));
      setNewDisplayName("");
    } catch (nextError) {
      setAddError(errorMessage(nextError, "participants.saveError"));
    } finally {
      setAdding(false);
    }
  }

  async function handleSaveParticipant(participantId: string, input: Parameters<typeof api.updateParticipant>[1]) {
    setParticipantError(participantId, "");
    try {
      const response = await api.updateParticipant(participantId, input);
      props.onParticipantsChange(
        props.participants.map((participant) => (participant.id === participantId ? response.participant : participant))
      );
    } catch (nextError) {
      setParticipantError(participantId, errorMessage(nextError, "participants.saveError"));
      throw nextError;
    }
  }

  async function handleRemoveParticipant(participant: Participant) {
    const accepted = await confirm(t("participants.removeConfirm", { name: participant.displayName }), {
      heading: t("participants.removeHeading"),
      confirmLabel: t("participants.removeConfirmLabel")
    });
    if (!accepted) {
      return;
    }

    // The card leaves the roster, so focus goes to the neighbour's same button,
    // or to the add form once the roster is empty (snz-design doc-9 §5.1).
    const index = roster.findIndex((current) => current.id === participant.id);
    const neighbour = roster[index + 1] ?? roster[index - 1];

    setRemovingId(participant.id);
    setParticipantError(participant.id, "");
    try {
      const response = await api.removeParticipant(participant.id);
      pendingFocus.current = neighbour ? { participantId: neighbour.id, action: "remove" } : "add";
      props.onParticipantsChange(
        props.participants.map((current) => (current.id === participant.id ? response.participant : current))
      );
    } catch (nextError) {
      setParticipantError(participant.id, errorMessage(nextError, "participants.saveError"));
    } finally {
      setRemovingId(null);
    }
  }

  // Reordering swaps the two neighbours' sort_order rather than renumbering the
  // roster, so a move is two writes regardless of how long the roster is and no
  // other participant's order changes under it.
  // The moved card keeps focus on the button that moved it, and its new place is
  // read out, since the move itself is only seen (snz-design doc-9 §6.9).
  async function handleMove(index: number, direction: MoveDirection) {
    const target = roster[index];
    const neighbour = roster[index + direction];
    if (!target || !neighbour || moving) {
      return;
    }

    setMoving({ participantId: target.id, direction });
    setParticipantError(target.id, "");
    try {
      const [first, second] = await Promise.all([
        api.updateParticipant(target.id, { sortOrder: neighbour.sortOrder }),
        api.updateParticipant(neighbour.id, { sortOrder: target.sortOrder })
      ]);
      const updated = new Map([
        [first.participant.id, first.participant],
        [second.participant.id, second.participant]
      ]);
      pendingFocus.current = { participantId: target.id, action: direction === -1 ? "up" : "down" };
      props.onParticipantsChange(
        props.participants
          .map((participant) => updated.get(participant.id) ?? participant)
          .sort((left, right) => left.sortOrder - right.sortOrder)
      );
      announce(
        t("participants.moved", { name: target.displayName, position: index + direction + 1, total: roster.length })
      );
    } catch (nextError) {
      setParticipantError(target.id, errorMessage(nextError, "participants.saveError"));
    } finally {
      setMoving(null);
    }
  }

  async function handleApplyPreset() {
    if (!presetChoice) {
      return;
    }
    // Only when there is something to lose: applying to the empty roster the
    // sidebar creates is the ordinary path and asking there would be noise.
    if (
      roster.length > 0 &&
      !(await confirm(t("preset.applyConfirm", { count: roster.length }), {
        heading: t("preset.applyHeading"),
        confirmLabel: t("preset.applyConfirmLabel")
      }))
    ) {
      return;
    }

    setApplyingPreset(true);
    setPresetError("");
    try {
      const response = await api.applyMultiAgentPreset(props.chat.id, presetChoice.selection);
      props.onChatChange(response.chat);
      props.onParticipantsChange(response.participants);
    } catch (nextError) {
      setPresetError(errorMessage(nextError, "preset.applyError"));
    } finally {
      setApplyingPreset(false);
    }
  }

  async function saveSettings(input: Parameters<typeof api.updateChatMultiAgentSettings>[1], rethrow = false) {
    setSettingsError("");
    try {
      const response = await api.updateChatMultiAgentSettings(props.chat.id, input);
      props.onChatChange(response.chat);
    } catch (nextError) {
      setSettingsError(errorMessage(nextError, "participants.settingsSaveError"));
      if (rethrow) {
        throw nextError;
      }
    }
  }

  async function saveChatStateSheet(stateSheet: string) {
    setStateErrors((current) => ({ ...current, chat: "" }));
    try {
      const response = await api.updateChatMultiAgentSettings(props.chat.id, { stateSheet });
      props.onChatChange(response.chat);
    } catch (nextError) {
      setStateErrors((current) => ({ ...current, chat: errorMessage(nextError, "participants.settingsSaveError") }));
      throw nextError;
    }
  }

  async function saveParticipantStateSheet(participantId: string, stateSheet: string) {
    setStateErrors((current) => ({ ...current, [participantId]: "" }));
    try {
      const response = await api.updateParticipant(participantId, { stateSheet });
      props.onParticipantsChange(
        props.participants.map((participant) => (participant.id === participantId ? response.participant : participant))
      );
    } catch (nextError) {
      setStateErrors((current) => ({ ...current, [participantId]: errorMessage(nextError, "participants.saveError") }));
      throw nextError;
    }
  }

  const presetReason = props.lockedReason ?? (presetChoice ? undefined : t("preset.chooseFirst"));
  const addReason = props.lockedReason ?? (newDisplayName.trim() ? undefined : t("participants.nameRequired"));

  // The participant's name set in bold inside its state label (owner's sketch,
  // 2026-09-26). The translation keeps {name} as a placeholder, so the words are
  // split around a sentinel no display name can contain.
  function participantStateLabel(name: string): ReactNode {
    const [before, after] = t("participants.participantStateOf", { name: "\u0000" }).split("\u0000");
    return (
      <>
        {before}
        <strong>{name}</strong>
        {after}
      </>
    );
  }

  return (
    <Stack>
      <SectionTitle>{t("multiAgent.inspector")}</SectionTitle>

      {!started ? (
        <PanelCard>
          {/* Folded by default, and the open state is deliberately not kept: a
              chat created from a preset already has its line-up, so an expanded
              section above the roster would be shouting about a decision already
              taken. Opening it is one press when the preset is what is wanted. */}
          <CollapsibleSection heading={t("preset.section")}>
            <Stack>
              {/* The when-and-what of applying lives in the label's (?) rather
                  than as a paragraph above the select (owner's real-window
                  feedback, 2026-09-26). */}
              <PresetPicker labelHint={t("preset.applyNote")} disabledReason={props.lockedReason} onChange={setPresetChoice} />
              <Row style={{ justifyContent: "flex-end" }}>
                <ActionButton
                  type="button"
                  variant="normal"
                  icon={<SparklesIcon />}
                  busy={applyingPreset}
                  disabledReason={applyingPreset ? undefined : presetReason}
                  onClick={() => void handleApplyPreset()}
                >
                  {t("preset.apply")}
                </ActionButton>
              </Row>
              {presetError ? <FailureNotice>{presetError}</FailureNotice> : null}
            </Stack>
          </CollapsibleSection>
        </PanelCard>
      ) : null}

      <PanelCard>
        {/* First and open by default once the conversation runs: the sheets are
            the one part of the panel that changes with the play (design §4.7). */}
        <CollapsibleSection
          heading={t("participants.stateSection")}
          collapsed={stateCollapsed}
          onToggle={setStateCollapsed}
        >
          <Stack>
            {/* Keyed on the stored value so a sheet replaced from outside the
                field (a preset applied, a save that trimmed it) resets the
                draft. */}
            <StateSheetField
              key={props.chat.stateSheet}
              label={t("participants.sharedState")}
              placeholder={t("participants.sharedStatePlaceholder")}
              value={props.chat.stateSheet}
              maxChars={CHAT_STATE_SHEET_MAX_CHARS}
              onSave={saveChatStateSheet}
            />
            {stateErrors.chat ? <FailureNotice>{stateErrors.chat}</FailureNotice> : null}
            {roster.map((participant) => (
              <Stack key={participant.id}>
                <StateSheetField
                  key={participant.stateSheet}
                  label={t("participants.participantStateOf", { name: participant.displayName })}
                  labelContent={participantStateLabel(participant.displayName)}
                  placeholder={t("participants.participantStatePlaceholder")}
                  value={participant.stateSheet}
                  maxChars={PARTICIPANT_STATE_SHEET_MAX_CHARS}
                  onSave={(stateSheet) => saveParticipantStateSheet(participant.id, stateSheet)}
                />
                {stateErrors[participant.id] ? <FailureNotice>{stateErrors[participant.id]}</FailureNotice> : null}
              </Stack>
            ))}
          </Stack>
        </CollapsibleSection>
      </PanelCard>

      <PanelCard>
        <CollapsibleSection
          heading={t("participants.settings")}
          collapsed={settingsCollapsed}
          onToggle={setSettingsCollapsed}
        >
          <Stack>
          <Field>
            {t("participants.turnRule")}
            <GuardedSelect
              value={props.chat.turnRule}
              disabledReason={props.lockedReason}
              onChange={(event) => void saveSettings({ turnRule: event.target.value as TurnRule })}
            >
              <option value="round_robin">{t("participants.turnRuleRoundRobin")}</option>
              <option value="manual">{t("participants.turnRuleManual")}</option>
              <option value="facilitator_alternating">{t("participants.turnRuleFacilitator")}</option>
              <option value="weighted">{t("participants.turnRuleWeighted")}</option>
            </GuardedSelect>
          </Field>
          {props.chat.turnRule === "facilitator_alternating" || props.chat.turnRule === "weighted" ? (
            <Field>
              {t("participants.facilitator")}
              {/* The value is matched against the roster rather than taken from
                  the chat as stored: a facilitator that has been removed is an
                  id the select has no option for, and showing it as unset is
                  what the turn then actually does. */}
              <GuardedSelect
                value={facilitatorOnRoster ? props.chat.facilitatorId : ""}
                disabledReason={props.lockedReason}
                onChange={(event) => void saveSettings({ facilitatorId: event.target.value })}
              >
                <option value="">{t("participants.facilitatorUnset")}</option>
                {roster.map((participant) => (
                  <option key={participant.id} value={participant.id}>
                    {participant.displayName}
                  </option>
                ))}
              </GuardedSelect>
              <Subtle style={{ margin: 0 }}>
                {props.chat.turnRule === "weighted"
                  ? t("participants.facilitatorWeightedNote")
                  : facilitatorOnRoster
                    ? t("participants.facilitatorNote")
                    : t("participants.facilitatorMissing")}
              </Subtle>
            </Field>
          ) : null}
          <ScenePromptField
            value={props.chat.scenePrompt}
            lockedReason={props.lockedReason}
            onSave={(scenePrompt) => saveSettings({ scenePrompt }, true)}
          />
          {settingsError ? <FailureNotice>{settingsError}</FailureNotice> : null}
          </Stack>
        </CollapsibleSection>
      </PanelCard>

      <Row style={{ justifyContent: "space-between", alignItems: "baseline" }}>
        <SubsectionTitle>{t("participants.roster")}</SubsectionTitle>
        <MetaText>{t("participants.orderNote")}</MetaText>
      </Row>

      {roster.length === 0 ? <Subtle>{t("multiAgent.rosterEmpty")}</Subtle> : null}

      {/* One move is saved at a time; the list says so while it is. */}
      <Stack ref={rosterRef} aria-busy={moving ? true : undefined}>
        {roster.map((participant, index) => (
          <ParticipantEditor
            key={participant.id}
            participant={participant}
            probes={probes}
            lockedReason={props.lockedReason}
            moveUpReason={index === 0 ? t("participants.atTop") : undefined}
            moveDownReason={index === roster.length - 1 ? t("participants.atBottom") : undefined}
            movingDirection={moving?.participantId === participant.id ? moving.direction : null}
            removing={removingId === participant.id}
            error={participantErrors[participant.id] ?? ""}
            collapsed={cardCollapsed[participant.id] ?? started}
            onToggleCollapsed={(collapsed) =>
              setCardCollapsed((current) => ({ ...current, [participant.id]: collapsed }))
            }
            onProbe={probeEndpoint}
            onSave={handleSaveParticipant}
            onRemove={handleRemoveParticipant}
            onMove={(direction) => void handleMove(index, direction)}
          />
        ))}
      </Stack>

      <PanelCard as="form" onSubmit={handleAddParticipant}>
        <Stack>
          <Field>
            {t("participants.displayName")}
            <Input
              ref={addInputRef}
              value={newDisplayName}
              onChange={(event) => setNewDisplayName(event.target.value)}
              placeholder={t("participants.displayNamePlaceholder")}
            />
          </Field>
          <div>
            <ActionButton
              type="submit"
              variant="normal"
              icon={<UserPlusIcon />}
              busy={adding}
              disabledReason={adding ? undefined : addReason}
            >
              {t("participants.add")}
            </ActionButton>
          </div>
          {addError ? <FailureNotice>{addError}</FailureNotice> : null}
        </Stack>
      </PanelCard>

      {removed.length ? (
        <PanelCard>
          <CollapsibleSection heading={t("participants.removed", { count: removed.length })}>
            <Stack>
              {removed.map((participant) => (
                <Subtle key={participant.id}>
                  {participant.displayName}
                  {participant.modelName ? ` — ${participant.modelName}` : ""}
                </Subtle>
              ))}
            </Stack>
          </CollapsibleSection>
        </PanelCard>
      ) : null}
    </Stack>
  );
}

function ScenePromptField(props: { value: string; lockedReason?: string; onSave: (value: string) => Promise<void> }) {
  const { t } = useLanguage();
  const [draft, setDraft] = useState(props.value);
  const [saving, setSaving] = useState(false);

  async function handleSave() {
    setSaving(true);
    try {
      await props.onSave(draft);
    } catch {
      // The panel already surfaced the failure; the draft is kept so the edit is
      // not lost behind the error.
    } finally {
      setSaving(false);
    }
  }

  const reason = props.lockedReason ?? (draft === props.value ? t("participants.unchanged") : undefined);

  return (
    <Stack>
      <Field>
        {t("participants.scenePrompt")}
        <Textarea
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder={t("participants.scenePromptPlaceholder")}
          style={{ minHeight: 90 }}
        />
      </Field>
      <Row style={{ justifyContent: "flex-end" }}>
        <ActionButton
          type="button"
          variant="normal"
          icon={<CheckIcon />}
          busy={saving}
          disabledReason={saving ? undefined : reason}
          onClick={() => void handleSave()}
        >
          {t("participants.save")}
        </ActionButton>
      </Row>
    </Stack>
  );
}

// StateSheetField edits one state sheet and saves it on its own. It takes no
// locked reason, unlike every other control in the panel: a turn reads the
// sheets when it starts, so a sheet saved while a turn or the auto-advance is
// running is simply what the next turn reads — and keeping HP and inventory in
// step with the play as it happens is what the sheet is for (design §4.7).
function StateSheetField(props: {
  label: string;
  // The label with the participant's name set in bold; label stays the plain
  // words for the hint's and the field's name.
  labelContent?: ReactNode;
  placeholder: string;
  value: string;
  maxChars: number;
  onSave: (value: string) => Promise<void>;
}) {
  const { t } = useLanguage();
  const [draft, setDraft] = useState(props.value);
  const [saving, setSaving] = useState(false);
  const length = stateSheetLength(draft);
  const over = length > props.maxChars;

  async function handleSave() {
    setSaving(true);
    try {
      await props.onSave(draft);
    } catch {
      // Reported by the panel; the draft stays so the edit survives the failure.
    } finally {
      setSaving(false);
    }
  }

  const reason = over
    ? t("participants.overLimit", { max: props.maxChars })
    : draft === props.value
      ? t("participants.unchanged")
      : undefined;

  return (
    <Stack>
      <HintedField
        label={props.label}
        labelContent={props.labelContent}
        labelEnd={
          <StateSheetCounter over={over}>
            {t("participants.stateCount", { count: length, max: props.maxChars })}
          </StateSheetCounter>
        }
        hint={t("participants.stateHint")}
      >
        {(id) => (
          // Read-only while its own save is in flight: the saved value becomes
          // the field's key, so the remount that follows would drop anything
          // typed after the request left.
          <Textarea
            id={id}
            value={draft}
            readOnly={saving}
            onChange={(event) => setDraft(event.target.value)}
            placeholder={props.placeholder}
            style={{ minHeight: 72 }}
          />
        )}
      </HintedField>
      <Row style={{ justifyContent: "flex-end" }}>
        <ActionButton
          type="button"
          variant="normal"
          icon={<CheckIcon />}
          busy={saving}
          disabledReason={saving ? undefined : reason}
          onClick={() => void handleSave()}
        >
          {t("participants.save")}
        </ActionButton>
      </Row>
    </Stack>
  );
}

interface ParticipantEditorProps {
  participant: Participant;
  // Keyed by endpoint rather than by participant, so a check made for one
  // participant already answers for every other pointed at the same server.
  probes: Record<string, EndpointProbe>;
  lockedReason?: string;
  moveUpReason?: string;
  moveDownReason?: string;
  movingDirection: MoveDirection | null;
  removing: boolean;
  error: string;
  collapsed: boolean;
  onToggleCollapsed: (collapsed: boolean) => void;
  onProbe: (baseUrl: string) => Promise<void>;
  onSave: (participantId: string, input: Parameters<typeof api.updateParticipant>[1]) => Promise<void>;
  onRemove: (participant: Participant) => Promise<void>;
  onMove: (direction: MoveDirection) => void;
}

// A participant's card, foldable by its heading (the display name). The move and
// remove buttons sit in the heading area so the roster can be reordered and left
// without opening a card. The fields split at the divider into what shapes the
// speaker (name, role prompt, project material) and where its turn runs
// (endpoint, model), each with its own save — the owner's grouping by how often
// each is touched (2026-09-26).
function ParticipantEditor(props: ParticipantEditorProps) {
  const { t } = useLanguage();
  const [displayName, setDisplayName] = useState(props.participant.displayName);
  const [rolePrompt, setRolePrompt] = useState(props.participant.rolePrompt);
  const [baseUrl, setBaseUrl] = useState(props.participant.baseUrl);
  const [modelName, setModelName] = useState(props.participant.modelName);
  const [receivesProjectMaterial, setReceivesProjectMaterial] = useState(props.participant.receivesProjectMaterial);
  const [savingProfile, setSavingProfile] = useState(false);
  const [savingConnection, setSavingConnection] = useState(false);

  const profileDirty =
    displayName !== props.participant.displayName ||
    rolePrompt !== props.participant.rolePrompt ||
    receivesProjectMaterial !== props.participant.receivesProjectMaterial;
  const connectionDirty = baseUrl !== props.participant.baseUrl || modelName !== props.participant.modelName;

  async function handleSaveProfile() {
    setSavingProfile(true);
    try {
      await props.onSave(props.participant.id, { displayName, rolePrompt, receivesProjectMaterial });
    } catch {
      // Reported in this card; the drafts stay so the edit survives the failure.
    } finally {
      setSavingProfile(false);
    }
  }

  async function handleSaveConnection() {
    setSavingConnection(true);
    try {
      await props.onSave(props.participant.id, { baseUrl, modelName });
    } catch {
      // Reported in this card; the drafts stay so the edit survives the failure.
    } finally {
      setSavingConnection(false);
    }
  }

  // The draft endpoint, not props.participant.baseUrl: the check runs against
  // what is typed in, so an endpoint entered but not yet saved would otherwise
  // never find its own result.
  const probe = props.probes[baseUrl.trim()];
  const moving = props.movingDirection !== null;
  const profileReason = props.lockedReason ?? (profileDirty ? undefined : t("participants.unchanged"));
  const connectionReason = props.lockedReason ?? (connectionDirty ? undefined : t("participants.unchanged"));

  return (
    <PanelCard data-participant={props.participant.id}>
      <Stack>
      <CollapsibleSection
        heading={props.participant.displayName}
        collapsed={props.collapsed}
        onToggle={props.onToggleCollapsed}
        actions={
          <Row style={{ alignItems: "center", flexWrap: "nowrap", gap: 4 }}>
            <ActionButton
              type="button"
              iconOnly
              data-action="up"
              aria-label={t("participants.moveUp")}
              busy={props.movingDirection === -1}
              disabledReason={moving ? undefined : (props.lockedReason ?? props.moveUpReason)}
              onClick={() => props.onMove(-1)}
            >
              <MoveUpIcon />
            </ActionButton>
            <ActionButton
              type="button"
              iconOnly
              data-action="down"
              aria-label={t("participants.moveDown")}
              busy={props.movingDirection === 1}
              disabledReason={moving ? undefined : (props.lockedReason ?? props.moveDownReason)}
              onClick={() => props.onMove(1)}
            >
              <MoveDownIcon />
            </ActionButton>
            <ActionButton
              type="button"
              iconOnly
              data-action="remove"
              aria-label={t("participants.remove")}
              title={t("participants.remove")}
              busy={props.removing}
              disabledReason={props.removing ? undefined : props.lockedReason}
              onClick={() => void props.onRemove(props.participant)}
            >
              <TrashIcon />
            </ActionButton>
          </Row>
        }
      >
        <Stack>
          <Field>
            {t("participants.displayName")}
            <Input
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
              placeholder={t("participants.displayNamePlaceholder")}
            />
          </Field>

          <Field>
            {t("participants.rolePrompt")}
            <Textarea
              value={rolePrompt}
              onChange={(event) => setRolePrompt(event.target.value)}
              placeholder={t("participants.rolePromptPlaceholder")}
              style={{ minHeight: 90 }}
            />
          </Field>

          {/* Beside the role prompt rather than beside the endpoint: both say what
              this speaker is given to work with, while the endpoint and the model
              say where its turn runs. The checkbox and its words form one press
              target, and the (?) sits beside it, so pointing at the hint cannot
              flip the setting. */}
          <HintRow>
            <Checkbox checked={receivesProjectMaterial} onChange={setReceivesProjectMaterial}>
              {t("participants.receivesProjectMaterial")}
            </Checkbox>
            <Hint
              name={t("hint.about", { label: t("participants.receivesProjectMaterial") })}
              body={t("participants.receivesProjectMaterialHint")}
            />
          </HintRow>

          <Row style={{ justifyContent: "flex-end" }}>
            <ActionButton
              type="button"
              variant="normal"
              icon={<CheckIcon />}
              busy={savingProfile}
              disabledReason={savingProfile ? undefined : profileReason}
              onClick={() => void handleSaveProfile()}
            >
              {t("participants.save")}
            </ActionButton>
          </Row>

          <Divider />

          {/* The check button shares the endpoint's row and carries no figure
              (owner's call, 2026-09-26: the column is narrow, the space matters
              more than the busy figure keeping the width). */}
          <HintedField label={t("participants.baseUrl")} hint={t("participants.baseUrlHint")}>
            {(id) => (
              <Row style={{ alignItems: "center", flexWrap: "nowrap" }}>
                <Input
                  id={id}
                  value={baseUrl}
                  onChange={(event) => setBaseUrl(event.target.value)}
                  placeholder={t("participants.baseUrlPlaceholder")}
                  style={{ flex: 1, minWidth: 0 }}
                />
                <ActionButton
                  type="button"
                  variant="normal"
                  busy={probe?.state === "checking"}
                  disabledReason={baseUrl.trim() ? undefined : t("participants.baseUrlRequired")}
                  onClick={() => void props.onProbe(baseUrl)}
                >
                  {t("participants.checkConnection")}
                </ActionButton>
              </Row>
            )}
          </HintedField>
          {probe?.state === "ok" ? (
            <Row>
              <Badge tone="accent">{t("participants.connected", { count: probe.models.length })}</Badge>
            </Row>
          ) : null}
          {probe?.state === "failed" ? (
            <FailureNotice>{probe.error ?? t("participants.connectionFailed")}</FailureNotice>
          ) : null}

          <HintedField label={t("participants.model")} hint={t("participants.modelHint")}>
            {(id) =>
              probe?.state === "ok" && probe.models.length ? (
                <Select id={id} value={modelName} onChange={(event) => setModelName(event.target.value)}>
                  <option value="">{t("participants.pickModel")}</option>
                  {probe.models.map((model) => (
                    <option key={model} value={model}>
                      {model}
                    </option>
                  ))}
                </Select>
              ) : (
                <Input
                  id={id}
                  value={modelName}
                  onChange={(event) => setModelName(event.target.value)}
                  placeholder={t("participants.modelPlaceholder")}
                />
              )
            }
          </HintedField>

          <Row style={{ justifyContent: "flex-end" }}>
            <ActionButton
              type="button"
              variant="normal"
              icon={<CheckIcon />}
              busy={savingConnection}
              disabledReason={savingConnection ? undefined : connectionReason}
              onClick={() => void handleSaveConnection()}
            >
              {t("participants.save")}
            </ActionButton>
          </Row>
        </Stack>
      </CollapsibleSection>

      {/* Below the fold, not inside it: a move or a removal can fail while the
          card is folded, and its failure must not be folded with the fields. */}
      {props.error ? <FailureNotice>{props.error}</FailureNotice> : null}
      </Stack>
    </PanelCard>
  );
}
