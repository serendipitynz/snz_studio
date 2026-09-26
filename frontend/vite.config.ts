import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import pkg from "../package.json";

export default defineConfig({
  root: "./frontend",
  envDir: "..",
  plugins: [react()],
  // The sidebar's footer shows the app version; package.json is its single
  // source, injected at build time so the SPA ships no JSON import of its own.
  define: {
    __APP_VERSION__: JSON.stringify(pkg.version)
  },
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
