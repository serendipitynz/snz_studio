export type DocumentType = "markdown" | "text" | "image";
export type DocumentCategory = "world" | "character" | "rule" | "plot" | "timeline" | "index" | "story" | "misc";
export type MessageRole = "system" | "user" | "assistant";
export type MemoryKind = "semantic" | "procedural" | "episodic";
export type MemorySource = "manual" | "chat" | "organized";
export type ReferenceSourceType = "project" | "summary" | "document" | "memory" | "chat";

export interface Project {
  id: string;
  title: string;
  description: string;
  systemPrompt: string;
  createdAt: string;
  updatedAt: string;
}

export interface DocumentRecord {
  id: string;
  projectId: string;
  type: DocumentType;
  category: DocumentCategory;
  title: string;
  note: string;
  tags: string[];
  derivedText: string;
  contentText: string;
  filePath: string | null;
  mimeType: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface Chat {
  id: string;
  projectId: string;
  title: string;
  createdAt: string;
  updatedAt: string;
}

export interface Message {
  id: string;
  chatId: string;
  role: MessageRole;
  content: string;
  createdAt: string;
  responseMs: number | null;
  outputTokens: number | null;
  tokensPerSecond: number | null;
}

export interface ChatSummary {
  chatId: string;
  summary: string;
  updatedAt: string;
}

export interface Memory {
  id: string;
  projectId: string;
  kind: MemoryKind;
  title: string;
  content: string;
  sourceChatId: string | null;
  source: MemorySource;
  locked: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface MemoryOrganizationChange {
  action: "create" | "update" | "remove";
  memoryId?: string;
  kind?: MemoryKind;
  title?: string;
  content?: string;
  reason: string;
}

export interface MemoryOrganizationPlan {
  summary: string;
  changes: MemoryOrganizationChange[];
}

export interface AssistantMessageReference {
  id: string;
  assistantMessageId: string;
  sourceType: ReferenceSourceType;
  sourceId: string;
  label: string;
  excerpt: string;
  score: number;
  createdAt: string;
}

export interface ProjectDetail {
  project: Project;
  documents: DocumentRecord[];
  memories: Memory[];
  chats: Chat[];
}

export interface MessageWithReferences extends Message {
  references: AssistantMessageReference[];
}

export interface ChatDetail {
  project: Project;
  chat: Chat;
  summary: ChatSummary | null;
  messages: MessageWithReferences[];
}

export interface SearchReference {
  sourceType: ReferenceSourceType;
  sourceId: string;
  label: string;
  excerpt: string;
  score: number;
}

export interface RetrievedDocumentChunk {
  chunkId: string;
  chunkIndex: number;
  content: string;
  score: number;
}

export interface RetrievedDocumentReference extends SearchReference {
  sourceType: "document";
  category?: DocumentCategory;
  chunks: RetrievedDocumentChunk[];
  includeFullDocument: boolean;
  fullDocumentContent: string;
  retrievalMode: "search" | "quote";
}

export interface AssembledContext {
  project: Project;
  chat: Chat;
  summary: string;
  recentMessages: Message[];
  promptContext: string;
  references: SearchReference[];
  isQuoteRequest: boolean;
  targetDocumentTitle: string | null;
}
