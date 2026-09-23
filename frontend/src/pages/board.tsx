"use client";

import { useState } from "react";
import { Info } from "lucide-react";
import { useData } from "@/hooks/useData";
import { useUrlState } from "@/hooks/useUrlState";
import { ENDPOINTS } from "@/lib/api";
import type { BoardResp, BoardStage, EventItem } from "@/lib/types";
import EventTable from "@/components/events/EventTable";
import EventDetail from "@/components/events/EventDetail";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";

const STAGES: { key: BoardStage; label: string; hint: string }[] = [
  { key: "early", label: "早盘", hint: "前一日 18:30 → 当日 09:15（隔夜 + 盘前）" },
  { key: "late", label: "晚盘", hint: "当日 09:15 → 当日 18:30（全天）" },
];

/**
 * 榜单（issue #83 P-3 接口 / P-4 前端落地）—— 早/晚档事件榜。
 *
 * 🔴 **不排名**（issue P-1 红线）：榜单按**时间序**呈现（最新在前），每行只有
 *   **印证度标签**（单一来源 / 多家印证 / 广泛报道，由 `independent_count` 派生）。
 *   本版**无热度排序** —— 页面显式标注，绝不把「印证度」偷偷当排序键。
 *
 * 展示单元 = **簇**（P-4 / P8）：行标题取簇标题、来源按机构分组（后端已供）。
 * 档位与日期**全部写 URL query**（`?stage=&date=`，规范第 7 条可分享）。
 */
export default function Page() {
  const [query, , setParams] = useUrlState();
  const [selected, setSelected] = useState<EventItem | null>(null);
  const stage = (query.stage as BoardStage) || "early";
  const date = query.date ?? "";

  const board = useData<BoardResp>({
    path: ENDPOINTS.board,
    params: { stage, date: date || undefined },
  });
  const items = board.data?.items ?? [];

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>榜单</h1>
          <div className="psub">
            按早/晚两个时段看消息 —— 只看**几家在各自报**，不排热度
          </div>
        </div>
        {board.data && (
          <div className="meta">
            <span className="st st-dim num-t" title="该档位的起止时刻">
              {board.data.window_start.slice(5, 16).replace("T", " ")} →{" "}
              {board.data.window_end.slice(5, 16).replace("T", " ")}
            </span>
          </div>
        )}
      </div>

      <div className="filter-bar">
        {STAGES.map((s) => (
          <button
            key={s.key}
            onClick={() => setParams({ stage: s.key, date: "" })}
            className={`chip-btn ${stage === s.key ? "on" : ""}`}
            title={s.hint}
          >
            {s.label}
          </button>
        ))}
        <input
          type="date"
          className="f-sel"
          value={date}
          onChange={(e) => setParams({ date: e.target.value })}
          title="选择日期（留空 = 今天）"
        />
        <span className="st st-dim">共 {board.data?.count ?? 0} 条</span>
      </div>

      <div className="notice-bar">
        <Info size={13} className="shrink-0" />
        <span>
          本版**无热度排序**，只按时间先后 + 一个**印证度标签**（几家在各自报）。
          印证度是客观计数、**不是推荐**，也不代表消息一定可信。
        </span>
      </div>

      <div className="panel">
        {board.loading ? (
          <LoadingBlock rows={8} />
        ) : board.error ? (
          <ErrorState msg={board.error} />
        ) : items.length === 0 ? (
          <EmptyState tip="这一档没有消息 —— 换个档位或日期看看" />
        ) : (
          <EventTable events={items} onSelect={setSelected} />
        )}
      </div>

      <EventDetail event={selected} onClose={() => setSelected(null)} />
    </div>
  );
}
