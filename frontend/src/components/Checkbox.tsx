import styled from "@emotion/styled";
import type { ReactNode } from "react";
import { snzTokens } from "../styles/themes/snz-tokens";

// The checkbox of snz-design doc-8 §6.4. The native input stays in the tree for its
// keyboard and screen-reader behaviour; a drawn mark replaces its look, so on and off
// are told by the check rather than by the platform's styling. The pressable area
// holds the mark and the words and takes the hover face and the focus ring.
export function Checkbox({
  checked,
  onChange,
  children
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  children: ReactNode;
}) {
  return (
    <ChoiceArea>
      <NativeBox type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <Mark aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
          <path d="M20 6 9 17l-5-5" />
        </svg>
      </Mark>
      <Words>{children}</Words>
    </ChoiceArea>
  );
}

const ChoiceArea = styled.label`
  position: relative;
  display: flex;
  align-items: flex-start;
  gap: ${snzTokens.space.sm};
  inline-size: fit-content;
  max-inline-size: 100%;
  min-block-size: ${snzTokens.size.targetMin};
  padding: ${snzTokens.space.xs};
  border-radius: ${({ theme }) => theme.radiusSm};
  color: ${({ theme }) => theme.ink};
  cursor: pointer;

  &:hover {
    background: ${({ theme }) => theme.surfaceHover};
  }

  &:has(input:focus-visible) {
    outline: ${snzTokens.border.focus} solid ${({ theme }) => theme.focus};
    outline-offset: ${snzTokens.border.focusOffset};
  }
`;

const NativeBox = styled.input`
  position: absolute;
  inset-block-start: ${snzTokens.space.xs};
  inset-inline-start: ${snzTokens.space.xs};
  inline-size: 1px;
  block-size: 1px;
  margin: 0;
  opacity: 0;
`;

const Mark = styled.span`
  flex: none;
  display: grid;
  place-items: center;
  inline-size: ${snzTokens.icon.sizeMd};
  block-size: ${snzTokens.icon.sizeMd};
  /* Whole pixels, so the check does not land on a half pixel. */
  margin-block-start: round(calc((1lh - ${snzTokens.icon.sizeMd}) / 2), 1px);
  box-sizing: border-box;
  border: ${snzTokens.border.line} solid ${({ theme }) => theme.fieldBorder};
  border-radius: ${({ theme }) => theme.radiusSm};
  background: ${({ theme }) => theme.surfaceField};
  color: ${({ theme }) => theme.onAccent};

  & > svg {
    inline-size: 100%;
    block-size: 100%;
    visibility: hidden;
  }

  input:checked + & {
    border-color: ${({ theme }) => theme.accent};
    background: ${({ theme }) => theme.accent};
  }

  input:checked + & > svg {
    visibility: visible;
  }
`;

const Words = styled.span`
  min-inline-size: 0;
  overflow-wrap: anywhere;
`;
