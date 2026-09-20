"use client";

import { EVENT_TYPE_LABEL } from "@/lib/format";
import { ConfidenceBar } from "@/components/ui/Num";
import { SourceLink } from "@/components/ui/SourceLink";
import type { EventItem } from "@/lib/types";

export const TYPE_TAG: Record<string, string> = {
  policy: "t-mix",
  earnings: "t-idx",
  product_launch: "t-ev",
  supply_agreement: "t-bond",
  industry_event: "t-idx",
  investment: "t-mix",
  sales_data: "t-ev",
  rumor: "t-gray",
};
export const STATUS_ST: Record<string, { cls: string; label: string }> = {
  confirmed: { cls: "st-down", label: "已确认" },
  pending: { cls: "st-amber", label: "待复核" },
  archived: { cls: "st-dim", label: "已归档" },
};

/** 事件表：点击行打开详情抽屉。 */
export default function EventTable({
  events,
  onSelect,
}: {
  events: EventItem[];
  onSelect: (e: EventItem) => void;
}) {
  return (
    <table className="table">
      <thead>
        <tr>
          <th className="text-left">事件标题</th>
          <th>类型</th>
          <th>影响实体</th>
          <th>发生时间</th>
          <th>置信度</th>
          <th>状态</th>
        </tr>
      </thead>
      <tbody>
        {events.map((e) => (
          <tr key={e.id} className="cursor-pointer" onClick={() => onSelect(e)}>
            <td>
              <div className="ev-title">
                <b>{e.title}</b>
                <span className="meta">
                  来源：<SourceLink source={e.source} url={e.source_url} />
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
  );
}
