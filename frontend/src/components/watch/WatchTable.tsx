"use client";

import { Link } from "react-router-dom";
import { Newspaper } from "lucide-react";
import { Chip } from "@/components/ui/Num";
import type { WatchItem } from "@/lib/types";

/**
 * 自选行：代码/名称 → 个股中心；持有则显现价与盈亏，未持有如实标 `—`。
 * showPnl=false（观察中）时不渲染价格列，避免把无持仓的 `—` 堆成噪声。
 * 徽标：有新消息（红点）/ 未深研（灰点）—— P6-3 富化，纯提示不伪造。
 */
export default function WatchTable({
  items,
  showPnl = false,
}: {
  items: WatchItem[];
  showPnl?: boolean;
}) {
  return (
    <div className="overflow-x-auto">
      <table className="table">
        <thead>
          <tr>
            <th className="text-left">标的</th>
            <th className="text-left">状态</th>
            {showPnl && (
              <>
                <th>现价</th>
                <th>成本</th>
                <th>盈亏</th>
              </>
            )}
            <th className="text-left">最近</th>
          </tr>
        </thead>
        <tbody>
          {items.map((it) => (
            <tr key={it.entity_id}>
              <td className="text-left">
                <Link to={`/stock/${it.code}`} className="chip no-underline hover:border-accent">
                  {it.code || "—"}
                </Link>
                <span className="ml-2 font-semibold">{it.name}</span>
              </td>
              <td className="text-left">
                {it.held ? <Chip tone="up">持有</Chip> : <Chip tone="dim">观察</Chip>}
              </td>
              {showPnl && (
                <>
                  <td className="num-t">{it.position ? it.position.last.toFixed(2) : "—"}</td>
                  <td className="num-t">{it.position ? it.position.cost.toFixed(2) : "—"}</td>
                  <td className="num-t font-bold">
                    {it.position ? (
                      <span className={it.position.pnl_pct >= 0 ? "text-up" : "text-down"}>
                        {it.position.pnl_pct >= 0 ? "+" : ""}
                        {it.position.pnl_pct.toFixed(2)}%
                      </span>
                    ) : (
                      "—"
                    )}
                  </td>
                </>
              )}
              <td className="max-w-[300px] text-left text-[12.5px]">
                <Badges it={it} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** 一行徽标：新消息（点详情）/ 未深研 / 最近深研日。都没有则如实「—」。 */
function Badges({ it }: { it: WatchItem }) {
  const bits: React.ReactNode[] = [];
  if (it.latest_event) {
    bits.push(
      <Link
        key="ev"
        to={`/events/${it.latest_event.latest_id}`}
        className="inline-flex items-center gap-1 no-underline hover:text-accent"
        title={it.latest_event.title}
      >
        <Newspaper size={12} className="text-up" />
        <span className="text-up">新消息</span>
        {it.latest_event.count > 1 && <span className="num txt-faint">×{it.latest_event.count}</span>}
      </Link>
    );
  }
  if (it.code && !it.has_research) {
    bits.push(
      <span key="nr" className="text-amber">
        还没深研
      </span>
    );
  } else if (it.has_research) {
    bits.push(
      <span key="rs" className="num txt-faint">
        深研 {it.latest_research_asof}
      </span>
    );
  }
  if (bits.length === 0) return <span className="txt-faint">—</span>;
  return <span className="inline-flex flex-wrap items-center gap-2">{bits}</span>;
}
