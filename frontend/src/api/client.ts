export type DocumentType = "markdown" | "text" | "image";
export type DocumentCategory = "world" | "character" | "rule" | "plot" | "timeline" | "index" | "story" | "misc";
export type MemoryKind = "semantic" | "procedural" | "episodic";
export type MessageRole = "user" | "assistant" | "system";

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

export interface MemoryRecord {
  id: string;
  projectId: string;
  kind: MemoryKind;
  title: string;
  content: string;
  sourceChatId: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface ChatRecord {
  id: string;
  projectId: string;
  title: string;
  createdAt: string;
  updatedAt: string;
}

export interface ChatSummary {
  chatId: string;
  summary: string;
  updatedAt: string;
}

export interface AssistantReference {
  id: string;
  assistantMessageId: string;
  sourceType: "project" | "summary" | "document" | "memory";
  sourceId: string;
  label: string;
  excerpt: string;
  score: number;
  createdAt: string;
}

export interface MessageRecord {
  id: string;
  chatId: string;
  role: MessageRole;
  content: string;
  createdAt: string;
  responseMs: number | null;
  outputTokens: number | null;
  tokensPerSecond: number | null;
  references: AssistantReference[];
}

export interface WorkspaceConfiguration {
  llmBaseUrl: string;
  llmModel: string;
  llmResponseFormat: "standard" | "llm_jp_thinking";
  embeddingBaseUrl: string;
  embeddingModel: string;
  llmConnected: boolean;
  embeddingConnected: boolean;
}

async function request<T>(input: RequestInfo, init?: RequestInit): Promise<T> {
  const response = await fetch(input, init);
  if (!response.ok) {
    const data = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new Error(data?.error ?? `Request failed with ${response.status}`);
  }
  return (await response.json()) as T;
}

export const api = {
  getConfiguration: () => request<{ configuration: WorkspaceConfiguration }>("/api/configuration"),
  updateConfiguration: (input: {
    llmBaseUrl: string;
    llmModel: string;
    llmResponseFormat: "standard" | "llm_jp_thinking";
    embeddingBaseUrl: string;
    embeddingModel: string;
  }) =>
    request<{ configuration: WorkspaceConfiguration }>("/api/configuration", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input)
    }),
  listConfigurationModels: (input: { kind: "llm" | "embedding"; baseUrl: string }) =>
    request<{ models: string[] }>("/api/configuration/models", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input)
    }),
  getProjects: () => request<{ projects: Project[] }>("/api/projects"),
  createProject: (input: { title: string; description: string; systemPrompt: string }) =>
    request<{ project: Project }>("/api/projects", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input)
    }),
  updateProjectTitle: (projectId: string, title: string) =>
    request<{ project: Project }>(`/api/projects/${projectId}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title })
    }),
  updateProjectSystemPrompt: (projectId: string, systemPrompt: string) =>
    request<{ project: Project }>(`/api/projects/${projectId}/system-prompt`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ systemPrompt })
    }),
  deleteProject: (projectId: string) =>
    request<{ ok: boolean }>(`/api/projects/${projectId}`, {
      method: "DELETE"
    }),
  getProjectDetail: (projectId: string) =>
    request<{ project: Project; documents: DocumentRecord[]; memories: MemoryRecord[]; chats: ChatRecord[] }>(
      `/api/projects/${projectId}`
    ),
  createChat: (projectId: string, input: { title: string }) =>
    request<{ chat: ChatRecord }>(`/api/projects/${projectId}/chats`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input)
    }),
  updateChatTitle: (chatId: string, title: string) =>
    request<{ chat: ChatRecord }>(`/api/chats/${chatId}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title })
    }),
  deleteChat: (chatId: string) =>
    request<{ ok: boolean; chat: ChatRecord }>(`/api/chats/${chatId}`, {
      method: "DELETE"
    }),
  createMemory: (projectId: string, input: { title: string; content: string; kind: MemoryKind }) =>
    request<{ memory: MemoryRecord }>(`/api/projects/${projectId}/memories`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input)
    }),
  createDocument: (projectId: string, formData: FormData) =>
    request<{ document: DocumentRecord }>(`/api/projects/${projectId}/documents`, {
      method: "POST",
      body: formData
    }),
  updateDocumentCategory: (documentId: string, category: DocumentCategory) =>
    request<{ document: DocumentRecord }>(`/api/documents/${documentId}/category`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ category })
    }),
  deleteDocument: (documentId: string) =>
    request<{ ok: boolean; document: DocumentRecord }>(`/api/documents/${documentId}`, {
      method: "DELETE"
    }),
  getChatDetail: (chatId: string) =>
    request<{ project: Project; chat: ChatRecord; summary: ChatSummary | null; messages: MessageRecord[] }>(`/api/chats/${chatId}`),
  sendMessage: (chatId: string, content: string) =>
    request<{ chat: ChatRecord; messages: MessageRecord[]; summary: ChatSummary | null }>(`/api/chats/${chatId}/messages`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content })
    })
};
