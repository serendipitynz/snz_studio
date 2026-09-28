import { ChatCommands } from "../api/client";
import { MessageKey } from "../i18n";

type Translate = (key: MessageKey, vars?: Record<string, string | number>) => string;

// A candidate replaces the command being typed on the composer's last line;
// select is the part of it chosen once inserted, the first word to fill in.
export interface CommandCandidate {
  text: string;
  select: { start: number; end: number };
}

export interface CommandSuggestions {
  candidates: CommandCandidate[];
  formats: string[];
}

interface CommandHelp {
  keyword: string;
  candidates: CommandCandidate[];
  format: string;
}

// commandSuggestions is what the composer offers for the last line of draft
// (design §4.8.3 item 3, §4.8.8): while a command name is being typed, the
// candidates of every enabled command it could still become, and the format
// once only one can; while the arguments are written, that command's format.
// A chat that has not enabled a command reads its line as text (§4.8.7), so it
// is never offered there.
export function commandSuggestions(draft: string, commands: ChatCommands, t: Translate): CommandSuggestions {
  const line = draft.slice(draft.lastIndexOf("\n") + 1).trimStart();
  const helps = enabledCommandHelp(commands, t);
  if (line.startsWith("/") && !/\s/.test(line)) {
    const typing = helps.filter((help) => help.keyword.startsWith(line));
    return {
      candidates: typing.flatMap((help) => help.candidates),
      formats: typing.length === 1 ? [typing[0].format] : []
    };
  }
  const writing = helps.find((help) => line.startsWith(`${help.keyword} `) || line.startsWith(`${help.keyword}　`));
  return { candidates: [], formats: writing ? [writing.format] : [] };
}

function enabledCommandHelp(commands: ChatCommands, t: Translate): CommandHelp[] {
  const helps: CommandHelp[] = [];
  if (commands.roll) {
    const target = commands.roll.target;
    const action = t("multiAgent.rollAction");
    helps.push({
      keyword: "/roll",
      candidates: [
        selectingLast(`/roll 1d20+0 ${action}`, action),
        selectingLast(`/roll 1d20+0 ${t("multiAgent.rollTargetKeyword")}${target || 12} ${action}`, action)
      ],
      format: t("multiAgent.rollFormat") + (target > 0 ? t("multiAgent.rollFormatDefault", { target }) : "")
    });
  }
  const owner = t("multiAgent.effectOwner");
  const item = t("multiAgent.effectItem");
  const effects: [keyof ChatCommands, string, string, MessageKey][] = [
    ["add", "/add", ` ${owner} ${item} -1`, "multiAgent.addFormat"],
    ["use", "/use", ` ${owner} ${item}`, "multiAgent.useFormat"],
    ["set", "/set", ` ${owner} ${item} ${t("multiAgent.effectValue")}`, "multiAgent.setFormat"]
  ];
  for (const [name, keyword, args, format] of effects) {
    if (commands[name]) {
      const start = keyword.length + 1;
      helps.push({
        keyword,
        candidates: [{ text: keyword + args, select: { start, end: start + owner.length } }],
        format: t(format)
      });
    }
  }
  return helps;
}

function selectingLast(text: string, word: string): CommandCandidate {
  return { text, select: { start: text.length - word.length, end: text.length } };
}
