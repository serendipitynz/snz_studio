import Database from "better-sqlite3";
import { Project } from "../lib/types.js";
import { createId, nowIso } from "../lib/utils.js";

function mapProject(row: Record<string, unknown>): Project {
  return {
    id: String(row.id),
    title: String(row.title),
    description: String(row.description),
    systemPrompt: String(row.system_prompt),
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at)
  };
}

export class ProjectRepository {
  constructor(private readonly db: Database.Database) {}

  listProjects() {
    const rows = this.db
      .prepare("SELECT * FROM projects ORDER BY updated_at DESC, created_at DESC")
      .all() as Record<string, unknown>[];
    return rows.map(mapProject);
  }

  getProject(projectId: string) {
    const row = this.db
      .prepare("SELECT * FROM projects WHERE id = ?")
      .get(projectId) as Record<string, unknown> | undefined;
    return row ? mapProject(row) : null;
  }

  createProject(input: { title: string; description?: string; systemPrompt?: string }) {
    const project: Project = {
      id: createId("project"),
      title: input.title.trim(),
      description: input.description?.trim() ?? "",
      systemPrompt: input.systemPrompt?.trim() ?? "",
      createdAt: nowIso(),
      updatedAt: nowIso()
    };

    this.db
      .prepare(
        `
          INSERT INTO projects (id, title, description, system_prompt, created_at, updated_at)
          VALUES (@id, @title, @description, @systemPrompt, @createdAt, @updatedAt)
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
}
