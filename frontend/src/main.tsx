import { Global, css } from "@emotion/react";
import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
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

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <Global styles={globalStyles} />
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>
);
