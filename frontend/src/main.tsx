import { Global, css } from "@emotion/react";
import React from "react";
import ReactDOM from "react-dom/client";
import { HashRouter } from "react-router-dom";
import App from "./App";
import { GetApiBase } from "./wailsjs/go/main/App";
import { theme } from "./styles/theme";

const globalStyles = css`
  :root {
    color-scheme: light;
    font-family: ${theme.font};
    background: ${theme.colors.surface};
  }

  * {
    box-sizing: border-box;
  }

  body {
    margin: 0;
    background: ${theme.colors.surface};
    color: ${theme.colors.ink};
  }

  button,
  input,
  textarea,
  select {
    font: inherit;
  }
`;

// Resolve the API origin from the Wails binding before the first render so every
// request (incl. the SSE streams in ChatPage) targets the loopback Go server
// directly. HashRouter is required because Wails does not support BrowserRouter.
async function bootstrap() {
  try {
    window.__API_BASE__ = (await GetApiBase()) ?? "";
  } catch {
    // window.go is absent when the SPA is opened directly in a browser against
    // the Vite dev server; fall back to relative URLs handled by the Vite proxy.
    window.__API_BASE__ = "";
  }

  ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
    <React.StrictMode>
      <Global styles={globalStyles} />
      <HashRouter>
        <App />
      </HashRouter>
    </React.StrictMode>
  );
}

void bootstrap();
