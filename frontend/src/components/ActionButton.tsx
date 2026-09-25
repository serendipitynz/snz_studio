import { ButtonHTMLAttributes, forwardRef, MouseEvent, ReactNode, useId } from "react";
import { Button, IconButton } from "../styles/ui";
import { announce } from "./announce";
import { SpinnerIcon } from "./icons";

type ActionButtonProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "disabled"> & {
  variant?: "primary" | "normal" | "danger";
  // The figure slot. A button that can go busy needs one: the busy figure takes its
  // place, so the words and the width stay put (snz-design doc-8 §6.1).
  icon?: ReactNode;
  // The icon-only shape: children is the figure and aria-label the name.
  iconOnly?: boolean;
  busy?: boolean;
  // Given, the button is disabled for this reason. It stays focusable, describes
  // itself with the reason and reads it out when pressed (doc-8 §5.4).
  disabledReason?: string;
};

export const ActionButton = forwardRef<HTMLButtonElement, ActionButtonProps>(function ActionButton(
  { variant, icon, iconOnly, busy, disabledReason, children, onClick, title, "aria-describedby": describedBy, ...rest },
  ref
) {
  const reasonId = useId();

  function handleClick(event: MouseEvent<HTMLButtonElement>) {
    // preventDefault also holds back the form submission a submit button would start,
    // including the implicit one from Enter in a field.
    if (disabledReason) {
      event.preventDefault();
      announce(disabledReason);
      return;
    }
    if (busy) {
      event.preventDefault();
      return;
    }
    onClick?.(event);
  }

  const shared = {
    ...rest,
    ref,
    onClick: handleClick,
    "aria-busy": busy || undefined,
    "aria-disabled": disabledReason ? true : undefined,
    "aria-describedby": [disabledReason ? reasonId : undefined, describedBy].filter(Boolean).join(" ") || undefined,
    title: disabledReason ?? title
  };
  // hidden rather than visually hidden: inside the button it would join the button's
  // name, while aria-describedby still reads a hidden element.
  const reason = disabledReason ? (
    <span hidden id={reasonId}>
      {disabledReason}
    </span>
  ) : null;

  if (iconOnly) {
    return (
      <IconButton {...shared}>
        {busy ? <SpinnerIcon /> : children}
        {reason}
      </IconButton>
    );
  }

  return (
    <Button
      {...shared}
      variant={variant}
      style={{ display: "inline-flex", alignItems: "center", justifyContent: "center", gap: 8, ...rest.style }}
    >
      {busy ? <SpinnerIcon /> : icon}
      {children}
      {reason}
    </Button>
  );
});
