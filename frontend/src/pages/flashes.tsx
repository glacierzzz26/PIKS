"use client";

import { Link } from "react-router-dom";
import { useData } from "@/hooks/useData";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import { FLASH_SOURCES, FLASH_SORTS } from "@/lib/constants";
import { ENDPOINTS } from "@/lib/api";
import Pagination from "@/components/ui/Pagination";
import { Chip } from "@/components/ui/Num";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { Flash } from "@/lib/types";

/**
 * 快讯 tab：来源筛选 + 排序 + 分页（默认 20/页，URL 驱动），重要快讯高亮。
 *
 * 排序（issue #37）：默认按时间倒序 → 按天分组（组内即时间倒序，符合直觉）；
 * 「重要优先」= 全部重要项排在前面，跨天交错，**此时不做按天分组**——
 * 否则 Map 会按「首次出现」定组序，日期标题出现 20→19→10→18 的错乱。
 */
export default function FlashesTab() {
  const { query, setFilter, page, size, setPage, setSize, paginate } =
    usePagedQuery();
  const flashes = useData<Flash[]>({
    path: ENDPOINTS.flashes,
    params: { q: query.q, source: query.source, sort: query.sort },
  });
  const data = flashes.data ?? [];
  const paged = paginate(data);
  const byDay = (query.sort ?? "") !== "important";

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
        <select
          className="f-sel"
          value={query.sort ?? ""}
          onChange={(e) => setFilter("sort", e.target.value)}
          title="列表排序"
        >
          {FLASH_SORTS.map((s) => (
            <option key={s.key} value={s.key}>
              {s.label}
            </option>
          ))}
        </select>
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
            {byDay ? (
              groupByDay(paged).map(([day, list]) => (
                <div key={day}>
                  <div className="day-title px-4">{day}</div>
                  {list.map((f) => (
                    <FlashRow key={f.id} flash={f} />
                  ))}
                </div>
              ))
            ) : (
              <div>
                {paged.map((f) => (
                  <FlashRow key={f.id} flash={f} />
                ))}
              </div>
            )}
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

/** 按天分组（顺序 = 首次出现顺序，仅在时间倒序数据上使用）。 */
function groupByDay(list: Flash[]): [string, Flash[]][] {
  const groups = new Map<string, Flash[]>();
  for (const f of list) {
    const day = f.time.slice(0, 10);
    (groups.get(day) ?? groups.set(day, []).get(day)!).push(f);
  }
  return [...groups.entries()];
}

/** 单条快讯：时间 + 重要标记 + 正文 + 来源。 */
function FlashRow({ flash: f }: { flash: Flash }) {
  return (
    <div className="flex gap-4 border-b border-line px-4 py-3 last:border-b-0">
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
  );
}
