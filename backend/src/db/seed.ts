import fs from "node:fs";
import { config } from "../config.js";
import { getDb } from "./connection.js";
import { ChatRepository } from "../repositories/chatRepository.js";
import { DocumentRepository } from "../repositories/documentRepository.js";
import { MemoryRepository } from "../repositories/memoryRepository.js";
import { ProjectRepository } from "../repositories/projectRepository.js";

fs.mkdirSync(config.dataDir, { recursive: true });
fs.mkdirSync(config.uploadDir, { recursive: true });

const db = getDb();
const projects = new ProjectRepository(db);
const documents = new DocumentRepository(db);
const memories = new MemoryRepository(db);
const chats = new ChatRepository(db);

db.exec(`
  DELETE FROM assistant_message_references;
  DELETE FROM messages;
  DELETE FROM chat_summaries;
  DELETE FROM chats;
  DELETE FROM document_chunks_fts;
  DELETE FROM document_chunks;
  DELETE FROM document_chunk_embeddings;
  DELETE FROM documents;
  DELETE FROM memories_fts;
  DELETE FROM memory_embeddings;
  DELETE FROM memories;
  DELETE FROM projects;
`);

const project = projects.createProject({
  title: "Personal LLM Workspace",
  description: "A lightweight local workspace for product notes, prompts, and experiments.",
  systemPrompt: "Prefer practical implementation advice. Use concise structured answers."
});

const roadmap = documents.createDocument({
  projectId: project.id,
  type: "markdown",
  title: "Roadmap",
  contentText: `# Roadmap

- Build a fast local-only project workspace
- Keep the retrieval layer replaceable
- Avoid vector databases in the first version
- Make references visible in the chat UI`,
  note: "Shared project plan",
  tags: ["planning", "product"]
});

documents.createDocument({
  projectId: project.id,
  type: "text",
  title: "Prompt Guidelines",
  contentText: `Respond clearly.
Use references only when relevant.
Prefer stable local storage and simple architecture.`,
  note: "Prompt rules for the assistant",
  tags: ["prompt", "rules"]
});

memories.createMemory({
  projectId: project.id,
  kind: "procedural",
  title: "Default response style",
  content: "Use short, practical answers and keep implementation decisions explicit."
});

const runtimeTarget = memories.createMemory({
  projectId: project.id,
  kind: "semantic",
  title: "Runtime target",
  content: "The app runs locally on a single user machine with SQLite and filesystem storage."
});

const chat = chats.createChat({
  projectId: project.id,
  title: "Architecture notes"
});

chats.addMessage({
  chatId: chat.id,
  role: "user",
  content: "Summarize the intended architecture for this project."
});

const assistantMessage = chats.addMessage({
  chatId: chat.id,
  role: "assistant",
  content: "Use a small React frontend, a local TypeScript API server, SQLite for persistence, and FTS-based retrieval. Keep memory and summaries separate from raw message history."
});

chats.replaceAssistantReferences(assistantMessage.id, [
  {
    sourceType: "document",
    sourceId: roadmap.id,
    label: roadmap.title,
    excerpt: "Build a fast local-only project workspace and keep retrieval replaceable.",
    score: 0.91
  },
  {
    sourceType: "memory",
    sourceId: runtimeTarget.id,
    label: runtimeTarget.title,
    excerpt: "The app runs locally on a single user machine with SQLite and filesystem storage.",
    score: 0.84
  }
]);

chats.upsertSummary(
  chat.id,
  "The chat established a minimal local architecture: React + Vite frontend, TypeScript API server, SQLite, filesystem document storage, FTS retrieval, and visible references."
);

console.log(`Seeded sample project: ${project.title}`);
