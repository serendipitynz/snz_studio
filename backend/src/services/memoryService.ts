import { MemoryRepository } from "../repositories/memoryRepository.js";
import { MemoryKind, Message } from "../lib/types.js";
import { truncate } from "../lib/utils.js";
import { LlmClient } from "./llmClient.js";

const DURABLE_CUES: Array<{ regex: RegExp; kind: MemoryKind; title: string }> = [
  {
    regex:
      /\b(i prefer|prefer|always|please use|use .* for|style|tone|format|answer in|respond in|avoid using)\b/i,
    kind: "procedural",
    title: "Working preference"
  },
  {
    regex:
      /(を優先|を使って|を使いたい|は使わない|は禁止|避けてください|簡潔に|日本語で|英語で|箇条書きで|短く|詳しく|敬語で|常に|毎回|必ず|今後は|以後は)/u,
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
const MAX_MEMORY_LENGTH = 300;
const REMEMBER_CUES =
  /(覚えて|記憶して|メモリに保存|memory に保存|メモリ化|今後の前提に|今のことを覚えて|保存しておいて|残しておいて)/iu;
const TRANSIENT_REQUEST_CUES =
  /(作成してください|書いてください|考えてください|提案してください|説明してください|要約してください|レビューしてください|翻訳してください|生成してください|直してください|修正してください|教えてください|検討してください|ください。?$|お願いします。?$)/u;
const DURABLE_PROCEDURAL_CUES =
  /(を優先|は使わない|は禁止|避けてください|簡潔に|日本語で|英語で|箇条書きで|短く|詳しく|敬語で|常に|毎回|必ず|今後は|以後は|文体|口調|フォーマット|形式)/u;

function extractJsonObject(input: string) {
  const fenced = input.match(/```json\s*([\s\S]*?)```/i)?.[1];
  if (fenced) {
    return fenced.trim();
  }

  const firstBrace = input.indexOf("{");
  const lastBrace = input.lastIndexOf("}");
  if (firstBrace >= 0 && lastBrace > firstBrace) {
    return input.slice(firstBrace, lastBrace + 1);
  }

  return "";
}

function normalizeMessage(message: Message) {
  const roleLabel = message.role === "assistant" ? "assistant" : message.role === "user" ? "user" : "system";
  return `${roleLabel}: ${truncate(message.content.replace(/\s+/g, " ").trim(), 320)}`;
}

function generateMemoryTitle(content: string, kind: MemoryKind) {
  const normalized = content
    .replace(/^#+\s*/gm, "")
    .replace(/\s+/g, " ")
    .trim();

  const firstSentence = normalized.split(/[。！？.!?\n]/)[0]?.trim() || normalized;
  const base = truncate(firstSentence, 48);

  if (base) {
    return base;
  }

  if (kind === "procedural") {
    return "Working preference";
  }

  if (kind === "episodic") {
    return "Project event";
  }

  return "Project fact";
}

function inferKindFromText(input: string): MemoryKind {
  const matched = DURABLE_CUES.find((cue) => cue.regex.test(input));
  return matched?.kind ?? "episodic";
}

function shouldSkipAutoMemorySentence(sentence: string, kind: MemoryKind) {
  if (kind !== "procedural") {
    return false;
  }

  if (DURABLE_PROCEDURAL_CUES.test(sentence)) {
    return false;
  }

  return TRANSIENT_REQUEST_CUES.test(sentence);
}

export class MemoryService {
  constructor(
    private readonly memories: MemoryRepository,
    private readonly llm: LlmClient
  ) {}

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

      if (sentence.length > MAX_MEMORY_LENGTH) {
        continue;
      }

      const matched = DURABLE_CUES.find((cue) => cue.regex.test(sentence));
      if (!matched) {
        continue;
      }

      if (shouldSkipAutoMemorySentence(sentence, matched.kind)) {
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
          sourceChatId: input.chatId,
          source: "chat",
          locked: false
        })
      );
    }

    return created;
  }

  async maybeStoreFromExplicitRequest(input: {
    projectId: string;
    chatId: string;
    content: string;
    recentMessages: Message[];
  }) {
    if (!REMEMBER_CUES.test(input.content)) {
      return null;
    }

    const recentContext = input.recentMessages
      .filter((message) => message.role !== "system")
      .slice(-4);

    if (!recentContext.length) {
      return null;
    }

    try {
      const response = await this.llm.createChatCompletion({
        systemPrompt: [
          "あなたは project memory 抽出専用アシスタントです。",
          "ユーザーが『覚えて』と明示したので、直前の会話から project をまたいで有用な durable memory を 1 件だけ抽出してください。",
          "返答は JSON のみとし、形式は {\"kind\":\"semantic|procedural|episodic\",\"content\":\"...\"} または {\"skip\":true,\"reason\":\"...\"} にしてください。",
          "content は 1 から 2 文の短い日本語にしてください。",
          "雑談や一時的な話題は保存しないでください。"
        ].join("\n"),
        messages: [],
        userInput: [
          "recent conversation:",
          recentContext.map((message) => normalizeMessage(message)).join("\n"),
          "",
          `latest user request: ${truncate(input.content, 220)}`
        ].join("\n"),
        temperature: 0.1
      });

      const jsonText = extractJsonObject(response.content);
      if (jsonText) {
        const parsed = JSON.parse(jsonText) as {
          skip?: boolean;
          kind?: string;
          content?: string;
        };

        const normalizedContent = typeof parsed.content === "string" ? truncate(parsed.content.trim(), 220) : "";
        const kind = parsed.kind === "semantic" || parsed.kind === "procedural" || parsed.kind === "episodic" ? parsed.kind : inferKindFromText(normalizedContent);

        if (!parsed.skip && normalizedContent) {
          const title = generateMemoryTitle(normalizedContent, kind);
          if (!this.memories.hasSimilarMemory(input.projectId, title, normalizedContent)) {
            return this.memories.createMemory({
              projectId: input.projectId,
              kind,
              title,
              content: normalizedContent,
              sourceChatId: input.chatId,
              source: "chat",
              locked: true
            });
          }
        }
      }
    } catch {
      // Fall through to heuristic fallback.
    }

    const fallbackSource =
      [...recentContext].reverse().find((message) => message.role === "assistant") ??
      [...recentContext].reverse().find((message) => message.role === "user");

    if (!fallbackSource) {
      return null;
    }

    const normalizedContent = truncate(
      (fallbackSource.content.split(/\n+/).find((line) => line.trim()) ?? fallbackSource.content).replace(/\s+/g, " ").trim(),
      220
    );

    if (!normalizedContent) {
      return null;
    }

    const kind = inferKindFromText(`${input.content}\n${normalizedContent}`);
    const title = generateMemoryTitle(normalizedContent, kind);

    if (this.memories.hasSimilarMemory(input.projectId, title, normalizedContent)) {
      return null;
    }

    return this.memories.createMemory({
      projectId: input.projectId,
      kind,
      title,
      content: normalizedContent,
      sourceChatId: input.chatId,
      source: "chat",
      locked: true
    });
  }
}
