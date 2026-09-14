"use client";

import { useNavigate } from "react-router-dom";
import { EVENT_TYPE_LABEL } from "@/lib/format";
import { ConfidenceBar } from "@/components/ui/Num";
import { StockSectionEmpty } from "@/components/stock/StockHeader";
import type { StockEvent } from "@/lib/types";

/** 个股中心「相关事件」区块：affects 到该公司实体的事件（时间倒序）。 */
export default function StockEvents({ events }: { events: StockEvent[] }) {
  const navigate = useNavigate();

  if (events.length === 0) {
    return <StockSectionEmpty tip="暂无相关事件（该股尚未被事件抽取关联）" />;
  }

  return (
    <div className="overflow-x-auto">
      <table className="table">
        <thead>
          <tr>
            <th className="text-left">时间</th>
            <th className="text-left">类型</th>
            <th className="text-left">标题</th>
            <th className="text-left">来源</th>
            <th className="text-left">置信</th>
          </tr>
        </thead>
        <tbody>
          {events.map((e) => (
            <tr
              key={e.id}
              role="button"
              tabIndex={0}
              className="cursor-pointer"
              onClick={() => navigate(`/events/${e.id}`)}
              onKeyDown={(ev) => {
                if (ev.key === "Enter" || ev.key === " ") {
                  ev.preventDefault();
                  navigate(`/events/${e.id}`);
                }
              }}
            >
              <td className="num-t text-muted">
                {e.occurred_at.slice(0, 10)}
              </td>
              <td>
                <span className="type-tag t-ev">
                  {EVENT_TYPE_LABEL[e.event_type] ?? e.event_type}
                </span>
              </td>
              <td className="text-left font-medium">
                {e.title}
              </td>
              <td className="text-[12.5px] text-faint text-left">
                {e.source}
              </td>
              <td>
                <ConfidenceBar v={e.confidence} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
