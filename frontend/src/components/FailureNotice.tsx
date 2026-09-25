import styled from "@emotion/styled";
import type { ReactNode } from "react";
import { useLanguage } from "../i18n";
import { snzTokens } from "../styles/themes/snz-tokens";

// The failure level of snz-design doc-9 §6.4. It is placed next to what failed
// rather than at the top of the page (§5.5), cannot be dismissed, and is read
// out without taking focus (doc-8 §5.8). The figure tells the level apart
// beyond its colour, since some palettes share it with the warning level.
export function FailureNotice({ children }: { children: ReactNode }) {
  const { t } = useLanguage();
  return (
    <NoticeBox role="alert">
      <svg
        viewBox="0 0 24 24"
        width={snzTokens.icon.sizeMd}
        height={snzTokens.icon.sizeMd}
        fill="none"
        role="img"
        aria-label={t("notice.failure")}
      >
        <circle cx="12" cy="12" r="9" stroke="currentColor" strokeWidth={snzTokens.icon.strokeWidth} />
        <path
          d="M9 9l6 6M15 9l-6 6"
          stroke="currentColor"
          strokeWidth={snzTokens.icon.strokeWidth}
          strokeLinecap="round"
        />
      </svg>
      <NoticeText>{children}</NoticeText>
    </NoticeBox>
  );
}

const NoticeBox = styled.div`
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: start;
  gap: ${snzTokens.space.sm};
  padding: ${snzTokens.space.sm} ${snzTokens.space.md};
  border-inline-start: ${snzTokens.border.band} solid ${({ theme }) => theme.danger};
  border-radius: ${({ theme }) => theme.radiusSm};
  background: ${({ theme }) => theme.dangerSoft};
  color: ${({ theme }) => theme.onDangerSoft};
  line-height: 1.5;

  & > svg {
    color: ${({ theme }) => theme.danger};
    /* Centres the figure on the first line of the words. */
    margin-top: calc((1lh - ${snzTokens.icon.sizeMd}) / 2);
  }
`;

const NoticeText = styled.p`
  margin: 0;
  overflow-wrap: anywhere;
`;
