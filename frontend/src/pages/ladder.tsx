"use client";

import { useMemo } from "react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import { fmtYi, fmtWan } from "@/lib/format";
import EChart from "@/components/charts/EChart";
import { Num } from "@/components/ui/Num";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { MarketSnapshot } from "@/lib/types";
import type { EChartsOption } from "echarts";

/** 涨停梯队（对齐 dev market 视图）：最新快照 + 连板阶梯 + 行业分布 + 涨停池表 */
export default function Page() {
  const market = useData<MarketSnapshot>({ path: ENDPOINTS.marketSnapshot });
  const m = market.data;

  if (market.loading) {
    return (
      <div className="panel mt-6">
        <LoadingBlock rows={8} />
      </div>
    );
  }
  if (market.error) {
    return (
      <div className="panel mt-6">
        <ErrorState msg={market.error} />
      </div>
    );
  }
  if (!m) {
    return (
      <div className="panel mt-6">
        <EmptyState tip="当日市场快照尚未生成（交易日 17:00 后更新）" />
      </div>
    );
  }
  return (
    <div>
      <div className="page-head">
        <div>
          <h1>涨停梯队</h1>
          <div className="psub">最新快照 · 连板阶梯 · 行业分布 · 涨停池</div>
        </div>
        <div className="meta">
          <span className="st st-accent">{m.trade_date}</span>
          <span className="st st-up">最高 {m.max_board} 板</span>
          <span className="st st-up">涨停 {m.limit_up}</span>
          <span className="st st-down">跌停 {m.limit_down}</span>
          <span className="st st-amber">炸板 {m.broken_limit}</span>
          <span className="st st-dim">两市成交 {fmtYi(m.turnover_yi)}</span>
        </div>
      </div>

      <div className="two-col">
        <div className="panel panel-pad">
          <h2 className="mb-3 text-[15px] font-bold tracking-wide">连板梯队</h2>
          <LadderChart market={m} />
        </div>
        <div className="panel panel-pad">
          <h2 className="mb-3 text-[15px] font-bold tracking-wide">行业分布</h2>
          <DistChart market={m} />
        </div>
      </div>

      <div className="section">
        <div className="panel">
          <div className="flex h-12 items-center border-b border-line px-4">
            <h2 className="mb-0 text-[15px] font-bold tracking-wide">涨停池</h2>
            <span className="num ml-auto text-xs text-faint">
              共 {m.ladder.length} 只
            </span>
          </div>
          <LadderTable market={m} />
        </div>
      </div>
    </div>
  );
}

function LadderChart({ market }: { market: MarketSnapshot }) {
  const option = useMemo<EChartsOption>(() => {
    const groups = new Map<number, string[]>();
    for (const s of market.ladder) {
      (groups.get(s.boards) ?? groups.set(s.boards, []).get(s.boards)!).push(
        `${s.name}(${s.code})`
      );
    }
    const boards = Array.from(groups.keys()).sort((a, b) => b - a);
    return {
      grid: { left: 48, right: 24, top: 12, bottom: 24 },
      xAxis: {
        type: "value",
        splitLine: { lineStyle: { color: "#e7ebf2" } },
        axisLabel: { fontSize: 10, color: "#5b6678" },
      },
      yAxis: {
        type: "category",
        data: boards.map((b) => `${b} 板`),
        axisLabel: { fontSize: 11, color: "#5b6678" },
        axisLine: { lineStyle: { color: "#e7ebf2" } },
      },
      tooltip: {
        trigger: "item",
        formatter: (params) => {
          const p = Array.isArray(params) ? params[0] : params;
          const b = boards[p.dataIndex];
          return `<b>${b} 板梯队</b><br/>${groups.get(b)!.join("<br/>")}`;
        },
      },
      series: [
        {
          type: "bar",
          data: boards.map((b) => groups.get(b)!.length),
          barWidth: 18,
          itemStyle: { color: "#e0392b", borderRadius: [0, 4, 4, 0] },
          label: { show: true, position: "right", fontSize: 11, color: "#5b6678" },
        },
      ],
    };
  }, [market]);
  return <EChart option={option} height={260} />;
}

function DistChart({ market }: { market: MarketSnapshot }) {
  const option = useMemo<EChartsOption>(() => {
    const rows = [...market.industry_dist].reverse();
    return {
      grid: { left: 70, right: 30, top: 10, bottom: 20 },
      xAxis: {
        type: "value",
        splitLine: { lineStyle: { color: "#e7ebf2" } },
        axisLabel: { fontSize: 10, color: "#5b6678" },
      },
      yAxis: {
        type: "category",
        data: rows.map((d) => d.name),
        axisLabel: { fontSize: 11, color: "#5b6678" },
        axisLine: { show: false },
      },
      series: [
        {
          type: "bar",
          data: rows.map((d) => d.count),
          barWidth: 12,
          itemStyle: { color: "#28457e", borderRadius: [0, 4, 4, 0] },
        },
      ],
    };
  }, [market]);
  return <EChart option={option} height={260} />;
}

function LadderTable({ market }: { market: MarketSnapshot }) {
  const rows = useMemo(
    () => [...market.ladder].sort((a, b) => b.boards - a.boards),
    [market]
  );
  return (
    <div className="overflow-x-auto">
      <table className="table">
        <thead>
          <tr>
            <th style={{ textAlign: "left" }}>代码 / 名称</th>
            <th>连板</th>
            <th style={{ textAlign: "left" }}>行业</th>
            <th style={{ textAlign: "left" }}>涨停原因</th>
            <th>封单额</th>
            <th>首封时间</th>
            <th>换手</th>
            <th>流通市值</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((s) => (
            <tr key={s.code}>
              <td style={{ textAlign: "left" }}>
                <span className="num rounded-[5px] bg-bg-soft px-1.5 py-0.5 text-xs text-faint">
                  {s.code}
                </span>
                <span className="ml-2 text-[13px] font-semibold">{s.name}</span>
              </td>
              <td>
                <span
                  className={`num inline-block min-w-6 font-bold ${
                    s.boards >= 5
                      ? "rounded-full bg-red-soft px-2 text-up"
                      : s.boards >= 3
                        ? "text-up"
                        : ""
                  }`}
                >
                  {s.boards}
                </span>
              </td>
              <td style={{ textAlign: "left" }} className="text-muted">
                {s.industry}
              </td>
              <td style={{ textAlign: "left" }} className="max-w-[200px] truncate">
                {s.reason}
              </td>
              <td className="num-t">{fmtWan(s.seal_amount)}</td>
              <td className="num-t">{s.first_time}</td>
              <td className="num-t">{s.turnover.toFixed(1)}%</td>
              <td className="num-t">{s.float_mv} 亿</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
