import { SaveTextFile } from "../wailsjs/go/main/App";

export type SaveOutcome = "saved" | "cancelled";

// Characters no mainstream filesystem accepts in a name, plus the control
// range. The chat title they come from is free text, so it is sanitised rather
// than trusted.
const unsafeFilenameChars = /[/\\:*?"<>|\x00-\x1f]/g;

const maxFilenameStem = 80;

// markdownFilename turns a chat title into the name the save dialog preselects.
export function markdownFilename(title: string): string {
  return `${filenameStem(title)}.md`;
}

// presetFilename is markdownFilename for a preset exported from the chat.
export function presetFilename(title: string): string {
  return `${filenameStem(title)}.json`;
}

function filenameStem(title: string): string {
  const stem = title
    .replace(unsafeFilenameChars, " ")
    .replace(/\s+/g, " ")
    .trim()
    .slice(0, maxFilenameStem)
    // A leading dot would hide the file; a trailing dot or space is rejected on
    // Windows.
    .replace(/^\.+/, "")
    .trim();
  return stem || "chat";
}

// saveTextFile writes content to a location the user picks.
//
// Inside the Wails WebView this goes through the Go SaveTextFile binding, which
// opens the native save dialog: Wails v2.16's macOS WebView implements no
// download callback, so a download-attribute click there is dropped in silence.
// The Blob path is kept for the host where the binding is absent, which is the
// only host on which it can work.
//
// An error from the binding is propagated rather than retried through the Blob
// path, so a failed write is reported instead of being replaced by a download
// that would not happen either.
export async function saveTextFile(
  suggestedName: string,
  content: string,
  mimeType = "text/markdown;charset=utf-8"
): Promise<SaveOutcome> {
  if (window.go?.main?.App?.SaveTextFile) {
    return (await SaveTextFile(suggestedName, content)) ? "saved" : "cancelled";
  }
  downloadAsFile(suggestedName, content, mimeType);
  return "saved";
}

function downloadAsFile(filename: string, content: string, mimeType: string): void {
  const url = URL.createObjectURL(new Blob([content], { type: mimeType }));
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  // Revoking immediately can cut the download short in some engines; the next
  // frame is late enough for the click to have been taken up.
  requestAnimationFrame(() => URL.revokeObjectURL(url));
}
