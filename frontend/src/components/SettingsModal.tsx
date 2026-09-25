import { FormEvent, useEffect, useState } from "react";
import { api, EmbeddingStatus, WorkspaceConfiguration } from "../api/client";
import { Language, MessageKey, useLanguage } from "../i18n";
import { ThemeMode, useThemeController } from "../styles/ThemeController";
import type { ThemeFamily } from "../styles/themes";
import { Dialog, DialogTitle } from "./Dialog";
import {
  Badge,
  Button,
  Card,
  ErrorText,
  Field,
  FieldHeader,
  Input,
  Row,
  Select,
  Stack,
  StatusDot,
  Subtle
} from "../styles/ui";

interface SettingsModalProps {
  onClose: () => void;
}

// embeddingStatusLabel renders the internal sidecar's lifecycle into a short status
// line, reassuring the user that keyword search keeps working while the model loads.
function embeddingStatusLabel(t: (key: MessageKey, vars?: Record<string, string | number>) => string, status: EmbeddingStatus | null): string {
  if (!status) {
    return t("settings.embedPreparing");
  }
  switch (status.state) {
    case "downloading": {
      const pct = status.total > 0 ? Math.floor((status.downloaded / status.total) * 100) : 0;
      return t("settings.embedDownloading", { pct });
    }
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
  const { family, mode, families, setFamily, setMode } = useThemeController();
  const [configuration, setConfiguration] = useState<WorkspaceConfiguration | null>(null);
  const [configDraft, setConfigDraft] = useState({
    llmBaseUrl: "",
    llmModel: "",
    llmResponseFormat: "standard" as "standard" | "llm_jp_thinking",
    reviewBaseUrl: "",
    reviewModel: "",
    embeddingBaseUrl: "",
    embeddingModel: "",
    embeddingMode: "internal" as "internal" | "external",
    imageDescriptionBaseUrl: "",
    imageDescriptionModel: ""
  });
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
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    setLoading(true);
    api
      .getConfiguration()
      .then((response) => {
        if (!active) return;
        setConfiguration(response.configuration);
        setConfigDraft({
          llmBaseUrl: response.configuration.llmBaseUrl,
          llmModel: response.configuration.llmModel,
          llmResponseFormat: response.configuration.llmResponseFormat,
          reviewBaseUrl: response.configuration.reviewBaseUrl,
          reviewModel: response.configuration.reviewModel,
          embeddingBaseUrl: response.configuration.embeddingBaseUrl,
          embeddingModel: response.configuration.embeddingModel,
          embeddingMode: response.configuration.embeddingMode,
          imageDescriptionBaseUrl: response.configuration.imageDescriptionBaseUrl,
          imageDescriptionModel: response.configuration.imageDescriptionModel
        });
      })
      .catch((nextError) => {
        if (!active) return;
        setError(nextError instanceof Error ? nextError.message : t("settings.loadError"));
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
    setError("");

    try {
      const response = await api.updateConfiguration(configDraft);
      setConfiguration(response.configuration);
      setConfigDraft({
        llmBaseUrl: response.configuration.llmBaseUrl,
        llmModel: response.configuration.llmModel,
        llmResponseFormat: response.configuration.llmResponseFormat,
        reviewBaseUrl: response.configuration.reviewBaseUrl,
        reviewModel: response.configuration.reviewModel,
        embeddingBaseUrl: response.configuration.embeddingBaseUrl,
        embeddingModel: response.configuration.embeddingModel,
        embeddingMode: response.configuration.embeddingMode,
        imageDescriptionBaseUrl: response.configuration.imageDescriptionBaseUrl,
        imageDescriptionModel: response.configuration.imageDescriptionModel
      });
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("settings.saveError"));
    } finally {
      setSavingConfig(false);
    }
  }

  function modelCandidatesLabel(loadingModels: boolean, options: string[]): string {
    if (loadingModels) {
      return t("settings.loadingModels");
    }
    return options.length ? t("settings.candidatesFound", { count: options.length }) : t("settings.noCandidates");
  }

  return (
    <Dialog onClose={onClose}>
      <Stack>
        <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
          <DialogTitle>{t("settings.title")}</DialogTitle>
          <Button type="button" variant="normal" onClick={onClose}>
            {t("common.close")}
          </Button>
        </Row>

        {error ? <ErrorText>{error}</ErrorText> : null}

        <Card>
          <Stack>
            <Badge tone="accent">{t("settings.appearance")}</Badge>
            <Field>
              {t("settings.theme")}
              <Select value={family} onChange={(event) => setFamily(event.target.value as ThemeFamily)}>
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
            <Badge tone="accent">{t("settings.language")}</Badge>
            <Field>
              {t("settings.language")}
              <Select value={lang} onChange={(event) => setLang(event.target.value as Language)}>
                <option value="ja">日本語</option>
                <option value="en">English</option>
              </Select>
            </Field>
          </Stack>
        </Card>

        <Card as="form" onSubmit={handleConfigurationSubmit}>
          <Stack>
            <Badge tone="accent">{t("settings.connection")}</Badge>
            {loading ? <Subtle>{t("settings.loadingConfig")}</Subtle> : null}
            <Field>
              <FieldHeader>
                <span>{t("settings.llmEndpoint")}</span>
                <StatusDot $connected={Boolean(configuration?.llmConnected)} />
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
                <StatusDot $connected={Boolean(configuration?.reviewConnected)} />
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
                {configDraft.embeddingMode === "external" ? (
                  <StatusDot $connected={Boolean(configuration?.embeddingConnected)} />
                ) : null}
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
              <Subtle>
                {configDraft.embeddingMode === "internal"
                  ? embeddingStatusLabel(t, embeddingStatus)
                  : t("settings.embeddingExternalNote")}
              </Subtle>
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
            <div style={{ display: "flex", justifyContent: "flex-end", gap: 10 }}>
              <Button type="submit" disabled={savingConfig || loading}>
                {savingConfig ? t("settings.saving") : t("settings.save")}
              </Button>
            </div>
          </Stack>
        </Card>
      </Stack>
    </Dialog>
  );
}
