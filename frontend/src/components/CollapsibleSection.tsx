import styled from "@emotion/styled";
import { ReactNode, useId, useState } from "react";
import { snzTokens } from "../styles/themes/snz-tokens";
import { focusRing } from "../styles/ui";
import { ChevronRightIcon } from "./icons";

// A section folded by a trigger in its own heading (snz-design doc-9 §6.3). The
// heading stays when folded, so the trigger never leaves the screen and focus
// stays on it. The owner decides whether the state is kept; this keeps none.
export function CollapsibleSection({
  heading,
  defaultCollapsed = true,
  children
}: {
  heading: string;
  defaultCollapsed?: boolean;
  children: ReactNode;
}) {
  const bodyId = useId();
  const [collapsed, setCollapsed] = useState(defaultCollapsed);

  return (
    <div>
      <Heading>
        <Trigger
          type="button"
          aria-expanded={!collapsed}
          aria-controls={bodyId}
          onClick={() => setCollapsed((current) => !current)}
        >
          <ChevronRightIcon />
          <span>{heading}</span>
        </Trigger>
      </Heading>
      <Body id={bodyId} hidden={collapsed}>
        {children}
      </Body>
    </div>
  );
}

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
    color: ${({ theme }) => theme.figure};
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
