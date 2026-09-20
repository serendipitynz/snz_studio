import { Global, css, Theme } from "@emotion/react";
import React from "react";
import ReactDOM from "react-dom/client";
import { HashRouter } from "react-router-dom";
import App from "./App";
import { ConfirmProvider } from "./components/ConfirmDialog";
import { LanguageProvider } from "./i18n";
import { ThemeController } from "./styles/ThemeController";
import { GetApiBase, GetApiToken } from "./wailsjs/go/main/App";

const globalStyles = (theme: Theme) => css`
  :root {
    color-scheme: ${theme.scheme};
    font-family: ${theme.font};
    background: ${theme.bg};
  }

  * {
    box-sizing: border-box;
  }

  body {
    margin: 0;
    background: ${theme.bg};
    color: ${theme.ink};
  }

  button,
  input,
  textarea,
  select {
    font: inherit;
  }
`;

// Resolve the API origin and auth token from the Wails bindings before the first
// render so every request (incl. the SSE streams in ChatPage) targets the
// loopback Go server directly and carries the token. HashRouter is required
// because Wails does not support BrowserRouter.
async function bootstrap() {
  try {
    const [base, token] = await Promise.all([GetApiBase(), GetApiToken()]);
    window.__API_BASE__ = base ?? "";
    window.__API_TOKEN__ = token ?? "";
  } catch {
    // window.go is absent only when the SPA is loaded outside the Wails WebView
    // (the only supported host). There is no Vite proxy, so fall back to
    // same-origin relative URLs with no token; the loopback server then rejects
    // API calls, which is the intended outcome for an unsupported host.
    window.__API_BASE__ = "";
    window.__API_TOKEN__ = "";
  }

  ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
    <React.StrictMode>
      <LanguageProvider>
        <ThemeController>
          <Global styles={globalStyles} />
          <ConfirmProvider>
            <HashRouter>
              <App />
            </HashRouter>
          </ConfirmProvider>
        </ThemeController>
      </LanguageProvider>
    </React.StrictMode>
  );
}

void bootstrap();
