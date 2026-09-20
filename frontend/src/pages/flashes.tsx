"use client";

import { Link } from "react-router-dom";
import { useData } from "@/hooks/useData";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import { FLASH_SOURCES } from "@/lib/constants";
import { ENDPOINTS } from "@/lib/api";
import Pagination from "@/components/ui/Pagination";
import { Chip } from "@/components/ui/Num";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { Flash } from "@/lib/types";

/** 快讯 tab：来源筛选 + 分页（默认 20/页，URL 驱动），重要快讯高亮。 */
export default function FlashesTab() {
  const { query, setFilter, page, size, setPage, setSize, paginate } =
    usePagedQuery();
  const flashes = useData<Flash[]>({
    path: ENDPOINTS.flashes,
    params: { q: query.q, source: query.source },
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
      <div className="filter-bar">
        {FLASH_SOURCES.map((s) => (
          <button
            key={s.key}
            onClick={() => setFilter("source", s.key)}
            className={`chip-btn ${(query.source ?? "") === s.key ? "on" : ""}`}
          >
            {s.label}
          </button>
        ))}
      </div>

      <div className="panel">
        {flashes.loading ? (
          <LoadingBlock rows={8} />
        ) : flashes.error ? (
          <ErrorState msg={flashes.error} />
        ) : data.length === 0 ? (
          <EmptyState tip="没有匹配的快讯" />
        ) : (
          <>
            {[...groups.entries()].map(([day, list]) => (
              <div key={day}>
                <div className="day-title px-4">{day}</div>
                {list.map((f, i) => (
                  <div
                    key={f.id}
                    className={`flex gap-4 px-4 py-3 ${
                      i < list.length - 1 ? "border-b border-line" : ""
                    }`}
                  >
                    <span className="num w-10 shrink-0 pt-0.5 text-xs txt-faint">
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
                    <Chip tone={f.important ? "up" : "dim"} href={f.url}>
                      {f.source}
                    </Chip>
                  </div>
                ))}
              </div>
            ))}
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
    </div>
  );
}

