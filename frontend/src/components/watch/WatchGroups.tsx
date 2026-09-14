"use client";

import { useState } from "react";
import { Link } from "react-router-dom";
import { ChevronDown, RefreshCw } from "lucide-react";
import WatchTable from "@/components/watch/WatchTable";
import { EmptyState } from "@/components/ui/States";
import type { WatchItem } from "@/lib/types";

/**
 * 「我的自选」：持有中 / 观察中两组（设计 phase6/ux-ia.md §2 区块4）。
 * 盈亏带「截至 {快照日}」——无实时行情源，快照真实但过期（数据诚实硬约束）。
 * 引入/移出只经截图镜像同步，故此处提供「同步自选截图」入口。
 */
export default function WatchGroups({
  held,
  watching,
  positionDate,
  researched,
  onSynced,
}: {
  held: WatchItem[];
  watching: WatchItem[];
  positionDate: string;
  researched: number;
  onSynced: () => void;
}) {
  const [open, setOpen] = useState(true);
  return (
    <>
      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>持有中</h2>
          <span className="hint">
            {held.length} 只
            {positionDate && held.length > 0 && <> · 盈亏截至 {positionDate} 快照</>}
          </span>
        </div>
        {held.length === 0 ? (
          <div className="panel">
            <EmptyState tip="当前没有持仓。持仓来自同花顺持仓截图，同步后会显示在这里。" />
          </div>
        ) : (
          <div className="panel">
            <WatchTable items={held} showPnl />
          </div>
        )}
      </section>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>观察中</h2>
          <span className="hint">
            {watching.length} 只 · 深研覆盖 {researched}/{held.length + watching.length}
          </span>
          <button
            onClick={() => setOpen((v) => !v)}
            className="inline-flex items-center gap-1 text-[12px] text-muted hover:text-accent"
          >
            <ChevronDown size={13} className={open ? "" : "-rotate-90"} />
            {open ? "收起" : "展开"}
          </button>
        </div>
        {!open ? null : watching.length === 0 ? (
          <div className="panel">
            <EmptyState tip="观察中的票为空。" />
          </div>
        ) : (
          <div className="panel">
            <WatchTable items={watching} />
          </div>
        )}
      </section>

      <div className="panel panel-pad flex flex-wrap items-center gap-3">
        <span className="text-[13px] text-muted">
          自选与持仓都来自同花顺截图。有新股票要加入？到「交易与持仓」同步自选截图。
        </span>
        <Link
          to="/trades"
          onClick={onSynced}
          className="ml-auto inline-flex h-8 items-center gap-1.5 rounded-[10px] border border-line bg-card px-3 text-xs font-semibold text-muted no-underline hover:border-accent hover:text-accent"
        >
          <RefreshCw size={13} />
          同步自选截图
        </Link>
      </div>
    </>
  );
}
