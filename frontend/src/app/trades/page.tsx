"use client";

import { Suspense } from "react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { TradeRow, PositionRow } from "@/lib/mock/trading";
import { TRADES, POSITIONS } from "@/lib/mock/trading";
import Pagination from "@/components/ui/Pagination";
import { Chip, Num } from "@/components/ui/Num";
import { usePagedQuery } from "@/hooks/usePagedQuery";

type TradesData = { trades: TradeRow[]; positions: PositionRow[] };

/** 交易（只读）：方向筛选 + 分页成交记录 + 持仓快照；录入/AI 解读留在 Go 端 */
export default function Page() {
  return (
    <Suspense fallback={<div className="mt-6 h-40 animate-pulse rounded bg-card" />}>
      <TradesInner />
    </Suspense>
  );
}

function TradesInner() {
  const { query, setFilter, page, size, setPage, setSize, paginate } =
    usePagedQuery();
  const side = query.side ?? "";

  const tr = useData<TradesData>({
    path: ENDPOINTS.trades,
    fallback: () => ({ trades: TRADES, positions: POSITIONS }),
  });
  const records = tr.data?.trades ?? [];
  const positions = tr.data?.positions ?? [];

  const all = side ? records.filter((t) => t.side === side) : records;
  const rows = paginate(all);
  const buys = records.filter((t) => t.side === "buy");
  const sells = records.filter((t) => t.side === "sell");
  const buyAmt = buys.reduce((s, t) => s + t.amount, 0);
  const sellAmt = sells.reduce((s, t) => s + t.amount, 0);

  return (
    <div>
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">交易</h1>
          <span className="text-[13px] text-muted">
            成交记录与持仓快照 · 录入与 AI 解读在 Go 端 /trades
          </span>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          <Chip tone="up">买入 {buys.length} 笔</Chip>
          <Chip tone="down">卖出 {sells.length} 笔</Chip>
          <Chip tone="dim">
            买入额 <Num value={buyAmt} className="inline text-ink" /> 元
          </Chip>
          <Chip tone="dim">
            卖出额 <Num value={sellAmt} className="inline text-ink" /> 元
          </Chip>
        </div>
      </div>

      <div className="mt-4 rounded border border-line bg-card shadow-card">
        <div className="flex h-12 items-center gap-1.5 border-b border-line px-4">
          <h2 className="card-title mb-0">成交记录</h2>
          <span className="mx-2 h-4 w-px bg-line" />
          {[
            { k: "", label: "全部" },
            { k: "buy", label: "买入" },
            { k: "sell", label: "卖出" },
          ].map((o) => (
            <button
              key={o.k}
              onClick={() => setFilter("side", o.k)}
              className={`chip ${side === o.k ? "chip-accent" : "chip-dim"}`}
            >
              {o.label}
            </button>
          ))}
        </div>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-[13.5px]">
            <thead>
              <tr className="border-b border-line-strong text-xs font-semibold text-muted">
                <th className="px-4 py-2.5 text-left">日期</th>
                <th className="px-4 py-2.5 text-left">标的</th>
                <th className="px-4 py-2.5 text-left">方向</th>
                <th className="px-4 py-2.5 text-right">价格</th>
                <th className="px-4 py-2.5 text-right">数量</th>
                <th className="px-4 py-2.5 text-right">金额</th>
                <th className="px-4 py-2.5 text-left">来源</th>
                <th className="px-4 py-2.5 text-left">备注</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((t, i) => (
                <tr
                  key={i}
                  className="h-[46px] border-b border-line last:border-0"
                >
                  <td className="num px-4">{t.date}</td>
                  <td className="px-4">
                    <span className="num chip-dim rounded px-1.5 py-0.5 text-xs text-muted">
                      {t.code}
                    </span>
                    <span className="ml-2 font-semibold">{t.name}</span>
                  </td>
                  <td className="px-4">
                    <Chip tone={t.side === "buy" ? "up" : "down"}>
                      {t.side === "buy" ? "买入" : "卖出"}
                    </Chip>
                  </td>
                  <td className="num px-4">{t.price.toFixed(2)}</td>
                  <td className="num px-4">{t.qty}</td>
                  <td className="num px-4">{t.amount.toLocaleString()}</td>
                  <td className="px-4 text-muted">
                    {t.source === "screenshot" ? "截图识别" : "手动"}
                  </td>
                  <td className="max-w-[180px] truncate px-4 text-muted">
                    {t.note ?? "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <Pagination
          page={page}
          pageSize={size}
          total={all.length}
          onPage={setPage}
          onPageSize={setSize}
        />
      </div>

      <div className="mt-3.5 rounded border border-line bg-card shadow-card">
        <div className="flex h-12 items-center border-b border-line px-4">
          <h2 className="card-title mb-0">当前持仓</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-[13.5px]">
            <thead>
              <tr className="border-b border-line-strong text-xs font-semibold text-muted">
                <th className="px-4 py-2.5 text-left">标的</th>
                <th className="px-4 py-2.5 text-right">数量</th>
                <th className="px-4 py-2.5 text-right">成本</th>
                <th className="px-4 py-2.5 text-right">现价</th>
                <th className="px-4 py-2.5 text-right">盈亏</th>
              </tr>
            </thead>
            <tbody>
              {positions.map((p) => (
                <tr
                  key={p.code}
                  className="h-[46px] border-b border-line last:border-0"
                >
                  <td className="px-4">
                    <span className="num chip-dim rounded px-1.5 py-0.5 text-xs text-muted">
                      {p.code}
                    </span>
                    <span className="ml-2 font-semibold">{p.name}</span>
                  </td>
                  <td className="num px-4">{p.qty}</td>
                  <td className="num px-4">{p.cost.toFixed(2)}</td>
                  <td className="num px-4">{p.last.toFixed(2)}</td>
                  <td className="num px-4 font-bold">
                    <span className={p.pnl_pct >= 0 ? "text-up" : "text-down"}>
                      {p.pnl_pct >= 0 ? "+" : ""}
                      {p.pnl_pct.toFixed(2)}%
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
