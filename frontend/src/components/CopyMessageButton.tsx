import { useEffect, useRef, useState } from "react";
import { useLanguage } from "../i18n";
import { IconButton } from "../styles/ui";

interface CopyMessageButtonProps {
  content: string;
  // The page owns the error banner, so a clipboard failure is reported through
  // it rather than rendered here, inside the message's action row.
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
      style={{
        width: 24,
        height: 24,
        border: "none",
        background: "transparent",
        padding: 0,
        opacity: copied ? 1 : 0.82
      }}
    >
      {/* The icon carries the feedback, not the title swap: a native tooltip is
          dismissed by the click and does not come back within the 1400ms, so
          the title alone was never visible in the WebView. */}
      {copied ? <CopiedIcon /> : <CopyIcon />}
    </IconButton>
  );
}

function CopiedIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="m3.25 8.5 3.25 3.25 6.25-7"
        stroke="currentColor"
        strokeWidth="1.2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function CopyIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M5.25 5V3.75c0-.55.45-1 1-1h5a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H10"
        stroke="currentColor"
        strokeWidth="1.2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <rect x="3" y="5.25" width="7.75" height="8" rx="1" stroke="currentColor" strokeWidth="1.2" />
    </svg>
  );
}
