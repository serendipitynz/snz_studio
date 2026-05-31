import { useTheme } from "@emotion/react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

interface MarkdownPreviewProps {
  source: string;
}

export function MarkdownPreview({ source }: MarkdownPreviewProps) {
  const t = useTheme();
  return (
    <div style={{ lineHeight: 1.7, overflowWrap: "anywhere" }}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          h1: ({ children }) => <h1 style={{ margin: "0 0 14px", fontSize: 30, lineHeight: 1.15 }}>{children}</h1>,
          h2: ({ children }) => <h2 style={{ margin: "0 0 14px", fontSize: 24, lineHeight: 1.2 }}>{children}</h2>,
          h3: ({ children }) => <h3 style={{ margin: "0 0 12px", fontSize: 20, lineHeight: 1.25 }}>{children}</h3>,
          h4: ({ children }) => <h4 style={{ margin: "0 0 10px", fontSize: 17, lineHeight: 1.3 }}>{children}</h4>,
          p: ({ children }) => <p style={{ margin: "0 0 14px" }}>{children}</p>,
          ul: ({ children }) => <ul style={{ margin: "0 0 14px", paddingLeft: 22 }}>{children}</ul>,
          ol: ({ children }) => <ol style={{ margin: "0 0 14px", paddingLeft: 22 }}>{children}</ol>,
          li: ({ children }) => <li style={{ marginBottom: 6 }}>{children}</li>,
          blockquote: ({ children }) => (
            <blockquote
              style={{
                margin: "0 0 14px",
                padding: "10px 14px",
                borderLeft: `4px solid ${t.accentStrong}`,
                background: t.accentDragBg,
                borderRadius: 12
              }}
            >
              {children}
            </blockquote>
          ),
          a: ({ href, children }) => (
            <a href={href} target="_blank" rel="noreferrer" style={{ color: t.accent }}>
              {children}
            </a>
          ),
          table: ({ children }) => (
            <div style={{ overflowX: "auto", marginBottom: 14 }}>
              <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 14 }}>{children}</table>
            </div>
          ),
          th: ({ children }) => (
            <th
              style={{
                textAlign: "left",
                padding: "8px 10px",
                border: `1px solid ${t.tableBorder}`,
                background: t.preBg
              }}
            >
              {children}
            </th>
          ),
          td: ({ children }) => (
            <td style={{ padding: "8px 10px", border: `1px solid ${t.tableBorder}` }}>{children}</td>
          ),
          code: ({ className, children }) =>
            !className && !String(children).includes("\n") ? (
              <code
                style={{
                  padding: "0.12em 0.35em",
                  borderRadius: 8,
                  background: t.codeBg,
                  fontSize: "0.92em"
                }}
              >
                {children}
              </code>
            ) : (
              <code className={className}>{children}</code>
            ),
          pre: ({ children }) => (
            <pre
              style={{
                margin: "0 0 14px",
                padding: 14,
                borderRadius: 14,
                overflow: "auto",
                background: t.preBg
              }}
            >
              {children}
            </pre>
          ),
          hr: () => <hr style={{ border: "none", borderTop: `1px solid ${t.hrBorder}`, margin: "18px 0" }} />
        }}
      >
        {source}
      </ReactMarkdown>
    </div>
  );
}
