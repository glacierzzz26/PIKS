"use client";

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

/** Markdown 渲染（笔记 / 复盘 / 周报共用） */
export default function MarkdownBody({ content }: { content: string }) {
  return (
    <div className="prose-piks">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
    </div>
  );
}
