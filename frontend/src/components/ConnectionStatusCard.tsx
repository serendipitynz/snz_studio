import { useEffect, useRef, useState } from "react";
import { api, EmbeddingStatus, WorkspaceConfiguration } from "../api/client";
import { MessageKey, useLanguage } from "../i18n";
import { Card, Grid, Row, SectionTitle, Stack, StateBadge, Subtle, VisuallyHidden } from "../styles/ui";
import { ActionButton } from "./ActionButton";
import { FailureNotice } from "./FailureNotice";
import { SettingsIcon, SpinnerIcon } from "./icons";
import { SettingsModal } from "./SettingsModal";

type Tone = "success" | "danger" | "neutral";

interface Entry {
  label: MessageKey;
  tone: Tone;
  state: MessageKey;
  model: string;
}

export function ConnectionStatusCard() {
  const { t } = useLanguage();
  const [configuration, setConfiguration] = useState<WorkspaceConfiguration | null>(null);
  const [embeddingStatus, setEmbeddingStatus] = useState<EmbeddingStatus | null>(null);
  // Starts false so the status region mounts empty and the first check is read out.
  const [checking, setChecking] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  // Only the latest load may settle: a check started before the settings changed
  // can finish after the one started once they closed.
  const loadGeneration = useRef(0);

  // The server checks every endpoint on each read, so this can take a while and
  // is repeated after the settings close, when the endpoints may have changed.
  async function load() {
    const generation = ++loadGeneration.current;
    setChecking(true);
    try {
      const [configurationResponse, nextEmbeddingStatus] = await Promise.all([
        api.getConfiguration(),
        // The status only colours the internal embedding row, so its failure
        // must not hide the others.
        api.getEmbeddingStatus().catch(() => null)
      ]);
      if (generation !== loadGeneration.current) {
        return;
      }
      setConfiguration(configurationResponse.configuration);
      setEmbeddingStatus(nextEmbeddingStatus);
      setLoadError("");
    } catch (nextError) {
      if (generation === loadGeneration.current) {
        setLoadError(nextError instanceof Error ? nextError.message : t("dashboard.connectionLoadError"));
      }
    } finally {
      if (generation === loadGeneration.current) {
        setChecking(false);
      }
    }
  }

  useEffect(() => {
    void load();
    return () => {
      loadGeneration.current += 1;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const entries: Entry[] = configuration
    ? [
        {
          label: "dashboard.connectionChat",
          ...connectedState(configuration.llmConnected),
          model: configuration.llmModel
        },
        // An empty review model is checked, and used, as the chat model.
        {
          label: "dashboard.connectionReview",
          ...connectedState(configuration.reviewConnected),
          model: configuration.reviewModel.trim() || configuration.llmModel
        },
        configuration.embeddingMode === "internal"
          ? {
              label: "dashboard.connectionEmbedding",
              ...internalEmbeddingState(embeddingStatus),
              model: t("dashboard.embeddingInternal")
            }
          : {
              label: "dashboard.connectionEmbedding",
              ...connectedState(configuration.embeddingConnected),
              model: configuration.embeddingModel
            },
        // Optional: without a model the feature is off rather than broken.
        {
          label: "dashboard.connectionImageDescription",
          ...(configuration.imageDescriptionModel.trim()
            ? connectedState(configuration.imageDescriptionConnected)
            : { tone: "neutral" as const, state: "dashboard.notSet" as const }),
          model: configuration.imageDescriptionModel
        }
      ]
    : [];

  return (
    <Card>
      <Stack>
        <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
          <Row style={{ alignItems: "center", gap: 8 }}>
            <SectionTitle>{t("dashboard.connection")}</SectionTitle>
            <span role="status" style={{ display: "inline-flex" }}>
              {checking ? (
                <>
                  <SpinnerIcon />
                  <VisuallyHidden>{t("dashboard.connectionChecking")}</VisuallyHidden>
                </>
              ) : null}
            </span>
          </Row>
          <ActionButton type="button" variant="normal" icon={<SettingsIcon />} onClick={() => setIsSettingsOpen(true)}>
            {t("dashboard.openSettings")}
          </ActionButton>
        </Row>
        {loadError ? <FailureNotice>{loadError}</FailureNotice> : null}
        {entries.length > 0 ? (
          <Grid columns="repeat(4, minmax(0, 1fr))">
            {entries.map((entry) => (
              <Stack key={entry.label} style={{ gap: 4 }}>
                <Row style={{ alignItems: "center", gap: 8 }}>
                  <strong>{t(entry.label)}</strong>
                  <StateBadge tone={entry.tone}>{t(entry.state)}</StateBadge>
                </Row>
                <Subtle style={{ overflowWrap: "anywhere" }}>{entry.model.trim() || t("dashboard.modelUnset")}</Subtle>
              </Stack>
            ))}
          </Grid>
        ) : null}
      </Stack>
      {isSettingsOpen ? (
        <SettingsModal
          onClose={() => {
            setIsSettingsOpen(false);
            void load();
          }}
        />
      ) : null}
    </Card>
  );
}

function connectedState(connected: boolean): { tone: Tone; state: MessageKey } {
  return connected
    ? { tone: "success", state: "settings.connected" }
    : { tone: "danger", state: "settings.notConnected" };
}

// The bundled model has no endpoint to reach: its sidecar's lifecycle is the
// state, as the settings show it. Downloading and starting are on the way to
// ready, not failures.
function internalEmbeddingState(status: EmbeddingStatus | null): { tone: Tone; state: MessageKey } {
  switch (status?.state) {
    case "ready":
      return { tone: "success", state: "dashboard.available" };
    case "error":
      return { tone: "danger", state: "dashboard.unavailable" };
    case "disabled":
      return { tone: "neutral", state: "dashboard.unavailable" };
    default:
      return { tone: "neutral", state: "dashboard.preparing" };
  }
}
