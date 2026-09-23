"use client";

import { ConfidenceBar } from "@/components/ui/Num";
import { SourceLink } from "@/components/ui/SourceLink";
import { corroborationLabel } from "@/lib/constants";
import { useEventTypes } from "@/lib/eventTypes";
import type { EventItem } from "@/lib/types";

// 类型 label 与配色**不在前端**（issue #61）：唯一真源是后端 `model.EventTypes`，
// 经 `useEventTypes()` 取。此处曾硬编码 8 值 TYPE_TAG，与后端 9 值枚举漂移。
// 抽取态两个**在产**取值（issue #80）：extracted = 抽取成功（唯一由 internal/extract
// 写入）；merged = 被聚类并入代表（仍是库里的真实事件）。历史 verified/published 已无
// 写入方（旧 vault 发布器随 P6 下线），后端将其并入 extracted。
export const STATUS_ST: Record<string, { cls: string; label: string }> = {
  extracted: { cls: "st-amber", label: "已抽取" },
  merged: { cls: "st-dim", label: "已被合并" },
};

/** 事件表：点击行打开详情抽屉。 */
export default function EventTable({
  events,
  onSelect,
}: {
  events: EventItem[];
  onSelect: (e: EventItem) => void;
}) {
  const { labelOf, toneOf } = useEventTypes();
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
                {/* 展示单元 = 簇（issue #83 P-4 / P8）：有簇标题就用它，否则本事件标题。 */}
                <b>{e.cluster_title || e.title}</b>
                <span className="meta">
                  来源：<SourceLink source={e.source} url={e.source_url} />
                  {/* 印证度三级（issue #83 P-1）：判据是**独立来源数**（把近逐字转载并成一源后），
                      不是机构数 —— 转载不得把「几家在报」刷高。未聚类/老数据回落机构数或 1。 */}
                  {(() => {
                    const n = e.independent_count ?? e.source_count ?? 1;
                    if (n <= 1) {
                      // 单源：客观陈述，不是异常（独家报道很常见），不用警示色。
                      return <span className="ml-1.5 text-faint">· 单一来源</span>;
                    }
                    return (
                      <span className="ml-1.5 text-accent">
                        · {n} 家印证 · {corroborationLabel(n)}
                      </span>
                    );
                  })()}
                  {e.event_conflicts && e.event_conflicts.length > 0 ? (
                    <span className="ml-1.5 text-[var(--warn)]">
                      · 说法不一致
                    </span>
                  ) : null}
                  {e.summary ? ` · ${e.summary.slice(0, 24)}` : ""}
                </span>
              </div>
            </td>
            <td>
              <span className={`type-tag ${toneOf(e.event_type)}`}>
                {labelOf(e.event_type) ?? e.event_type}
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
