import styled from "@emotion/styled";
import { PointerEvent, ReactNode, useEffect, useId, useRef, useState } from "react";
import { snzTokens } from "../styles/themes/snz-tokens";
import { Field, focusRing } from "../styles/ui";
import { CircleHelpIcon } from "./icons";
import { useLanguage } from "../i18n";

// A field whose label carries a hint. The words are a label of their own, tied to
// the field by id, and the (?) sits beside it: inside a wrapping label the button
// would become the labelled control and take the field's name.
export const HintRow = styled.div`
  position: relative;
  display: flex;
  align-items: center;
  gap: 2px;
`;

const FieldBox = styled(Field.withComponent("div"))``;

export function HintedField(props: { label: string; hint: string; children: (id: string) => ReactNode }) {
  const { t } = useLanguage();
  const id = useId();
  return (
    <FieldBox>
      <HintRow>
        <label htmlFor={id}>{props.label}</label>
        <Hint name={t("hint.about", { label: props.label })} body={props.hint} />
      </HintRow>
      {props.children(id)}
    </FieldBox>
  );
}

// How long a hover-opened hint waits after the pointer leaves, so the pointer can
// cross between the trigger and the body. The spec fixes no length.
const CLOSE_GRACE_MS = 300;

// The hint of snz-design doc-8 §6.9: a (?) that opens a short note on a press as
// well as on hover, so touch and the keyboard reach it. It closes on Escape (focus
// back to the trigger), on another press or a press outside, and a hover-opened one
// after the grace period once the pointer is on neither part (WCAG 1.4.13).
// The note is also the trigger's description, so it is read out on focus.
export function Hint({ name, body }: { name: string; body: string }) {
  const bodyId = useId();
  const rootRef = useRef<HTMLSpanElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const [open, setOpen] = useState(false);
  const hoverOpened = useRef(false);
  const closing = useRef<number | null>(null);

  function cancelClosing() {
    if (closing.current !== null) {
      window.clearTimeout(closing.current);
      closing.current = null;
    }
  }

  useEffect(() => cancelClosing, []);

  useEffect(() => {
    if (!open) {
      return;
    }
    // Captured and marked as handled, so a region the hint sits in does not
    // close on the same Escape: only the innermost closes (doc-9 §5.2).
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !event.isComposing) {
        event.preventDefault();
        setOpen(false);
        triggerRef.current?.focus();
      }
    };
    const handlePointerDown = (event: globalThis.PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("keydown", handleKeyDown, true);
    document.addEventListener("pointerdown", handlePointerDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown, true);
      document.removeEventListener("pointerdown", handlePointerDown);
    };
  }, [open]);

  function handlePress() {
    cancelClosing();
    // A mouse press lands on a hint its hover has already opened; toggling then
    // would close what the user reached for, so the press keeps it open instead.
    if (open && hoverOpened.current) {
      hoverOpened.current = false;
      return;
    }
    hoverOpened.current = false;
    setOpen(!open);
  }

  function handlePointerEnter(event: PointerEvent) {
    if (event.pointerType !== "mouse") {
      return;
    }
    cancelClosing();
    if (!open) {
      hoverOpened.current = true;
      setOpen(true);
    }
  }

  function handlePointerLeave(event: PointerEvent) {
    // Only a hover-opened hint closes on leaving; one opened or kept by a press
    // waits for Escape, another press or a press outside.
    if (event.pointerType !== "mouse" || !hoverOpened.current) {
      return;
    }
    cancelClosing();
    closing.current = window.setTimeout(() => {
      closing.current = null;
      if (!rootRef.current?.matches(":hover")) {
        setOpen(false);
      }
    }, CLOSE_GRACE_MS);
  }

  return (
    <Root ref={rootRef} onPointerEnter={handlePointerEnter} onPointerLeave={handlePointerLeave}>
      <Trigger
        ref={triggerRef}
        type="button"
        aria-label={name}
        aria-expanded={open}
        aria-controls={bodyId}
        aria-describedby={bodyId}
        onClick={handlePress}
      >
        <CircleHelpIcon />
      </Trigger>
      <Body id={bodyId} role="tooltip" data-open={open || undefined}>
        {body}
      </Body>
    </Root>
  );
}

// The body spans the owner's row (the nearest positioned ancestor) rather than
// being sized to its words, so a long translation cannot overflow the pane sideways.
const Root = styled.span`
  display: inline-flex;
`;

// The pressable area meets the 24px minimum around the figure (snz-design doc-8 §5.7).
const Trigger = styled.button`
  display: inline-grid;
  place-items: center;
  min-inline-size: ${snzTokens.size.targetMin};
  min-block-size: ${snzTokens.size.targetMin};
  padding: 0;
  border: none;
  border-radius: 50%;
  background: transparent;
  color: ${({ theme }) => theme.figure};
  cursor: pointer;

  &:hover {
    background: ${({ theme }) => theme.surfaceHover};
  }

  ${focusRing}
`;

// Opens upward. Downward it lands on the very field it describes, which is focused
// exactly when someone reaches for the hint. It touches the row, so the pointer
// can move onto it (1.4.13). Only its opacity fades, which reduced motion allows.
const Body = styled.span`
  position: absolute;
  bottom: 100%;
  left: 0;
  right: 0;
  z-index: 3;
  padding: ${snzTokens.space.xs} ${snzTokens.space.sm};
  border: ${snzTokens.border.line} solid ${({ theme }) => theme.fieldBorder};
  border-radius: ${({ theme }) => theme.radiusSm};
  background: ${({ theme }) => theme.surfaceCard};
  box-shadow: ${({ theme }) => theme.shadowPopover};
  color: ${({ theme }) => theme.ink};
  font-size: 12px;
  line-height: 1.5;
  opacity: 0;
  visibility: hidden;
  transition:
    opacity ${snzTokens.motion.state}ms,
    visibility 0s linear ${snzTokens.motion.state}ms;

  &[data-open] {
    opacity: 1;
    visibility: visible;
    transition:
      opacity ${snzTokens.motion.state}ms,
      visibility 0s;
  }
`;
