"use client";

import { Link } from "react-router-dom";
import { X } from "lucide-react";
import type { EventItem } from "@/lib/types";
import { useEventTypes } from "@/lib/eventTypes";
import { SINGLE_SOURCE_LABEL } from "@/lib/constants";
import { Chip, ConfidenceBar } from "@/components/ui/Num";
import EventConflicts from "@/components/events/EventConflicts";
import EventSources from "@/components/events/EventSources";

/** 事件详情抽屉：事实 / 影响 / 来源（机器事实与 AI 推断分开展示，只读） */
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

      <h2 className="mt-3 text-lg font-semibold leading-snug">{event.title}</h2>

      <div className="mt-3 flex items-center justify-between rounded-[10px] border border-line bg-card-soft px-3 py-2">
        <span className="text-xs text-muted">AI 置信度</span>
        <ConfidenceBar v={event.confidence} />
      </div>

      <Section title="摘要">{event.summary}</Section>

      <Section title="事实（Fact）">
        <ul className="m-0 list-disc pl-5">
          {event.facts.map((f, i) => (
            <li key={i} className="mb-1.5 text-sm leading-relaxed">
              {f}
            </li>
          ))}
        </ul>
      </Section>

      <Section title="影响实体">
        <div className="flex flex-wrap gap-1.5">
          {event.affected.map((a, i) =>
            a.code ? (
              <Link key={i} to={`/stock/${a.code}`} className="st st-accent no-underline">
                {a.entity_name ?? a.word}
              </Link>
            ) : (
              <Chip key={i} tone="accent">
                {a.entity_name ?? a.word}
              </Chip>
            )
          )}
        </div>
      </Section>

      <Section title="来源">
        <EventSources event={event} />
      </Section>

      {/* 跨源数值冲突（issue #49 T3）：只在真检出时出现。 */}
      <EventConflicts items={event.event_conflicts ?? []} />
    </div>
  );
}

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="mt-5 border-t border-line pt-3.5">
      <h3 className="mb-2 text-[15px] font-bold">{title}</h3>
      <div className="text-sm leading-relaxed">{children}</div>
    </div>
  );
}
