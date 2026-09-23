"use client";

import { X } from "lucide-react";
import type { EventItem } from "@/lib/types";
import { useEventTypes } from "@/lib/eventTypes";
import { SINGLE_SOURCE_LABEL } from "@/lib/constants";
import { Chip, ConfidenceBar } from "@/components/ui/Num";
import EventConflicts from "@/components/events/EventConflicts";
import EventSources from "@/components/events/EventSources";
import EventContent, { Section } from "@/components/events/EventContent";

/** 事件详情抽屉：事实 / 影响 / 来源（机器事实与 AI 推断分开展示，只读）。
 *  issue #83 P-4 / P8：展示单元 = **簇** —— 标题取簇标题、内容取成员并集。 */
export default function EventDetail({
  event,
  onClose,
}: {
  event: EventItem | null;
  onClose: () => void;
}) {
  if (!event) return null;
  return (
    <>
      <div className="drawer-scrim fixed inset-0 z-40 bg-black/25" onClick={onClose} />
      <aside className="drawer-panel fixed right-0 top-0 z-50 flex h-full w-[460px] max-w-[92vw] flex-col border-l border-line bg-card shadow-pop">
        <div className="flex h-[58px] shrink-0 items-center justify-between border-b border-line px-5">
          <span className="text-[15px] font-bold">事件详情</span>
          <button
            onClick={onClose}
            className="flex h-8 w-8 items-center justify-center rounded-[10px] txt-faint hover:bg-bg-soft"
          >
            <X size={16} />
          </button>
        </div>
        <EventBody event={event} />
      </aside>
    </>
  );
}

function EventBody({ event }: { event: EventItem }) {
  const { labelOf } = useEventTypes();
  // 展示单元 = 簇（issue #83 P-4 / P8）：有簇标题就用它（同一个真实事件的多家报道合成的标题），
  // 没有（未聚类/单成员）就用本事件自己的 title。
  const title = event.cluster_title || event.title;
  return (
    <div className="flex-1 overflow-auto p-5">
      <div className="flex flex-wrap items-center gap-2">
        <Chip tone="accent">
          {labelOf(event.event_type) ?? event.event_type}
        </Chip>
        {/* 抽取态（issue #80 重定义）：extracted=已抽取 / merged=已被聚类并入。
            与下面的来源维度是**两条正交的轴**，用色亦区分（merged 用 dim）。 */}
        <Chip tone={event.status === "merged" ? "dim" : "amber"}>
          {event.status === "merged" ? "已被合并" : "已抽取"}
        </Chip>
        {/* 来源维度（issue #49 T3）：与上面的抽取态是**两条正交的轴**。
            用 dim —— 单源是客观陈述，不是异常（独家报道很常见）。
            判据取**独立来源数**（issue #83 P-1，把转载并成一源后），非机构数。 */}
        {(event.independent_count ?? event.source_count) === 1 && (
          <Chip tone="dim">{SINGLE_SOURCE_LABEL}</Chip>
        )}
        <span className="num ml-auto text-xs text-faint">
          {event.occurred_at.slice(0, 16).replace("T", " ")}
        </span>
      </div>

      <h2 className="mt-3 text-lg font-semibold leading-snug">{title}</h2>

      <div className="mt-3 flex items-center justify-between rounded-[10px] border border-line bg-card-soft px-3 py-2">
        <span className="text-xs text-muted">AI 置信度</span>
        <ConfidenceBar v={event.confidence} />
      </div>

      <Section title="摘要">{event.summary}</Section>

      <EventContent event={event} />

      <Section title="来源">
        <EventSources event={event} />
      </Section>

      {/* 跨源数值冲突（issue #49 T3）：只在真检出时出现。 */}
      <EventConflicts items={event.event_conflicts ?? []} />
    </div>
  );
}
