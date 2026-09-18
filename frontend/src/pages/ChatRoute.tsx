import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { api, ChatKind } from "../api/client";
import { useLanguage } from "../i18n";
import { Card, ErrorText } from "../styles/ui";
import { ChatPage } from "./ChatPage";
import { MultiAgentChatPage } from "./MultiAgentChatPage";

// ChatRoute picks the screen a chat id belongs to. kind is only knowable from the
// server, so the chat is read here and then read again by whichever page renders:
// both pages stay self-contained owners of their own state, and the extra read is
// one loopback GET against local SQLite.
export function ChatRoute() {
  const { chatId = "" } = useParams();
  const { t } = useLanguage();
  const [kind, setKind] = useState<ChatKind | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let current = true;
    setKind(null);
    setError("");

    api
      .getChatDetail(chatId)
      .then((response) => {
        if (current) {
          setKind(response.chat.kind);
        }
      })
      .catch((nextError: unknown) => {
        if (current) {
          setError(nextError instanceof Error ? nextError.message : t("chat.loadError"));
        }
      });

    return () => {
      current = false;
    };
  }, [chatId, t]);

  if (error) {
    return (
      <Card>
        <ErrorText>{error}</ErrorText>
      </Card>
    );
  }

  if (!kind) {
    return <Card>{t("chat.loading")}</Card>;
  }

  return kind === "multi_agent" ? <MultiAgentChatPage /> : <ChatPage />;
}
