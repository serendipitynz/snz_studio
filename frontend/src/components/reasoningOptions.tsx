import type { ReasoningChoice } from "../api/client";
import { MessageKey, useLanguage } from "../i18n";

const OPTION_LABELS: Record<string, MessageKey> = {
  off: "reasoning.off",
  on: "reasoning.on",
  minimal: "reasoning.minimal",
  low: "reasoning.low",
  medium: "reasoning.medium",
  high: "reasoning.high",
  xhigh: "reasoning.xhigh"
};

// The options of a model's reasoning select: the model's own default first (sent
// as nothing at all), then what LM Studio says the model accepts, in its order.
export function ReasoningOptions({ choice }: { choice: ReasoningChoice }) {
  const { t } = useLanguage();
  const label = (value: string) => (OPTION_LABELS[value] ? t(OPTION_LABELS[value]) : value);
  return (
    <>
      <option value="">{t("reasoning.modelDefault", { value: label(choice.default) })}</option>
      {choice.allowedOptions.map((value) => (
        <option key={value} value={value}>
          {label(value)}
        </option>
      ))}
    </>
  );
}
