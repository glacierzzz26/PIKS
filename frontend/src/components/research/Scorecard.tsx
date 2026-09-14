"use client";

import { Chip } from "@/components/ui/Num";
import type { ResearchMetrics } from "@/lib/types";
import { DIM_LABEL } from "./metricRows";

/** 评分卡：确定性打分（null = 数据不可得，如实标空，不猜分）。 */
export default function Scorecard({ metrics }: { metrics: ResearchMetrics }) {
  const sc = metrics.scorecard;
  if (!sc?.dimensions?.length) return null;
  return (
    <div className="panel panel-pad">
      <div className="mb-3 flex items-center gap-2">
        <h3 className="m-0 text-[15px] font-bold">评分卡</h3>
        <Chip tone={sc.overall == null ? "dim" : sc.overall >= 3 ? "accent" : "amber"}>
          {sc.overall_label ?? "—"}
        </Chip>
      </div>
      <table className="table">
        <tbody>
          {sc.dimensions.map((d) => (
            <tr key={d.dimension} className="h-[38px]">
              <td className="text-left text-muted">
                {DIM_LABEL[d.dimension] ?? d.dimension}
              </td>
              <td className="num-t font-semibold">
                {d.unavailable ? (
                  <span className="text-faint">数据不可得</span>
                ) : (
                  <span className={d.score && d.score > 0 ? "text-up" : "text-muted"}>
                    {d.score}
                  </span>
                )}
              </td>
              <td className="text-left text-[12px] text-faint">
                {d.reason}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
