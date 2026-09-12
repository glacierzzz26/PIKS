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
import { ConfidenceBar } from "@/components/ui/Num";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { EventItem } from "@/lib/types";

const TYPE_TAG: Record<string, string> = {
  policy: "t-mix",
  earnings: "t-idx",
  product_launch: "t-ev",
  supply_agreement: "t-bond",
  industry_event: "t-idx",
  investment: "t-mix",
  sales_data: "t-ev",
  rumor: "t-gray",
};
const STATUS_ST: Record<string, { cls: string; label: string }> = {
  confirmed: { cls: "st-down", label: "已确认" },
  pending: { cls: "st-amber", label: "待复核" },
  archived: { cls: "st-dim", label: "已归档" },
};

/** 事件流（核心）：筛选 + 搜索 + 分页全部写入 URL query（规范第 7 条，默认 20/页） */
export default function Page() {
  return (
    <Suspense fallback={<div className="panel mt-6"><LoadingBlock rows={8} /></div>}>
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

  const submitKw = () => setFilter("q", kw.trim());

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>事件流</h1>
          <div className="psub">
            AI 从快讯中抽取的结构化事实 · 含置信度 · 与个人推断严格分域
          </div>
        </div>
        <div className="meta">
          <span className="st st-accent">
            共 {events.loading ? "…" : data.length} 条
          </span>
        </div>
      </div>

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
          <Search size={15} className="text-faint" strokeWidth={2} />
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
              className="text-faint hover:text-up"
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
          <EmptyState tip="没有符合筛选条件的事件，试试放宽条件" />
        ) : (
          <>
            <table className="table">
              <thead>
                <tr>
                  <th style={{ textAlign: "left" }}>事件标题</th>
                  <th>类型</th>
                  <th>影响实体</th>
                  <th>发生时间</th>
                  <th>置信度</th>
                  <th>状态</th>
                </tr>
              </thead>
              <tbody>
                {paged.map((e) => (
                  <tr
                    key={e.id}
                    className="cursor-pointer"
                    onClick={() => setSelected(e)}
                  >
                    <td>
                      <div className="ev-title">
                        <b>{e.title}</b>
                        <span className="meta">
                          来源：{e.source}
                          {e.summary ? ` · ${e.summary.slice(0, 24)}` : ""}
                        </span>
                      </div>
                    </td>
                    <td>
                      <span className={`type-tag ${TYPE_TAG[e.event_type] ?? "t-gray"}`}>
                        {EVENT_TYPE_LABEL[e.event_type] ?? e.event_type}
                      </span>
                    </td>
                    <td className="num-t">{e.affected.length} 个</td>
                    <td className="num-t">{e.occurred_at.slice(5, 16).replace("T", " ")}</td>
                    <td>
                      <ConfidenceBar v={e.confidence} />
                    </td>
                    <td>
                      <span className={`st ${STATUS_ST[e.status]?.cls ?? "st-dim"}`}>
                        {STATUS_ST[e.status]?.label ?? e.status}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
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
