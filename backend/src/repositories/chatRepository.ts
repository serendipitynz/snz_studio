import Database from "better-sqlite3";
import {
  AssistantMessageReference,
  Chat,
  ChatSummary,
  Message,
  MessageRole,
  MessageWithReferences
} from "../lib/types.js";
import { createId, nowIso } from "../lib/utils.js";

function mapChat(row: Record<string, unknown>): Chat {
  return {
    id: String(row.id),
    projectId: String(row.project_id),
    title: String(row.title),
    isTemporary: Boolean(row.is_temporary),
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at)
  };
}

function mapMessage(row: Record<string, unknown>): Message {
  return {
    id: String(row.id),
    chatId: String(row.chat_id),
    role: row.role as MessageRole,
    content: String(row.content),
    createdAt: String(row.created_at),
    responseMs: row.response_ms == null ? null : Number(row.response_ms),
    outputTokens: row.output_tokens == null ? null : Number(row.output_tokens),
    tokensPerSecond: row.tokens_per_second == null ? null : Number(row.tokens_per_second)
  };
}

function mapSummary(row: Record<string, unknown>): ChatSummary {
  return {
    chatId: String(row.chat_id),
    summary: String(row.summary),
    updatedAt: String(row.updated_at)
  };
}

function mapReference(row: Record<string, unknown>): AssistantMessageReference {
  return {
    id: String(row.id),
    assistantMessageId: String(row.assistant_message_id),
    sourceType: row.source_type as AssistantMessageReference["sourceType"],
    sourceId: String(row.source_id),
    label: String(row.label),
    excerpt: String(row.excerpt),
    score: Number(row.score),
    createdAt: String(row.created_at)
  };
}

export class ChatRepository {
  constructor(private readonly db: Database.Database) {}

  listByProject(projectId: string) {
    const rows = this.db
      .prepare("SELECT * FROM chats WHERE project_id = ? ORDER BY updated_at DESC, created_at DESC")
      .all(projectId) as Record<string, unknown>[];
    return rows.map(mapChat);
  }

  getChat(chatId: string) {
    const row = this.db
      .prepare("SELECT * FROM chats WHERE id = ?")
      .get(chatId) as Record<string, unknown> | undefined;
    return row ? mapChat(row) : null;
  }

  createChat(input: { projectId: string; title: string; isTemporary?: boolean }) {
    const chat: Chat = {
      id: createId("chat"),
      projectId: input.projectId,
      title: input.title.trim(),
      isTemporary: input.isTemporary ?? false,
      createdAt: nowIso(),
      updatedAt: nowIso()
    };

    this.db
      .prepare(
        `
          INSERT INTO chats (id, project_id, title, is_temporary, created_at, updated_at)
          VALUES (?, ?, ?, ?, ?, ?)
        `
      )
      .run(chat.id, chat.projectId, chat.title, Number(chat.isTemporary), chat.createdAt, chat.updatedAt);

    this.upsertSummary(chat.id, "");
    return chat;
  }

  updateChatTitle(chatId: string, title: string) {
    const updatedAt = nowIso();
    const result = this.db
      .prepare("UPDATE chats SET title = ?, updated_at = ? WHERE id = ?")
      .run(title.trim(), updatedAt, chatId);

    if (!result.changes) {
      return null;
    }

    return this.getChat(chatId);
  }

  setTemporary(chatId: string, isTemporary: boolean) {
    const updatedAt = nowIso();
    const result = this.db
      .prepare("UPDATE chats SET is_temporary = ?, updated_at = ? WHERE id = ?")
      .run(Number(isTemporary), updatedAt, chatId);

    if (!result.changes) {
      return null;
    }

    return this.getChat(chatId);
  }

  deleteChat(chatId: string) {
    const chat = this.getChat(chatId);
    if (!chat) {
      return null;
    }

    this.db.prepare("DELETE FROM chats WHERE id = ?").run(chatId);
    return chat;
  }

  addMessage(input: {
    chatId: string;
    role: MessageRole;
    content: string;
    responseMs?: number | null;
    outputTokens?: number | null;
    tokensPerSecond?: number | null;
  }) {
    const message: Message = {
      id: createId("msg"),
      chatId: input.chatId,
      role: input.role,
      content: input.content.trim(),
      createdAt: nowIso(),
      responseMs: input.responseMs ?? null,
      outputTokens: input.outputTokens ?? null,
      tokensPerSecond: input.tokensPerSecond ?? null
    };

    const tx = this.db.transaction(() => {
      this.db
        .prepare(
          "INSERT INTO messages (id, chat_id, role, content, created_at, response_ms, output_tokens, tokens_per_second) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
        )
        .run(
          message.id,
          message.chatId,
          message.role,
          message.content,
          message.createdAt,
          message.responseMs,
          message.outputTokens,
          message.tokensPerSecond
        );
      this.db.prepare("UPDATE chats SET updated_at = ? WHERE id = ?").run(nowIso(), message.chatId);
    });

    tx();
    return message;
  }

  listMessages(chatId: string) {
    const rows = this.db
      .prepare("SELECT * FROM messages WHERE chat_id = ? ORDER BY created_at ASC")
      .all(chatId) as Record<string, unknown>[];
    return rows.map(mapMessage);
  }

  listRecentMessages(chatId: string, limit = 8) {
    const rows = this.db
      .prepare(
        `
          SELECT * FROM messages
          WHERE chat_id = ?
          ORDER BY created_at DESC
          LIMIT ?
        `
      )
      .all(chatId, limit) as Record<string, unknown>[];
    return rows.reverse().map(mapMessage);
  }

  getSummary(chatId: string) {
    const row = this.db
      .prepare("SELECT * FROM chat_summaries WHERE chat_id = ?")
      .get(chatId) as Record<string, unknown> | undefined;
    return row ? mapSummary(row) : null;
  }

  listSummariesByProject(projectId: string, includeTemporary = true) {
    const rows = this.db
      .prepare(
        `
          SELECT s.*
          FROM chat_summaries s
          JOIN chats c ON c.id = s.chat_id
          WHERE c.project_id = ?
            AND (? = 1 OR c.is_temporary = 0)
          ORDER BY s.updated_at DESC
        `
      )
      .all(projectId, includeTemporary ? 1 : 0) as Record<string, unknown>[];
    return rows.map(mapSummary);
  }

  upsertSummary(chatId: string, summary: string) {
    const updatedAt = nowIso();
    this.db
      .prepare(
        `
          INSERT INTO chat_summaries (chat_id, summary, updated_at)
          VALUES (?, ?, ?)
          ON CONFLICT(chat_id) DO UPDATE SET summary = excluded.summary, updated_at = excluded.updated_at
        `
      )
      .run(chatId, summary, updatedAt);
  }

  replaceAssistantReferences(assistantMessageId: string, references: Omit<AssistantMessageReference, "id" | "assistantMessageId" | "createdAt">[]) {
    const tx = this.db.transaction(() => {
      this.db.prepare("DELETE FROM assistant_message_references WHERE assistant_message_id = ?").run(assistantMessageId);
      const insert = this.db.prepare(
        `
          INSERT INTO assistant_message_references (
            id, assistant_message_id, source_type, source_id, label, excerpt, score, created_at
          ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        `
      );

      references.forEach((reference) => {
        insert.run(
          createId("ref"),
          assistantMessageId,
          reference.sourceType,
          reference.sourceId,
          reference.label,
          reference.excerpt,
          reference.score,
          nowIso()
        );
      });
    });

    tx();
  }

  getMessagesWithReferences(chatId: string): MessageWithReferences[] {
    const messages = this.listMessages(chatId);
    const assistantIds = messages.filter((message) => message.role === "assistant").map((message) => message.id);
    if (!assistantIds.length) {
      return messages.map((message) => ({ ...message, references: [] }));
    }

    const placeholder = assistantIds.map(() => "?").join(", ");
    const rows = this.db
      .prepare(
        `SELECT * FROM assistant_message_references WHERE assistant_message_id IN (${placeholder}) ORDER BY score DESC, created_at ASC`
      )
      .all(...assistantIds) as Record<string, unknown>[];

    const refsByMessage = new Map<string, AssistantMessageReference[]>();
    rows.map(mapReference).forEach((reference) => {
      const current = refsByMessage.get(reference.assistantMessageId) ?? [];
      current.push(reference);
      refsByMessage.set(reference.assistantMessageId, current);
    });

    return messages.map((message) => ({
      ...message,
      references: refsByMessage.get(message.id) ?? []
    }));
  }
}
