export type DocumentType = "markdown" | "text" | "image";
export type DocumentCategory = "world" | "character" | "rule" | "plot" | "timeline" | "index" | "story" | "misc";
export type MemoryKind = "semantic" | "procedural" | "episodic";
export type MemorySource = "manual" | "chat" | "organized";
export type MessageRole = "user" | "assistant" | "system";

export interface Project {
  id: string;
  title: string;
  description: string;
  systemPrompt: string;
  sortOrder: number;
  chatCount: number;
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

export type ChatKind = "assistant" | "multi_agent";
export type TurnRule = "round_robin" | "manual";

export interface ChatRecord {
  id: string;
  projectId: string;
  title: string;
  isTemporary: boolean;
  kind: ChatKind;
  turnRule: TurnRule;
  scenePrompt: string;
  createdAt: string;
  updatedAt: string;
}

// Participant mirrors the Go model.Participant. deletedAt non-null means the
// participant is off the roster; the row survives so an older message still
// resolves to a speaker name (docs/multi-agent-chat-design.md §3).
export interface Participant {
  id: string;
  chatId: string;
  displayName: string;
  rolePrompt: string;
  baseUrl: string;
  modelName: string;
  sortOrder: number;
  createdAt: string;
  deletedAt: string | null;
}

export interface ChatSummary {
  chatId: string;
  summary: string;
  updatedAt: string;
}

// MultiAgentPreset mirrors the Go preset.MultiAgentPreset: the roster, turn rule
// and scene a new multi-agent chat starts from (design §6). Endpoint and model
// are not preset data; they are picked per participant after creation.
export type PresetGroup = "discussion" | "drama" | "hosted" | "pair";

export interface MultiAgentPresetParticipant {
  displayName: string;
  rolePrompt: string;
}

export interface MultiAgentPreset {
  id: string;
  title: string;
  description: string;
  group: PresetGroup | string;
  turnRule: TurnRule;
  scenePrompt: string;
  participants: MultiAgentPresetParticipant[];
}

export interface AssistantReference {
  id: string;
  assistantMessageId: string;
  sourceType: "project" | "summary" | "document" | "memory" | "chat";
  sourceId: string;
  label: string;
  excerpt: string;
  score: number;
  createdAt: string;
}

export interface ReviewReference {
  sourceType: "project" | "summary" | "document" | "memory" | "chat";
  sourceId: string;
  label: string;
  excerpt: string;
  score: number;
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
  modelName: string | null;
  participantId: string | null;
  references: AssistantReference[];
}

export interface WorkspaceConfiguration {
  llmBaseUrl: string;
  llmModel: string;
  llmResponseFormat: "standard" | "llm_jp_thinking";
  reviewBaseUrl: string;
  reviewModel: string;
  embeddingBaseUrl: string;
  embeddingModel: string;
  embeddingMode: "internal" | "external";
  llmConnected: boolean;
  reviewConnected: boolean;
  embeddingConnected: boolean;
}

// EmbeddingStatus mirrors the Go embed.Status: the lifecycle of the bundled internal
// embedding sidecar (model download + llama-server).
export interface EmbeddingStatus {
  state: "disabled" | "downloading" | "starting" | "ready" | "error";
  modelId: string;
  dim: number;
  downloaded: number;
  total: number;
  error?: string;
}

export interface ReviewResponse {
  review: string;
  references: ReviewReference[];
}

// authHeaders merges the per-launch auth token (set in main.tsx) into any
// caller-supplied headers. The loopback server rejects /api calls without it, so
// a stray browser tab on the same machine can't drive the local API.
export function authHeaders(init?: HeadersInit): Headers {
  const headers = new Headers(init);
  const token = window.__API_TOKEN__ ?? "";
  if (token) {
    headers.set("X-SNZ-Studio-Token", token);
  }
  return headers;
}

// fileSrc builds an absolute /files URL carrying the auth token as a query param.
// <img> elements can't send custom headers, so the token rides in the query
// string for static file reads.
export function fileSrc(filePath: string): string {
  const base = `${window.__API_BASE__ ?? ""}${filePath}`;
  const token = window.__API_TOKEN__ ?? "";
  return token ? `${base}?t=${encodeURIComponent(token)}` : base;
}

async function request<T>(input: RequestInfo, init?: RequestInit): Promise<T> {
  // Prefix string paths with the loopback API origin (set in main.tsx). All
  // callers below pass a relative "/api/..." string; Request objects pass through.
  const target = typeof input === "string" ? `${window.__API_BASE__ ?? ""}${input}` : input;
  const response = await fetch(target, { ...init, headers: authHeaders(init?.headers) });
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
    reviewBaseUrl: string;
    reviewModel: string;
    embeddingBaseUrl: string;
    embeddingModel: string;
    embeddingMode: "internal" | "external";
  }) =>
    request<{ configuration: WorkspaceConfiguration }>("/api/configuration", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input)
    }),
  // Normalised here rather than at each call site: a Go handler that marshals a
  // nil slice sends `null`, and every caller feeds this straight into a .map().
  listConfigurationModels: (input: { kind: "llm" | "embedding"; baseUrl: string }) =>
    request<{ models: string[] | null }>("/api/configuration/models", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input)
    }).then((response) => ({ models: response.models ?? [] })),
  getEmbeddingStatus: () => request<EmbeddingStatus>("/api/embedding/status"),
  getProjects: () => request<{ projects: Project[] }>("/api/projects"),
  createProject: (input: { title: string; description: string; systemPrompt: string }) =>
    request<{ project: Project }>("/api/projects", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input)
    }),
  reorderProjects: (projectIds: string[]) =>
    request<{ projects: Project[] }>("/api/projects/reorder", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ projectIds })
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
  // presetId applies a bundled preset; preset applies one read from a file. The
  // server refuses both together and either on a single-assistant chat.
  createChat: (
    projectId: string,
    input: { title: string; isTemporary?: boolean; kind?: ChatKind; presetId?: string; preset?: MultiAgentPreset }
  ) =>
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
  updateChatTemporary: (chatId: string, isTemporary: boolean) =>
    request<{ chat: ChatRecord }>(`/api/chats/${chatId}/temporary`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ isTemporary })
    }),
  deleteChat: (chatId: string) =>
    request<{ ok: boolean; chat: ChatRecord }>(`/api/chats/${chatId}`, {
      method: "DELETE"
    }),
  createMemory: (projectId: string, input: { content: string; kind: MemoryKind; locked?: boolean }) =>
    request<{ memory: MemoryRecord }>(`/api/projects/${projectId}/memories`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input)
    }),
  updateMemoryLock: (memoryId: string, locked: boolean) =>
    request<{ memory: MemoryRecord }>(`/api/memories/${memoryId}/lock`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ locked })
    }),
  deleteMemory: (memoryId: string) =>
    request<{ ok: boolean; memory: MemoryRecord }>(`/api/memories/${memoryId}`, {
      method: "DELETE"
    }),
  analyzeMemoryOrganization: (projectId: string) =>
    request<{ plan: MemoryOrganizationPlan }>(`/api/projects/${projectId}/memories/organize/analyze`, {
      method: "POST"
    }),
  applyMemoryOrganization: (projectId: string, plan: MemoryOrganizationPlan) =>
    request<{ memories: MemoryRecord[] }>(`/api/projects/${projectId}/memories/organize/apply`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ plan })
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
  reviewMessage: (messageId: string) =>
    request<ReviewResponse>(`/api/messages/${messageId}/review`, {
      method: "POST"
    }),
  sendMessage: (chatId: string, content: string) =>
    request<{ chat: ChatRecord; messages: MessageRecord[]; summary: ChatSummary | null }>(`/api/chats/${chatId}/messages`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content })
    }),
  updateChatMultiAgentSettings: (chatId: string, input: { turnRule?: TurnRule; scenePrompt?: string }) =>
    request<{ chat: ChatRecord }>(`/api/chats/${chatId}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input)
    }),
  // The list carries removed participants too, so the spectator view can name the
  // speaker of an older message; callers that want the roster filter on deletedAt.
  listMultiAgentPresets: () => request<{ presets: MultiAgentPreset[] }>("/api/multi-agent-presets"),
  listParticipants: (chatId: string) =>
    request<{ participants: Participant[] }>(`/api/chats/${chatId}/participants`),
  createParticipant: (chatId: string, input: { displayName: string; rolePrompt?: string; baseUrl?: string; modelName?: string }) =>
    request<{ participant: Participant }>(`/api/chats/${chatId}/participants`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input)
    }),
  updateParticipant: (
    participantId: string,
    input: { displayName?: string; rolePrompt?: string; baseUrl?: string; modelName?: string; sortOrder?: number }
  ) =>
    request<{ participant: Participant }>(`/api/participants/${participantId}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input)
    }),
  removeParticipant: (participantId: string) =>
    request<{ ok: boolean; participant: Participant }>(`/api/participants/${participantId}`, {
      method: "DELETE"
    })
};
