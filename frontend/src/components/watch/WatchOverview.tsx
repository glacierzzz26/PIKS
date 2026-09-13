"use client";

import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import { fmtYi } from "@/lib/format";
import type { DashboardData, MarketSnapshot } from "@/lib/types";

/** 首页顶部紧凑市场概览条：情绪 + 涨停/跌停 + 两市成交 + 指数（数据源 = dashboard 的 market）。 */
export default function WatchOverview() {
  const dash = useData<DashboardData>({ path: ENDPOINTS.dashboard });
  const m = dash.data?.market;

  if (!m) return null; // 概览条非核心：加载中/失败不阻断自选列表（诚实由自选区承载）

  return (
    <div className="panel panel-pad mb-4 flex flex-wrap items-center gap-x-6 gap-y-2">
      <div className="flex items-baseline gap-2">
        <span className="text-[12px] text-faint">市场</span>
        <span className="num text-[13px] font-semibold">{m.trade_date}</span>
      </div>
      <Emotion m={m} />
      <Stat label="涨停" value={m.limit_up} tone="up" />
      <Stat label="跌停" value={m.limit_down} tone="down" />
      <Stat label="炸板" value={m.broken_limit} tone="amber" />
      <span className="num text-[12.5px] text-muted">
        两市成交 <b>{fmtYi(m.turnover_yi)}</b>
      </span>
      <div className="ml-auto flex flex-wrap items-center gap-x-4 gap-y-1">
        {m.indices.map((ix) => (
          <span key={ix.name} className="num text-[12.5px]">
            <span className="text-muted">{ix.name}</span>{" "}
            <b style={{ color: ix.change_pct >= 0 ? "var(--red)" : "var(--green)" }}>
              {ix.change_pct >= 0 ? "+" : ""}
              {ix.change_pct.toFixed(2)}%
            </b>
          </span>
        ))}
      </div>
    </div>
  );
}

function Emotion({ m }: { m: MarketSnapshot }) {
  const score = Math.max(0, Math.min(100, m.emotion_score));
  return (
    <span className="flex items-center gap-2">
      <span className="text-[12px] text-faint">情绪</span>
      <span className="pct-bar" style={{ width: 72 }}>
        <i style={{ width: `${score}%` }} />
      </span>
      <span className="num text-[12.5px] font-semibold">{m.emotion_state}</span>
      <span className="num text-[12px] text-faint">{Math.round(score)}</span>
    </span>
  );
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: "up" | "down" | "amber";
}) {
  const color =
    tone === "up" ? "var(--red)" : tone === "down" ? "var(--green)" : "var(--amber, #b45309)";
  return (
    <span className="num text-[12.5px]">
      <span className="text-muted">{label}</span>{" "}
      <b style={{ color }}>{value}</b>
    </span>
  );
}
