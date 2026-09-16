"use client";

import { type ReactNode } from "react";
import MarkdownBody from "@/components/md/MarkdownBody";
import DomainTag from "@/components/report/DomainTag";
import { stripAiSlot, type ReportChapter } from "@/lib/report";

/** 单章：标题（挂锚点 + 右侧三域标签）+ markdown 正文。 */
function Chapter({ ch, first }: { ch: ReportChapter; first: boolean }) {
  const body = stripAiSlot(ch.markdown);
  return (
    <section id={ch.id} className="scroll-mt-[88px]">
      <div className={`sec-head ${first ? "first" : ""}`}>
        <h2>{ch.title}</h2>
        <DomainTag domains={ch.domains} />
      </div>
      {body ? (
        <MarkdownBody content={body} />
      ) : (
        // 正文缺席（如合成未跑、槽位为空）如实留白，不造占位内容
        <p className="text-[13px] text-faint italic">本章内容暂不可得。</p>
      )}
    </section>
  );
}

/**
 * 正文列（§4.1）。章节标题由 React 渲染而非交给 markdown —— 这样锚点 id 与三域标签
 * 都能挂上，且无需引入 rehype-slug（D-R8 明确不加插件）。
 *
 * 表格/数字/涨跌色的既有 prose 规则继续生效（`MarkdownBody` 内的 `.prose-piks`）。
 */
export default function ReportBody({ chapters }: { chapters: ReportChapter[] }) {
  return (
    <article className="report-body">
      {chapters.map((ch, i) => (
        <Chapter key={ch.id} ch={ch} first={i === 0} />
      ))}
    </article>
  );
}

/** 页脚（§4.6）：免责章的正文 + 生成信息，与正文视觉分离。 */
export function ReportFoot({ children }: { children: ReactNode }) {
  return <footer className="report-foot">{children}</footer>;
}
