"use client";

import { useState } from "react";
import { useData } from "@/hooks/useData";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import { ENDPOINTS } from "@/lib/api";
import Pagination from "@/components/ui/Pagination";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import { AnnouncementRow, gradeOf } from "@/components/events/AnnounceRow";
import { GradeFilterBar } from "@/components/events/GradeFilterBar";
import type { Announcement } from "@/lib/types";

/**
 * 公告 tab：官方披露的原始流（巨潮，issue #50）+ 分级折叠（issue #68 A 层）。
 *
 * 与快讯 tab 的关键差别：公告**不经 AI 抽取**，是交易所/上市公司原文，
 * 故列表不给置信度 —— 官方披露没有真假问题。
 *
 * 分级（issue #68）：公告实测 ~1200 条/交易日（最多实测 1800），全量平铺「看不过来」。
 * 按标题规则分四档（必读/重要/常规/噪音），**默认只看必读+重要**，其余一键展开。
 * ⚠️ 分级是**机器判定（Inference）不是事实**，筛选条上方如实标注；且**只折叠不隐藏**
 * （红线 §3.3）—— 每个级别都有入口，未分级的历史行按常规显示。折叠是**客户端**行为，
 * 后端一次性全量下发（沿用本项目 usePagedQuery 客户端切片的既有约定）。
 */
export default function AnnouncementsTab() {
  const { query, setFilter, page, size, setPage, setSize, paginate } = usePagedQuery();
  const [showAll, setShowAll] = useState(false);
  const anns = useData<Announcement[]>({
    path: ENDPOINTS.announcements,
    params: { q: query.q, source: query.source },
  });

  // 默认只看必读+重要；「查看全部」放开。级别筛选走 URL（规范第 7 条）。
  const wanted = showAll ? null : new Set(["must", "important"]);
  const filtered = (anns.data ?? []).filter((a) => !wanted || wanted.has(gradeOf(a.grade)));
  const hidden = (anns.data?.length ?? 0) - filtered.length;
  const paged = paginate(filtered);

  return (
    <div>
      <GradeFilterBar
        q={query.q ?? ""}
        setFilter={setFilter}
        showAll={showAll}
        hidden={hidden}
        onToggle={() => {
          setShowAll((v) => !v);
          setPage(1);
        }}
      />

      <div className="panel">
        {anns.loading ? (
          <LoadingBlock rows={8} />
        ) : anns.error ? (
          <ErrorState msg={anns.error} />
        ) : filtered.length === 0 ? (
          <EmptyState
            tip={
              query.q
                ? "没有匹配的公告"
                : showAll
                  ? "暂无公告数据"
                  : "今日没有必读/重要的公告 —— 可点「查看全部」"
            }
          />
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
              total={filtered.length}
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
