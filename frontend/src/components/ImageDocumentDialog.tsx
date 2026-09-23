import { DragEvent, FormEvent, useEffect, useRef, useState } from "react";
import { api, DocumentRecord, ImageDescriptionAvailability } from "../api/client";
import { useLanguage } from "../i18n";
import { Button, DropZone, ErrorText, Field, Input, Row, Stack, Subtle, Textarea } from "../styles/ui";
import { useConfirm } from "./ConfirmDialog";
import { Dialog, DialogTitle } from "./Dialog";
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
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const generateAbortRef = useRef<AbortController | null>(null);
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
  const [error, setError] = useState("");
  const [dragActive, setDragActive] = useState(false);
  const [dropNotice, setDropNotice] = useState("");

  useEffect(() => {
    api
      .getImageDescription()
      .then(setAvailability)
      .catch((nextError) => setError(nextError instanceof Error ? nextError.message : t("imageDialog.availabilityError")));
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

  const busy = generating || saving;
  const generateBlocker = file && availability ? generateBlockerReason(t, availability, file.type || file.name, prepared) : "";

  function handleChooseFile(next: File | undefined) {
    if (!next) {
      return;
    }
    setFile(next);
    setTitle(next.name);
    setError("");
    setDropNotice("");
  }

  function handleDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragActive(false);
    if (busy) {
      return;
    }
    const dropped = Array.from(event.dataTransfer.files);
    const image = dropped.find((candidate) => candidate.type.startsWith("image/"));
    if (!image) {
      if (dropped.length > 0) {
        setError(t("imageDialog.dropNotImage"));
      }
      return;
    }
    handleChooseFile(image);
    if (dropped.length > 1) {
      setDropNotice(t("imageDialog.dropOnlyFirst", { name: image.name, count: dropped.length }));
    }
  }

  function handleDragOver(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragActive(!busy);
  }

  function handleDragLeave(event: DragEvent<HTMLDivElement>) {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
      setDragActive(false);
    }
  }

  async function handleGenerate() {
    if (!prepared || !("blob" in prepared)) {
      return;
    }
    if (derivedText.trim() && !(await confirm(t("imageDialog.replaceDraftPrompt")))) {
      return;
    }
    const controller = new AbortController();
    generateAbortRef.current = controller;
    setGenerating(true);
    setError("");
    try {
      const response = await api.describeImage(prepared.blob, controller.signal);
      setDerivedText(response.description);
    } catch (nextError) {
      if (!controller.signal.aborted) {
        setError(nextError instanceof Error ? nextError.message : t("imageDialog.generateError"));
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
    if (!file) {
      return;
    }
    const documentTitle = title.trim() || file.name;
    setSaving(true);
    setError("");
    try {
      // Same rule as the bulk upload: a title names one document, so a clash is an
      // overwrite. Declining keeps the form as it is.
      const existing = documents.find(
        (document) => document.title === documentTitle && !deletedIdsRef.current.has(document.id)
      );
      if (existing) {
        if (!(await confirm(t("project.overwritePrompt", { name: documentTitle })))) {
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
      setError(nextError instanceof Error ? nextError.message : t("project.uploadError"));
    } finally {
      setSaving(false);
    }
  }

  function handleClose() {
    if (saving) {
      return;
    }
    generateAbortRef.current?.abort();
    onClose();
  }

  return (
    <Dialog onClose={handleClose}>
      <form onSubmit={handleSubmit}>
        <Stack>
          <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
            <DialogTitle>{t("imageDialog.title")}</DialogTitle>
            <Button type="button" variant="ghost" onClick={handleClose} disabled={saving}>
              {t("common.close")}
            </Button>
          </Row>

          {error ? <ErrorText>{error}</ErrorText> : null}

          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            style={{ display: "none" }}
            onChange={(event) => {
              handleChooseFile(event.target.files?.[0]);
              event.target.value = "";
            }}
          />
          <DropZone $active={dragActive} onDrop={handleDrop} onDragOver={handleDragOver} onDragLeave={handleDragLeave}>
            <Stack>
              <Row style={{ alignItems: "center" }}>
                <Button type="button" variant="ghost" onClick={() => fileInputRef.current?.click()} disabled={busy} autoFocus>
                  {file ? t("imageDialog.changeFile") : t("imageDialog.chooseFile")}
                </Button>
                {file ? <Subtle>{file.name}</Subtle> : null}
              </Row>
              <Subtle>{t("imageDialog.dropHint")}</Subtle>
              {dropNotice ? <Subtle>{dropNotice}</Subtle> : null}
            </Stack>
          </DropZone>
          {previewUrl ? (
            <img src={previewUrl} alt="" style={{ maxWidth: "100%", maxHeight: 280, objectFit: "contain", borderRadius: 16 }} />
          ) : null}

          <Field>
            {t("imageDialog.titleField")}
            <Input value={title} onChange={(event) => setTitle(event.target.value)} disabled={!file || saving} />
          </Field>
          <Field>
            {t("imageDialog.note")}
            <Textarea
              value={note}
              onChange={(event) => setNote(event.target.value)}
              disabled={!file || saving}
              style={{ minHeight: 64 }}
            />
          </Field>
          <Field>
            {t("imageDialog.tags")}
            <Input value={tags} onChange={(event) => setTags(event.target.value)} disabled={!file || saving} />
          </Field>
          <Field>
            {t("imageDialog.derivedText")}
            <Textarea value={derivedText} onChange={(event) => setDerivedText(event.target.value)} disabled={!file || busy} />
            <Subtle>{t("imageDialog.derivedTextHint")}</Subtle>
          </Field>

          <Row style={{ alignItems: "center" }}>
            <Button
              type="button"
              variant="ghost"
              onClick={() => void handleGenerate()}
              disabled={!file || busy || !availability || !prepared || Boolean(generateBlocker)}
            >
              {generating ? t("imageDialog.generating") : t("imageDialog.generate")}
            </Button>
            {generateBlocker ? <Subtle>{generateBlocker}</Subtle> : null}
          </Row>

          <Row style={{ justifyContent: "flex-end" }}>
            <Button type="submit" disabled={!file || busy}>
              {saving ? t("imageDialog.saving") : t("imageDialog.create")}
            </Button>
          </Row>
        </Stack>
      </form>
    </Dialog>
  );
}
