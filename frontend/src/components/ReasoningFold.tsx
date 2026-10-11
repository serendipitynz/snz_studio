import styled from "@emotion/styled";
import { useEffect, useState } from "react";
import { useLanguage } from "../i18n";
import { snzTokens } from "../styles/themes/snz-tokens";
import { MetaText, Summary } from "../styles/ui";
import { SpinnerIcon } from "./icons";

// The model's reasoning, folded above the answer in its bubble — the same native
// disclosure as the bubble's "why this speaker". While the model is still
// thinking the fold stays open, since the reasoning is then the only sign the
// turn is moving; once the answer starts it folds, and reopening it is the
// reader's call.
export function ReasoningFold({ reasoning, thinking }: { reasoning: string; thinking: boolean }) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(thinking);

  useEffect(() => {
    if (!thinking) {
      setOpen(false);
    }
  }, [thinking]);

  if (!reasoning) {
    return null;
  }

  return (
    <details open={open} onToggle={(event) => setOpen(event.currentTarget.open)}>
      <Summary>
        <SummaryLabel as="span">
          {thinking ? <SpinnerIcon size={12} /> : null}
          {thinking ? t("reasoning.thinking") : t("reasoning.label")}
        </SummaryLabel>
      </Summary>
      <Body>{reasoning}</Body>
    </details>
  );
}

const SummaryLabel = styled(MetaText)`
  display: inline-flex;
  align-items: center;
  gap: ${snzTokens.space.xs};
`;

const Body = styled(MetaText)`
  margin-top: ${snzTokens.space.xs};
  padding-inline-start: ${snzTokens.space.sm};
  border-inline-start: 2px solid ${({ theme }) => theme.lineMedium};
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
`;
