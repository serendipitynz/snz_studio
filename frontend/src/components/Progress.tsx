import styled from "@emotion/styled";
import { snzTokens } from "../styles/themes/snz-tokens";

// The known-amount progress of snz-design doc-8 §6.7.1: a band filled to done / total
// and the words for what is being waited on. It announces nothing itself: the owner
// announces the start, and the value stays readable through the progressbar role
// without cutting into other speech. The unknown-amount variant is not built yet, as
// nothing here needs it.
// wordLines reserves that many lines for the words, for an owner that keeps its size
// while the label changes (snz-design doc-9 §5.6).
export function Progress({
  label,
  done,
  total,
  readout,
  wordLines
}: {
  label: string;
  done: number;
  total: number;
  readout: string;
  wordLines?: number;
}) {
  const ratio = total > 0 ? Math.min(1, done / total) : 0;
  return (
    <ProgressBox>
      <ProgressWords style={wordLines ? { minBlockSize: `${wordLines}lh`, alignContent: "flex-start" } : undefined}>
        <span>{label}</span>
        <ProgressAmount>{readout}</ProgressAmount>
      </ProgressWords>
      <Track
        role="progressbar"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={total}
        aria-valuenow={done}
        aria-valuetext={readout}
      >
        <Fill style={{ inlineSize: `${ratio * 100}%` }} />
      </Track>
    </ProgressBox>
  );
}

// A set line height: with "normal", a Japanese fallback font makes the lines taller
// than the reserved ones.
const ProgressBox = styled.div`
  display: grid;
  line-height: 1.5;
  gap: ${snzTokens.space.xs};
  inline-size: min(100%, 24rem);
`;

// The amount keeps its place beside the words rather than wrapping under them, so
// the words' height is the only thing a long label changes.
const ProgressWords = styled.div`
  display: flex;
  justify-content: space-between;
  gap: ${snzTokens.space.sm};
  color: ${({ theme }) => theme.ink};

  & > span:first-of-type {
    flex: 1 1 auto;
    min-width: 0;
    overflow-wrap: anywhere;
  }
`;

const ProgressAmount = styled.span`
  flex: none;
  color: ${({ theme }) => theme.muted};
  font-variant-numeric: tabular-nums;
`;

// The outline keeps the band findable while nothing is filled yet.
const Track = styled.div`
  block-size: 0.5rem;
  overflow: hidden;
  border: ${snzTokens.border.line} solid ${({ theme }) => theme.fieldBorder};
  border-radius: 999px;
  background: ${({ theme }) => theme.surfaceDropzone};
`;

const Fill = styled.div`
  block-size: 100%;
  background: ${({ theme }) => theme.accent};
`;
