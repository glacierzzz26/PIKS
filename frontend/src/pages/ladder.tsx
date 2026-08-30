"use client";

import { useMemo } from "react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import { fmtYi, fmtWan } from "@/lib/format";
import EChart from "@/components/charts/EChart";
import { Chip, Num } from "@/components/ui/Num";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { MarketSnapshot } from "@/lib/types";
import type { EChartsOption } from "echarts";

/** 涨停梯队（对齐 dev market 视图）：最新快照 + 连板阶梯 + 行业分布 + 涨停池表 */
export default function Page() {
  const market = useData<MarketSnapshot>({ path: ENDPOINTS.marketSnapshot });
  const m = market.data;

  if (market.loading) {
    return <div className="mt-6"><LoadingBlock rows={8} /></div>;
  }
  if (market.error) {
    return (
      <div className="mt-6 rounded border border-line bg-card shadow-card">
        <ErrorState msg={market.error} />
      </div>
    );
  }
  if (!m) {
    return (
      <div className="mt-6 rounded border border-line bg-card shadow-card">
        <EmptyState tip="当日市场快照尚未生成（交易日 17:00 后更新）" />
      </div>
    );
  }
  return (
    <div>
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">涨停梯队</h1>
          <span className="num text-[13px] text-muted">{m.trade_date}</span>
          <Chip tone="accent">最高 {m.max_board} 板</Chip>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          <Chip tone="up">涨停 {m.limit_up}</Chip>
          <Chip tone="down">跌停 {m.limit_down}</Chip>
          <Chip tone="amber">炸板 {m.broken_limit}</Chip>
          <Chip tone="dim">两市成交 {fmtYi(m.turnover_yi)}</Chip>
          <Chip tone="accent">
            情绪 {m.emotion_score} · {m.emotion_state}
          </Chip>
        </div>
      </div>

      <div className="mt-4 grid grid-cols-12 gap-3.5">
        <div className="rounded border border-line bg-card p-[18px] shadow-card xl:col-span-7">
          <h2 className="card-title">连板梯队</h2>
          <LadderChart market={m} />
        </div>
        <div className="rounded border border-line bg-card p-[18px] shadow-card xl:col-span-5">
          <h2 className="card-title">行业分布</h2>
          <DistChart market={m} />
        </div>
      </div>

      <div className="mt-3.5 overflow-hidden rounded border border-line bg-card shadow-card">
        <div className="flex h-12 items-center border-b border-line px-4">
          <h2 className="card-title mb-0">涨停池</h2>
          <span className="num ml-auto text-xs text-muted">
            共 {m.ladder.length} 只
          </span>
        </div>
        <LadderTable market={m} />
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
        splitLine: { lineStyle: { color: "#e9edf3" } },
        axisLabel: { fontSize: 10, color: "#5c6b80" },
      },
      yAxis: {
        type: "category",
        data: boards.map((b) => `${b} 板`),
        axisLabel: { fontSize: 11, color: "#5c6b80" },
        axisLine: { lineStyle: { color: "#e2e7ef" } },
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
          itemStyle: { color: "#e5484d", borderRadius: [0, 4, 4, 0] },
          label: { show: true, position: "right", fontSize: 11, color: "#5c6b80" },
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
        splitLine: { lineStyle: { color: "#e9edf3" } },
        axisLabel: { fontSize: 10, color: "#5c6b80" },
      },
      yAxis: {
        type: "category",
        data: rows.map((d) => d.name),
        axisLabel: { fontSize: 11, color: "#5c6b80" },
        axisLine: { show: false },
      },
      series: [
        {
          type: "bar",
          data: rows.map((d) => d.count),
          barWidth: 12,
          itemStyle: { color: "#2f6bff", borderRadius: [0, 4, 4, 0] },
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
      <table className="w-full border-collapse text-[13.5px]">
        <thead>
          <tr className="border-b border-line-strong text-xs font-semibold text-muted">
            <th className="px-4 py-2.5 text-left">代码 / 名称</th>
            <th className="px-4 py-2.5 text-right">连板</th>
            <th className="px-4 py-2.5 text-left">行业</th>
            <th className="px-4 py-2.5 text-left">涨停原因</th>
            <th className="px-4 py-2.5 text-right">封单额</th>
            <th className="px-4 py-2.5 text-right">首封时间</th>
            <th className="px-4 py-2.5 text-right">换手</th>
            <th className="px-4 py-2.5 text-right">流通市值</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((s) => (
            <tr
              key={s.code}
              className="h-[46px] border-b border-line last:border-0 hover:bg-accent-soft"
            >
              <td className="px-4">
                <span className="num chip-dim rounded px-1.5 py-0.5 text-xs text-muted">
                  {s.code}
                </span>
                <span className="ml-2 text-[13px] font-semibold">{s.name}</span>
              </td>
              <td className="px-4 text-right">
                <span
                  className={`num inline-block min-w-7 rounded-full px-2 text-[13px] font-bold ${
                    s.boards >= 5 ? "chip-up" : s.boards >= 3 ? "text-up" : ""
                  }`}
                >
                  {s.boards}
                </span>
              </td>
              <td className="px-4 text-muted">{s.industry}</td>
              <td className="max-w-[200px] truncate px-4">{s.reason}</td>
              <td className="num px-4">{fmtWan(s.seal_amount)}</td>
              <td className="num px-4">{s.first_time}</td>
              <td className="num px-4">{s.turnover.toFixed(1)}%</td>
              <td className="num px-4">{s.float_mv} 亿</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
