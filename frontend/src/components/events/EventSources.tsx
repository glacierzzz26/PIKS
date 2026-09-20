"use client";

import type { EventItem } from "@/lib/types";
import { SourceLink } from "@/components/ui/SourceLink";

/**
 * 来源区（issue #49 T3 拆出）：单源时一行；跨源簇（≥2 家机构）时列出**各源来源** ——
 * 哪家机构报道的 + 各家原文链接 + 上游一级源（如金十标注的「新华社」）。
 *
 * 拆成独立文件是为了守住「组件单文件 ≤150 行」硬规则（CLAUDE.md 规则 5）。
 * 白话文案，不出现 cluster / 聚类 等实现黑话（P6-2 纪律）。
 */
export default function EventSources({ event }: { event: EventItem }) {
  const srcs = event.cluster_sources;
  if (!srcs || srcs.length < 2) {
    return (
      <div className="text-[13px]">
        <SourceLink source={event.source} url={event.source_url} showIcon />
        {/* 单源说明（issue #49 T3）：明说「只有一家在报」，但**不说**它可疑 ——
            独家报道是正常且常见的，这里只陈述事实，判断权交给用户。 */}
        {event.source_count === 1 && (
          <div className="mt-1.5 text-xs leading-relaxed text-faint">
            目前只有这一家机构在报。独家报道很常见，不代表消息不实，只是还没有别家旁证。
          </div>
        )}
      </div>
    );
  }
  return (
    <div className="text-[13px]">
      <div className="mb-2 text-faint">{srcs.length} 家媒体报道了同一件事</div>
      <ul className="m-0 list-none p-0">
        {srcs.map((s, i) => (
          <li
            key={`${s.source}-${i}`}
            className="mb-1.5 flex flex-wrap items-baseline gap-2"
          >
            <SourceLink source={s.source} url={s.url} showIcon />
            {s.origin && <span className="text-xs text-faint">转自 {s.origin}</span>}
            {!s.url && <span className="text-xs text-faint">（无原文链接）</span>}
          </li>
        ))}
      </ul>
    </div>
  );
}
