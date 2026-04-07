import fs from "node:fs";
import path from "node:path";
import multer from "multer";
import { config } from "../config.js";

const storage = multer.diskStorage({
  destination: (_req, _file, callback) => {
    fs.mkdirSync(config.uploadDir, { recursive: true });
    callback(null, config.uploadDir);
  },
  filename: (_req, file, callback) => {
    const ext = path.extname(file.originalname);
    callback(null, `${crypto.randomUUID()}${ext}`);
  }
});

export const upload = multer({ storage });

export function toPublicFilePath(filename: string) {
  return `/files/${filename}`;
}
