import styled from "@emotion/styled";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, RecentChat } from "../api/client";
import { useLanguage } from "../i18n";
import { snzTokens } from "../styles/themes/snz-tokens";
import { Card, Item, Row, SectionTitle, Stack, Subtle } from "../styles/ui";
import { FailureNotice } from "./FailureNotice";
import { renderChatTitle } from "./WorkspaceSidebar";

// The way back into recent work across projects; the sidebar lists only the
// projects on this screen, so this is what the dashboard adds beside them.
export function RecentChatsCard() {
  const { t } = useLanguage();
  const [chats, setChats] = useState<RecentChat[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");

  useEffect(() => {
    let cancelled = false;
    api
      .getRecentChats()
      .then((response) => {
        if (!cancelled) {
          setChats(response.chats);
        }
      })
      .catch((nextError) => {
        if (!cancelled) {
          setLoadError(nextError instanceof Error ? nextError.message : t("dashboard.recentLoadError"));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
    // Loaded once per visit, like the project list beside it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <Card>
      <Stack>
        {/* As tall as the project list's heading, whose button sets its height,
            so the two lists start on one line. */}
        <Row style={{ alignItems: "center", minHeight: snzTokens.size.control }}>
          <SectionTitle>{t("dashboard.recentChats")}</SectionTitle>
        </Row>
        {loadError ? <FailureNotice>{loadError}</FailureNotice> : null}
        {!loading && !loadError && chats.length === 0 ? <Item>{t("dashboard.noRecentChats")}</Item> : null}
        {chats.length > 0 ? (
          <RowList>
            {chats.map((chat) => (
              <li key={chat.id}>
                <RowLink to={`/chats/${chat.id}`}>
                  {renderChatTitle(t, chat)}
                  <Subtle style={{ overflowWrap: "anywhere" }}>
                    {t("dashboard.recentChatMeta", {
                      project: chat.projectTitle,
                      time: new Date(chat.updatedAt).toLocaleString()
                    })}
                  </Subtle>
                </RowLink>
              </li>
            ))}
          </RowList>
        ) : null}
      </Stack>
    </Card>
  );
}

// The same one box of rows split by divider lines as the project list beside
// it (snz-design doc-9 §6.1), without the reorder handle.
const RowList = styled.ul`
  display: flex;
  flex-direction: column;
  min-width: 0;
  margin: 0;
  padding: 0;
  list-style: none;
  border: ${snzTokens.border.line} solid ${({ theme }) => theme.fieldBorder};
  border-radius: ${({ theme }) => theme.radius};
  background: ${({ theme }) => theme.surfaceItem};

  & > li {
    /* Row separators are decorative: no contrast minimum (doc-5 §3.2). */
    border-bottom: ${snzTokens.border.line} solid ${({ theme }) => theme.line};
  }

  & > li:last-child {
    border-bottom: 0;
  }
`;

// Rows touch their neighbours, so the ring is drawn just inside the link rather
// than over the next row, as the project list's row link does.
const RowLink = styled(Link)`
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 10px ${snzTokens.space.sm};
  color: inherit;
  text-decoration: none;
  min-width: 0;

  li:first-of-type > & {
    border-top-left-radius: calc(${({ theme }) => theme.radius} - ${snzTokens.border.line});
    border-top-right-radius: calc(${({ theme }) => theme.radius} - ${snzTokens.border.line});
  }

  li:last-of-type > & {
    border-bottom-left-radius: calc(${({ theme }) => theme.radius} - ${snzTokens.border.line});
    border-bottom-right-radius: calc(${({ theme }) => theme.radius} - ${snzTokens.border.line});
  }

  &:hover {
    background: ${({ theme }) => theme.surfaceHover};
  }

  &:focus-visible {
    outline: ${snzTokens.border.focus} solid ${({ theme }) => theme.focus};
    outline-offset: calc(-1 * ${snzTokens.border.focus});
  }
`;
