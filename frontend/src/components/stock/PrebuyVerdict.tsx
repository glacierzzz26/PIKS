"use client";

import { AlertOctagon } from "lucide-react";
import type { ResearchMetrics } from "@/lib/types";

const RISK_LABEL: Record<string, string> = { low: "低", medium: "中", high: "高" };
const RISK_TONE: Record<string, string> = { low: "st-dim", medium: "st-amber", high: "st-down" };

/** 评分卡总分 → 结论胶囊色（结论受限/不可得 = 灰，偏正 = 蓝，其余 = 琥珀）。 */
function verdictTone(overall: number | null | undefined): string {
  if (overall == null) return "st-dim";
  if (overall >= 3) return "st-accent";
  if (overall <= -3) return "st-down";
  return "st-amber";
}

/**
 * 买入前结论横幅：评分卡结论 + 风险等级 + 一票否决。
 * 三项均为**规则确定性**结果，不依赖 AI —— 无 LLM 配置时照样给结论。
 */
export default function PrebuyVerdict({ metrics }: { metrics: ResearchMetrics }) {
  const sc = metrics.scorecard;
  const risk = metrics.risk;
  const veto = risk?.veto_buy === true;

  return (
    <div className="mb-4 flex flex-wrap items-center gap-3 rounded-[var(--radius-md)] border border-line bg-bg-soft px-4 py-3">
      <span className="text-[12.5px] text-faint">速评结论</span>
      <span className={`st ${verdictTone(sc?.overall)}`}>
        {sc?.overall_label ?? "数据不足"}
      </span>
      {sc?.overall != null && (
        <span className="num text-[12px] text-muted">总分 {sc.overall > 0 ? "+" : ""}{sc.overall}</span>
      )}
      {risk?.overall_level && (
        <span className="flex items-center gap-1.5 text-[12.5px] text-muted">
          风险
          <span className={`st ${RISK_TONE[risk.overall_level] ?? "st-dim"}`}>
            {RISK_LABEL[risk.overall_level] ?? risk.overall_level}
          </span>
        </span>
      )}
      {veto && (
        <span className="st st-down inline-flex items-center gap-1">
          <AlertOctagon size={12} />
          一票否决：当前风险不宜入场
        </span>
      )}
      {!sc && !risk && (
        <span className="text-[12.5px] text-faint">未取得评分卡与风险数据</span>
      )}
    </div>
  );
}
