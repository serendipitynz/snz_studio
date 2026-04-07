import fs from "node:fs";
import Database from "better-sqlite3";
import { config } from "../config.js";
import { applyMigrations } from "./schema.js";

let dbInstance: Database.Database | null = null;

export function getDb() {
  if (dbInstance) {
    return dbInstance;
  }

  fs.mkdirSync(config.dataDir, { recursive: true });
  fs.mkdirSync(config.uploadDir, { recursive: true });

  dbInstance = new Database(config.sqlitePath);
  applyMigrations(dbInstance);
  return dbInstance;
}
