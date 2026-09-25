import { useState } from "react";
import { api } from "../api/client";
import { markdownFilename, saveTextFile } from "../api/savefile";
import { useLanguage } from "../i18n";
import { ActionButton } from "./ActionButton";
import { DownloadIcon } from "./icons";

interface ExportChatButtonProps {
  chatId: string;
  chatTitle: string;
  // The page places the failure next to the header (snz-design doc-9 §5.5), so it
  // is handed up rather than rendered inside the header's row of buttons.
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
    onError("");
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
    <ActionButton
      type="button"
      iconOnly
      aria-label={t("chat.export")}
      title={t("chat.export")}
      busy={busy}
      onClick={() => void handleExport()}
    >
      <DownloadIcon />
    </ActionButton>
  );
}
