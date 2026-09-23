"use client";

import type { EventItem } from "@/lib/types";
import { SourceLink } from "@/components/ui/SourceLink";

type ClusterSource = NonNullable<EventItem["cluster_sources"]>[number];

/**
 * 来源区（issue #49 T3 拆出；issue #83 P-4 / P8 改为**按机构分组**）：
 * 单源时一行；跨源簇（≥2 家）时**每机构一行** —— 该机构**全部**原文链接逐个列出。
 *
 * 🔴 P8 硬约束（三条，UI 须逐条兑现）：
 *   1. **链接取 raw 层全集**（后端已供 `urls[]`），不只列「已抽成事件」的那条；
 *   2. **无 URL 的源如实写「该源无外链」**，绝不拼假链接（同 SourceLink 纪律）；
 *   3. 一级源（金十转述新华社等）：渠道名与一级源名**分区**，链接归属**一级源**。
 *
 * 拆成独立文件是为了守住「组件单文件 ≤150 行」硬规则（CLAUDE.md 规则 5）。
 * 白话文案，不出现 cluster / canonical 等实现黑话（P6-2 纪律）。
 */
export default function EventSources({ event }: { event: EventItem }) {
  const srcs = event.cluster_sources;
  if (!srcs || srcs.length < 2) {
    return (
      <div className="text-[13px]">
        <SourceLink source={event.source} url={event.source_url} showIcon />
        {/* 单源说明（issue #49 T3）：明说「只有一家在报」，但**不说**它可疑 ——
            独家报道是正常且常见的，这里只陈述事实，判断权交给用户。 */}
        {(event.independent_count ?? event.source_count) === 1 && (
          <div className="mt-1.5 text-xs leading-relaxed text-faint">
            目前只有这一家机构在报。独家报道很常见，不代表消息不实，只是还没有别家旁证。
          </div>
        )}
      </div>
    );
  }
  // 独立来源数（issue #83 P-1）：把近逐字的转载并成一源后的计数；缺字段（老数据）回落到机构数。
  const total = srcs.length;
  const independent = event.independent_count ?? total;
  // 转载数 = 被归并掉的来源数（机构数 − 独立来源数），用于如实说明「N 家转的是同一篇」。
  const reprints = total - independent;
  return (
    <div className="text-[13px]">
      <div className="mb-2 text-faint">
        {total} 家媒体报道了同一件事
        {reprints > 0 && (
          // 诚实标注（issue #83 P-1）：机构数看着多，但其中若干家是**同一篇通稿的转载**，
          // 独立来源只有这么多 —— 不把转载算成「几家各自在报」。
          <>
            {" "}
            · 其中 {reprints} 家为转载，独立来源 {independent} 家
          </>
        )}
      </div>
      <ul className="m-0 list-none p-0">
        {srcs.map((s, i) => (
          <OrgSource key={`${s.source}-${i}`} s={s} />
        ))}
      </ul>
      <div className="mt-2 text-xs leading-relaxed text-faint">
        来源含「报道了这件事、但未被结构化抽取」的机构（链接取自原始采集层）。
      </div>
    </div>
  );
}

/** 一家机构一行：机构名 + 转载标注 + 该机构**全部**链接（无链接如实标注）。 */
function OrgSource({ s }: { s: ClusterSource }) {
  // 优先 `urls[]`（raw 层全集）；旧数据只有单条 `url` 时退化为一条。
  const urls = s.urls?.length ? s.urls : s.url ? [s.url] : [];
  return (
    <li className="mb-1.5 flex flex-wrap items-baseline gap-2">
      <span className="font-medium">{s.source}</span>
      {/* 转载标注（issue #83 P-1）：与簇内另一家近逐字，如实标出、**不隐藏也不合并显示**。 */}
      {s.reprint && <span className="text-xs text-faint">（转载）</span>}
      {urls.length === 0 ? (
        // 🔴 硬约束 2：无外链就说无外链，不拼假 URL、不给误导性外链图标。
        <span className="text-xs text-faint">该源无外链</span>
      ) : (
        urls.map((u, idx) => (
          <a
            key={u}
            href={u}
            target="_blank"
            rel="noopener noreferrer"
            onClick={(e) => e.stopPropagation()}
            className="text-accent text-xs no-underline hover:underline"
            title="在新标签页打开原文"
          >
            {/* 🔴 硬约束 3：一级源存在时，**链接归属一级源**（渠道名已在上方分区显示）。 */}
            {s.origin ? `原文来自 ${s.origin}` : "原文"}
            {urls.length > 1 ? `（${idx + 1}）` : ""}
          </a>
        ))
      )}
    </li>
  );
}
