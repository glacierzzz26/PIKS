"use client";

import { Link } from "react-router-dom";
import { Chip } from "@/components/ui/Num";
import type { WatchItem } from "@/lib/types";

/** 自选行：代码/名称 → 个股中心；持有则显现价与盈亏，未持有如实标注。 */
export default function WatchTable({ items }: { items: WatchItem[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="table">
        <thead>
          <tr>
            <th style={{ textAlign: "left" }}>标的</th>
            <th style={{ textAlign: "left" }}>状态</th>
            <th>现价</th>
            <th>成本</th>
            <th>盈亏</th>
            <th style={{ textAlign: "left" }}>备注</th>
          </tr>
        </thead>
        <tbody>
          {items.map((it) => (
            <tr key={it.entity_id}>
              <td style={{ textAlign: "left" }}>
                <Link to={`/stock/${it.code}`} className="chip no-underline hover:border-accent">
                  {it.code}
                </Link>
                <span className="ml-2 font-semibold">{it.name}</span>
              </td>
              <td style={{ textAlign: "left" }}>
                {it.held ? <Chip tone="up">持有</Chip> : <Chip tone="dim">观察</Chip>}
              </td>
              <td className="num-t">{it.position ? it.position.last.toFixed(2) : "—"}</td>
              <td className="num-t">{it.position ? it.position.cost.toFixed(2) : "—"}</td>
              <td className="num-t font-bold">
                {it.position ? (
                  <span style={{ color: it.position.pnl_pct >= 0 ? "var(--red)" : "var(--green)" }}>
                    {it.position.pnl_pct >= 0 ? "+" : ""}
                    {it.position.pnl_pct.toFixed(2)}%
                  </span>
                ) : (
                  "—"
                )}
              </td>
              <td
                className="max-w-[260px] truncate text-[12.5px]"
                style={{ color: "var(--ink-faint)", textAlign: "left" }}
              >
                {it.description || "—"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
