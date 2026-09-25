import { FormEvent, useEffect, useId, useRef, useState } from "react";
import { api, DocumentRecord, fileSrc, ImageDescriptionAvailability } from "../api/client";
import { useLanguage } from "../i18n";
import { Field, Input, Row, Stack, Subtle, Textarea } from "../styles/ui";
import { ActionButton } from "./ActionButton";
import { useConfirm } from "./ConfirmDialog";
import { FailureNotice } from "./FailureNotice";
import { CheckIcon, SparklesIcon } from "./icons";
import { generateBlockerReason, PreparedImage, prepareForDescription } from "./prepareForDescription";

type StoredImage = { blob: Blob } | { error: string } | null;

interface ImageDocumentEditorProps {
  document: DocumentRecord;
  onSaved: (document: DocumentRecord) => void;
  onCancel: () => void;
  onSavingChange: (saving: boolean) => void;
  // Lets the dialog around it ask before closing over unsaved edits.
  onDirtyChange?: (dirty: boolean) => void;
}

// ImageDocumentEditor edits a stored image document's note, tags and description.
// "Generate description" fetches the stored image and sends it through the same
// preparation as the add dialog, rather than asking the server to describe the
// file on disk: the model runtime refuses images above one megapixel and
// blackens transparent ones, and only the browser can downscale and flatten
// every accepted format without a new server dependency.
export function ImageDocumentEditor({ document, onSaved, onCancel, onSavingChange, onDirtyChange }: ImageDocumentEditorProps) {
  const { t } = useLanguage();
  const confirm = useConfirm();
  const generateAbortRef = useRef<AbortController | null>(null);
  const lockedReasonId = useId();
  const [availability, setAvailability] = useState<ImageDescriptionAvailability | null>(null);
  const [stored, setStored] = useState<StoredImage>(null);
  const [prepared, setPrepared] = useState<PreparedImage>(null);
  const [note, setNote] = useState(document.note);
  const [tags, setTags] = useState(document.tags.join(", "));
  const [derivedText, setDerivedText] = useState(document.derivedText);
  const [generating, setGenerating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [generateError, setGenerateError] = useState("");
  const [saveError, setSaveError] = useState("");

  const dirty = note !== document.note || tags !== document.tags.join(", ") || derivedText !== document.derivedText;
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty]);

  useEffect(() => {
    api
      .getImageDescription()
      .then(setAvailability)
      .catch((nextError) => setGenerateError(nextError instanceof Error ? nextError.message : t("imageDialog.availabilityError")));
    return () => generateAbortRef.current?.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    let active = true;
    setStored(null);
    setPrepared(null);
    loadStoredImage(document.filePath)
      .then((blob) => {
        if (!active) {
          return;
        }
        setStored({ blob });
        return prepareForDescription(blob).then(
          (preparedBlob) => active && setPrepared({ blob: preparedBlob }),
          (nextError) => active && setPrepared({ error: nextError instanceof Error ? nextError.message : String(nextError) })
        );
      })
      .catch((nextError) => active && setStored({ error: nextError instanceof Error ? nextError.message : String(nextError) }));
    return () => {
      active = false;
    };
  }, [document.filePath]);

  const generateBlocker = describeBlocker();

  function describeBlocker(): string {
    if (stored && "error" in stored) {
      return t("documentEditor.storedImageError", { detail: stored.error });
    }
    if (!stored || !availability) {
      return "";
    }
    return generateBlockerReason(t, availability, stored.blob.type, prepared);
  }

  async function handleGenerate() {
    if (!prepared || !("blob" in prepared)) {
      return;
    }
    if (
      derivedText.trim() &&
      !(await confirm(t("imageDialog.replaceDraftPrompt"), {
        heading: t("imageDialog.replaceDraftHeading"),
        confirmLabel: t("imageDialog.replaceConfirm")
      }))
    ) {
      return;
    }
    const controller = new AbortController();
    generateAbortRef.current = controller;
    setGenerating(true);
    setGenerateError("");
    try {
      const response = await api.describeImage(prepared.blob, controller.signal);
      setDerivedText(response.description);
    } catch (nextError) {
      if (!controller.signal.aborted) {
        setGenerateError(nextError instanceof Error ? nextError.message : t("imageDialog.generateError"));
      }
    } finally {
      if (generateAbortRef.current === controller) {
        generateAbortRef.current = null;
        setGenerating(false);
      }
    }
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (saving || generating) {
      return;
    }
    setSaving(true);
    onSavingChange(true);
    setSaveError("");
    try {
      const response = await api.updateDocumentContent(document.id, { note, tags, derivedText });
      onSavingChange(false);
      onSaved(response.document);
    } catch (nextError) {
      setSaveError(nextError instanceof Error ? nextError.message : t("documentEditor.saveError"));
      setSaving(false);
      onSavingChange(false);
    }
  }

  function handleCancel() {
    generateAbortRef.current?.abort();
    onCancel();
  }

  // Locked fields stay focusable and say why (snz-design doc-8 §5.4), so they are
  // read-only rather than disabled.
  const fieldsLocked = saving ? t("project.saving") : "";
  const derivedLocked = fieldsLocked || (generating ? t("imageDialog.generatingNote") : "");
  const lockedProps = (reason: string) =>
    reason ? { readOnly: true, "aria-disabled": true, "aria-describedby": lockedReasonId, title: reason } : {};
  const generateReason = saving
    ? t("project.saving")
    : generateBlocker || (!availability || !prepared ? t("imageDialog.checking") : undefined);

  return (
    <form onSubmit={handleSubmit}>
      <Stack>
        <span hidden id={lockedReasonId}>
          {derivedLocked}
        </span>
        <Field>
          {t("imageDialog.note")}
          <Textarea value={note} onChange={(event) => setNote(event.target.value)} style={{ minHeight: 64 }} {...lockedProps(fieldsLocked)} />
        </Field>
        <Field>
          {t("imageDialog.tags")}
          <Input value={tags} onChange={(event) => setTags(event.target.value)} {...lockedProps(fieldsLocked)} />
        </Field>
        <Field>
          {t("imageDialog.derivedText")}
          <Textarea value={derivedText} onChange={(event) => setDerivedText(event.target.value)} {...lockedProps(derivedLocked)} />
          <Subtle>{t("documentEditor.derivedTextHint")}</Subtle>
        </Field>

        <Row style={{ alignItems: "center" }}>
          <ActionButton
            type="button"
            variant="normal"
            icon={<SparklesIcon />}
            busy={generating}
            title={generating ? t("imageDialog.generatingNote") : undefined}
            disabledReason={generating ? undefined : generateReason}
            onClick={() => void handleGenerate()}
          >
            {t("imageDialog.generate")}
          </ActionButton>
          {generating ? <Subtle>{t("imageDialog.generatingNote")}</Subtle> : generateBlocker ? <Subtle>{generateBlocker}</Subtle> : null}
        </Row>
        {generateError ? <FailureNotice>{generateError}</FailureNotice> : null}

        {saveError ? <FailureNotice>{saveError}</FailureNotice> : null}
        <Row style={{ justifyContent: "flex-end" }}>
          <ActionButton
            type="button"
            variant="normal"
            disabledReason={saving ? t("project.saving") : undefined}
            onClick={handleCancel}
          >
            {t("common.cancel")}
          </ActionButton>
          <ActionButton
            type="submit"
            icon={<CheckIcon />}
            busy={saving}
            title={saving ? t("project.saving") : undefined}
            disabledReason={generating ? t("imageDialog.waitGenerate") : undefined}
          >
            {t("documentEditor.save")}
          </ActionButton>
        </Row>
      </Stack>
    </form>
  );
}

async function loadStoredImage(filePath: string | null): Promise<Blob> {
  if (!filePath) {
    throw new Error("no stored file");
  }
  const response = await fetch(fileSrc(filePath));
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }
  return response.blob();
}
