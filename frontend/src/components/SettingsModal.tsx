import { FormEvent, useEffect, useRef, useState } from "react";
import { api, EmbeddingStatus, WorkspaceConfiguration } from "../api/client";
import { Language, MessageKey, useLanguage } from "../i18n";
import { ThemeMode, useThemeController } from "../styles/ThemeController";
import type { ThemeFamily } from "../styles/themes";
import { ActionButton } from "./ActionButton";
import { announce } from "./announce";
import { useConfirm } from "./ConfirmDialog";
import { Dialog, DialogTitle } from "./Dialog";
import { FailureNotice, InfoNotice } from "./FailureNotice";
import { CheckIcon } from "./icons";
import { Progress } from "./Progress";
import {
  Card,
  Field,
  FieldHeader,
  Input,
  Row,
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
  const [reviewModelOptions, setReviewModelOptions] = useState<string[]>([]);
  const [embeddingModelOptions, setEmbeddingModelOptions] = useState<string[]>([]);
  const [loadingLlmModels, setLoadingLlmModels] = useState(false);
  const [loadingReviewModels, setLoadingReviewModels] = useState(false);
  const [loadingEmbeddingModels, setLoadingEmbeddingModels] = useState(false);
  const [imageDescriptionModelOptions, setImageDescriptionModelOptions] = useState<string[]>([]);
  const [loadingImageDescriptionModels, setLoadingImageDescriptionModels] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [saveError, setSaveError] = useState("");
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

  useEffect(() => {
    if (!configDraft.llmBaseUrl.trim()) {
      return;
    }

    const timeout = window.setTimeout(() => {
      setLoadingLlmModels(true);
      api
        .listConfigurationModels({ kind: "llm", baseUrl: configDraft.llmBaseUrl })
        .then((response) => setLlmModelOptions(response.models))
        .catch(() => setLlmModelOptions([]))
        .finally(() => setLoadingLlmModels(false));
    }, 250);

    return () => window.clearTimeout(timeout);
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

  async function handleConfigurationSubmit(event: FormEvent) {
    event.preventDefault();
    setSavingConfig(true);
    setSaveError("");

    try {
      const response = await api.updateConfiguration(configDraft);
      setConfiguration(response.configuration);
      setConfigDraft(draftFrom(response.configuration));
    } catch (nextError) {
      setSaveError(nextError instanceof Error ? nextError.message : t("settings.saveError"));
    } finally {
      setSavingConfig(false);
    }
  }

  // Only the connection settings are a draft: the theme and the language apply as
  // they are chosen. A modal that is saving cannot close, since the result would
  // land on a screen that no longer shows what it was for (doc-9 §6.6). Unsaved
  // connection edits ask before they are thrown away, keeping them by default
  // (doc-9 §5.7).
  const connectionDirty = !sameDraft(configDraft, configuration ? draftFrom(configuration) : EMPTY_DRAFT);

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
      <Stack>
        <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
          <DialogTitle>{t("settings.title")}</DialogTitle>
          <ActionButton
            type="button"
            variant="normal"
            disabledReason={savingConfig ? t("settings.savingClose") : undefined}
            onClick={() => void requestClose()}
          >
            {t("common.close")}
          </ActionButton>
        </Row>

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
              {t("settings.imageDescriptionEndpoint")}
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
      </Stack>
    </Dialog>
  );
}
