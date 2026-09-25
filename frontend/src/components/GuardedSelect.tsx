import { ComponentProps, KeyboardEvent, MouseEvent, useId } from "react";
import { Select } from "../styles/ui";
import { announce } from "./announce";

type GuardedSelectProps = Omit<ComponentProps<typeof Select>, "disabled"> & {
  // Given, the select is disabled for this reason: it stays focusable, describes
  // itself with the reason and reads it out when someone tries to open it
  // (snz-design doc-8 §5.4). A native disabled select would drop out of the Tab order.
  disabledReason?: string;
};

const PASS_KEYS = new Set(["Tab", "Shift", "Escape"]);

export function GuardedSelect({ disabledReason, onChange, onMouseDown, onKeyDown, title, ...rest }: GuardedSelectProps) {
  const reasonId = useId();
  const describedBy = [disabledReason ? reasonId : undefined, rest["aria-describedby"]].filter(Boolean).join(" ") || undefined;

  function handleMouseDown(event: MouseEvent<HTMLSelectElement>) {
    if (disabledReason) {
      // Keeps the option list from opening; focus still lands on the select.
      event.preventDefault();
      event.currentTarget.focus();
      announce(disabledReason);
      return;
    }
    onMouseDown?.(event);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLSelectElement>) {
    if (disabledReason && !PASS_KEYS.has(event.key)) {
      event.preventDefault();
      if (event.key === "Enter" || event.key === " " || event.key.startsWith("Arrow")) {
        announce(disabledReason);
      }
      return;
    }
    onKeyDown?.(event);
  }

  return (
    <>
      <Select
        {...rest}
        aria-disabled={disabledReason ? true : undefined}
        aria-describedby={describedBy}
        title={disabledReason ?? title}
        onMouseDown={handleMouseDown}
        onKeyDown={handleKeyDown}
        // A no-op rather than none, so React still treats the value as controlled.
        onChange={disabledReason ? () => undefined : onChange}
      />
      {disabledReason ? (
        <span hidden id={reasonId}>
          {disabledReason}
        </span>
      ) : null}
    </>
  );
}
