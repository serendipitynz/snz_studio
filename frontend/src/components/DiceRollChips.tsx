import { DiceRoll } from "../api/client";
import { useLanguage } from "../i18n";
import { Row, StateBadge, VisuallyHidden } from "../styles/ui";
import { DiceIcon } from "./icons";

// The chip is a state badge rather than message text so the numbers read as the
// app's, not as something the speaker wrote (design §4.8.3 item 3). The tone
// follows the outcome, and a roll with no target stays neutral. The text wraps
// instead of keeping the badge on one line: an action can run longer than the
// bubble is wide. Relative so the visually hidden label is placed inside the
// badge rather than against the scrolling pane.
const CHIP_STYLE = { gap: 6, flexShrink: 1, overflowWrap: "anywhere", position: "relative" } as const;

export function DiceRollChips({ rolls }: { rolls: DiceRoll[] }) {
  const { t } = useLanguage();
  if (rolls.length === 0) {
    return null;
  }

  return (
    <Row style={{ gap: 6 }}>
      {rolls.map((roll, index) => (
        <StateBadge
          key={index}
          tone={roll.success === null ? "neutral" : roll.success ? "success" : "danger"}
          title={roll.command}
          style={CHIP_STYLE}
        >
          <DiceIcon />
          <VisuallyHidden>{t("multiAgent.diceRoll")}</VisuallyHidden>
          <span>
            {rollText(roll)}
            {roll.success === null
              ? ""
              : ` / ${t("multiAgent.diceOutcome", {
                  target: roll.target,
                  outcome: t(roll.success ? "multiAgent.diceSuccess" : "multiAgent.diceFailure")
                })}`}
          </span>
        </StateBadge>
      ))}
    </Row>
  );
}

// "岩棚を渡る 1d20+3 → 4+3 = 7", the same breakdown the prompt's 【ダイス】 line
// gives the speakers (service.diceRollLine).
function rollText(roll: DiceRoll): string {
  let breakdown = roll.dice.join("+");
  if (roll.modifier > 0) {
    breakdown += `+${roll.modifier}`;
  } else if (roll.modifier < 0) {
    breakdown += `-${-roll.modifier}`;
  }
  if (roll.dice.length > 1 || roll.modifier !== 0) {
    breakdown += ` = ${roll.total}`;
  }
  return `${roll.action ? `${roll.action} ` : ""}${roll.expression} → ${breakdown}`;
}
