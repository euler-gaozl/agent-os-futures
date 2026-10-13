import Markdown from "react-markdown";
import React from "react";
import remarkGfm from "remark-gfm";

// Untrusted model output: no raw HTML, remote image loads or executable links.
export function sourceDocument(url: string): string | null {
  return /^source:\/\/document\/([A-Za-z0-9][A-Za-z0-9_-]{0,119})$/.exec(url)?.[1] || null;
}
export function Answer({children, onSource}: {children: string; onSource?: (id: string) => void}) {
  return <div className="markdown-body"><Markdown
    remarkPlugins={[remarkGfm]} skipHtml
    urlTransform={(url) => /^https?:\/\//i.test(url) || sourceDocument(url) ? url : ""}
    components={{
      a: ({children, href}) => {
        const id = sourceDocument(href || "");
        if (id) return onSource ? <button className="source-citation" onClick={() => onSource(id)} title="查看本次保存的阅读原文">{children}</button> : <span>{children}</span>;
        return href ? <a href={href} target="_blank" rel="noopener noreferrer">{children}</a> : <span>{children}</span>;
      },
      img: ({alt}) => <span>{alt || "图片引用"}</span>,
      table: ({children}) => <div className="markdown-table"><table>{children}</table></div>,
    }}
  >{children}</Markdown></div>;
}
