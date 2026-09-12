"use client";

import DeepResearchButton from "@/components/research/DeepResearchButton";
import type { PositionRow } from "@/lib/types";

/** 持仓快照表（持仓行带「深研」入口，§4.8） */
export default function PositionTable({ positions }: { positions: PositionRow[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="table">
        <thead>
          <tr>
            <th style={{ textAlign: "left" }}>标的</th>
            <th>数量</th>
            <th>成本</th>
            <th>现价</th>
            <th>盈亏</th>
            <th style={{ textAlign: "left" }}>深研</th>
          </tr>
        </thead>
        <tbody>
          {positions.map((p) => (
            <tr key={p.code}>
              <td>
                <span className="chip">{p.code}</span>
                <span className="ml-2 font-semibold">{p.name}</span>
              </td>
              <td className="num-t">{p.qty}</td>
              <td className="num-t">{p.cost.toFixed(2)}</td>
              <td className="num-t">{p.last.toFixed(2)}</td>
              <td className="num-t font-bold">
                <span
                  style={{ color: p.pnl_pct >= 0 ? "var(--red)" : "var(--green)" }}
                >
                  {p.pnl_pct >= 0 ? "+" : ""}
                  {p.pnl_pct.toFixed(2)}%
                </span>
              </td>
              <td>
                {p.code ? <DeepResearchButton code={p.code} /> : "—"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
