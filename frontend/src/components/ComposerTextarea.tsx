import { forwardRef, KeyboardEvent, useEffect, useImperativeHandle, useRef, useState } from "react";
import { Textarea } from "../styles/ui";

const MIN_LINES = 3;
const MAX_LINES = 20;

// The text field of both chat composers: it grows with its text between
// MIN_LINES and MAX_LINES, and Enter submits its form while Shift+Enter breaks
// the line. Enter that confirms an IME conversion is left alone — WebKit reports
// it as a plain Enter with keyCode 229, so isComposing alone misses it.
// The form's own onSubmit decides whether there is anything to send.
export const ComposerTextarea = forwardRef<
  HTMLTextAreaElement,
  { value: string; onChange: (value: string) => void; placeholder: string; "aria-label": string }
>(function ComposerTextarea({ value, onChange, placeholder, "aria-label": ariaLabel }, ref) {
  const nodeRef = useRef<HTMLTextAreaElement | null>(null);
  const [isComposing, setIsComposing] = useState(false);
  useImperativeHandle(ref, () => nodeRef.current as HTMLTextAreaElement);

  useEffect(() => {
    const node = nodeRef.current;
    if (!node) {
      return;
    }

    const computedStyle = window.getComputedStyle(node);
    const lineHeight = Number.parseFloat(computedStyle.lineHeight) || 22;
    const padding =
      Number.parseFloat(computedStyle.paddingTop || "0") + Number.parseFloat(computedStyle.paddingBottom || "0");
    const border =
      Number.parseFloat(computedStyle.borderTopWidth || "0") + Number.parseFloat(computedStyle.borderBottomWidth || "0");
    const minHeight = Math.round(lineHeight * MIN_LINES + padding + border);
    const maxHeight = Math.round(lineHeight * MAX_LINES + padding + border);

    node.style.height = "auto";
    const nextHeight = value.trim() ? Math.min(Math.max(node.scrollHeight, minHeight), maxHeight) : minHeight;
    node.style.height = `${nextHeight}px`;
    node.style.overflowY = node.scrollHeight > maxHeight ? "auto" : "hidden";
  }, [value]);

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (isComposing || event.nativeEvent.isComposing || event.keyCode === 229) {
      return;
    }
    if (event.key !== "Enter" || event.shiftKey) {
      return;
    }

    event.preventDefault();
    event.currentTarget.form?.requestSubmit();
  }

  return (
    <Textarea
      ref={nodeRef}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      onKeyDown={handleKeyDown}
      onCompositionStart={() => setIsComposing(true)}
      onCompositionEnd={() => setIsComposing(false)}
      placeholder={placeholder}
      aria-label={ariaLabel}
      style={{ minHeight: 0, resize: "none" }}
    />
  );
});
