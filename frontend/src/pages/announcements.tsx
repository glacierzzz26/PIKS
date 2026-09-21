"use client";

import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Search, X } from "lucide-react";
import { useData } from "@/hooks/useData";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import { ENDPOINTS } from "@/lib/api";
import Pagination from "@/components/ui/Pagination";
import { Chip } from "@/components/ui/Num";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { Announcement } from "@/lib/types";

/**
 * 公告 tab：官方披露的原始流（巨潮，issue #50）。
 *
 * 与快讯 tab 的关键差别：公告**不经 AI 抽取**，是交易所/上市公司原文，
 * 故列表只给 代码 名称 / 标题 / 查看原文，不给置信度、不标「重要」——
 * 官方披露没有真假问题，加一个「已解读」的伪标签只会误导。
 * 按天分组（时间倒序数据上分组，顺序即首次出现序，同 flashes）。
 */
export default function AnnouncementsTab() {
  const { query, setFilter, page, size, setPage, setSize, paginate } =
    usePagedQuery();
  const [localQ, setLocalQ] = useState(query.q ?? "");
  const anns = useData<Announcement[]>({
    path: ENDPOINTS.announcements,
    params: { q: query.q, source: query.source },
  });
  const data = anns.data ?? [];
  const paged = paginate(data);

  return (
    <div>
      <div className="filter-bar">
        <form
          className="f-search"
          onSubmit={(e) => {
            e.preventDefault();
            setFilter("q", localQ.trim());
          }}
        >
          <Search size={15} className="txt-faint" strokeWidth={2} />
          <input
            value={localQ}
            onChange={(e) => setLocalQ(e.target.value)}
            placeholder="搜索公告标题 / 代码…"
          />
          {query.q ? (
            <button
              type="button"
              onClick={() => {
                setLocalQ("");
                setFilter("q", "");
              }}
              className="txt-faint hover:text-up"
            >
              <X size={13} />
            </button>
          ) : null}
        </form>
      </div>

      <div className="panel">
        {anns.loading ? (
          <LoadingBlock rows={8} />
        ) : anns.error ? (
          <ErrorState msg={anns.error} />
        ) : data.length === 0 ? (
          <EmptyState tip={query.q ? "没有匹配的公告" : "暂无公告数据"} />
        ) : (
          <>
            {groupByDay(paged).map(([day, list]) => (
              <div key={day}>
                <div className="day-title px-4">{day}</div>
                {list.map((a) => (
                  <AnnouncementRow key={a.id} ann={a} />
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

/** 按天分组（顺序 = 首次出现顺序，仅在时间倒序数据上使用）。 */
function groupByDay(list: Announcement[]): [string, Announcement[]][] {
  const groups = new Map<string, Announcement[]>();
  for (const a of list) {
    const day = a.time.slice(0, 10);
    (groups.get(day) ?? groups.set(day, []).get(day)!).push(a);
  }
  return [...groups.entries()];
}

/** 单条公告：时间 + 代码/名称 + 标题 + 查看原文。 */
function AnnouncementRow({ ann: a }: { ann: Announcement }) {
  const nav = useNavigate();
  return (
    <div className="flex gap-4 border-b border-line px-4 py-3 last:border-b-0">
      <span className="num w-10 shrink-0 pt-0.5 text-xs txt-faint">
        {a.time.slice(11, 16)}
      </span>
      <span className="w-[13rem] shrink-0 text-xs">
        {a.sec_code ? (
          <button
            className="num mr-1.5 text-accent hover:underline"
            title={`打开个股中心 ${a.sec_code}`}
            onClick={() => nav(`/stock/${a.sec_code}`)}
          >
            {a.sec_code}
          </button>
        ) : null}
        <span className="text-muted">{a.sec_name}</span>
      </span>
      <p className="flex-1 text-sm leading-relaxed">{a.title}</p>
      <Chip tone="dim" href={a.url}>
        {a.url ? "查看原文" : a.source}
      </Chip>
    </div>
  );
}
