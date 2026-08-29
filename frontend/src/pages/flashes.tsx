"use client";

import { Suspense } from "react";
import { Link } from "react-router-dom";
import { useData } from "@/hooks/useData";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import { getFlashes } from "@/lib/mockService";
import { FLASH_SOURCES } from "@/lib/mock/flashes";
import { ENDPOINTS } from "@/lib/api";
import Pagination from "@/components/ui/Pagination";
import { Chip } from "@/components/ui/Num";
import { EmptyState } from "@/components/ui/States";
import type { Flash } from "@/lib/types";

/** 快讯流：来源筛选 + 分页（默认 20/页，URL 驱动），重要快讯高亮 */
export default function Page() {
  return (
    <Suspense fallback={<div className="mt-6 h-40 animate-pulse rounded bg-card" />}>
      <FlashesInner />
    </Suspense>
  );
}

function FlashesInner() {
  const { query, setFilter, page, size, setPage, setSize, paginate } =
    usePagedQuery();
  const flashes = useData({
    path: ENDPOINTS.flashes,
    params: { q: query.q, source: query.source },
    fallback: () => getFlashes({ q: query.q, source: query.source }),
  });
  const data = flashes.data ?? [];
  const paged = paginate(data);

  const groups = new Map<string, Flash[]>();
  for (const f of paged) {
    const day = f.time.slice(0, 10);
    (groups.get(day) ?? groups.set(day, []).get(day)!).push(f);
  }

  return (
    <div>
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">快讯流</h1>
          <span className="num text-[13px] text-muted">{data.length} 条</span>
        </div>
        <div className="mt-3 flex flex-wrap gap-1.5">
          {FLASH_SOURCES.map((s) => (
            <button
              key={s.key}
              onClick={() => setFilter("source", s.key)}
              className={`chip ${
                (query.source ?? "") === s.key ? "chip-accent" : "chip-dim"
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
      </div>

      {data.length === 0 ? (
        <div className="mt-4 rounded border border-line bg-card shadow-card">
          <EmptyState tip="没有匹配的快讯" />
        </div>
      ) : (
        <>
          {[...groups.entries()].map(([day, list]) => (
            <section key={day}>
              <div className="day-title">{day}</div>
              <div className="rounded border border-line bg-card shadow-card">
                {list.map((f, i) => (
                  <div
                    key={f.id}
                    className={`flex gap-4 px-4 py-3 ${
                      i < list.length - 1 ? "border-b border-line" : ""
                    }`}
                  >
                    <span className="num w-10 shrink-0 pt-0.5 text-xs text-muted">
                      {f.time.slice(11)}
                    </span>
                    <span
                      className={`mt-[9px] h-1.5 w-1.5 shrink-0 rounded-full ${
                        f.important ? "bg-up" : "bg-line-strong"
                      }`}
                    />
                    <p className="flex-1 text-sm leading-relaxed">
                      {f.content}
                      {f.event_id && (
                        <Link
                          to={`/events?q=${encodeURIComponent(f.content.slice(0, 8))}`}
                          className="ml-2 inline-flex items-center gap-1 text-accent no-underline hover:underline"
                        >
                          关联事件
                        </Link>
                      )}
                    </p>
                    <Chip tone={f.important ? "up" : "dim"}>{f.source}</Chip>
                  </div>
                ))}
              </div>
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
    </div>
  );
}
