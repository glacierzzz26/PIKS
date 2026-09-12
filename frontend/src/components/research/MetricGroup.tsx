"use client";

import type { ResearchEvidence } from "@/lib/types";
import { fmt, isColored, numAt, type Row } from "./metricRows";

/** 一节指标表 + 「来源：确定性计算 + Evidence N 条」溯源脚注。 */
export default function MetricGroup({
  title,
  rows,
  data,
  evidence,
}: {
  title: string;
  rows: Row[];
  data: Record<string, unknown> | undefined;
  evidence: ResearchEvidence[];
}) {
  if (!data) return null;
  const shown = rows.filter((r) => data[r.key] !== undefined);
  if (shown.length === 0) return null;
  return (
    <div className="panel panel-pad">
      <div className="mb-3 flex items-center gap-2">
        <h3 className="m-0 text-[15px] font-bold">{title}</h3>
        <span className="num text-[11px] text-faint">
          来源：确定性计算 + Evidence {evidence.length} 条
        </span>
      </div>
      <table className="table">
        <tbody>
          {shown.map((r) => {
            const v = numAt(data, r.key);
            const cls =
              v === null
                ? "text-faint"
                : isColored(r.key)
                  ? v > 0
                    ? "text-up"
                    : v < 0
                      ? "text-down"
                      : "text-muted"
                  : "";
            return (
              <tr key={r.key} className="h-[38px]">
                <td style={{ textAlign: "left" }} className="text-muted">
                  {r.label}
                </td>
                <td className="num-t font-semibold">
                  <span className={cls}>{fmt(v, r.kind)}</span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
