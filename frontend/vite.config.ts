import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  root: "./frontend",
  envDir: "..",
  plugins: [react()],
  // Development runs via `wails dev`, which launches this Vite server as its
  // watcher and serves the SPA through the WebView. The SPA reaches the Go API
  // by the absolute origin from GetApiBase() (see frontend/src/main.tsx), so no
  // /api or /files proxy is needed here.
  server: {
    port: 5173
  },
  build: {
    // Resolved relative to `root` (./frontend) -> ./frontend/dist, which is the
    // path embedded by main.go (`//go:embed all:frontend/dist`).
    outDir: "dist",
    emptyOutDir: true
  }
});
