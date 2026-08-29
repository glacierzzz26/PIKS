"use client";

import Link from "next/link";
import { Num } from "@/components/ui/Num";
import { Chip } from "@/components/ui/Num";
import type { SnapRow } from "@/lib/mock/activity";

/** 历史情绪快照卡（对齐 base.css .snap） */
export function SnapCard({
  s,
  latest,
}: {
  s: SnapRow;
  latest?: boolean;
}) {
  return (
    <div
      className={`rounded-sm border bg-card p-3 ${
        latest ? "border-accent shadow-card" : "border-line"
      }`}
    >
      <div className="mb-2.5 flex items-center gap-2">
        <span className="text-[15px] font-bold">{s.date}</span>
        {latest && (
          <span className="rounded-full bg-accent px-2 py-px text-[10px] font-bold text-accent-ink">
            最新
          </span>
        )}
        <Chip tone={s.emotion_score >= 60 ? "up" : s.emotion_score >= 45 ? "amber" : "down"}>
          {s.emotion_state}
        </Chip>
      </div>
      <div className="grid grid-cols-4 gap-1.5">
        {[
          { label: "涨停", v: s.limit_up, cls: "text-up" },
          { label: "跌停", v: s.limit_down, cls: "text-down" },
          { label: "炸板", v: s.broken_limit, cls: "text-amber" },
          { label: "最高板", v: s.max_board, cls: "" },
        ].map((m) => (
          <div key={m.label} className="text-center">
            <b className={`num block text-[17px] font-extrabold ${m.cls}`}>
              {m.v}
            </b>
            <span className="text-[11px] text-muted">{m.label}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

/** 知识库规模统计（对齐 .stat） */
export function StatCard({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex items-center justify-between rounded border border-line bg-card px-4 py-4 shadow-card">
      <b className="num text-[28px] font-extrabold text-accent">{value}</b>
      <span className="text-[13px] text-muted">{label}</span>
    </div>
  );
}

/** 横向条形图（对齐 .bar-row） */
export function Bars({ data }: { data: { name: string; count: number }[] }) {
  const max = Math.max(...data.map((d) => d.count));
  return (
    <div className="flex flex-col gap-1.5">
      {data.map((d) => (
        <div key={d.name} className="grid grid-cols-[104px_1fr_28px] items-center gap-2">
          <span className="truncate whitespace-nowrap text-right text-[13px] text-muted">
            {d.name}
          </span>
          <span className="h-3 overflow-hidden rounded-[5px] border border-line bg-bg-soft">
            <span
              className="block h-full rounded-[5px] bg-accent transition-[width] duration-500"
              style={{ width: `${(d.count / max) * 100}%` }}
            />
          </span>
          <Num value={d.count} className="text-xs" />
        </div>
      ))}
    </div>
  );
}

/** 事件排行列表（对齐 .evlist） */
export function EventRank({
  items,
}: {
  items: { title: string; score: number; id: string }[];
}) {
  return (
    <div className="flex flex-col">
      {items.map((e, i) => (
        <Link
          key={e.id}
          href={`/events?q=${encodeURIComponent(e.title.slice(0, 8))}`}
          className="group flex items-baseline gap-2.5 border-b border-dashed border-line py-2 text-sm last:border-0"
        >
          <span className="w-4 shrink-0 text-xs text-muted">{i + 1}</span>
          <span className="flex-1 truncate group-hover:text-accent">
            {e.title}
          </span>
          <Num value={e.score} suffix="分" className="text-xs text-muted" />
        </Link>
      ))}
    </div>
  );
}
