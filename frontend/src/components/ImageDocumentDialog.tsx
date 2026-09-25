import { FormEvent, useEffect, useId, useRef, useState } from "react";
import { api, DocumentRecord, ImageDescriptionAvailability } from "../api/client";
import { useLanguage } from "../i18n";
import { Field, Input, Row, Stack, Subtle, Textarea } from "../styles/ui";
import { ActionButton } from "./ActionButton";
import { useConfirm } from "./ConfirmDialog";
import { Dialog, DialogTitle } from "./Dialog";
import { FailureNotice } from "./FailureNotice";
import { FileDropZone } from "./FileDropZone";
import { ImageIcon, PlusIcon, SparklesIcon } from "./icons";
import { generateBlockerReason, PreparedImage, prepareForDescription } from "./prepareForDescription";

interface ImageDocumentDialogProps {
  projectId: string;
  documents: DocumentRecord[];
  onClose: () => void;
  onCreated: () => void;
}

export function ImageDocumentDialog({ projectId, documents, onClose, onCreated }: ImageDocumentDialogProps) {
  const { t } = useLanguage();
  const confirm = useConfirm();
  const generateAbortRef = useRef<AbortController | null>(null);
  const lockedReasonId = useId();
  // Documents this dialog already deleted to overwrite them. If the create after the
  // delete fails, a retry must not try to delete them again from the stale list.
  const deletedIdsRef = useRef(new Set<string>());
  const [availability, setAvailability] = useState<ImageDescriptionAvailability | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState("");
  const [prepared, setPrepared] = useState<PreparedImage>(null);
  const [title, setTitle] = useState("");
  const [note, setNote] = useState("");
  const [tags, setTags] = useState("");
  const [derivedText, setDerivedText] = useState("");
  const [generating, setGenerating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [generateError, setGenerateError] = useState("");
  const [saveError, setSaveError] = useState("");
  const [dropNotice, setDropNotice] = useState("");

  useEffect(() => {
    api
      .getImageDescription()
      .then(setAvailability)
      .catch((nextError) => setGenerateError(nextError instanceof Error ? nextError.message : t("imageDialog.availabilityError")));
    return () => generateAbortRef.current?.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!file) {
      return;
    }
    const url = URL.createObjectURL(file);
    setPreviewUrl(url);
    setPrepared(null);
    let active = true;
    prepareForDescription(file)
      .then((blob) => active && setPrepared({ blob }))
      .catch((nextError) => active && setPrepared({ error: nextError instanceof Error ? nextError.message : String(nextError) }));
    return () => {
      active = false;
      URL.revokeObjectURL(url);
    };
  }, [file]);

  const generateBlocker = file && availability ? generateBlockerReason(t, availability, file.type || file.name, prepared) : "";

  // The drop zone hands over every image it accepted; one image is one document.
  function handleFilesChosen(images: File[]) {
    const [next] = images;
    if (!next || saving || generating) {
      return;
    }
    setFile(next);
    setTitle(next.name);
    setGenerateError("");
    setSaveError("");
    setDropNotice(images.length > 1 ? t("imageDialog.dropOnlyFirst", { name: next.name, count: images.length }) : "");
  }

  async function handleGenerate() {
    if (!prepared || !("blob" in prepared)) {
      return;
    }
    if (derivedText.trim() && !(await confirm(t("imageDialog.replaceDraftPrompt"), {
        heading: t("imageDialog.replaceDraftHeading"),
        confirmLabel: t("imageDialog.replaceConfirm")
      }))) {
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
    if (!file || saving || generating) {
      return;
    }
    const documentTitle = title.trim() || file.name;
    setSaving(true);
    setSaveError("");
    try {
      // Same rule as the bulk upload: a title names one document, so a clash is an
      // overwrite. Declining keeps the form as it is.
      const existing = documents.find(
        (document) => document.title === documentTitle && !deletedIdsRef.current.has(document.id)
      );
      if (existing) {
        const overwrite = await confirm(t("project.overwritePrompt", { name: documentTitle }), {
          heading: t("project.overwriteHeading"),
          confirmLabel: t("project.overwriteConfirm")
        });
        if (!overwrite) {
          return;
        }
        await api.deleteDocument(existing.id);
        deletedIdsRef.current.add(existing.id);
      }

      const formData = new FormData();
      formData.set("type", "image");
      formData.set("title", documentTitle);
      formData.set("note", note);
      formData.set("tags", tags);
      formData.set("derivedText", derivedText);
      formData.set("file", file);
      await api.createDocument(projectId, formData);
      onCreated();
    } catch (nextError) {
      setSaveError(nextError instanceof Error ? nextError.message : t("project.uploadError"));
    } finally {
      setSaving(false);
    }
  }

  // Closing drops the chosen image and everything typed for it, so it asks first
  // when there is any, through the button and Escape alike (snz-design doc-9 §5.7).
  // While saving it does not close at all.
  async function handleClose() {
    if (saving) {
      return;
    }
    const dirty = Boolean(file || note.trim() || tags.trim() || derivedText.trim());
    if (
      dirty &&
      !(await confirm(t("discard.message"), {
        heading: t("discard.heading"),
        confirmLabel: t("discard.confirm"),
        cancelLabel: t("discard.keepEditing")
      }))
    ) {
      return;
    }
    generateAbortRef.current?.abort();
    onClose();
  }

  // Locked fields stay focusable and say why (snz-design doc-8 §5.4).
  const fieldsLocked = !file ? t("imageDialog.chooseFirst") : saving ? t("project.saving") : "";
  const derivedLocked = fieldsLocked || (generating ? t("imageDialog.generatingNote") : "");
  const lockedProps = (reason: string) =>
    reason ? { readOnly: true, "aria-disabled": true, "aria-describedby": lockedReasonId, title: reason } : {};
  const generateReason = !file
    ? t("imageDialog.chooseFirst")
    : saving
      ? t("project.saving")
      : generateBlocker || (!availability || !prepared ? t("imageDialog.checking") : undefined);
  const submitReason = !file ? t("imageDialog.chooseFirst") : generating ? t("imageDialog.waitGenerate") : undefined;

  return (
    <Dialog onClose={() => void handleClose()}>
      <form onSubmit={handleSubmit}>
        <Stack>
          <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
            <DialogTitle>{t("imageDialog.title")}</DialogTitle>
            <ActionButton
              type="button"
              variant="normal"
              disabledReason={saving ? t("project.savingClose") : undefined}
              onClick={() => void handleClose()}
            >
              {t("common.close")}
            </ActionButton>
          </Row>

          <FileDropZone
            label={t("imageDialog.dropLabel")}
            acceptWords={t("imageDialog.dropAccept")}
            accepts={(candidate) => candidate.type.startsWith("image/")}
            acceptsType={(mime) => mime.startsWith("image/")}
            inputAccept="image/*"
            chooseLabel={file ? t("imageDialog.changeFile") : t("imageDialog.chooseFile")}
            chooseIcon={<ImageIcon />}
            autoFocusChoose
            disabledReason={saving ? t("project.saving") : generating ? t("imageDialog.waitGenerate") : undefined}
            onFilesChosen={handleFilesChosen}
          >
            {file ? <Subtle>{file.name}</Subtle> : null}
            {dropNotice ? <Subtle>{dropNotice}</Subtle> : null}
          </FileDropZone>
          {previewUrl ? (
            <img src={previewUrl} alt="" style={{ maxWidth: "100%", maxHeight: 280, objectFit: "contain", borderRadius: 16 }} />
          ) : null}

          <span hidden id={lockedReasonId}>
            {derivedLocked}
          </span>
          <Field>
            {t("imageDialog.titleField")}
            <Input value={title} onChange={(event) => setTitle(event.target.value)} {...lockedProps(fieldsLocked)} />
          </Field>
          <Field>
            {t("imageDialog.note")}
            <Textarea
              value={note}
              onChange={(event) => setNote(event.target.value)}
              style={{ minHeight: 64 }}
              {...lockedProps(fieldsLocked)}
            />
          </Field>
          <Field>
            {t("imageDialog.tags")}
            <Input value={tags} onChange={(event) => setTags(event.target.value)} {...lockedProps(fieldsLocked)} />
          </Field>
          <Field>
            {t("imageDialog.derivedText")}
            <Textarea value={derivedText} onChange={(event) => setDerivedText(event.target.value)} {...lockedProps(derivedLocked)} />
            <Subtle>{t("imageDialog.derivedTextHint")}</Subtle>
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
              type="submit"
              icon={<PlusIcon />}
              busy={saving}
              title={saving ? t("project.saving") : undefined}
              disabledReason={submitReason}
            >
              {t("imageDialog.create")}
            </ActionButton>
          </Row>
        </Stack>
      </form>
    </Dialog>
  );
}
