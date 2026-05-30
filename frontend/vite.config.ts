import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  root: "./frontend",
  envDir: "..",
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:8787",
      "/files": "http://127.0.0.1:8787"
    }
  },
  build: {
    // Resolved relative to `root` (./frontend) -> ./frontend/dist, which is the
    // path embedded by main.go (`//go:embed all:frontend/dist`).
    outDir: "dist",
    emptyOutDir: true
  }
});
