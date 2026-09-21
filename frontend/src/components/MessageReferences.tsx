import { AssistantReference } from "../api/client";
import { useLanguage } from "../i18n";
import { Badge, Item, List, Row, Subtle } from "../styles/ui";

// The collapsed "references used" list under a message. One component serves the
// single-assistant chat and the multi-agent conversation so that both screens
// show a turn's references in the same shape.
export function MessageReferences({ references }: { references: AssistantReference[] }) {
  const { t } = useLanguage();
  if (!references.length) {
    return null;
  }

  return (
    <details>
      <summary>{t("chat.referencesUsed", { count: references.length })}</summary>
      <List style={{ marginTop: 10 }}>
        {references.map((reference) => (
          <Item key={reference.id}>
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <strong>{reference.label}</strong>
              <Badge tone={reference.sourceType === "document" ? "warm" : "accent"}>{reference.sourceType}</Badge>
            </Row>
            <Subtle>{reference.excerpt || t("chat.noExcerpt")}</Subtle>
          </Item>
        ))}
      </List>
    </details>
  );
}
