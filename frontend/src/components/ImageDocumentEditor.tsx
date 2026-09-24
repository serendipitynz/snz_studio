import { FormEvent, useEffect, useRef, useState } from "react";
import { api, DocumentRecord, fileSrc, ImageDescriptionAvailability } from "../api/client";
import { useLanguage } from "../i18n";
import { Button, ErrorText, Field, Input, Row, Stack, Subtle, Textarea } from "../styles/ui";
import { useConfirm } from "./ConfirmDialog";
import { generateBlockerReason, PreparedImage, prepareForDescription } from "./prepareForDescription";

type StoredImage = { blob: Blob } | { error: string } | null;

interface ImageDocumentEditorProps {
  document: DocumentRecord;
  onSaved: (document: DocumentRecord) => void;
  onCancel: () => void;
  onSavingChange: (saving: boolean) => void;
}

// ImageDocumentEditor edits a stored image document's note, tags and description.
// "Generate description" fetches the stored image and sends it through the same
// preparation as the add dialog, rather than asking the server to describe the
// file on disk: the model runtime refuses images above one megapixel and
// blackens transparent ones, and only the browser can downscale and flatten
// every accepted format without a new server dependency.
export function ImageDocumentEditor({ document, onSaved, onCancel, onSavingChange }: ImageDocumentEditorProps) {
  const { t } = useLanguage();
  const confirm = useConfirm();
  const generateAbortRef = useRef<AbortController | null>(null);
  const [availability, setAvailability] = useState<ImageDescriptionAvailability | null>(null);
  const [stored, setStored] = useState<StoredImage>(null);
  const [prepared, setPrepared] = useState<PreparedImage>(null);
  const [note, setNote] = useState(document.note);
  const [tags, setTags] = useState(document.tags.join(", "));
  const [derivedText, setDerivedText] = useState(document.derivedText);
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
    setSaving(true);
    onSavingChange(true);
    setError("");
    try {
      const response = await api.updateDocumentContent(document.id, { note, tags, derivedText });
      onSavingChange(false);
      onSaved(response.document);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : t("documentEditor.saveError"));
      setSaving(false);
      onSavingChange(false);
    }
  }

  function handleCancel() {
    generateAbortRef.current?.abort();
    onCancel();
  }

  const busy = generating || saving;

  return (
    <form onSubmit={handleSubmit}>
      <Stack>
        {error ? <ErrorText>{error}</ErrorText> : null}
        <Field>
          {t("imageDialog.note")}
          <Textarea value={note} onChange={(event) => setNote(event.target.value)} disabled={saving} style={{ minHeight: 64 }} />
        </Field>
        <Field>
          {t("imageDialog.tags")}
          <Input value={tags} onChange={(event) => setTags(event.target.value)} disabled={saving} />
        </Field>
        <Field>
          {t("imageDialog.derivedText")}
          <Textarea value={derivedText} onChange={(event) => setDerivedText(event.target.value)} disabled={busy} />
          <Subtle>{t("documentEditor.derivedTextHint")}</Subtle>
        </Field>

        <Row style={{ alignItems: "center" }}>
          <Button
            type="button"
            variant="normal"
            onClick={() => void handleGenerate()}
            disabled={busy || !availability || !prepared || Boolean(generateBlocker)}
          >
            {generating ? t("imageDialog.generating") : t("imageDialog.generate")}
          </Button>
          {generateBlocker ? <Subtle>{generateBlocker}</Subtle> : null}
        </Row>

        <Row style={{ justifyContent: "flex-end" }}>
          <Button type="button" variant="normal" onClick={handleCancel} disabled={saving}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" disabled={busy}>
            {saving ? t("imageDialog.saving") : t("documentEditor.save")}
          </Button>
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
