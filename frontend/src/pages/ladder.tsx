"use client";

import { useMemo } from "react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import { fmtYi, fmtWan } from "@/lib/format";
import LadderCharts from "@/components/ladder/LadderCharts";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { MarketSnapshot } from "@/lib/types";

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

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>连板梯队 与 行业分布</h2>
          <span className="hint">
            最高 {m.max_board} 板 · 涨停 {m.limit_up} 家
          </span>
        </div>
        <div className="two-col">
          <LadderCharts market={m} />
        </div>
      </section>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>涨停池</h2>
          <span className="hint">共 {m.ladder.length} 只 · 按连板数降序</span>
        </div>
        <div className="panel">
          <LadderTable market={m} />
        </div>
      </section>
    </div>
  );
}

function LadderTable({ market }: { market: MarketSnapshot }) {
  const rows = useMemo(
    () => [...market.ladder].sort((a, b) => b.boards - a.boards),
    [market]  );
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
                <span className="chip">{s.code}</span>
                <span className="ml-2 font-semibold">{s.name}</span>
              </td>
              <td className="num-t">
                {s.boards >= 5 ? (
                  <span className="st st-up">{s.boards} 板</span>
                ) : (
                  <span className={s.boards >= 3 ? "text-up" : ""}>
                    {s.boards}
                  </span>
                )}
              </td>
              <td style={{ textAlign: "left", color: "var(--ink-faint)" }}>
                {s.industry}
              </td>
              <td
                className="max-w-[200px] truncate text-[12.5px]"
                style={{ textAlign: "left", color: "var(--ink-faint)" }}
              >
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
