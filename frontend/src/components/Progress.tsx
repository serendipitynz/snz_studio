import { keyframes } from "@emotion/react";
import styled from "@emotion/styled";
import { snzTokens } from "../styles/themes/snz-tokens";

// The progress of snz-design doc-8 §6.7.1: a band and the words for what is being
// waited on. Given a total, the band fills to done / total and readout sits beside
// the words; without one, a mark flows through the band. Zero is not used for "not
// known yet": an empty band reads as stalled. Both variants keep the same size, so an
// owner switches from one to the other in place. It announces nothing itself: the
// owner announces the start, and the value stays readable through the progressbar
// role without cutting into other speech.
// wordLines reserves that many lines for the words, for an owner that keeps its size
// while the label changes (snz-design doc-9 §5.6). fullWidth lets the band span its
// container, for a place wide enough to keep the words and the amount on one line.
export function Progress({
  label,
  done = 0,
  total,
  readout,
  wordLines,
  fullWidth
}: {
  label: string;
  done?: number;
  total?: number;
  readout?: string;
  wordLines?: number;
  fullWidth?: boolean;
}) {
  const known = total !== undefined;
  const ratio = known && total > 0 ? Math.min(1, done / total) : 0;
  const amount = known ? (readout ?? `${Math.floor(ratio * 100)}%`) : undefined;
  return (
    <ProgressBox style={fullWidth ? { inlineSize: "100%" } : undefined}>
      <ProgressWords style={wordLines ? { minBlockSize: `${wordLines}lh`, alignContent: "flex-start" } : undefined}>
        <span>{label}</span>
        {known ? <ProgressAmount>{amount}</ProgressAmount> : null}
      </ProgressWords>
      <Track
        role="progressbar"
        aria-label={label}
        aria-valuemin={known ? 0 : undefined}
        aria-valuemax={total}
        aria-valuenow={known ? done : undefined}
        aria-valuetext={amount}
      >
        {known ? <Fill style={{ inlineSize: `${ratio * 100}%` }} /> : <FlowingMark />}
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

const flow = keyframes`
  from { transform: translateX(-100%); }
  to { transform: translateX(340%); }
`;

// Slowed rather than stopped under reduced motion: a still band cannot be told from a
// stalled one (doc-8 §6.7.1).
const FlowingMark = styled.div`
  block-size: 100%;
  inline-size: 30%;
  border-radius: 999px;
  background: ${({ theme }) => theme.accent};
  animation: ${flow} ${snzTokens.motion.spin * 2}ms linear infinite;

  @media (prefers-reduced-motion: reduce) {
    animation-duration: ${snzTokens.motion.spinReduced * 2}ms;
  }
`;
