import { FormEvent, useState } from "react";
import { api } from "../api/client";
import { useLanguage } from "../i18n";
import { Field, Input, Textarea } from "../styles/ui";
import { ActionButton } from "./ActionButton";
import { useConfirm } from "./ConfirmDialog";
import { Dialog, DialogActions, DialogBody, DialogForm, DialogHeader, DialogTitle } from "./Dialog";
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
      <DialogHeader
        actions={
          <ActionButton
            type="button"
            variant="normal"
            disabledReason={submitting ? t("dashboard.creatingClose") : undefined}
            onClick={() => void handleClose()}
          >
            {t("common.close")}
          </ActionButton>
        }
      >
        <DialogTitle>{t("dashboard.createProject")}</DialogTitle>
      </DialogHeader>
      <DialogForm onSubmit={handleSubmit}>
        <DialogBody>
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
        </DialogBody>
        <DialogActions notice={createError ? <FailureNotice>{createError}</FailureNotice> : null}>
          <ActionButton
            type="submit"
            icon={<PlusIcon />}
            busy={submitting}
            title={submitting ? t("dashboard.creating") : undefined}
          >
            {t("dashboard.createButton")}
          </ActionButton>
        </DialogActions>
      </DialogForm>
    </Dialog>
  );
}
