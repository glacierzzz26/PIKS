"use client";

import type { PositionRow } from "@/lib/types";

/** 持仓快照表 */
export default function PositionTable({ positions }: { positions: PositionRow[] }) {
  return (
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
            <tr key={p.code} className="h-[46px] border-b border-line last:border-0">
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
  );
}
