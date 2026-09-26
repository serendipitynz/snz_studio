import { useEffect, useState } from "react";
import { api, WorkspaceConfiguration } from "../api/client";
import { MessageKey, useLanguage } from "../i18n";
import { Card, Grid, Row, SectionTitle, Stack, StateBadge, Subtle, VisuallyHidden } from "../styles/ui";
import { ActionButton } from "./ActionButton";
import { FailureNotice } from "./FailureNotice";
import { SettingsIcon, SpinnerIcon } from "./icons";
import { SettingsModal } from "./SettingsModal";

export function ConnectionStatusCard() {
  const { t } = useLanguage();
  const [configuration, setConfiguration] = useState<WorkspaceConfiguration | null>(null);
  const [checking, setChecking] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);

  // The server checks every endpoint on each read, so this can take a while and
  // is repeated after the settings close, when the endpoints may have changed.
  async function load() {
    setChecking(true);
    try {
      const response = await api.getConfiguration();
      setConfiguration(response.configuration);
      setLoadError("");
    } catch (nextError) {
      setLoadError(nextError instanceof Error ? nextError.message : t("dashboard.connectionLoadError"));
    } finally {
      setChecking(false);
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const entries: { label: MessageKey; connected: boolean; model: string }[] = configuration
    ? [
        { label: "dashboard.connectionChat", connected: configuration.llmConnected, model: configuration.llmModel },
        // An empty review model is checked, and used, as the chat model.
        {
          label: "dashboard.connectionReview",
          connected: configuration.reviewConnected,
          model: configuration.reviewModel.trim() || configuration.llmModel
        },
        {
          label: "dashboard.connectionEmbedding",
          connected: configuration.embeddingConnected,
          model:
            configuration.embeddingMode === "internal" ? t("dashboard.embeddingInternal") : configuration.embeddingModel
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
          <Grid columns="repeat(3, minmax(0, 1fr))">
            {entries.map((entry) => (
              <Stack key={entry.label} style={{ gap: 4 }}>
                <Row style={{ alignItems: "center", gap: 8 }}>
                  <strong>{t(entry.label)}</strong>
                  <StateBadge tone={entry.connected ? "success" : "danger"}>
                    {entry.connected ? t("settings.connected") : t("settings.notConnected")}
                  </StateBadge>
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
