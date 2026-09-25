import { useEffect, useRef, useState } from "react";
import { useLanguage } from "../i18n";
import { IconButton } from "../styles/ui";
import { ClipboardCheckIcon, ClipboardIcon } from "./icons";

interface CopyMessageButtonProps {
  content: string;
  // The page places a clipboard failure in the message it belongs to (snz-design
  // doc-9 §5.5), so it is handed up rather than rendered inside the action row.
  // An empty message clears it.
  onError: (message: string) => void;
}

// CopyMessageButton copies one message's raw text. It serves both chat kinds so
// that the single-assistant and multi-agent action rows stay identical by
// construction rather than by keeping two copies of the same markup in step.
export function CopyMessageButton({ content, onError }: CopyMessageButtonProps) {
  const { t } = useLanguage();
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
      }
    };
  }, []);

  async function handleCopy() {
    onError("");
    try {
      await navigator.clipboard.writeText(content);
      setCopied(true);
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
      }
      timerRef.current = window.setTimeout(() => {
        timerRef.current = null;
        setCopied(false);
      }, 1400);
    } catch (nextError) {
      onError(nextError instanceof Error ? nextError.message : t("chat.copyMessageError"));
    }
  }

  return (
    <IconButton
      type="button"
      aria-label={t("chat.copyMessage")}
      onClick={() => void handleCopy()}
      title={copied ? t("chat.copied") : t("chat.copy")}
      style={{ width: 24, height: 24, border: "none", background: "transparent", padding: 0 }}
    >
      {/* The icon carries the feedback, not the title swap: a native tooltip is
          dismissed by the click and does not come back within the 1400ms, so
          the title alone was never visible in the WebView. */}
      {copied ? <ClipboardCheckIcon /> : <ClipboardIcon />}
    </IconButton>
  );
}
