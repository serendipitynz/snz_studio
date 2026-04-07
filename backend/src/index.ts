import fs from "node:fs";
import path from "node:path";
import cors from "cors";
import express from "express";
import { config } from "./config.js";
import { getDb } from "./db/connection.js";
import { ChatRepository } from "./repositories/chatRepository.js";
import { DocumentRepository } from "./repositories/documentRepository.js";
import { MemoryRepository } from "./repositories/memoryRepository.js";
import { ProjectRepository } from "./repositories/projectRepository.js";
import { ChatService } from "./services/chatService.js";
import { ContextService } from "./services/contextService.js";
import { LlmClient } from "./services/llmClient.js";
import { MemoryService } from "./services/memoryService.js";
import { EmbeddingClient } from "./services/embeddingClient.js";
import { EmbeddingSyncService } from "./services/embeddingSyncService.js";
import { RetrievalService } from "./services/retrievalService.js";
import { SummaryService } from "./services/summaryService.js";
import { parseTags, truncate } from "./lib/utils.js";
import { upload, toPublicFilePath } from "./storage/fileStorage.js";

const db = getDb();
const projects = new ProjectRepository(db);
const documents = new DocumentRepository(db);
const memories = new MemoryRepository(db);
const chats = new ChatRepository(db);
documents.rebuildSearchIndex();
memories.rebuildSearchIndex();
const embeddingClient = new EmbeddingClient();
const embeddingSync = new EmbeddingSyncService(documents, memories, embeddingClient);
const retrieval = new RetrievalService(db, embeddingClient);
const context = new ContextService(projects, chats, documents, memories, retrieval);
const memoryService = new MemoryService(memories);
const summaryService = new SummaryService();
const llm = new LlmClient();
const chatService = new ChatService(chats, context, llm, summaryService, memoryService, embeddingSync);

const app = express();

app.use(cors({ origin: config.appOrigin, credentials: false }));
app.use(express.json({ limit: "2mb" }));
app.use("/files", express.static(config.uploadDir));

app.get("/api/health", (_req, res) => {
  res.json({ ok: true });
});

app.get("/api/projects", (_req, res) => {
  res.json({ projects: projects.listProjects() });
});

app.post("/api/projects", (req, res) => {
  const title = String(req.body?.title ?? "").trim();
  if (!title) {
    res.status(400).json({ error: "title is required" });
    return;
  }

  const project = projects.createProject({
    title,
    description: typeof req.body?.description === "string" ? req.body.description : "",
    systemPrompt: typeof req.body?.systemPrompt === "string" ? req.body.systemPrompt : ""
  });

  res.status(201).json({ project });
});

app.delete("/api/projects/:projectId", (req, res) => {
  const project = projects.getProject(req.params.projectId);
  if (!project) {
    res.status(404).json({ error: "project not found" });
    return;
  }

  const relatedDocuments = documents.listByProject(project.id);
  const deleted = projects.deleteProject(project.id);

  if (!deleted) {
    res.status(500).json({ error: "failed to delete project" });
    return;
  }

  for (const document of relatedDocuments) {
    if (!document.filePath?.startsWith("/files/")) {
      continue;
    }

    const filename = path.basename(document.filePath);
    const absolutePath = path.join(config.uploadDir, filename);
    if (fs.existsSync(absolutePath)) {
      fs.unlinkSync(absolutePath);
    }
  }

  res.json({ ok: true });
});

app.get("/api/projects/:projectId", (req, res) => {
  const project = projects.getProject(req.params.projectId);
  if (!project) {
    res.status(404).json({ error: "project not found" });
    return;
  }

  res.json({
    project,
    documents: documents.listByProject(project.id),
    memories: memories.listByProject(project.id),
    chats: chats.listByProject(project.id)
  });
});

app.patch("/api/projects/:projectId", (req, res) => {
  const title = String(req.body?.title ?? "").trim();
  if (!title) {
    res.status(400).json({ error: "title is required" });
    return;
  }

  const project = projects.updateProjectTitle(req.params.projectId, title);
  if (!project) {
    res.status(404).json({ error: "project not found" });
    return;
  }

  res.json({ project });
});

app.patch("/api/projects/:projectId/system-prompt", (req, res) => {
  const systemPrompt = typeof req.body?.systemPrompt === "string" ? req.body.systemPrompt : "";
  const project = projects.updateProjectSystemPrompt(req.params.projectId, systemPrompt);
  if (!project) {
    res.status(404).json({ error: "project not found" });
    return;
  }

  res.json({ project });
});

app.post("/api/projects/:projectId/chats", (req, res) => {
  const project = projects.getProject(req.params.projectId);
  if (!project) {
    res.status(404).json({ error: "project not found" });
    return;
  }

  const title = String(req.body?.title ?? "").trim() || "New chat";
  const chat = chats.createChat({
    projectId: project.id,
    title
  });

  res.status(201).json({ chat });
});

app.post("/api/projects/:projectId/memories", async (req, res, next) => {
  try {
    const project = projects.getProject(req.params.projectId);
    if (!project) {
      res.status(404).json({ error: "project not found" });
      return;
    }

    const title = String(req.body?.title ?? "").trim();
    const content = String(req.body?.content ?? "").trim();
    const kindValue = String(req.body?.kind ?? "semantic");

    if (!title || !content) {
      res.status(400).json({ error: "title and content are required" });
      return;
    }

    if (!["semantic", "procedural", "episodic"].includes(kindValue)) {
      res.status(400).json({ error: "invalid memory kind" });
      return;
    }

    const memory = memories.createMemory({
      projectId: project.id,
      title,
      content,
      kind: kindValue as "semantic" | "procedural" | "episodic"
    });

    await embeddingSync.syncMemories([memory.id]);

    res.status(201).json({ memory });
  } catch (error) {
    next(error);
  }
});

app.post("/api/projects/:projectId/documents", upload.single("file"), async (req, res, next) => {
  try {
    const project = projects.getProject(req.params.projectId);
    if (!project) {
      res.status(404).json({ error: "project not found" });
      return;
    }

    const type = String(req.body?.type ?? "").trim();
    if (!["markdown", "text", "image"].includes(type)) {
      res.status(400).json({ error: "invalid document type" });
      return;
    }

    const uploadedFile = req.file;
    let contentText = typeof req.body?.content === "string" ? req.body.content : "";
    let filePath: string | null = null;
    let mimeType: string | null = null;

    if (type === "image") {
      if (!uploadedFile) {
        res.status(400).json({ error: "image file is required" });
        return;
      }
      filePath = toPublicFilePath(uploadedFile.filename);
      mimeType = uploadedFile.mimetype;
    } else if (uploadedFile) {
      contentText = fs.readFileSync(uploadedFile.path, "utf8");
      fs.unlinkSync(uploadedFile.path);
    }

    const title =
      String(req.body?.title ?? "").trim() ||
      uploadedFile?.originalname ||
      truncate(contentText.split("\n")[0]?.trim() || "Untitled document", 80);

    const document = documents.createDocument({
      projectId: project.id,
      type: type as "markdown" | "text" | "image",
      title,
      note: typeof req.body?.note === "string" ? req.body.note : "",
      tags: parseTags(req.body?.tags),
      derivedText: typeof req.body?.derivedText === "string" ? req.body.derivedText : "",
      contentText,
      filePath,
      mimeType
    });

    await embeddingSync.syncDocument(document.id);

    res.status(201).json({ document });
  } catch (error) {
    next(error);
  }
});

app.delete("/api/documents/:documentId", (req, res) => {
  const document = documents.deleteDocument(req.params.documentId);
  if (!document) {
    res.status(404).json({ error: "document not found" });
    return;
  }

  if (document.filePath?.startsWith("/files/")) {
    const filename = path.basename(document.filePath);
    const absolutePath = path.join(config.uploadDir, filename);
    if (fs.existsSync(absolutePath)) {
      fs.unlinkSync(absolutePath);
    }
  }

  res.json({ ok: true, document });
});

app.get("/api/chats/:chatId", (req, res) => {
  const chat = chats.getChat(req.params.chatId);
  if (!chat) {
    res.status(404).json({ error: "chat not found" });
    return;
  }

  const project = projects.getProject(chat.projectId);
  if (!project) {
    res.status(404).json({ error: "project not found" });
    return;
  }

  res.json({
    project,
    chat,
    summary: chats.getSummary(chat.id),
    messages: chats.getMessagesWithReferences(chat.id)
  });
});

app.patch("/api/chats/:chatId", (req, res) => {
  const title = String(req.body?.title ?? "").trim();
  if (!title) {
    res.status(400).json({ error: "title is required" });
    return;
  }

  const chat = chats.updateChatTitle(req.params.chatId, title);
  if (!chat) {
    res.status(404).json({ error: "chat not found" });
    return;
  }

  res.json({ chat });
});

app.delete("/api/chats/:chatId", (req, res) => {
  const chat = chats.deleteChat(req.params.chatId);
  if (!chat) {
    res.status(404).json({ error: "chat not found" });
    return;
  }

  res.json({ ok: true, chat });
});

app.post("/api/chats/:chatId/messages", async (req, res, next) => {
  try {
    const content = String(req.body?.content ?? "").trim();
    if (!content) {
      res.status(400).json({ error: "content is required" });
      return;
    }

    const assistantMessage = await chatService.sendMessage(req.params.chatId, content);
    const chat = chats.getChat(req.params.chatId);
    if (!chat) {
      res.status(404).json({ error: "chat not found" });
      return;
    }

    res.status(201).json({
      message: assistantMessage,
      summary: chats.getSummary(chat.id),
      messages: chats.getMessagesWithReferences(chat.id)
    });
  } catch (error) {
    next(error);
  }
});

app.use((error: unknown, _req: express.Request, res: express.Response, _next: express.NextFunction) => {
  const message = error instanceof Error ? error.message : "Unexpected server error";
  res.status(500).json({ error: message });
});

const uploadRelative = path.relative(process.cwd(), config.uploadDir) || config.uploadDir;

app.listen(config.port, () => {
  console.log(`API server listening on http://127.0.0.1:${config.port}`);
  console.log(`SQLite: ${config.sqlitePath}`);
  console.log(`Uploads: ${uploadRelative}`);
  if (embeddingClient.isEnabled()) {
    console.log(`Embeddings: ${config.embeddingModel} @ ${config.embeddingBaseUrl}`);
    void embeddingSync.rebuildAll().catch((error) => {
      const message = error instanceof Error ? error.message : "unknown embedding rebuild error";
      console.warn(`Embedding rebuild skipped: ${message}`);
    });
  } else {
    console.log("Embeddings: disabled");
  }
});
