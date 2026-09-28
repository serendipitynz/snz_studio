import { CHAT_STATE_SHEET_MAX_CHARS, PARTICIPANT_STATE_SHEET_MAX_CHARS, StateEffect } from "../api/client";
import { MessageKey, useLanguage } from "../i18n";
import { Row, StateBadge, VisuallyHidden } from "../styles/ui";
import { PencilIcon } from "./icons";

// Same badge as a roll's chip (DiceRollChips), so the change reads as the app's
// rather than as something the speaker wrote. An applied effect is neutral —
// a changed value is neither a success nor a failure — and one the app did not
// apply is danger, with why, since the speaker's intent did not happen
// (design §4.8.8).
const CHIP_STYLE = { gap: 6, flexShrink: 1, overflowWrap: "anywhere", position: "relative" } as const;

export function StateEffectChips({ effects }: { effects: StateEffect[] }) {
  const { t } = useLanguage();
  if (effects.length === 0) {
    return null;
  }

  return (
    <Row style={{ gap: 6 }}>
      {effects.map((effect, index) => (
        <StateBadge key={index} tone={effect.applied ? "neutral" : "danger"} title={effect.command} style={CHIP_STYLE}>
          <PencilIcon size={14} />
          <VisuallyHidden>{t(effect.applied ? "multiAgent.stateEffect" : "multiAgent.stateEffectRefused")}</VisuallyHidden>
          <span>{stateEffectText(effect, t)}</span>
        </StateBadge>
      ))}
    </Row>
  );
}

type Translate = (key: MessageKey, vars?: Record<string, string | number>) => string;

// "レン HP -3: 7/10 → 4/10", or "レン たいまつ -1 — not applied (たいまつ is below
// 1)": the line the prompt's 【効果】 gives the speakers (commands.EffectLine),
// with the words in the UI's language.
export function stateEffectText(effect: StateEffect, t: Translate): string {
  const subject = `${effect.owner} ${effect.item}`;
  const refused = () => ` — ${t("multiAgent.effectNotApplied", { reason: effectReason(effect, t) })}`;
  if (effect.kind === "set") {
    if (!effect.applied) {
      return `${subject} → ${effect.value ?? ""}${refused()}`;
    }
    return `${subject}: ${effect.before ?? t("multiAgent.effectNoValue")} → ${effect.after}`;
  }
  const change = effectAmount(effect);
  if (!effect.applied) {
    return `${subject} ${change}${refused()}`;
  }
  return `${subject} ${change}: ${effect.before ?? ""} → ${effect.after}`;
}

function effectAmount(effect: StateEffect): string {
  if (!effect.expression) {
    return effect.delta >= 0 ? `+${effect.delta}` : `${effect.delta}`;
  }
  const dice = effect.dice ?? [];
  if (dice.length === 0) {
    return effect.expression;
  }
  const modifier = effect.modifier ?? 0;
  let breakdown = dice.join("+");
  if (modifier !== 0) {
    breakdown += modifier > 0 ? `+${modifier}` : `${modifier}`;
  }
  if (dice.length > 1 || modifier !== 0) {
    breakdown += ` = ${Math.abs(effect.delta)}`;
  }
  return `${effect.expression}（${breakdown}）`;
}

function effectReason(effect: StateEffect, t: Translate): string {
  switch (effect.reason) {
    case "missing_item":
      return t("multiAgent.effectMissingItem", { item: effect.item });
    case "not_integer":
      return t("multiAgent.effectNotInteger", { item: effect.item });
    case "not_positive":
      return t("multiAgent.effectNotPositive", { item: effect.item });
    case "over_limit":
      return t("multiAgent.effectOverLimit", {
        limit: effect.participantId ? PARTICIPANT_STATE_SHEET_MAX_CHARS : CHAT_STATE_SHEET_MAX_CHARS
      });
    default:
      return effect.reason ?? "";
  }
}
