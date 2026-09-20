import { useState } from "react";
import { api } from "../api/client";
import { markdownFilename, saveTextFile } from "../api/savefile";
import { useLanguage } from "../i18n";
import { IconButton } from "../styles/ui";

interface ExportChatButtonProps {
  chatId: string;
  chatTitle: string;
  // The page owns the error banner, so a failure is reported through it rather
  // than rendered here, where it would sit inside the header row.
  onError: (message: string) => void;
}

// ExportChatButton downloads the chat's markdown transcript and hands it to the
// save dialog. It serves both chat kinds: the route behind it is chat-scoped and
// the server decides which sections a transcript carries.
export function ExportChatButton({ chatId, chatTitle, onError }: ExportChatButtonProps) {
  const { t } = useLanguage();
  const [busy, setBusy] = useState(false);

  async function handleExport() {
    setBusy(true);
    try {
      const markdown = await api.exportChatMarkdown(chatId);
      await saveTextFile(markdownFilename(chatTitle), markdown);
    } catch (nextError) {
      onError(nextError instanceof Error ? nextError.message : t("chat.exportError"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <IconButton
      type="button"
      aria-label={t("chat.export")}
      title={t("chat.export")}
      disabled={busy}
      onClick={() => void handleExport()}
    >
      <DownloadIcon />
    </IconButton>
  );
}

function DownloadIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M8 2.5v7m0 0L5.2 6.7M8 9.5l2.8-2.8M3 11.5v1a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1v-1"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
