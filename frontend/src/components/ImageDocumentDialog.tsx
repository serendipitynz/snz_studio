import { FormEvent, useEffect, useRef, useState } from "react";
import { api, DocumentRecord, ImageDescriptionAvailability } from "../api/client";
import { useLanguage } from "../i18n";
import { Button, ErrorText, Field, Input, Row, Stack, Subtle, Textarea } from "../styles/ui";
import { useConfirm } from "./ConfirmDialog";
import { Dialog, DialogTitle } from "./Dialog";

// LM Studio with Gemma 4 answered a 1024x1024 or 2048x512 image but refused 1280x1280
// and 1600x900 outright ({"error":"terminated"}) rather than resizing them, so anything
// above one megapixel is downscaled here before it is sent. Done in the browser because
// the WebView already decodes PNG, JPEG and WebP, where the Go side would need a new
// dependency for WebP and resampling.
const MAX_DESCRIPTION_PIXELS = 1024 * 1024;

type PreparedImage = { blob: Blob } | { error: string } | null;

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

  const generateBlocker = describeGenerateBlocker();

  function describeGenerateBlocker(): string {
    if (!file || !availability) {
      return "";
    }
    if (!availability.enabled) {
      return t("imageDialog.disabledNoModel");
    }
    if (!availability.formats.includes(file.type)) {
      return t("imageDialog.disabledFormat", { type: file.type || file.name });
    }
    if (prepared && "error" in prepared) {
      return t("imageDialog.disabledDecode", { detail: prepared.error });
    }
    if (prepared && prepared.blob.size > availability.maxBytes) {
      return t("imageDialog.disabledSize", { mb: Math.floor(availability.maxBytes / (1024 * 1024)) });
    }
    return "";
  }

  function handleChooseFile(next: File | undefined) {
    if (!next) {
      return;
    }
    setFile(next);
    setTitle(next.name);
    setError("");
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

  const busy = generating || saving;

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
          <Row style={{ alignItems: "center" }}>
            <Button type="button" variant="ghost" onClick={() => fileInputRef.current?.click()} disabled={busy} autoFocus>
              {file ? t("imageDialog.changeFile") : t("imageDialog.chooseFile")}
            </Button>
            {file ? <Subtle>{file.name}</Subtle> : null}
          </Row>
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

// prepareForDescription returns the image to send for description: the file itself
// when it is within MAX_DESCRIPTION_PIXELS, otherwise a downscaled copy (PNG stays
// PNG to keep text crisp; anything else becomes JPEG). The stored document keeps the
// original file either way.
async function prepareForDescription(file: File): Promise<Blob> {
  const url = URL.createObjectURL(file);
  try {
    const image = new Image();
    image.src = url;
    await image.decode();
    const pixels = image.naturalWidth * image.naturalHeight;
    if (pixels <= MAX_DESCRIPTION_PIXELS) {
      return file;
    }
    const scale = Math.sqrt(MAX_DESCRIPTION_PIXELS / pixels);
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.floor(image.naturalWidth * scale));
    canvas.height = Math.max(1, Math.floor(image.naturalHeight * scale));
    const context = canvas.getContext("2d");
    if (!context) {
      throw new Error("canvas 2d context is unavailable");
    }
    context.drawImage(image, 0, 0, canvas.width, canvas.height);
    const type = file.type === "image/png" ? "image/png" : "image/jpeg";
    return await new Promise<Blob>((resolve, reject) =>
      canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error("image could not be re-encoded"))), type, 0.9)
    );
  } finally {
    URL.revokeObjectURL(url);
  }
}
