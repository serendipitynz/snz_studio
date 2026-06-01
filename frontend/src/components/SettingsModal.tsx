import { FormEvent, useEffect, useState } from "react";
import { api, EmbeddingStatus, WorkspaceConfiguration } from "../api/client";
import { ThemeMode, useThemeController } from "../styles/ThemeController";
import type { ThemeFamily } from "../styles/themes";
import {
  Badge,
  Button,
  Card,
  ErrorText,
  Field,
  FieldHeader,
  Input,
  ModalCard,
  ModalOverlay,
  Row,
  SectionTitle,
  Select,
  Stack,
  StatusDot,
  Subtle
} from "../styles/ui";

interface SettingsModalProps {
  onClose: () => void;
}

// Persisted UI language preference. Wiring the actual i18n switch is a separate
// task; the modal only captures the preference here for forward compatibility.
const LANGUAGE_KEY = "snz.language";

type UiLanguage = "ja" | "en";

function readLanguage(): UiLanguage {
  if (typeof window === "undefined") {
    return "ja";
  }
  const stored = window.localStorage.getItem(LANGUAGE_KEY);
  return stored === "ja" || stored === "en" ? stored : "ja";
}

// embeddingStatusLabel renders the internal sidecar's lifecycle into a short status
// line, reassuring the user that keyword search keeps working while the model loads.
function embeddingStatusLabel(status: EmbeddingStatus | null): string {
  if (!status) {
    return "Preparing the bundled embedding model… keyword search is active meanwhile.";
  }
  switch (status.state) {
    case "downloading": {
      const pct = status.total > 0 ? Math.floor((status.downloaded / status.total) * 100) : 0;
      return `Downloading the embedding model (${pct}%)… keyword search is active meanwhile.`;
    }
    case "starting":
      return "Starting the embedding model… keyword search is active meanwhile.";
    case "ready":
      return "Bundled embedding model is ready — semantic search is active.";
    case "error":
      return `Embedding model unavailable — keyword search only.${status.error ? ` (${status.error})` : ""}`;
    default:
      return "Keyword search only.";
  }
}

export function SettingsModal({ onClose }: SettingsModalProps) {
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
    embeddingMode: "internal" as "internal" | "external"
  });
  const [embeddingStatus, setEmbeddingStatus] = useState<EmbeddingStatus | null>(null);
  const [language, setLanguage] = useState<UiLanguage>(readLanguage);
  const [loading, setLoading] = useState(true);
  const [savingConfig, setSavingConfig] = useState(false);
  const [llmModelOptions, setLlmModelOptions] = useState<string[]>([]);
  const [reviewModelOptions, setReviewModelOptions] = useState<string[]>([]);
  const [embeddingModelOptions, setEmbeddingModelOptions] = useState<string[]>([]);
  const [loadingLlmModels, setLoadingLlmModels] = useState(false);
  const [loadingReviewModels, setLoadingReviewModels] = useState(false);
  const [loadingEmbeddingModels, setLoadingEmbeddingModels] = useState(false);
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
          embeddingMode: response.configuration.embeddingMode
        });
      })
      .catch((nextError) => {
        if (!active) return;
        setError(nextError instanceof Error ? nextError.message : "Failed to load configuration");
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
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

  function handleLanguageChange(next: UiLanguage) {
    setLanguage(next);
    if (typeof window !== "undefined") {
      window.localStorage.setItem(LANGUAGE_KEY, next);
    }
  }

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
        embeddingMode: response.configuration.embeddingMode
      });
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to save configuration");
    } finally {
      setSavingConfig(false);
    }
  }

  return (
    <ModalOverlay onClick={onClose}>
      <ModalCard onClick={(event) => event.stopPropagation()}>
        <Stack>
          <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
            <SectionTitle>設定</SectionTitle>
            <Button type="button" variant="ghost" onClick={onClose}>
              Close
            </Button>
          </Row>

          {error ? <ErrorText>{error}</ErrorText> : null}

          <Card>
            <Stack>
              <Badge tone="accent">Appearance</Badge>
              <Field>
                Theme
                <Select value={family} onChange={(event) => setFamily(event.target.value as ThemeFamily)}>
                  {families.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.label}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field>
                Mode
                <Select value={mode} onChange={(event) => setMode(event.target.value as ThemeMode)}>
                  <option value="light">Light</option>
                  <option value="dark">Dark</option>
                  <option value="auto">Auto (follow OS)</option>
                </Select>
              </Field>
            </Stack>
          </Card>

          <Card>
            <Stack>
              <Badge tone="accent">Language</Badge>
              <Field>
                Language
                <Select value={language} onChange={(event) => handleLanguageChange(event.target.value as UiLanguage)}>
                  <option value="ja">日本語</option>
                  <option value="en">English</option>
                </Select>
              </Field>
              <Subtle>UI language switching will be applied in a future update.</Subtle>
            </Stack>
          </Card>

          <Card as="form" onSubmit={handleConfigurationSubmit}>
            <Stack>
              <Badge tone="accent">Connection</Badge>
              {loading ? <Subtle>Loading configuration…</Subtle> : null}
              <Field>
                <FieldHeader>
                  <span>LLM Endpoint</span>
                  <StatusDot $connected={Boolean(configuration?.llmConnected)} />
                </FieldHeader>
                <Input
                  value={configDraft.llmBaseUrl}
                  onChange={(event) => setConfigDraft((current) => ({ ...current, llmBaseUrl: event.target.value }))}
                  placeholder="http://127.0.0.1:1234/v1"
                />
              </Field>
              <Field>
                LLM Model
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
                <Subtle>
                  {loadingLlmModels
                    ? "Loading model candidates..."
                    : llmModelOptions.length
                      ? `${llmModelOptions.length} candidates found`
                      : "No model candidates available"}
                </Subtle>
              </Field>
              <Field>
                LLM Response Format
                <Select
                  value={configDraft.llmResponseFormat}
                  onChange={(event) =>
                    setConfigDraft((current) => ({
                      ...current,
                      llmResponseFormat: event.target.value as "standard" | "llm_jp_thinking"
                    }))
                  }
                >
                  <option value="standard">Standard</option>
                  <option value="llm_jp_thinking">LLM-jp Thinking</option>
                </Select>
              </Field>
              <Field>
                <FieldHeader>
                  <span>Review Endpoint</span>
                  <StatusDot $connected={Boolean(configuration?.reviewConnected)} />
                </FieldHeader>
                <Input
                  value={configDraft.reviewBaseUrl}
                  onChange={(event) => setConfigDraft((current) => ({ ...current, reviewBaseUrl: event.target.value }))}
                  placeholder="http://127.0.0.1:1234/v1"
                />
              </Field>
              <Field>
                Review Model
                <Input
                  list="settings-review-model-options"
                  value={configDraft.reviewModel}
                  onChange={(event) => setConfigDraft((current) => ({ ...current, reviewModel: event.target.value }))}
                  placeholder="review model id"
                />
                <datalist id="settings-review-model-options">
                  {reviewModelOptions.map((model) => (
                    <option key={model} value={model} />
                  ))}
                </datalist>
                <Subtle>
                  {loadingReviewModels
                    ? "Loading model candidates..."
                    : reviewModelOptions.length
                      ? `${reviewModelOptions.length} candidates found`
                      : "No model candidates available"}
                </Subtle>
              </Field>
              <Field>
                <FieldHeader>
                  <span>Embedding Source</span>
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
                  <option value="internal">Internal — bundled ruri-v3-30m (recommended)</option>
                  <option value="external">External — OpenAI-compatible endpoint</option>
                </Select>
                <Subtle>
                  {configDraft.embeddingMode === "internal"
                    ? embeddingStatusLabel(embeddingStatus)
                    : "Embeddings are computed by the endpoint configured below."}
                </Subtle>
              </Field>
              {configDraft.embeddingMode === "external" ? (
                <>
                  <Field>
                    Embedding Endpoint
                    <Input
                      value={configDraft.embeddingBaseUrl}
                      onChange={(event) => setConfigDraft((current) => ({ ...current, embeddingBaseUrl: event.target.value }))}
                      placeholder="http://127.0.0.1:8080/v1"
                    />
                  </Field>
                  <Field>
                    Embedding Model
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
                    <Subtle>
                      {loadingEmbeddingModels
                        ? "Loading model candidates..."
                        : embeddingModelOptions.length
                          ? `${embeddingModelOptions.length} candidates found`
                          : "No model candidates available"}
                    </Subtle>
                  </Field>
                </>
              ) : null}
              <div style={{ display: "flex", justifyContent: "flex-end", gap: 10 }}>
                <Button type="submit" disabled={savingConfig || loading}>
                  {savingConfig ? "Saving..." : "Save configuration"}
                </Button>
              </div>
            </Stack>
          </Card>
        </Stack>
      </ModalCard>
    </ModalOverlay>
  );
}
