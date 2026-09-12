"use client";

import { useMemo } from "react";
import EChart from "@/components/charts/EChart";
import { useChartTheme } from "@/lib/chartTheme";
import type { MarketSnapshot } from "@/lib/types";
import type { EChartsOption } from "echarts";

/** 涨停梯队两个图表。色值经 useChartTheme 解析，暗色模式下自动同步。 */
export default function LadderCharts({ market }: { market: MarketSnapshot }) {
  const c = useChartTheme();
  const ladder = useMemo(() => buildLadder(market, c), [market, c]);
  const dist = useMemo(() => buildDist(market, c), [market, c]);
  return (
    <>
      <div className="panel panel-pad">
        <h3 className="mb-3 text-[15px] font-bold tracking-wide">连板梯队</h3>
        <EChart option={ladder} height={260} />
      </div>
      <div className="panel panel-pad">
        <h3 className="mb-3 text-[15px] font-bold tracking-wide">行业分布</h3>
        <EChart option={dist} height={260} />
      </div>
    </>
  );
}

function buildLadder(
  market: MarketSnapshot,
  c: ReturnType<typeof useChartTheme>): EChartsOption {
  const groups = new Map<number, string[]>();
  for (const s of market.ladder) {
    (groups.get(s.boards) ?? groups.set(s.boards, []).get(s.boards)!).push(
      `${s.name}(${s.code})`    );
  }
  const boards = Array.from(groups.keys()).sort((a, b) => b - a);
  return {
    grid: { left: 48, right: 24, top: 12, bottom: 24 },
    xAxis: {
      type: "value",
      splitLine: { lineStyle: { color: c.line } },
      axisLabel: { fontSize: 10, color: c.muted },
    },
    yAxis: {
      type: "category",
      data: boards.map((b) => `${b} 板`),
      axisLabel: { fontSize: 11, color: c.muted },
      axisLine: { lineStyle: { color: c.line } },
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
        itemStyle: { color: c.red, borderRadius: [0, 4, 4, 0] },
        label: { show: true, position: "right", fontSize: 11, color: c.muted },
      },
    ],
  };
}

function buildDist(
  market: MarketSnapshot,
  c: ReturnType<typeof useChartTheme>): EChartsOption {
  const rows = [...market.industry_dist].reverse();
  return {
    grid: { left: 70, right: 30, top: 10, bottom: 20 },
    xAxis: {
      type: "value",
      splitLine: { lineStyle: { color: c.line } },
      axisLabel: { fontSize: 10, color: c.muted },
    },
    yAxis: {
      type: "category",
      data: rows.map((d) => d.name),
      axisLabel: { fontSize: 11, color: c.muted },
      axisLine: { show: false },
    },
    series: [
      {
        type: "bar",
        data: rows.map((d) => d.count),
        barWidth: 12,
        itemStyle: { color: c.brand, borderRadius: [0, 4, 4, 0] },
      },
    ],
  };
}
