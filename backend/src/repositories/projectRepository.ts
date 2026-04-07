import Database from "better-sqlite3";
import { Project } from "../lib/types.js";
import { createId, nowIso } from "../lib/utils.js";

function mapProject(row: Record<string, unknown>): Project {
  return {
    id: String(row.id),
    title: String(row.title),
    description: String(row.description),
    systemPrompt: String(row.system_prompt),
    sortOrder: Number(row.sort_order ?? 0),
    chatCount: Number(row.chat_count ?? 0),
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at)
  };
}

export class ProjectRepository {
  constructor(private readonly db: Database.Database) {}

  listProjects() {
    const rows = this.db
      .prepare(
        `
          SELECT p.*, COUNT(c.id) AS chat_count
          FROM projects p
          LEFT JOIN chats c ON c.project_id = p.id
          GROUP BY p.id
          ORDER BY p.sort_order ASC, p.created_at ASC
        `
      )
      .all() as Record<string, unknown>[];
    return rows.map(mapProject);
  }

  getProject(projectId: string) {
    const row = this.db
      .prepare(
        `
          SELECT p.*, COUNT(c.id) AS chat_count
          FROM projects p
          LEFT JOIN chats c ON c.project_id = p.id
          WHERE p.id = ?
          GROUP BY p.id
        `
      )
      .get(projectId) as Record<string, unknown> | undefined;
    return row ? mapProject(row) : null;
  }

  createProject(input: { title: string; description?: string; systemPrompt?: string }) {
    const nextSortOrder =
      ((this.db.prepare("SELECT COALESCE(MAX(sort_order), -1) AS max_sort_order FROM projects").get() as {
        max_sort_order?: number;
      }).max_sort_order ?? -1) + 1;

    const project: Project = {
      id: createId("project"),
      title: input.title.trim(),
      description: input.description?.trim() ?? "",
      systemPrompt: input.systemPrompt?.trim() ?? "",
      sortOrder: nextSortOrder,
      chatCount: 0,
      createdAt: nowIso(),
      updatedAt: nowIso()
    };

    this.db
      .prepare(
        `
          INSERT INTO projects (id, title, description, system_prompt, sort_order, created_at, updated_at)
          VALUES (@id, @title, @description, @systemPrompt, @sortOrder, @createdAt, @updatedAt)
        `
      )
      .run(project);

    return project;
  }

  updateProjectTitle(projectId: string, title: string) {
    const updatedAt = nowIso();
    const result = this.db
      .prepare("UPDATE projects SET title = ?, updated_at = ? WHERE id = ?")
      .run(title.trim(), updatedAt, projectId);

    if (!result.changes) {
      return null;
    }

    return this.getProject(projectId);
  }

  updateProjectSystemPrompt(projectId: string, systemPrompt: string) {
    const updatedAt = nowIso();
    const result = this.db
      .prepare("UPDATE projects SET system_prompt = ?, updated_at = ? WHERE id = ?")
      .run(systemPrompt.trim(), updatedAt, projectId);

    if (!result.changes) {
      return null;
    }

    return this.getProject(projectId);
  }

  deleteProject(projectId: string) {
    const result = this.db.prepare("DELETE FROM projects WHERE id = ?").run(projectId);
    return result.changes > 0;
  }

  reorderProjects(projectIds: string[]) {
    const existing = new Set(
      (this.db.prepare("SELECT id FROM projects").all() as Array<{ id: string }>).map((row) => String(row.id))
    );

    if (existing.size !== projectIds.length || projectIds.some((id) => !existing.has(id))) {
      return null;
    }

    const updatedAt = nowIso();
    const tx = this.db.transaction(() => {
      const update = this.db.prepare("UPDATE projects SET sort_order = ?, updated_at = ? WHERE id = ?");
      projectIds.forEach((projectId, index) => {
        update.run(index, updatedAt, projectId);
      });
    });

    tx();
    return this.listProjects();
  }
}
