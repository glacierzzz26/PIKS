"use client";

import { useState, Suspense, useMemo } from "react";
import { Search, X } from "lucide-react";
import { useData } from "@/hooks/useData";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import { EVENT_TYPES, EVENT_STATUS } from "@/lib/constants";
import { ENDPOINTS } from "@/lib/api";
import { EVENT_TYPE_LABEL } from "@/lib/format";
import EventDetail from "@/components/events/EventDetail";
import Pagination from "@/components/ui/Pagination";
import { Chip } from "@/components/ui/Num";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { EventItem } from "@/lib/types";

/** 事件流（核心）：筛选 + 搜索 + 分页全部写入 URL query（规范第 7 条，默认 20/页） */
export default function Page() {
  return (
    <Suspense fallback={<div className="mt-6"><LoadingBlock rows={8} /></div>}>
      <EventsInner />
    </Suspense>
  );
}

function EventsInner() {
  const { query, setFilter, page, size, setPage, setSize, paginate } =
    usePagedQuery();
  const [selected, setSelected] = useState<EventItem | null>(null);
  const [kw, setKw] = useState(query.q ?? "");

  const events = useData<EventItem[]>({
    path: ENDPOINTS.events,
    params: { type: query.type, status: query.status, q: query.q },
  });

  const data = events.data ?? [];
  const paged = paginate(data);
  const groups = useMemo(() => {
    const map = new Map<string, EventItem[]>();
    for (const e of paged) {
      const day = e.occurred_at.slice(0, 10);
      (map.get(day) ?? map.set(day, []).get(day)!).push(e);
    }
    return [...map.entries()];
  }, [paged]);

  const submitKw = () => setFilter("q", kw.trim());

  return (
    <div>
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">事件流</h1>
          <span className="num text-[13px] text-muted">
            {events.loading ? "" : `${data.length} 条`}
          </span>
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-2">
          <ChipGroup
            options={EVENT_TYPES}
            value={query.type ?? ""}
            onChange={(v) => setFilter("type", v)}
          />
          <span className="h-4 w-px bg-line" />
          <ChipGroup
            options={EVENT_STATUS}
            value={query.status ?? ""}
            onChange={(v) => setFilter("status", v)}
          />
          <form
            className="ml-auto flex h-8 w-[240px] items-center gap-2 rounded-sm border border-line bg-card px-2.5 focus-within:border-accent"
            onSubmit={(e) => {
              e.preventDefault();
              submitKw();
            }}
          >
            <Search size={13} className="shrink-0 text-muted" />
            <input
              value={kw}
              onChange={(e) => setKw(e.target.value)}
              placeholder="搜索标题 / 摘要…"
              className="w-full bg-transparent text-xs outline-none placeholder:text-muted"
            />
            {query.q && (
              <button
                type="button"
                onClick={() => {
                  setKw("");
                  setFilter("q", "");
                }}
                className="text-muted hover:text-up"
              >
                <X size={12} />
              </button>
            )}
          </form>
        </div>
      </div>

      {events.loading ? (
        <LoadingBlock rows={8} />
      ) : events.error ? (
        <div className="rounded border border-line bg-card shadow-card">
          <ErrorState msg={events.error} />
        </div>
      ) : data.length === 0 ? (
        <div className="rounded border border-line bg-card shadow-card">
          <EmptyState tip="没有符合筛选条件的事件，试试放宽条件" />
        </div>
      ) : (
        <>
          {groups.map(([day, list]) => (
            <section key={day}>
              <div className="day-title">{day}</div>
              {list.map((e) => (
                <button
                  key={e.id}
                  onClick={() => setSelected(e)}
                  className="mb-2.5 block w-full rounded border border-line bg-card p-4 text-left shadow-card hover:border-accent"
                >
                  <div className="text-base font-semibold">{e.title}</div>
                  <div className="mt-1.5 flex flex-wrap gap-2">
                    <Chip tone="accent">
                      {EVENT_TYPE_LABEL[e.event_type] ?? e.event_type}
                    </Chip>
                    <Chip tone="dim">{e.source}</Chip>
                    <Chip tone={e.status === "confirmed" ? "down" : "amber"}>
                      {e.status === "confirmed" ? "已确认" : "待复核"}
                    </Chip>
                    <Chip tone="dim">
                      置信度 {(e.confidence * 100).toFixed(0)}%
                    </Chip>
                    {e.affected.slice(0, 3).map((a, i) => (
                      <Chip key={i} tone="accent">
                        {a.entity_name ?? a.word}
                      </Chip>
                    ))}
                  </div>
                  <p className="mb-0 mt-2 text-sm text-muted">{e.summary}</p>
                </button>
              ))}
            </section>
          ))}

          <div className="mt-3 rounded border border-line bg-card shadow-card">
            <Pagination
              page={page}
              pageSize={size}
              total={data.length}
              onPage={setPage}
              onPageSize={setSize}
            />
          </div>
        </>
      )}

      <EventDetail event={selected} onClose={() => setSelected(null)} />
    </div>
  );
}

function ChipGroup({
  options,
  value,
  onChange,
}: {
  options: { key: string; label: string }[];
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {options.map((o) => (
        <button
          key={o.key}
          onClick={() => onChange(o.key)}
          className={`chip ${value === o.key ? "chip-accent" : "chip-dim"}`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
