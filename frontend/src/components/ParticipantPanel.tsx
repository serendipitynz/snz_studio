import styled from "@emotion/styled";
import { FormEvent, useState } from "react";
import { api, ChatRecord, Participant, TurnRule } from "../api/client";
import { useConfirm } from "./ConfirmDialog";
import { PresetChoice, PresetPicker } from "./PresetPicker";
import { useLanguage } from "../i18n";
import {
  Badge,
  Button,
  Card,
  ErrorText,
  Field,
  IconButton,
  Input,
  MetaText,
  Row,
  SectionTitle,
  Select,
  Stack,
  Subtle,
  Textarea
} from "../styles/ui";

interface ParticipantPanelProps {
  chat: ChatRecord;
  participants: Participant[];
  onChatChange: (chat: ChatRecord) => void;
  onParticipantsChange: (participants: Participant[]) => void;
  disabled: boolean;
  // True while the conversation has no messages, which is the only time a preset
  // may be applied: it replaces the roster, the turn rule and the scene, and a
  // transcript would be left referring to a cast the chat no longer has. The
  // server refuses it either way; this only keeps the control off screen.
  canApplyPreset: boolean;
}

// The fallback rule of a blank endpoint / model hangs off a (?) beside the label
// rather than sitting in the field's placeholder or under the field. A
// placeholder is clipped by this panel's width and disappears once the field has
// a value — which is exactly when a roster is being reviewed — while a permanent
// hint line costs four lines of a narrow panel to say something that is read
// once. The bubble spans the row rather than being sized to its text, so it can
// never overflow the pane sideways however long a translation runs.
const HintRow = styled.div`
  position: relative;
  display: flex;
  align-items: center;
  gap: 6px;

  button:hover + [role="tooltip"],
  button:focus + [role="tooltip"] {
    opacity: 1;
    visibility: visible;
  }
`;

const HintToggle = styled.button`
  width: 16px;
  height: 16px;
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid ${({ theme }) => theme.fieldBorder};
  border-radius: 999px;
  background: transparent;
  color: ${({ theme }) => theme.muted};
  font: inherit;
  font-size: 11px;
  line-height: 1;
  cursor: help;
  padding: 0;
`;

// Opens upward. Downward it lands on the very input it describes, so reading the
// hint hid what had been typed into the field — and the field is focused exactly
// when someone reaches for the hint. Upward it covers the control above instead,
// which is never the one in use.
const HintBubble = styled.span`
  position: absolute;
  bottom: calc(100% + 6px);
  left: 0;
  right: 0;
  z-index: 3;
  padding: 8px 10px;
  border: 1px solid ${({ theme }) => theme.lineMedium};
  border-radius: 10px;
  background: ${({ theme }) => theme.surfacePane};
  box-shadow: ${({ theme }) => theme.shadowPopover};
  color: ${({ theme }) => theme.ink};
  font-size: 12px;
  line-height: 1.5;
  opacity: 0;
  visibility: hidden;
  transition: opacity 120ms ease;
  pointer-events: none;
`;

// label is the Field itself, so a click inside it is forwarded to the input and
// takes the focus straight back off the toggle. Cancelling that is what makes
// the tooltip reachable without a hover.
function FieldHint(props: { label: string; hint: string }) {
  return (
    <HintRow>
      {props.label}
      <HintToggle type="button" aria-label={props.hint} onClick={(event) => event.preventDefault()}>
        ?
      </HintToggle>
      <HintBubble role="tooltip">{props.hint}</HintBubble>
    </HintRow>
  );
}

// A per-endpoint probe result. Listing an endpoint's models is both the model
// picker's source and the connection check the design asks for (§6): the API has
// no separate check route, and an endpoint that answers with its model list is
// exactly an endpoint a turn can run against.
interface EndpointProbe {
  state: "checking" | "ok" | "failed";
  models: string[];
  error?: string;
}

// ParticipantPanel is the organisation panel of docs/multi-agent-chat-design.md
// §6: the participant CRUD with its endpoint check and model picker, plus the
// chat-level turn rule and scene prompt.
export function ParticipantPanel(props: ParticipantPanelProps) {
  const { t } = useLanguage();
  const confirm = useConfirm();
  const [probes, setProbes] = useState<Record<string, EndpointProbe>>({});
  const [newDisplayName, setNewDisplayName] = useState("");
  const [adding, setAdding] = useState(false);
  const [presetChoice, setPresetChoice] = useState<PresetChoice | null>(null);
  const [applyingPreset, setApplyingPreset] = useState(false);
  const [error, setError] = useState("");

  const roster = props.participants.filter((participant) => participant.deletedAt === null);
  const removed = props.participants.filter((participant) => participant.deletedAt !== null);

  async function probeEndpoint(baseUrl: string) {
    const key = baseUrl.trim();
    setProbes((current) => ({ ...current, [key]: { state: "checking", models: [] } }));
    try {
      const response = await api.listConfigurationModels({ kind: "llm", baseUrl: key });
      setProbes((current) => ({ ...current, [key]: { state: "ok", models: response.models } }));
    } catch (nextError) {
      setProbes((current) => ({
        ...current,
        [key]: {
          state: "failed",
          models: [],
          error: nextError instanceof Error ? nextError.message : t("participants.connectionFailed")
        }
      }));
    }
  }

  async function handleAddParticipant(event: FormEvent) {
    event.preventDefault();
    const displayName = newDisplayName.trim();
    if (!displayName) {
      return;
    }

    setAdding(true);
    setError("");
    try {
      const response = await api.createParticipant(props.chat.id, { displayName });
      props.onParticipantsChange([...props.participants, response.participant]);
      setNewDisplayName("");
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("participants.saveError"));
    } finally {
      setAdding(false);
    }
  }

  async function handleSaveParticipant(participantId: string, input: Parameters<typeof api.updateParticipant>[1]) {
    setError("");
    try {
      const response = await api.updateParticipant(participantId, input);
      props.onParticipantsChange(
        props.participants.map((participant) => (participant.id === participantId ? response.participant : participant))
      );
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("participants.saveError"));
      throw nextError;
    }
  }

  async function handleRemoveParticipant(participant: Participant) {
    if (!(await confirm(t("participants.removeConfirm", { name: participant.displayName })))) {
      return;
    }

    setError("");
    try {
      const response = await api.removeParticipant(participant.id);
      props.onParticipantsChange(
        props.participants.map((current) => (current.id === participant.id ? response.participant : current))
      );
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("participants.saveError"));
    }
  }

  // Reordering swaps the two neighbours' sort_order rather than renumbering the
  // roster, so a move is two writes regardless of how long the roster is and no
  // other participant's order changes under it.
  async function handleMove(index: number, direction: -1 | 1) {
    const target = roster[index];
    const neighbour = roster[index + direction];
    if (!target || !neighbour) {
      return;
    }

    setError("");
    try {
      const [first, second] = await Promise.all([
        api.updateParticipant(target.id, { sortOrder: neighbour.sortOrder }),
        api.updateParticipant(neighbour.id, { sortOrder: target.sortOrder })
      ]);
      const updated = new Map([
        [first.participant.id, first.participant],
        [second.participant.id, second.participant]
      ]);
      props.onParticipantsChange(
        props.participants
          .map((participant) => updated.get(participant.id) ?? participant)
          .sort((left, right) => left.sortOrder - right.sortOrder)
      );
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("participants.saveError"));
    }
  }

  async function handleApplyPreset() {
    if (!presetChoice) {
      return;
    }
    // Only when there is something to lose: applying to the empty roster the
    // sidebar creates is the ordinary path and asking there would be noise.
    if (roster.length > 0 && !(await confirm(t("preset.applyConfirm", { count: roster.length })))) {
      return;
    }

    setApplyingPreset(true);
    setError("");
    try {
      const response = await api.applyMultiAgentPreset(props.chat.id, presetChoice.selection);
      props.onChatChange(response.chat);
      props.onParticipantsChange(response.participants);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("preset.applyError"));
    } finally {
      setApplyingPreset(false);
    }
  }

  async function handleChangeTurnRule(turnRule: TurnRule) {
    setError("");
    try {
      const response = await api.updateChatMultiAgentSettings(props.chat.id, { turnRule });
      props.onChatChange(response.chat);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("participants.settingsSaveError"));
    }
  }

  async function handleSaveScenePrompt(scenePrompt: string) {
    setError("");
    try {
      const response = await api.updateChatMultiAgentSettings(props.chat.id, { scenePrompt });
      props.onChatChange(response.chat);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("participants.settingsSaveError"));
      throw nextError;
    }
  }

  return (
    <Stack>
      <SectionTitle>{t("participants.title")}</SectionTitle>
      {error ? <ErrorText>{error}</ErrorText> : null}

      {props.canApplyPreset ? (
        <Card>
          {/* Collapsed by default, and the open state is deliberately not kept: a
              chat created from a preset already has its line-up, so an expanded
              section above the roster would be shouting about a decision already
              taken. Opening it is one click when the preset is what is wanted. */}
          <details>
            <summary>{t("preset.applyTitle")}</summary>
            <Stack style={{ marginTop: 10 }}>
              <Subtle style={{ margin: 0 }}>{t("preset.applyNote")}</Subtle>
              <PresetPicker disabled={props.disabled || applyingPreset} onChange={setPresetChoice} />
              <div>
                <Button
                  type="button"
                  disabled={!presetChoice || applyingPreset || props.disabled}
                  onClick={() => void handleApplyPreset()}
                >
                  {applyingPreset ? t("preset.applying") : t("preset.apply")}
                </Button>
              </div>
            </Stack>
          </details>
        </Card>
      ) : null}

      <Card>
        <Stack>
          <Badge tone="accent">{t("participants.settings")}</Badge>
          <Field>
            {t("participants.turnRule")}
            <Select
              value={props.chat.turnRule}
              disabled={props.disabled}
              onChange={(event) => void handleChangeTurnRule(event.target.value as TurnRule)}
            >
              <option value="round_robin">{t("participants.turnRuleRoundRobin")}</option>
              <option value="manual">{t("participants.turnRuleManual")}</option>
            </Select>
          </Field>
          <ScenePromptField
            value={props.chat.scenePrompt}
            disabled={props.disabled}
            onSave={handleSaveScenePrompt}
          />
        </Stack>
      </Card>

      <Card>
        <Stack>
          <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
            <Badge tone="warm">{t("participants.roster")}</Badge>
            <MetaText style={{ opacity: 0.72 }}>{t("participants.orderNote")}</MetaText>
          </Row>

          {roster.length === 0 ? <Subtle>{t("multiAgent.rosterEmpty")}</Subtle> : null}

          {roster.map((participant, index) => (
            <ParticipantEditor
              key={participant.id}
              participant={participant}
              probes={probes}
              disabled={props.disabled}
              canMoveUp={index > 0}
              canMoveDown={index < roster.length - 1}
              onProbe={probeEndpoint}
              onSave={handleSaveParticipant}
              onRemove={handleRemoveParticipant}
              onMove={(direction) => void handleMove(index, direction)}
            />
          ))}

          <Card as="form" onSubmit={handleAddParticipant}>
            <Stack>
              <Field>
                {t("participants.displayName")}
                <Input
                  value={newDisplayName}
                  onChange={(event) => setNewDisplayName(event.target.value)}
                  placeholder={t("participants.displayNamePlaceholder")}
                />
              </Field>
              <div>
                <Button type="submit" disabled={adding || props.disabled || !newDisplayName.trim()}>
                  {adding ? t("participants.adding") : t("participants.add")}
                </Button>
              </div>
            </Stack>
          </Card>
        </Stack>
      </Card>

      {removed.length ? (
        <Card>
          <details>
            <summary>{t("participants.removed", { count: removed.length })}</summary>
            <Stack style={{ marginTop: 10 }}>
              {removed.map((participant) => (
                <Subtle key={participant.id}>
                  {participant.displayName}
                  {participant.modelName ? ` — ${participant.modelName}` : ""}
                </Subtle>
              ))}
            </Stack>
          </details>
        </Card>
      ) : null}
    </Stack>
  );
}

function ScenePromptField(props: { value: string; disabled: boolean; onSave: (value: string) => Promise<void> }) {
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
      <div>
        <Button type="button" variant="ghost" disabled={saving || props.disabled || draft === props.value} onClick={() => void handleSave()}>
          {saving ? t("participants.saving") : t("participants.save")}
        </Button>
      </div>
    </Stack>
  );
}

interface ParticipantEditorProps {
  participant: Participant;
  // Keyed by endpoint rather than by participant, so a check made for one
  // participant already answers for every other pointed at the same server.
  probes: Record<string, EndpointProbe>;
  disabled: boolean;
  canMoveUp: boolean;
  canMoveDown: boolean;
  onProbe: (baseUrl: string) => Promise<void>;
  onSave: (participantId: string, input: Parameters<typeof api.updateParticipant>[1]) => Promise<void>;
  onRemove: (participant: Participant) => Promise<void>;
  onMove: (direction: -1 | 1) => void;
}

function ParticipantEditor(props: ParticipantEditorProps) {
  const { t } = useLanguage();
  const [displayName, setDisplayName] = useState(props.participant.displayName);
  const [rolePrompt, setRolePrompt] = useState(props.participant.rolePrompt);
  const [baseUrl, setBaseUrl] = useState(props.participant.baseUrl);
  const [modelName, setModelName] = useState(props.participant.modelName);
  const [saving, setSaving] = useState(false);

  const dirty =
    displayName !== props.participant.displayName ||
    rolePrompt !== props.participant.rolePrompt ||
    baseUrl !== props.participant.baseUrl ||
    modelName !== props.participant.modelName;

  async function handleSave() {
    setSaving(true);
    try {
      await props.onSave(props.participant.id, { displayName, rolePrompt, baseUrl, modelName });
    } catch {
      // Reported by the panel; the drafts stay so the edit survives the failure.
    } finally {
      setSaving(false);
    }
  }

  // The draft endpoint, not props.participant.baseUrl: the check runs against
  // what is typed in, so an endpoint entered but not yet saved would otherwise
  // never find its own result.
  const probe = props.probes[baseUrl.trim()];

  return (
    <Card>
      <Stack>
        <Row style={{ justifyContent: "space-between", alignItems: "center", flexWrap: "nowrap" }}>
          <strong style={{ minWidth: 0, overflowWrap: "anywhere" }}>{props.participant.displayName}</strong>
          <Row style={{ alignItems: "center", flexWrap: "nowrap", gap: 4 }}>
            <IconButton
              type="button"
              aria-label={t("participants.moveUp")}
              title={t("participants.moveUp")}
              disabled={!props.canMoveUp || props.disabled}
              onClick={() => props.onMove(-1)}
            >
              <ArrowUpIcon />
            </IconButton>
            <IconButton
              type="button"
              aria-label={t("participants.moveDown")}
              title={t("participants.moveDown")}
              disabled={!props.canMoveDown || props.disabled}
              onClick={() => props.onMove(1)}
            >
              <ArrowDownIcon />
            </IconButton>
            <IconButton
              type="button"
              aria-label={t("participants.remove")}
              title={t("participants.remove")}
              disabled={props.disabled}
              onClick={() => void props.onRemove(props.participant)}
            >
              <TrashIcon />
            </IconButton>
          </Row>
        </Row>

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

        <Field>
          <FieldHint label={t("participants.baseUrl")} hint={t("participants.baseUrlHint")} />
          <Input
            value={baseUrl}
            onChange={(event) => setBaseUrl(event.target.value)}
            placeholder={t("participants.baseUrlPlaceholder")}
          />
        </Field>

        <Row style={{ alignItems: "center" }}>
          <Button
            type="button"
            variant="ghost"
            disabled={!baseUrl.trim() || probe?.state === "checking"}
            onClick={() => void props.onProbe(baseUrl)}
          >
            {probe?.state === "checking" ? t("participants.checking") : t("participants.checkConnection")}
          </Button>
          {probe?.state === "ok" ? (
            <Badge tone="accent">{t("participants.connected", { count: probe.models.length })}</Badge>
          ) : null}
          {probe?.state === "failed" ? <Badge tone="warm">{probe.error ?? t("participants.connectionFailed")}</Badge> : null}
        </Row>

        <Field>
          <FieldHint label={t("participants.model")} hint={t("participants.modelHint")} />
          {probe?.state === "ok" && probe.models.length ? (
            <Select value={modelName} onChange={(event) => setModelName(event.target.value)}>
              <option value="">{t("participants.pickModel")}</option>
              {probe.models.map((model) => (
                <option key={model} value={model}>
                  {model}
                </option>
              ))}
            </Select>
          ) : (
            <Input
              value={modelName}
              onChange={(event) => setModelName(event.target.value)}
              placeholder={t("participants.modelPlaceholder")}
            />
          )}
        </Field>

        <div>
          <Button type="button" disabled={!dirty || saving || props.disabled} onClick={() => void handleSave()}>
            {saving ? t("participants.saving") : t("participants.save")}
          </Button>
        </div>
      </Stack>
    </Card>
  );
}

function ArrowUpIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M8 12.5v-9M4.5 7 8 3.5 11.5 7" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function ArrowDownIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M8 3.5v9M4.5 9 8 12.5 11.5 9" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function TrashIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M3.5 4.5h9M6.5 4.5V3.25h3V4.5M5 4.5l.5 8h5l.5-8"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
