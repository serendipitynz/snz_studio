import { ChatRepository } from "../repositories/chatRepository.js";
import { MemoryRepository } from "../repositories/memoryRepository.js";
import { EmbeddingSyncService } from "./embeddingSyncService.js";
import { LlmClient } from "./llmClient.js";
import { MemoryKind, MemoryOrganizationChange, MemoryOrganizationPlan } from "../lib/types.js";

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

function isMemoryKind(value: string): value is MemoryKind {
  return value === "semantic" || value === "procedural" || value === "episodic";
}

function sanitizePlan(plan: Partial<MemoryOrganizationPlan>): MemoryOrganizationPlan {
  const changes = Array.isArray(plan.changes)
    ? plan.changes.reduce<MemoryOrganizationChange[]>((accumulator, change) => {
        const normalized = {
          action: change.action,
          memoryId: typeof change.memoryId === "string" ? change.memoryId : undefined,
          kind: typeof change.kind === "string" && isMemoryKind(change.kind) ? change.kind : undefined,
          title: typeof change.title === "string" ? change.title.trim() : undefined,
          content: typeof change.content === "string" ? change.content.trim() : undefined,
          reason: typeof change.reason === "string" ? change.reason.trim() : ""
        };

        if (!normalized.reason) {
          return accumulator;
        }

        if (normalized.action === "remove" && normalized.memoryId) {
          accumulator.push({ action: "remove", memoryId: normalized.memoryId, reason: normalized.reason });
          return accumulator;
        }

        if (normalized.action === "create" && normalized.kind && normalized.title && normalized.content) {
          accumulator.push({
            action: "create",
            kind: normalized.kind,
            title: normalized.title,
            content: normalized.content,
            reason: normalized.reason
          });
          return accumulator;
        }

        if (normalized.action === "update" && normalized.memoryId && normalized.kind && normalized.title && normalized.content) {
          accumulator.push({
            action: "update",
            memoryId: normalized.memoryId,
            kind: normalized.kind,
            title: normalized.title,
            content: normalized.content,
            reason: normalized.reason
          });
        }

        return accumulator;
      }, [])
    : [];

  return {
    summary: typeof plan.summary === "string" ? plan.summary.trim() : "",
    changes
  };
}

function buildFallbackPlan(
  memories: ReturnType<MemoryRepository["listByProject"]>,
  summaries: ReturnType<ChatRepository["listSummariesByProject"]>
): MemoryOrganizationPlan {
  const seen = new Map<string, string>();
  const changes: MemoryOrganizationChange[] = [];

  for (const memory of memories) {
    const key = `${memory.kind}:${memory.title.trim().toLowerCase()}:${memory.content.trim().toLowerCase()}`;
    const existingId = seen.get(key);
    if (existingId) {
      changes.push({
        action: "remove",
        memoryId: memory.id,
        reason: `同一内容の memory (${existingId}) と重複しています。`
      });
      continue;
    }

    seen.set(key, memory.id);
  }

  if (!changes.length && summaries.some((summary) => summary.summary.trim())) {
    changes.push({
      action: "create",
      kind: "episodic",
      title: "Recent project progress",
      content: summaries
        .filter((summary) => summary.summary.trim())
        .slice(0, 2)
        .map((summary) => summary.summary.trim())
        .join(" "),
      reason: "直近の chat summary から継続判断に役立つ経緯を 1 件だけ補完します。"
    });
  }

  return {
    summary: changes.length ? "重複統合と recent summary 由来の補完候補を作成しました。" : "整頓対象は見つかりませんでした。",
    changes
  };
}

export class MemoryOrganizerService {
  constructor(
    private readonly memories: MemoryRepository,
    private readonly chats: ChatRepository,
    private readonly llm: LlmClient,
    private readonly embeddingSync: EmbeddingSyncService
  ) {}

  async analyzeProject(projectId: string): Promise<MemoryOrganizationPlan> {
    const memories = this.memories.listByProject(projectId);
    const summaries = this.chats.listSummariesByProject(projectId).slice(0, 8);

    if (!memories.length && !summaries.length) {
      return {
        summary: "整理対象の memory と chat summary がありません。",
        changes: []
      };
    }

    const userInput = [
      "現在の memories:",
      JSON.stringify(
        memories.map((memory) => ({
          id: memory.id,
          kind: memory.kind,
          title: memory.title,
          content: memory.content
        })),
        null,
        2
      ),
      "recent chat summaries:",
      JSON.stringify(
        summaries.map((summary) => ({
          chatId: summary.chatId,
          summary: summary.summary
        })),
        null,
        2
      ),
      [
        "次の JSON だけを返してください。",
        '{ "summary": "short summary", "changes": [',
        '  { "action": "create"|"update"|"remove", "memoryId": "existing id for update/remove", "kind": "semantic|procedural|episodic", "title": "title", "content": "content", "reason": "reason" }',
        "] }",
        "更新は既存 memory の整理に限定し、無意味な全面書き換えは避けてください。",
        "雑談は memory にしないでください。",
        "changes は最大 8 件に抑えてください。"
      ].join("\n")
    ].join("\n\n");

    try {
      const response = await this.llm.createChatCompletion({
        systemPrompt: [
          "あなたは project memory の整理専用アシスタントです。",
          "semantic / procedural / episodic の区別を守り、重複統合、書き換え、不要候補の削除、新規 memory 候補の作成だけを提案してください。",
          "必ず JSON のみを返してください。"
        ].join("\n"),
        messages: [],
        userInput,
        temperature: 0.1
      });

      const jsonText = extractJsonObject(response.content);
      const parsed = jsonText ? (JSON.parse(jsonText) as Partial<MemoryOrganizationPlan>) : {};
      const plan = sanitizePlan(parsed);
      return plan.changes.length || plan.summary ? plan : buildFallbackPlan(memories, summaries);
    } catch {
      return buildFallbackPlan(memories, summaries);
    }
  }

  async applyProjectPlan(projectId: string, plan: MemoryOrganizationPlan) {
    const affectedMemoryIds: string[] = [];

    for (const change of plan.changes) {
      if (change.action === "remove" && change.memoryId) {
        this.memories.deleteMemory(change.memoryId);
        continue;
      }

      if (change.action === "update" && change.memoryId && change.kind && change.title && change.content) {
        const updated = this.memories.updateMemory({
          memoryId: change.memoryId,
          kind: change.kind,
          title: change.title,
          content: change.content
        });
        if (updated) {
          affectedMemoryIds.push(updated.id);
        }
        continue;
      }

      if (change.action === "create" && change.kind && change.title && change.content) {
        const created = this.memories.createMemory({
          projectId,
          kind: change.kind,
          title: change.title,
          content: change.content
        });
        affectedMemoryIds.push(created.id);
      }
    }

    if (affectedMemoryIds.length) {
      await this.embeddingSync.syncMemories(affectedMemoryIds);
    }

    this.memories.rebuildSearchIndex();
    return this.memories.listByProject(projectId);
  }
}
