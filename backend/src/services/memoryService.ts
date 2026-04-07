import { MemoryRepository } from "../repositories/memoryRepository.js";
import { MemoryKind } from "../lib/types.js";

const DURABLE_CUES: Array<{ regex: RegExp; kind: MemoryKind; title: string }> = [
  {
    regex:
      /\b(i prefer|prefer|always|please use|use .* for|style|tone|format|answer in|respond in|avoid using)\b/i,
    kind: "procedural",
    title: "Working preference"
  },
  {
    regex:
      /(してください|して下さい|でお願いします|をお願いします|を優先|を使って|を使いたい|は使わない|は禁止|避けてください|簡潔に|日本語で|英語で|箇条書きで|短く|詳しく|敬語で|常に)/u,
    kind: "procedural",
    title: "Working preference"
  },
  {
    regex: /\b(my |i am |i work on|project uses|we use|my stack|our stack|this project uses)\b/i,
    kind: "semantic",
    title: "Project fact"
  },
  {
    regex:
      /(このプロジェクトは|このアプリは|技術スタックは|フロントエンドは|バックエンドは|データベースは|前提です|使っています|採用しています|利用しています|で構成します|を使います|にします)/u,
    kind: "semantic",
    title: "Project fact"
  },
  {
    regex: /\b(we decided|we shipped|last time|on \d{4}-\d{2}-\d{2}|previously)\b/i,
    kind: "episodic",
    title: "Project event"
  },
  {
    regex:
      /(前回|以前|先ほど|さっき|今日|昨日|先週|\d{4}[-/年]\d{1,2}[-/月]\d{1,2}日?|に決めた|を決めた|変更した|追加した|削除した|対応した|採用した)/u,
    kind: "episodic",
    title: "Project event"
  }
];

const QUESTION_CUES = /[?？]|(ですか|ますか|でしょうか|できますか|してもいいですか|どうでしょう)/u;

export class MemoryService {
  constructor(private readonly memories: MemoryRepository) {}

  maybeStoreFromUserMessage(input: { projectId: string; chatId: string; content: string }) {
    const sentences = input.content
      .split(/\n|(?<=[.!?。！？])/)
      .map((sentence) => sentence.trim())
      .filter((sentence) => sentence.length >= 10)
      .slice(0, 6);

    const created = [];

    for (const sentence of sentences) {
      if (QUESTION_CUES.test(sentence)) {
        continue;
      }

      const matched = DURABLE_CUES.find((cue) => cue.regex.test(sentence));
      if (!matched) {
        continue;
      }

      if (this.memories.hasSimilarMemory(input.projectId, matched.title, sentence)) {
        continue;
      }

      created.push(
        this.memories.createMemory({
          projectId: input.projectId,
          kind: matched.kind,
          title: matched.title,
          content: sentence,
          sourceChatId: input.chatId
        })
      );
    }

    return created;
  }
}
