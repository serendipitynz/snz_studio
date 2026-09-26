// Ambient declarations for globals injected at runtime.

declare global {
  // package.json's version, injected by Vite's `define` (vite.config.ts).
  const __APP_VERSION__: string;

  interface Window {
    // Absolute origin of the local Go API server, set once at startup from the
    // Wails GetApiBase() binding (see main.tsx). Empty/undefined when the SPA is
    // loaded outside the Wails WebView (window.go absent); there is no Vite
    // proxy, so requests fall back to same-origin relative URLs.
    __API_BASE__?: string;
    // Per-launch token from the Wails GetApiToken() binding, attached to every
    // API/file request so a stray local browser tab can't drive the loopback
    // API. Empty when the binding is unavailable (unsupported host).
    __API_TOKEN__?: string;
    // The Wails binding bridge. It exists only inside the Wails WebView, so its
    // presence is what tells the SPA whether a Go binding can be called at all
    // — checked before use so a genuine binding failure is not mistaken for an
    // unsupported host.
    go?: {
      main?: {
        App?: Record<string, ((...args: never[]) => unknown) | undefined>;
      };
    };
  }
}

export {};
