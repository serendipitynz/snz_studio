import { ChatRepository } from "../repositories/chatRepository.js";
import { DocumentRepository } from "../repositories/documentRepository.js";
import { MemoryRepository } from "../repositories/memoryRepository.js";
import { ProjectRepository } from "../repositories/projectRepository.js";
import { AssembledContext, DocumentRecord, RetrievedDocumentReference, SearchReference } from "../lib/types.js";
import { RetrievalService } from "./retrievalService.js";
import { truncate } from "../lib/utils.js";
import { config } from "../config.js";

const QUOTE_REQUEST_PATTERN = /(引用|quote|quoted|引用して|原文|そのまま|抜き出|抜粋|該当箇所|当該箇所)/iu;
const FULL_DOCUMENT_REQUEST_PATTERN = /(全文|全体|全内容|全部|全編|full document|entire document|whole document|文書全体|ドキュメント全体)/iu;
const DOCUMENT_REVIEW_PATTERN = /(要約|まとめ|summary|review|説明|整理|構造|outline|全体像|レビュー)/iu;
const THIS_DOCUMENT_PATTERN = /(この document|this document|この文書|このドキュメント)/iu;
const CHAT_REFERENCE_PATTERN = /(チャット|chat)/iu;
const GENERIC_QUERY_NOISE_PATTERN =
  /(引用|quote|quoted|原文|そのまま|抜き出して|抜粋して|該当箇所|この document|this document|この文書|このドキュメント|全文|全体|要約|まとめ|説明|してください|お願いします)/giu;
const FULL_DOCUMENT_CHAR_LIMIT = 12000;

function normalizeForMatch(input: string) {
  return input.normalize("NFKC").toLowerCase();
}

function resolveExplicitDocument(userInput: string, documents: DocumentRecord[]) {
  const normalizedInput = normalizeForMatch(userInput);
  const matchingByTitle = documents
    .filter((document) => {
      const normalizedTitle = normalizeForMatch(document.title);
      return normalizedTitle.length >= 2 && normalizedInput.includes(normalizedTitle);
    })
    .sort((left, right) => right.title.length - left.title.length);

  if (matchingByTitle.length) {
    return matchingByTitle[0];
  }

  if (documents.length === 1 && THIS_DOCUMENT_PATTERN.test(userInput)) {
    return documents[0];
  }

  return null;
}

function buildDocumentFocusedQuery(userInput: string, documentTitle: string) {
  return userInput.replaceAll(documentTitle, " ").replace(GENERIC_QUERY_NOISE_PATTERN, " ").trim();
}

function resolveExplicitChat(userInput: string, chats: ReturnType<ChatRepository["listByProject"]>, currentChatId: string) {
  if (!CHAT_REFERENCE_PATTERN.test(userInput)) {
    return null;
  }

  const normalizedInput = normalizeForMatch(userInput);
  return chats
    .filter((chat) => chat.id !== currentChatId)
    .filter((chat) => {
      const title = chat.title.trim();
      if (!title) {
        return false;
      }

      const normalizedTitle = normalizeForMatch(title);
      return normalizedTitle.length >= 2 && normalizedInput.includes(normalizedTitle);
    })
    .sort((left, right) => right.title.length - left.title.length)[0] ?? null;
}

function shouldIncludeFullDocument(userInput: string, document: DocumentRecord) {
  const documentText = document.contentText || document.derivedText || document.note;
  if (!documentText || documentText.length > FULL_DOCUMENT_CHAR_LIMIT) {
    return false;
  }

  return QUOTE_REQUEST_PATTERN.test(userInput) || FULL_DOCUMENT_REQUEST_PATTERN.test(userInput) || DOCUMENT_REVIEW_PATTERN.test(userInput);
}

function formatDocumentContext(reference: RetrievedDocumentReference) {
  const matchedChunks = reference.chunks.length
    ? reference.chunks
        .map((chunk) => `- chunk ${chunk.chunkIndex + 1}: ${truncate(chunk.content, 400)}`)
        .join("\n")
    : "- no matching chunks were found";

  const fullDocumentSection = reference.includeFullDocument
    ? `\nFull document content:\n${reference.fullDocumentContent}`
    : "";

  return `[Document${reference.category ? `:${reference.category}` : ""}] ${reference.label}
Retrieval mode: ${reference.retrievalMode}
Matched passages:
${matchedChunks}${fullDocumentSection}`;
}

function formatChatContext(input: {
  title: string;
  summary: string;
  recentMessages: ReturnType<ChatRepository["listRecentMessages"]>;
}) {
  const recentTurns = input.recentMessages.length
    ? input.recentMessages.map((message) => `- ${message.role}: ${truncate(message.content, 220)}`).join("\n")
    : "- no recent turns";

  return `[Referenced chat] ${input.title}
Summary:
${input.summary || "No summary available."}

Recent turns:
${recentTurns}`;
}

function debugInfo(message: string) {
  if (config.debugChatFlow) {
    console.info(message);
  }
}

export class ContextService {
  constructor(
    private readonly projects: ProjectRepository,
    private readonly chats: ChatRepository,
    private readonly documents: DocumentRepository,
    private readonly memories: MemoryRepository,
    private readonly retrieval: RetrievalService
  ) {}

  async assemble(chatId: string, userInput: string): Promise<AssembledContext> {
    const startedAt = Date.now();
    debugInfo(`[context] assemble start ${JSON.stringify({ chatId, userInputChars: userInput.length })}`);

    const chat = this.chats.getChat(chatId);
    if (!chat) {
      throw new Error("Chat not found");
    }

    const project = this.projects.getProject(chat.projectId);
    if (!project) {
      throw new Error("Project not found");
    }

    const summary = this.chats.getSummary(chatId)?.summary ?? "";
    const proceduralMemories = this.memories.listByProjectAndKind(project.id, "procedural").slice(0, 4);
    const projectDocuments = this.documents.listByProject(project.id);
    const projectChats = this.chats.listByProject(project.id);
    const explicitDocument = resolveExplicitDocument(userInput, projectDocuments);
    const explicitChat = resolveExplicitChat(userInput, projectChats, chat.id);
    const isQuoteRequest = QUOTE_REQUEST_PATTERN.test(userInput);
    const recentMessages = this.chats.listRecentMessages(chatId, 6);
    const explicitChatSummary = explicitChat ? this.chats.getSummary(explicitChat.id)?.summary ?? "" : "";
    const explicitChatMessages = explicitChat ? this.chats.listRecentMessages(explicitChat.id, 6) : [];

    debugInfo(
      `[context] assemble inputs ${JSON.stringify({
        chatId,
        projectId: project.id,
        summaryChars: summary.length,
        proceduralMemoryCount: proceduralMemories.length,
        projectDocumentCount: projectDocuments.length,
        projectChatCount: projectChats.length,
        explicitDocument: explicitDocument?.title ?? null,
        explicitChat: explicitChat?.title ?? null,
        isQuoteRequest
      })}`
    );

    const [documentRefs, memoryRefs] = await Promise.all([
      this.retrieval.searchDocuments(project.id, userInput, 4, 3),
      this.retrieval.searchMemories(project.id, userInput, 4)
    ]);

    if (explicitDocument) {
      const focusedQuery = buildDocumentFocusedQuery(userInput, explicitDocument.title);
      const quoteChunks = this.retrieval.searchChunksInDocument(explicitDocument.id, focusedQuery, isQuoteRequest ? 5 : 3);
      const fullDocumentContent = explicitDocument.contentText || explicitDocument.derivedText || explicitDocument.note;
      const explicitReference: RetrievedDocumentReference = {
        sourceType: "document",
        sourceId: explicitDocument.id,
        label: explicitDocument.title,
        category: explicitDocument.category,
        excerpt: quoteChunks.length
          ? quoteChunks.map((chunk) => `[chunk ${chunk.chunkIndex + 1}] ${truncate(chunk.content, 150)}`).join("\n")
          : truncate(fullDocumentContent, 220),
        score: isQuoteRequest ? 1.15 : 1.02,
        chunks: quoteChunks,
        includeFullDocument: shouldIncludeFullDocument(userInput, explicitDocument),
        fullDocumentContent,
        retrievalMode: isQuoteRequest ? "quote" : "search"
      };

      documentRefs.unshift(explicitReference);
    }

    const normalizedDocumentRefs = documentRefs
      .filter((reference, index, items) => index === items.findIndex((candidate) => candidate.sourceId === reference.sourceId))
      .slice(0, explicitDocument ? 3 : 4);

    const references: SearchReference[] = [
      {
        sourceType: "project",
        sourceId: project.id,
        label: `${project.title} settings`,
        excerpt: truncate(project.description || project.systemPrompt || project.title, 220),
        score: 1
      }
    ];

    if (summary.trim()) {
      references.push({
        sourceType: "summary",
        sourceId: chat.id,
        label: "Chat summary",
        excerpt: truncate(summary, 220),
        score: 0.92
      });
    }

    if (explicitChat) {
      references.push({
        sourceType: "chat",
        sourceId: explicitChat.id,
        label: `Chat: ${explicitChat.title || "(undefined)"}`,
        excerpt: truncate(explicitChatSummary || explicitChatMessages.map((message) => message.content).join(" "), 220),
        score: 0.9
      });
    }

    proceduralMemories.forEach((memory, index) => {
      references.push({
        sourceType: "memory",
        sourceId: memory.id,
        label: `[procedural] ${memory.title}`,
        excerpt: truncate(memory.content, 220),
        score: 0.88 - index * 0.02
      });
    });

    [...normalizedDocumentRefs, ...memoryRefs].forEach((reference) => {
      if (!references.some((current) => current.sourceType === reference.sourceType && current.sourceId === reference.sourceId)) {
        references.push(reference);
      }
    });

    const promptContext = [
      `Project title: ${project.title}`,
      project.description ? `Project description:\n${project.description}` : "",
      project.systemPrompt ? `Project system prompt:\n${project.systemPrompt}` : "",
      summary ? `Chat summary:\n${summary}` : "",
      proceduralMemories.length
        ? `Persistent procedural memory:\n${proceduralMemories
            .map((memory) => `- ${memory.title}: ${memory.content}`)
            .join("\n")}`
        : "",
      isQuoteRequest
        ? "Document quote mode is active. Quote only from the provided document passages or full document content. If the exact supporting text is not present, say so plainly."
        : "",
      explicitChat
        ? `Referenced project chat:\n${formatChatContext({
            title: explicitChat.title || "(undefined)",
            summary: explicitChatSummary,
            recentMessages: explicitChatMessages
          })}`
        : "",
      normalizedDocumentRefs.length
        ? `Relevant project documents:\n${normalizedDocumentRefs
            .map((ref) => formatDocumentContext(ref))
            .join("\n\n")}`
        : "",
      memoryRefs.length
        ? `Relevant project memories:\n${memoryRefs
            .map((ref) => `- ${ref.label}: ${ref.excerpt}`)
            .join("\n")}`
        : ""
    ]
      .filter(Boolean)
      .join("\n\n");

    const assembled = {
      project,
      chat,
      summary,
      recentMessages,
      promptContext,
      references: references.slice(0, 8),
      isQuoteRequest,
      targetDocumentTitle: explicitDocument?.title ?? null
    };

    debugInfo(
      `[context] assemble done ${JSON.stringify({
        chatId,
        durationMs: Date.now() - startedAt,
        documentRefCount: documentRefs.length,
        memoryRefCount: memoryRefs.length,
        finalReferenceCount: assembled.references.length,
        promptContextChars: assembled.promptContext.length
      })}`
    );

    return assembled;
  }
}
