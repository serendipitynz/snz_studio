// Ambient declarations for globals injected at runtime.

declare global {
  interface Window {
    // Absolute origin of the local Go API server, set once at startup from the
    // Wails GetApiBase() binding (see main.tsx). Empty/undefined when the SPA is
    // opened directly in a browser against the Vite dev server, where the Vite
    // proxy handles /api and /files and relative URLs suffice.
    __API_BASE__?: string;
  }
}

export {};
