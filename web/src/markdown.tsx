import Markdown from "react-markdown";
import React from "react";
import remarkGfm from "remark-gfm";

// Untrusted model output: no raw HTML, remote image loads or executable links.
export function Answer({children}: {children: string}) {
  return <div className="markdown-body"><Markdown
    remarkPlugins={[remarkGfm]} skipHtml
    urlTransform={(url) => /^https?:\/\//i.test(url) ? url : ""}
    components={{
      a: ({children, href}) => href ? <a href={href} target="_blank" rel="noopener noreferrer">{children}</a> : <span>{children}</span>,
      img: ({alt}) => <span>{alt || "图片引用"}</span>,
      table: ({children}) => <div className="markdown-table"><table>{children}</table></div>,
    }}
  >{children}</Markdown></div>;
}
