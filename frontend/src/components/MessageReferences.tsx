import { AssistantReference } from "../api/client";
import { useLanguage } from "../i18n";
import { Badge, Item, List, Row, Subtle, Summary } from "../styles/ui";

// The collapsed "references used" list under a message. One component serves the
// single-assistant chat and the multi-agent conversation so that both screens
// show a turn's references in the same shape.
//
// A multi-agent turn also names the earlier utterances its speaker read (design
// §4.9.5). They follow the project material rather than mixing into it by score,
// since an utterance score and a retrieval score are not on one scale, and the
// shortened one comes first: it is the only place the screen and what the model
// read disagree. speakerOf names the speaker from the roster the screen shows,
// so a renamed participant reads the same here as on the message itself.
export function MessageReferences({
  references,
  speakerOf,
}: {
  references: AssistantReference[];
  speakerOf?: (messageId: string) => string | undefined;
}) {
  const { t } = useLanguage();
  if (!references.length) {
    return null;
  }

  const material = references.filter((reference) => !isUtterance(reference));
  const shortened = references.filter((reference) => reference.sourceType === "shortened_utterance");
  const read = references.filter((reference) => reference.sourceType === "utterance");
  const speaker = (reference: AssistantReference) => speakerOf?.(reference.sourceId) ?? reference.label;

  return (
    <details>
      <Summary>{t("chat.referencesUsed", { count: references.length })}</Summary>
      <List style={{ marginTop: 10 }}>
        {material.map((reference) => (
          <Item key={reference.id}>
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <strong>{reference.label}</strong>
              <Badge tone={reference.sourceType === "document" ? "warm" : "accent"}>{reference.sourceType}</Badge>
            </Row>
            <Subtle>{reference.excerpt || t("chat.noExcerpt")}</Subtle>
          </Item>
        ))}
        {shortened.map((reference) => (
          <Item key={reference.id}>
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <strong>{speaker(reference)}</strong>
              <Badge tone="warm">{t("chat.referenceShortened")}</Badge>
            </Row>
            <Subtle>{t("chat.referenceShortenedRead", { count: reference.readChars ?? 0 })}</Subtle>
            <Subtle>{reference.excerpt || t("chat.noExcerpt")}</Subtle>
          </Item>
        ))}
        {read.map((reference) => (
          <Item key={reference.id}>
            <Row style={{ justifyContent: "space-between", alignItems: "center" }}>
              <strong>{speaker(reference)}</strong>
              <Badge tone="muted">{t("chat.referenceUtterance")}</Badge>
            </Row>
            <Subtle>{reference.excerpt || t("chat.noExcerpt")}</Subtle>
          </Item>
        ))}
      </List>
    </details>
  );
}

function isUtterance(reference: AssistantReference): boolean {
  return reference.sourceType === "utterance" || reference.sourceType === "shortened_utterance";
}
