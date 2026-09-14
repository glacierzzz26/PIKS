"use client";

import type { ResearchRiskItem } from "@/lib/types";

const CAT_LABEL: Record<string, string> = {
  Market: "市场",
  Financial: "财务",
  Business: "经营",
  Event: "事件",
  Valuation: "估值",
  Liquidity: "流动性",
  Policy: "政策",
  Management: "管理层",
};

const LEVEL_LABEL: Record<string, string> = { low: "低", medium: "中", high: "高" };
const LEVEL_TONE: Record<string, "dim" | "amber" | "down"> = {
  low: "dim",
  medium: "amber",
  high: "down",
};

/**
 * 风险明细（规则判定）：category / level（低中高）/ evidence / description。
 * 这些条目每次深研都算了但从没上屏 —— 买入前最该看的「红线」。
 */
export default function RiskList({ items }: { items: ResearchRiskItem[] }) {
  if (items.length === 0) {
    return (
      <div className="py-3 text-[13px] text-faint italic">
        未发现显著风险信号（规则判定）。
      </div>
    );
  }
  return (
    <table className="table">
      <tbody>
        {items.map((it, i) => (
          <tr key={i} className="h-[40px]">
            <td className="w-[64px] text-left text-muted">
              {CAT_LABEL[it.category] ?? it.category}
            </td>
            <td className="w-[48px] text-left">
              <span className={`st st-${LEVEL_TONE[it.level] ?? "dim"}`}>
                {LEVEL_LABEL[it.level] ?? it.level}
              </span>
            </td>
            <td className="text-left text-[12.5px]">{it.description}</td>
            <td className="num text-right text-[11.5px] text-faint">{it.evidence}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
