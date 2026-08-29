"use client";

import { motion, AnimatePresence } from "framer-motion";
import { X, ExternalLink } from "lucide-react";
import type { EventItem } from "@/lib/types";
import { EVENT_TYPE_LABEL } from "@/lib/format";
import { Chip, ConfidenceBar } from "@/components/ui/Num";

/** 事件详情抽屉：事实 / 影响 / 来源（Fact ≠ Inference 分域展示，只读） */
export default function EventDetail({
  event,
  onClose,
}: {
  event: EventItem | null;
  onClose: () => void;
}) {
  return (
    <AnimatePresence>
      {event && (
        <>
          <motion.div
            className="fixed inset-0 z-40 bg-black/25"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={onClose}
          />
          <motion.aside
            className="fixed right-0 top-0 z-50 flex h-full w-[460px] max-w-[92vw] flex-col border-l border-line bg-card shadow-pop"
            initial={{ x: 460 }}
            animate={{ x: 0 }}
            exit={{ x: 460 }}
            transition={{ type: "tween", duration: 0.2 }}
          >
            <div className="flex h-[58px] shrink-0 items-center justify-between border-b border-line px-5">
              <span className="text-[15px] font-bold">事件详情</span>
              <button
                onClick={onClose}
                className="flex h-8 w-8 items-center justify-center rounded-sm text-muted hover:bg-bg-soft"
              >
                <X size={16} />
              </button>
            </div>
            <EventBody event={event} />
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );
}

function EventBody({ event }: { event: EventItem }) {
  return (
    <div className="flex-1 overflow-auto p-5">
      <div className="flex flex-wrap items-center gap-2">
        <Chip tone="accent">
          {EVENT_TYPE_LABEL[event.event_type] ?? event.event_type}
        </Chip>
        <Chip tone={event.status === "confirmed" ? "down" : "amber"}>
          {event.status === "confirmed" ? "已确认" : "待复核"}
        </Chip>
        <span className="num ml-auto text-xs text-muted">
          {event.occurred_at.slice(0, 16).replace("T", " ")}
        </span>
      </div>

      <h2 className="mt-3 text-lg font-semibold leading-snug">{event.title}</h2>

      <div className="mt-3 flex items-center justify-between rounded-sm border border-line bg-bg-soft px-3 py-2">
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
          {event.affected.map((a, i) => (
            <Chip key={i} tone="accent">
              {a.entity_name ?? a.word}
            </Chip>
          ))}
        </div>
      </Section>

      <Section title="来源">
        <span className="inline-flex items-center gap-1 text-[13px] text-muted">
          {event.source}
          {event.source_url && <ExternalLink size={12} className="text-accent" />}
        </span>
      </Section>
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
