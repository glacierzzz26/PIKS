"use client";

import { useEffect, useState } from "react";
import type { ReportChapter } from "@/lib/report";

/**
 * 左侧固定目录（design report-layout.md §4.7）。
 *
 * 数据源是 `metrics.meta.section_manifest`（经 `lib/report.ts` 整形），**不解析
 * markdown 标题** —— 裸 react-markdown 无 heading id、无锚点，解析不可靠（D-R8）。
 * 锚点 id 由前端按序号生成（`sec-1`…），因此也不需要 rehype-slug 之类的插件。
 *
 * 当前章节高亮用 IntersectionObserver 监听正文各章；窄屏由 CSS 折叠为横向 chip 行。
 */
export default function ReportToc({ items }: { items: ReportChapter[] }) {
  const [active, setActive] = useState<string>(items[0]?.id ?? "");

  useEffect(() => {
    const els = items
      .map((it) => document.getElementById(it.id))
      .filter((e): e is HTMLElement => e !== null);
    if (els.length === 0) return;

    // 视口顶部下方 96px 为「当前阅读位置」；取该线之上最后一个章节。
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) setActive(e.target.id);
        }
      },
      { rootMargin: "-96px 0px -70% 0px", threshold: 0 }
    );
    els.forEach((e) => io.observe(e));
    return () => io.disconnect();
  }, [items]);

  if (items.length === 0) return null;

  return (
    <nav className="report-toc" aria-label="报告目录">
      <div className="ttl">目录</div>
      {items.map((it, i) => (
        <a
          key={it.id}
          href={`#${it.id}`}
          className={it.id === active ? "on" : ""}
          onClick={(e) => {
            // 用 scrollIntoView 而非默认锚点跳转：外层有 sticky 定位，
            // 默认跳转会把标题顶到视口最上沿被遮住。
            e.preventDefault();
            document
              .getElementById(it.id)
              ?.scrollIntoView({ behavior: "smooth", block: "start" });
            setActive(it.id);
          }}
        >
          <span className="idx">{i + 1}</span>
          {it.title}
        </a>
      ))}
    </nav>
  );
}
