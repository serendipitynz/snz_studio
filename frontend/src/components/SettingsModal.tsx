import { FormEvent, useEffect, useRef, useState } from "react";
import { api, EmbeddingRebuildState, EmbeddingStatus, ReasoningChoice, WorkspaceConfiguration } from "../api/client";
import { Language, MessageKey, useLanguage } from "../i18n";
import { ThemeMode, useThemeController } from "../styles/ThemeController";
import type { ThemeFamily } from "../styles/themes";
import { CheckForUpdate, GetAutoCheckUpdates, GetVersion, SetAutoCheckUpdates } from "../wailsjs/go/main/App";
import { ActionButton } from "./ActionButton";
import { announce } from "./announce";
import { Checkbox } from "./Checkbox";
import { useConfirm } from "./ConfirmDialog";
import { Dialog, DialogBody, DialogHeader, DialogTitle } from "./Dialog";
import { FailureNotice, InfoNotice } from "./FailureNotice";
import { CheckIcon, RotateCwIcon } from "./icons";
import { Progress } from "./Progress";
import { pollRebuildState } from "./rebuildPoll";
import { ReasoningOptions } from "./reasoningOptions";
import { useUpdateOffer } from "./UpdateDialog";
import { AvailableUpdate, availableUpdate } from "./updateCheck";
import {
  Card,
  Field,
  FieldHeader,
  Input,
  Select,
  Stack,
  StateBadge,
  SubsectionTitle,
  Subtle
} from "../styles/ui";

interface SettingsModalProps {
  onClose: () => void;
}

type Translate = (key: MessageKey, vars?: Record<string, string | number>) => string;

type ConfigDraft = Pick<
  WorkspaceConfiguration,
  | "llmBaseUrl"
  | "llmModel"
  | "llmResponseFormat"
  | "reviewBaseUrl"
  | "reviewModel"
  | "embeddingBaseUrl"
  | "embeddingModel"
  | "embeddingMode"
  | "imageDescriptionBaseUrl"
  | "imageDescriptionModel"
>;

const EMPTY_DRAFT: ConfigDraft = {
  llmBaseUrl: "",
  llmModel: "",
  llmResponseFormat: "standard",
  reviewBaseUrl: "",
  reviewModel: "",
  embeddingBaseUrl: "",
  embeddingModel: "",
  embeddingMode: "internal",
  imageDescriptionBaseUrl: "",
  imageDescriptionModel: ""
};

function draftFrom(configuration: WorkspaceConfiguration): ConfigDraft {
  return {
    llmBaseUrl: configuration.llmBaseUrl,
    llmModel: configuration.llmModel,
    llmResponseFormat: configuration.llmResponseFormat,
    reviewBaseUrl: configuration.reviewBaseUrl,
    reviewModel: configuration.reviewModel,
    embeddingBaseUrl: configuration.embeddingBaseUrl,
    embeddingModel: configuration.embeddingModel,
    embeddingMode: configuration.embeddingMode,
    imageDescriptionBaseUrl: configuration.imageDescriptionBaseUrl,
    imageDescriptionModel: configuration.imageDescriptionModel
  };
}

function sameDraft(a: ConfigDraft, b: ConfigDraft): boolean {
  return (Object.keys(a) as (keyof ConfigDraft)[]).every((key) => a[key] === b[key]);
}

function megabytes(bytes: number): string {
  return (bytes / 1_000_000).toFixed(1);
}

// The download reads as a progress band (snz-design doc-8 §6.7.1): with the size in
// hand the band fills and the amount sits beside the words; before the first bytes
// tell the size, a mark flows instead of an empty band that would read as stalled.
function EmbeddingDownload({ t, status }: { t: Translate; status: EmbeddingStatus }) {
  const label = t("settings.embedDownloadingLabel");
  const known = status.total > 0;
  const readout = known
    ? t("settings.embedDownloadAmount", {
        done: megabytes(status.downloaded),
        total: megabytes(status.total),
        pct: Math.floor((status.downloaded / status.total) * 100)
      })
    : undefined;

  // Read out when the download starts and when its size becomes known, not on every
  // poll: the amount changes every two seconds and would drown out other speech.
  const announcedRef = useRef<"started" | "known" | null>(null);
  useEffect(() => {
    if (announcedRef.current === null) {
      announcedRef.current = known ? "known" : "started";
      announce(readout ? `${label} ${readout}` : label);
    } else if (announcedRef.current === "started" && known) {
      announcedRef.current = "known";
      announce(`${label} ${readout}`);
    }
  }, [known, label, readout]);

  return (
    <>
      <Progress
        label={label}
        total={known ? status.total : undefined}
        done={status.downloaded}
        readout={readout}
        fullWidth
      />
      <Subtle>{t("settings.embedKeywordMeanwhile")}</Subtle>
    </>
  );
}

// embeddingStatusLabel renders the internal sidecar's lifecycle into a short status
// line, reassuring the user that keyword search keeps working while the model loads.
function embeddingStatusLabel(t: Translate, status: EmbeddingStatus | null): string {
  if (!status) {
    return t("settings.embedPreparing");
  }
  switch (status.state) {
    case "starting":
      return t("settings.embedStarting");
    case "ready":
      return t("settings.embedReady");
    case "error":
      return t("settings.embedError", { detail: status.error ? ` (${status.error})` : "" });
    default:
      return t("settings.embedKeywordOnly");
  }
}

type CheckOutcome = { kind: "upToDate" } | { kind: "available"; update: AvailableUpdate } | { kind: "failed" };

// The running version, the startup check's switch, and a check on demand. The
// switch saves as it is changed, like the theme and the language above it.
function UpdatesSection({ t }: { t: Translate }) {
  const offerUpdate = useUpdateOffer();
  const [version, setVersion] = useState<string | null>(null);
  const [autoCheck, setAutoCheck] = useState<boolean | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [autoCheckSaveFailed, setAutoCheckSaveFailed] = useState(false);
  const [checking, setChecking] = useState(false);
  const [outcome, setOutcome] = useState<CheckOutcome | null>(null);

  useEffect(() => {
    let active = true;
    Promise.all([GetVersion(), GetAutoCheckUpdates()])
      .then(([running, enabled]) => {
        if (!active) {
          return;
        }
        setVersion(running);
        setAutoCheck(enabled);
      })
      .catch(() => {
        if (active) {
          setLoadFailed(true);
        }
      });
    return () => {
      active = false;
    };
  }, []);

  async function changeAutoCheck(enabled: boolean) {
    const previous = autoCheck;
    setAutoCheck(enabled);
    setAutoCheckSaveFailed(false);
    try {
      await SetAutoCheckUpdates(enabled);
    } catch {
      setAutoCheck(previous);
      setAutoCheckSaveFailed(true);
    }
  }

  async function checkNow() {
    setChecking(true);
    setOutcome(null);
    announce(t("settings.checkingForUpdates"));
    let next: CheckOutcome;
    try {
      const check = await CheckForUpdate();
      const update = availableUpdate(check);
      next = update ? { kind: "available", update } : check.status === "upToDate" ? { kind: "upToDate" } : { kind: "failed" };
    } catch {
      next = { kind: "failed" };
    }
    setChecking(false);
    setOutcome(next);
    if (next.kind === "upToDate") {
      announce(t("settings.upToDate"));
    } else if (next.kind === "available") {
      offerUpdate(next.update);
    }
  }

  return (
    <Card>
      <Stack>
        <SubsectionTitle>{t("settings.updates")}</SubsectionTitle>
        {loadFailed ? <FailureNotice>{t("settings.updateSettingsLoadFailed")}</FailureNotice> : null}
        {version !== null ? (
          <Subtle>
            {version ? t("settings.runningVersion", { version }) : t("settings.runningVersionUnknown")}
          </Subtle>
        ) : null}
        {autoCheck !== null ? (
          <Checkbox checked={autoCheck} onChange={(enabled) => void changeAutoCheck(enabled)}>
            {t("settings.autoCheckUpdates")}
          </Checkbox>
        ) : null}
        <Subtle>{t("settings.autoCheckUpdatesNote")}</Subtle>
        {autoCheckSaveFailed ? <FailureNotice>{t("settings.autoCheckSaveFailed")}</FailureNotice> : null}
        {outcome?.kind === "upToDate" ? <Subtle>{t("settings.upToDate")}</Subtle> : null}
        {outcome?.kind === "available" ? (
          <Subtle>{t("settings.updateAvailable", { version: outcome.update.version })}</Subtle>
        ) : null}
        {outcome?.kind === "failed" ? <FailureNotice>{t("settings.updateCheckFailed")}</FailureNotice> : null}
        <div style={{ display: "flex", flexWrap: "wrap", justifyContent: "flex-end", gap: 10 }}>
          {outcome?.kind === "available" ? (
            <ActionButton type="button" variant="normal" onClick={() => offerUpdate(outcome.update)}>
              {t("settings.openUpdate")}
            </ActionButton>
          ) : null}
          <ActionButton
            type="button"
            variant="normal"
            icon={<RotateCwIcon />}
            busy={checking}
            onClick={() => void checkNow()}
          >
            {t("settings.checkForUpdates")}
          </ActionButton>
        </div>
      </Stack>
    </Card>
  );
}

export function SettingsModal({ onClose }: SettingsModalProps) {
  const { lang, setLang, t } = useLanguage();
  const { family, mode, families, setFamily, setMode, storedChoiceUnknown, saveFailed } = useThemeController();
  const confirm = useConfirm();
  const [configuration, setConfiguration] = useState<WorkspaceConfiguration | null>(null);
  const [configDraft, setConfigDraft] = useState<ConfigDraft>(EMPTY_DRAFT);
  const [embeddingStatus, setEmbeddingStatus] = useState<EmbeddingStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [savingConfig, setSavingConfig] = useState(false);
  const [llmModelOptions, setLlmModelOptions] = useState<string[]>([]);
  // Kept with the endpoint they were read from, so a list still being fetched for
  // a changed endpoint is not read as that endpoint's.
  const [llmReasoning, setLlmReasoning] = useState<{ baseUrl: string; choices: Record<string, ReasoningChoice> }>({
    baseUrl: "",
    choices: {}
  });
  // The reasoning value is saved per endpoint and model, so the draft remembers
  // which one it was chosen for and applies only while the draft still names it.
  const [reasoningDraft, setReasoningDraft] = useState<{ baseUrl: string; model: string; value: string } | null>(null);
  const [reviewModelOptions, setReviewModelOptions] = useState<string[]>([]);
  const [embeddingModelOptions, setEmbeddingModelOptions] = useState<string[]>([]);
  const [loadingLlmModels, setLoadingLlmModels] = useState(false);
  const [loadingReviewModels, setLoadingReviewModels] = useState(false);
  const [loadingEmbeddingModels, setLoadingEmbeddingModels] = useState(false);
  const [imageDescriptionModelOptions, setImageDescriptionModelOptions] = useState<string[]>([]);
  const [loadingImageDescriptionModels, setLoadingImageDescriptionModels] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [saveError, setSaveError] = useState("");
  const [rebuilding, setRebuilding] = useState(false);
  const [rebuildState, setRebuildState] = useState<EmbeddingRebuildState | null>(null);
  // Bumped to read the rebuild state again after an action that may have started one.
  const [rebuildWatch, setRebuildWatch] = useState(0);
  const [rebuildError, setRebuildError] = useState("");
  // Dismissed for this opening only: the stored value is still unknown, so the
  // note returns the next time the modal opens (snz-design doc-9 §6.4).
  const [unknownChoiceDismissed, setUnknownChoiceDismissed] = useState(false);
  const familySelectRef = useRef<HTMLSelectElement | null>(null);

  useEffect(() => {
    let active = true;
    setLoading(true);
    api
      .getConfiguration()
      .then((response) => {
        if (!active) return;
        setConfiguration(response.configuration);
        // The load waits on the connection checks and can take seconds; a field the
        // user already edited keeps the edit, which then counts as unsaved.
        const loaded = draftFrom(response.configuration);
        setConfigDraft((current) => {
          const merged = { ...loaded };
          for (const key of Object.keys(current) as (keyof ConfigDraft)[]) {
            if (current[key] !== EMPTY_DRAFT[key]) {
              Object.assign(merged, { [key]: current[key] });
            }
          }
          return merged;
        });
      })
      .catch((nextError) => {
        if (!active) return;
        setLoadError(nextError instanceof Error ? nextError.message : t("settings.loadError"));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Poll the internal embedding sidecar's status while it is downloading/starting so
  // the modal can show progress and switch the messaging to "ready" once it is live.
  useEffect(() => {
    if (configuration?.embeddingMode !== "internal") {
      setEmbeddingStatus(null);
      return;
    }
    let active = true;
    let timer = 0;
    const tick = async () => {
      try {
        const status = await api.getEmbeddingStatus();
        if (!active) return;
        setEmbeddingStatus(status);
        if (status.state === "downloading" || status.state === "starting") {
          timer = window.setTimeout(tick, 2000);
        }
      } catch {
        /* transient; the next user action will refresh */
      }
    };
    void tick();
    return () => {
      active = false;
      if (timer) window.clearTimeout(timer);
    };
  }, [configuration?.embeddingMode]);

  // The rebuild runs on the server, so its state is read from there rather than
  // kept from the button press: a rebuild a save or startup began holds the button
  // too, and the outcome still shows after the modal is closed and opened again.
  useEffect(
    () =>
      pollRebuildState(async () => (await api.getEmbeddingRebuild()).state, setRebuildState),
    [rebuildWatch]
  );

  // Read out the end of a rebuild this opening watched run; an outcome already
  // there when the modal opened is only shown.
  const watchedRebuildRef = useRef(false);
  useEffect(() => {
    if (rebuildState === "running") {
      watchedRebuildRef.current = true;
    } else if (watchedRebuildRef.current && (rebuildState === "done" || rebuildState === "incomplete")) {
      watchedRebuildRef.current = false;
      announce(
        t(rebuildState === "done" ? "settings.rebuildEmbeddingsDone" : "settings.rebuildEmbeddingsIncomplete")
      );
    }
  }, [rebuildState, t]);

  useEffect(() => {
    if (!configDraft.llmBaseUrl.trim()) {
      return;
    }

    const baseUrl = configDraft.llmBaseUrl.trim();
    // A request still in flight when the endpoint changes must not land: its
    // choices would replace the new endpoint's and drop a Think value chosen
    // there before it is saved.
    let active = true;
    const timeout = window.setTimeout(() => {
      setLoadingLlmModels(true);
      api
        .listConfigurationModels({ kind: "llm", baseUrl })
        .then((response) => {
          if (!active) return;
          setLlmModelOptions(response.models);
          setLlmReasoning({ baseUrl, choices: response.reasoning });
        })
        .catch(() => {
          if (!active) return;
          setLlmModelOptions([]);
          setLlmReasoning({ baseUrl, choices: {} });
        })
        // Cleared even when superseded: an endpoint emptied meanwhile starts no
        // request that would clear it.
        .finally(() => setLoadingLlmModels(false));
    }, 250);

    return () => {
      active = false;
      window.clearTimeout(timeout);
    };
  }, [configDraft.llmBaseUrl]);

  useEffect(() => {
    if (!configDraft.reviewBaseUrl.trim()) {
      return;
    }

    const timeout = window.setTimeout(() => {
      setLoadingReviewModels(true);
      api
        .listConfigurationModels({ kind: "llm", baseUrl: configDraft.reviewBaseUrl })
        .then((response) => setReviewModelOptions(response.models))
        .catch(() => setReviewModelOptions([]))
        .finally(() => setLoadingReviewModels(false));
    }, 250);

    return () => window.clearTimeout(timeout);
  }, [configDraft.reviewBaseUrl]);

  useEffect(() => {
    if (!configDraft.embeddingBaseUrl.trim()) {
      return;
    }

    const timeout = window.setTimeout(() => {
      setLoadingEmbeddingModels(true);
      api
        .listConfigurationModels({ kind: "embedding", baseUrl: configDraft.embeddingBaseUrl })
        .then((response) => setEmbeddingModelOptions(response.models))
        .catch(() => setEmbeddingModelOptions([]))
        .finally(() => setLoadingEmbeddingModels(false));
    }, 250);

    return () => window.clearTimeout(timeout);
  }, [configDraft.embeddingBaseUrl]);

  // An empty image-description endpoint follows the LLM endpoint, so the candidates
  // come from whichever one will actually be called.
  const imageDescriptionEndpoint = configDraft.imageDescriptionBaseUrl.trim() || configDraft.llmBaseUrl.trim();

  useEffect(() => {
    if (!imageDescriptionEndpoint) {
      return;
    }

    const timeout = window.setTimeout(() => {
      setLoadingImageDescriptionModels(true);
      api
        .listConfigurationModels({ kind: "llm", baseUrl: imageDescriptionEndpoint })
        .then((response) => setImageDescriptionModelOptions(response.models))
        .catch(() => setImageDescriptionModelOptions([]))
        .finally(() => setLoadingImageDescriptionModels(false));
    }, 250);

    return () => window.clearTimeout(timeout);
  }, [imageDescriptionEndpoint]);

  const llmBaseUrlDraft = configDraft.llmBaseUrl.trim();
  const llmModelDraft = configDraft.llmModel.trim();
  const llmChoice = llmReasoning.baseUrl === llmBaseUrlDraft ? llmReasoning.choices[llmModelDraft] : undefined;
  const reasoningValue =
    reasoningDraft && reasoningDraft.baseUrl === llmBaseUrlDraft && reasoningDraft.model === llmModelDraft
      ? reasoningDraft.value
      : (llmChoice?.selected ?? "");
  const reasoningDirty = llmChoice !== undefined && reasoningValue !== llmChoice.selected;
  const fieldsDirty = !sameDraft(configDraft, configuration ? draftFrom(configuration) : EMPTY_DRAFT);

  async function handleConfigurationSubmit(event: FormEvent) {
    event.preventDefault();
    setSavingConfig(true);
    setSaveError("");

    try {
      if (fieldsDirty) {
        const response = await api.updateConfiguration(configDraft);
        setConfiguration(response.configuration);
        setConfigDraft(draftFrom(response.configuration));
        // A save that changed the embedding source starts a rebuild; read whether this one did.
        setRebuildWatch((current) => current + 1);
      }
      if (reasoningDirty && llmChoice) {
        await api.setReasoning({ baseUrl: llmBaseUrlDraft, model: llmModelDraft, value: reasoningValue });
        setLlmReasoning((current) => ({
          ...current,
          choices: { ...current.choices, [llmModelDraft]: { ...llmChoice, selected: reasoningValue } }
        }));
        setReasoningDraft(null);
      }
    } catch (nextError) {
      setSaveError(nextError instanceof Error ? nextError.message : t("settings.saveError"));
    } finally {
      setSavingConfig(false);
    }
  }

  async function handleRebuildEmbeddings() {
    setRebuilding(true);
    setRebuildError("");
    try {
      await api.rebuildEmbeddings();
      // The server queues the rebuild before it answers, so the button holds from
      // here rather than after the next read.
      setRebuildState("running");
      setRebuildWatch((current) => current + 1);
      announce(t("settings.rebuildEmbeddingsRunning"));
    } catch (nextError) {
      setRebuildError(nextError instanceof Error ? nextError.message : t("settings.rebuildEmbeddingsError"));
    } finally {
      setRebuilding(false);
    }
  }

  // Only the connection settings are a draft: the theme and the language apply as
  // they are chosen. A modal that is saving cannot close, since the result would
  // land on a screen that no longer shows what it was for (doc-9 §6.6). Unsaved
  // connection edits ask before they are thrown away, keeping them by default
  // (doc-9 §5.7).
  const connectionDirty = fieldsDirty || reasoningDirty;

  async function requestClose() {
    if (savingConfig) {
      announce(t("settings.savingClose"));
      return;
    }
    if (
      connectionDirty &&
      !(await confirm(t("discard.message"), {
        heading: t("discard.heading"),
        confirmLabel: t("discard.confirm"),
        cancelLabel: t("discard.keepEditing")
      }))
    ) {
      return;
    }
    onClose();
  }

  function rebuildDisabledReason(): string | undefined {
    if (loading) {
      return t("settings.loadingConfig");
    }
    if (rebuildState === "running") {
      return t("settings.rebuildRunningReason");
    }
    // The reasoning value does not touch the embeddings, so it does not hold the rebuild.
    if (fieldsDirty) {
      return t("settings.rebuildNeedsSave");
    }
    return undefined;
  }

  function saveDisabledReason(): string | undefined {
    if (loading) {
      return t("settings.loadingConfig");
    }
    // Without the stored values every untouched field would be sent empty and
    // wipe what is saved.
    if (!configuration) {
      return t("settings.saveNeedsLoad");
    }
    if (!connectionDirty) {
      return t("settings.unchanged");
    }
    return undefined;
  }

  // The state was checked for the saved values, so it says nothing about an
  // endpoint the draft has changed (keys: the draft fields the check used).
  function connectionBadge(connected: boolean, keys: (keyof ConfigDraft)[]) {
    if (!configuration || keys.some((key) => configDraft[key] !== configuration[key])) {
      return null;
    }
    return (
      <StateBadge tone={connected ? "success" : "danger"}>
        {connected ? t("settings.connected") : t("settings.notConnected")}
      </StateBadge>
    );
  }

  function modelCandidatesLabel(loadingModels: boolean, options: string[]): string {
    if (loadingModels) {
      return t("settings.loadingModels");
    }
    return options.length ? t("settings.candidatesFound", { count: options.length }) : t("settings.noCandidates");
  }

  return (
    <Dialog onClose={() => void requestClose()}>
      <DialogHeader
        actions={
          <ActionButton
            type="button"
            variant="normal"
            disabledReason={savingConfig ? t("settings.savingClose") : undefined}
            onClick={() => void requestClose()}
          >
            {t("common.close")}
          </ActionButton>
        }
      >
        <DialogTitle>{t("settings.title")}</DialogTitle>
      </DialogHeader>
      {/* No footer: each section saves on its own, so its save stays at the end of
          its section rather than in one footer that would not say which it saves. */}
      <DialogBody>

        <Card>
          <Stack>
            <SubsectionTitle>{t("settings.appearance")}</SubsectionTitle>
            {saveFailed ? <FailureNotice>{t("settings.themeSaveFailed")}</FailureNotice> : null}
            {storedChoiceUnknown && !unknownChoiceDismissed ? (
              <InfoNotice
                onDismiss={() => {
                  setUnknownChoiceDismissed(true);
                  familySelectRef.current?.focus();
                }}
              >
                {t("settings.storedChoiceUnknown")}
              </InfoNotice>
            ) : null}
            <Field>
              {t("settings.theme")}
              <Select
                ref={familySelectRef}
                value={family}
                onChange={(event) => setFamily(event.target.value as ThemeFamily)}
              >
                {families.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.id === "standard" ? t("settings.themeStandard") : item.label}
                  </option>
                ))}
              </Select>
            </Field>
            <Field>
              {t("settings.mode")}
              <Select value={mode} onChange={(event) => setMode(event.target.value as ThemeMode)}>
                <option value="light">{t("settings.modeLight")}</option>
                <option value="dark">{t("settings.modeDark")}</option>
                <option value="auto">{t("settings.modeAuto")}</option>
              </Select>
            </Field>
          </Stack>
        </Card>

        <Card>
          <Stack>
            <SubsectionTitle>{t("settings.language")}</SubsectionTitle>
            <Field>
              {t("settings.displayLanguage")}
              <Select value={lang} onChange={(event) => setLang(event.target.value as Language)}>
                <option value="ja">日本語</option>
                <option value="en">English</option>
              </Select>
            </Field>
          </Stack>
        </Card>

        <Card as="form" onSubmit={handleConfigurationSubmit}>
          <Stack>
            <SubsectionTitle>{t("settings.connection")}</SubsectionTitle>
            {loading ? <Subtle>{t("settings.loadingConfig")}</Subtle> : null}
            {loadError ? <FailureNotice>{loadError}</FailureNotice> : null}
            <Field>
              <FieldHeader>
                <span>{t("settings.llmEndpoint")}</span>
                {connectionBadge(Boolean(configuration?.llmConnected), ["llmBaseUrl", "llmModel"])}
              </FieldHeader>
              <Input
                value={configDraft.llmBaseUrl}
                onChange={(event) => setConfigDraft((current) => ({ ...current, llmBaseUrl: event.target.value }))}
                placeholder="http://127.0.0.1:1234/v1"
              />
            </Field>
            <Field>
              {t("settings.llmModel")}
              <Input
                list="settings-llm-model-options"
                value={configDraft.llmModel}
                onChange={(event) => setConfigDraft((current) => ({ ...current, llmModel: event.target.value }))}
                placeholder="openai/gpt-oss-20b"
              />
              <datalist id="settings-llm-model-options">
                {llmModelOptions.map((model) => (
                  <option key={model} value={model} />
                ))}
              </datalist>
              <Subtle>{modelCandidatesLabel(loadingLlmModels, llmModelOptions)}</Subtle>
            </Field>
            {/* Only where LM Studio lists the model with a reasoning switch: a model
                whose reasoning is null, or an endpoint that is not LM Studio, has
                nothing that could be sent. */}
            {llmChoice ? (
              <Field>
                {t("settings.llmReasoning")}
                <Select
                  value={reasoningValue}
                  onChange={(event) =>
                    setReasoningDraft({ baseUrl: llmBaseUrlDraft, model: llmModelDraft, value: event.target.value })
                  }
                >
                  <ReasoningOptions choice={llmChoice} />
                </Select>
                <Subtle>{t("settings.llmReasoningHint")}</Subtle>
              </Field>
            ) : null}
            <Field>
              {t("settings.llmResponseFormat")}
              <Select
                value={configDraft.llmResponseFormat}
                onChange={(event) =>
                  setConfigDraft((current) => ({
                    ...current,
                    llmResponseFormat: event.target.value as "standard" | "llm_jp_thinking"
                  }))
                }
              >
                <option value="standard">{t("settings.formatStandard")}</option>
                <option value="llm_jp_thinking">{t("settings.formatThinking")}</option>
              </Select>
            </Field>
            <Field>
              <FieldHeader>
                <span>{t("settings.reviewEndpoint")}</span>
                {connectionBadge(Boolean(configuration?.reviewConnected), ["reviewBaseUrl", "reviewModel"])}
              </FieldHeader>
              <Input
                value={configDraft.reviewBaseUrl}
                onChange={(event) => setConfigDraft((current) => ({ ...current, reviewBaseUrl: event.target.value }))}
                placeholder="http://127.0.0.1:1234/v1"
              />
            </Field>
            <Field>
              {t("settings.reviewModel")}
              <Input
                list="settings-review-model-options"
                value={configDraft.reviewModel}
                onChange={(event) => setConfigDraft((current) => ({ ...current, reviewModel: event.target.value }))}
                placeholder={t("settings.reviewModelPlaceholder")}
              />
              <datalist id="settings-review-model-options">
                {reviewModelOptions.map((model) => (
                  <option key={model} value={model} />
                ))}
              </datalist>
              <Subtle>{modelCandidatesLabel(loadingReviewModels, reviewModelOptions)}</Subtle>
            </Field>
            <Field>
              <FieldHeader>
                <span>{t("settings.embeddingSource")}</span>
                {configDraft.embeddingMode === "external"
                  ? connectionBadge(Boolean(configuration?.embeddingConnected), [
                      "embeddingMode",
                      "embeddingBaseUrl",
                      "embeddingModel"
                    ])
                  : null}
              </FieldHeader>
              <Select
                value={configDraft.embeddingMode}
                onChange={(event) =>
                  setConfigDraft((current) => ({
                    ...current,
                    embeddingMode: event.target.value as "internal" | "external"
                  }))
                }
              >
                <option value="internal">{t("settings.embeddingInternal")}</option>
                <option value="external">{t("settings.embeddingExternal")}</option>
              </Select>
              {configDraft.embeddingMode === "external" ? (
                <Subtle>{t("settings.embeddingExternalNote")}</Subtle>
              ) : embeddingStatus?.state === "downloading" ? (
                <EmbeddingDownload t={t} status={embeddingStatus} />
              ) : (
                <Subtle>{embeddingStatusLabel(t, embeddingStatus)}</Subtle>
              )}
            </Field>
            {configDraft.embeddingMode === "external" ? (
              <>
                <Field>
                  {t("settings.embeddingEndpoint")}
                  <Input
                    value={configDraft.embeddingBaseUrl}
                    onChange={(event) => setConfigDraft((current) => ({ ...current, embeddingBaseUrl: event.target.value }))}
                    placeholder="http://127.0.0.1:8080/v1"
                  />
                </Field>
                <Field>
                  {t("settings.embeddingModel")}
                  <Input
                    list="settings-embedding-model-options"
                    value={configDraft.embeddingModel}
                    onChange={(event) => setConfigDraft((current) => ({ ...current, embeddingModel: event.target.value }))}
                    placeholder="text-embeddings-inference"
                  />
                  <datalist id="settings-embedding-model-options">
                    {embeddingModelOptions.map((model) => (
                      <option key={model} value={model} />
                    ))}
                  </datalist>
                  <Subtle>{modelCandidatesLabel(loadingEmbeddingModels, embeddingModelOptions)}</Subtle>
                </Field>
              </>
            ) : null}
            <Field>
              <FieldHeader>
                <span>{t("settings.imageDescriptionEndpoint")}</span>
                {/* No badge without a saved model: the feature is then off, not
                    unreachable. An empty endpoint means the LLM endpoint's. */}
                {configuration?.imageDescriptionModel.trim()
                  ? connectionBadge(Boolean(configuration.imageDescriptionConnected), [
                      "imageDescriptionBaseUrl",
                      "imageDescriptionModel",
                      "llmBaseUrl"
                    ])
                  : null}
              </FieldHeader>
              <Input
                value={configDraft.imageDescriptionBaseUrl}
                onChange={(event) =>
                  setConfigDraft((current) => ({ ...current, imageDescriptionBaseUrl: event.target.value }))
                }
                placeholder={t("settings.imageDescriptionEndpointPlaceholder")}
              />
            </Field>
            <Field>
              {t("settings.imageDescriptionModel")}
              <Input
                list="settings-image-description-model-options"
                value={configDraft.imageDescriptionModel}
                onChange={(event) =>
                  setConfigDraft((current) => ({ ...current, imageDescriptionModel: event.target.value }))
                }
                placeholder={t("settings.imageDescriptionModelPlaceholder")}
              />
              <datalist id="settings-image-description-model-options">
                {imageDescriptionModelOptions.map((model) => (
                  <option key={model} value={model} />
                ))}
              </datalist>
              <Subtle>{modelCandidatesLabel(loadingImageDescriptionModels, imageDescriptionModelOptions)}</Subtle>
              <Subtle>{t("settings.imageDescriptionModelNote")}</Subtle>
            </Field>
            {saveError ? <FailureNotice>{saveError}</FailureNotice> : null}
            <div style={{ display: "flex", justifyContent: "flex-end", gap: 10 }}>
              <ActionButton
                type="submit"
                icon={<CheckIcon />}
                busy={savingConfig}
                title={savingConfig ? t("settings.saving") : undefined}
                disabledReason={saveDisabledReason()}
              >
                {t("settings.save")}
              </ActionButton>
            </div>
          </Stack>
        </Card>

        <Card>
          <Stack>
            <SubsectionTitle>{t("settings.rebuildEmbeddings")}</SubsectionTitle>
            <Subtle>{t("settings.rebuildEmbeddingsNote")}</Subtle>
            {rebuildState === "running" ? <Subtle>{t("settings.rebuildEmbeddingsRunning")}</Subtle> : null}
            {rebuildState === "done" ? <Subtle>{t("settings.rebuildEmbeddingsDone")}</Subtle> : null}
            {rebuildState === "incomplete" ? <Subtle>{t("settings.rebuildEmbeddingsIncomplete")}</Subtle> : null}
            {rebuildError ? <FailureNotice>{rebuildError}</FailureNotice> : null}
            <div style={{ display: "flex", justifyContent: "flex-end", gap: 10 }}>
              <ActionButton
                type="button"
                variant="normal"
                icon={<RotateCwIcon />}
                busy={rebuilding}
                disabledReason={rebuildDisabledReason()}
                onClick={() => void handleRebuildEmbeddings()}
              >
                {t("settings.rebuildEmbeddingsAction")}
              </ActionButton>
            </div>
          </Stack>
        </Card>

        <UpdatesSection t={t} />
      </DialogBody>
    </Dialog>
  );
}
