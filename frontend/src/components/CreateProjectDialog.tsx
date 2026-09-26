import { FormEvent, useState } from "react";
import { api } from "../api/client";
import { useLanguage } from "../i18n";
import { Field, Input, Row, Stack, Textarea } from "../styles/ui";
import { ActionButton } from "./ActionButton";
import { useConfirm } from "./ConfirmDialog";
import { Dialog, DialogTitle } from "./Dialog";
import { FailureNotice } from "./FailureNotice";
import { PlusIcon } from "./icons";

interface CreateProjectDialogProps {
  onClose: () => void;
  onCreated: () => void;
}

export function CreateProjectDialog({ onClose, onCreated }: CreateProjectDialogProps) {
  const { t } = useLanguage();
  const confirm = useConfirm();
  const [title, setTitle] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [createError, setCreateError] = useState("");

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (submitting) {
      return;
    }
    setSubmitting(true);
    setCreateError("");

    try {
      await api.createProject({ title, description: "", systemPrompt });
      onCreated();
    } catch (nextError) {
      setCreateError(nextError instanceof Error ? nextError.message : t("dashboard.createError"));
      setSubmitting(false);
    }
  }

  // Closing drops what was typed, so it asks first when there is any, through the
  // button and Escape alike (snz-design doc-9 §5.7). While creating it does not close.
  async function handleClose() {
    if (submitting) {
      return;
    }
    if (
      (title.trim() || systemPrompt.trim()) &&
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

  return (
    <Dialog onClose={() => void handleClose()}>
      <form onSubmit={handleSubmit}>
        <Stack>
          <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
            <DialogTitle>{t("dashboard.createProject")}</DialogTitle>
            <ActionButton
              type="button"
              variant="normal"
              disabledReason={submitting ? t("dashboard.creatingClose") : undefined}
              onClick={() => void handleClose()}
            >
              {t("common.close")}
            </ActionButton>
          </Row>
          <Field>
            {t("dashboard.titleField")}
            <Input
              autoFocus
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder={t("dashboard.titlePlaceholder")}
            />
          </Field>
          <Field>
            {t("dashboard.systemPrompt")}
            <Textarea
              value={systemPrompt}
              onChange={(event) => setSystemPrompt(event.target.value)}
              placeholder={t("dashboard.systemPromptPlaceholder")}
            />
          </Field>
          {createError ? <FailureNotice>{createError}</FailureNotice> : null}
          <Row style={{ justifyContent: "flex-end" }}>
            <ActionButton
              type="submit"
              icon={<PlusIcon />}
              busy={submitting}
              title={submitting ? t("dashboard.creating") : undefined}
            >
              {t("dashboard.createButton")}
            </ActionButton>
          </Row>
        </Stack>
      </form>
    </Dialog>
  );
}
