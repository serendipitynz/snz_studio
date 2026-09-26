import styled from "@emotion/styled";
import { ReactNode, useId, useState } from "react";
import { snzTokens } from "../styles/themes/snz-tokens";
import { focusRing } from "../styles/ui";
import { ChevronRightIcon } from "./icons";

// A section folded by a trigger in its own heading (snz-design doc-9 §6.3). The
// heading stays when folded, so the trigger never leaves the screen and focus
// stays on it. The owner decides whether the state is kept; this keeps none —
// unless the owner passes `collapsed`, which makes the fold controlled and the
// owner the keeper. `actions` sit in the heading beside the trigger (the §6.3
// heading area holds operations), so they stay usable while the body is folded.
// The body is hidden, not unmounted, so edits in progress survive the fold.
export function CollapsibleSection({
  heading,
  defaultCollapsed = true,
  collapsed: controlledCollapsed,
  onToggle,
  actions,
  children
}: {
  heading: string;
  defaultCollapsed?: boolean;
  collapsed?: boolean;
  onToggle?: (collapsed: boolean) => void;
  actions?: ReactNode;
  children: ReactNode;
}) {
  const bodyId = useId();
  const [ownCollapsed, setOwnCollapsed] = useState(defaultCollapsed);
  const collapsed = controlledCollapsed ?? ownCollapsed;

  function handleToggle() {
    if (controlledCollapsed === undefined) {
      setOwnCollapsed(!collapsed);
    }
    onToggle?.(!collapsed);
  }

  return (
    <div>
      <HeadingRow>
        <Heading>
          <Trigger type="button" aria-expanded={!collapsed} aria-controls={bodyId} onClick={handleToggle}>
            <ChevronRightIcon />
            <span>{heading}</span>
          </Trigger>
        </Heading>
        {actions}
      </HeadingRow>
      <Body id={bodyId} hidden={collapsed}>
        {children}
      </Body>
    </div>
  );
}

const HeadingRow = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: ${snzTokens.space.sm};

  & > h3 {
    flex: 1;
    min-width: 0;
  }
`;

const Heading = styled.h3`
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  line-height: 1.3;
`;

// The figure turns without animating: a rotation is movement, which the
// reduced-motion setting asks to leave out (snz-design doc-5 §4.2).
const Trigger = styled.button`
  display: flex;
  align-items: center;
  gap: ${snzTokens.space.xs};
  inline-size: 100%;
  min-block-size: ${snzTokens.size.targetMin};
  padding: ${snzTokens.space.xs};
  margin: calc(-1 * ${snzTokens.space.xs});
  box-sizing: content-box;
  border: none;
  border-radius: ${({ theme }) => theme.radiusSm};
  background: transparent;
  color: ${({ theme }) => theme.inkStrong};
  font: inherit;
  text-align: start;
  cursor: pointer;

  & > svg {
    flex-shrink: 0;
    color: ${({ theme }) => theme.figure};
  }

  & > span {
    min-inline-size: 0;
    overflow-wrap: anywhere;
  }

  &[aria-expanded="true"] > svg {
    transform: rotate(90deg);
  }

  &:hover {
    background: ${({ theme }) => theme.surfaceHover};
  }

  &:active {
    background: ${({ theme }) => theme.surfacePressed};
  }

  ${focusRing}
`;

const Body = styled.div`
  margin-top: ${snzTokens.space.md};
`;
