"use client";

import { useState } from "react";
import { Search, X } from "lucide-react";
import { useData } from "@/hooks/useData";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import { EVENT_TYPES, EVENT_STATUS } from "@/lib/constants";
import { ENDPOINTS } from "@/lib/api";
import EventDetail from "@/components/events/EventDetail";
import EventTable from "@/components/events/EventTable";
import Pagination from "@/components/ui/Pagination";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { EventItem } from "@/lib/types";

/** 重要消息 tab（= 结构化事件）：筛选 + 搜索 + 分页全部写入 URL query（默认 20/页）。 */
export default function EventsTab() {
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
  const submitKw = () => setFilter("q", kw.trim());

  return (
    <div>
      <div className="filter-bar">
        {EVENT_STATUS.map((s) => (
          <button
            key={s.key}
            onClick={() => setFilter("status", s.key)}
            className={`chip-btn ${(query.status ?? "") === s.key ? "on" : ""}`}
          >
            {s.label}
          </button>
        ))}
        <select
          className="f-sel"
          value={query.type ?? ""}
          onChange={(e) => setFilter("type", e.target.value)}
        >
          {EVENT_TYPES.map((t) => (
            <option key={t.key} value={t.key}>
              {t.label}
            </option>
          ))}
        </select>
        <form
          className="f-search"
          onSubmit={(e) => {
            e.preventDefault();
            submitKw();
          }}
        >
          <Search size={15} className="txt-faint" strokeWidth={2} />
          <input
            value={kw}
            onChange={(e) => setKw(e.target.value)}
            placeholder="搜索事件标题 / 影响实体 / 关键词…"
          />
          {query.q && (
            <button
              type="button"
              onClick={() => {
                setKw("");
                setFilter("q", "");
              }}
              className="txt-faint hover:text-up"
            >
              <X size={13} />
            </button>
          )}
        </form>
      </div>

      <div className="panel">
        {events.loading ? (
          <LoadingBlock rows={8} />
        ) : events.error ? (
          <ErrorState msg={events.error} />
        ) : data.length === 0 ? (
          <EmptyState
            tip="没有符合筛选条件的事件，试试放宽条件"
            action={{ to: "/help", label: "什么是「事件」？" }}
          />
        ) : (
          <>
            <EventTable events={paged} onSelect={setSelected} />
            <Pagination
              page={page}
              pageSize={size}
              total={data.length}
              onPage={setPage}
              onPageSize={setSize}
            />
          </>
        )}
      </div>

      <EventDetail event={selected} onClose={() => setSelected(null)} />
    </div>
  );
}
