"use client";

import { Link } from "react-router-dom";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import WatchOverview from "@/components/watch/WatchOverview";
import TodayFocus from "@/components/home/TodayFocus";
import WatchGroups from "@/components/watch/WatchGroups";
import QuickStart from "@/components/home/QuickStart";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import type { Watchlist } from "@/lib/types";

/**
 * 首页 = 今天（引导式）。设计 phase6/ux-ia.md §2。
 * 编排四段：市场一句话 → 今天该看什么 → 我的自选（分组）→ 快速开始。
 * 空自选 = 首次引导（生产新手的实际第一屏）。盈亏一律带「截至快照日」（数据诚实）。
 */
export default function Page() {
  const wl = useData<Watchlist>({ path: ENDPOINTS.watchlist });
  const items = wl.data?.items ?? [];
  const held = items.filter((i) => i.held);
  const watching = items.filter((i) => !i.held);

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>今天</h1>
          <div className="psub">你的自选、新消息和该做的事，一屏看全</div>
        </div>
        {wl.data && items.length > 0 && (
          <div className="meta">
            <span className="st st-accent">自选 {items.length} 只</span>
            <span className="st st-dim">持有 {held.length} 只</span>
          </div>
        )}
      </div>

      <WatchOverview />

      {wl.loading ? (
        <div className="panel">
          <LoadingBlock rows={6} />
        </div>
      ) : wl.error ? (
        <div className="panel">
          <ErrorState msg={wl.error} />
        </div>
      ) : items.length === 0 ? (
        <FirstRun />
      ) : (
        <>
          <TodayFocus items={items} />
          <WatchGroups
            held={held}
            watching={watching}
            positionDate={wl.data?.position_date ?? ""}
            researched={wl.data?.researched ?? 0}
            onSynced={() => wl.refresh()}
          />
          <QuickStart />
        </>
      )}

      <FooterFinding />
    </div>
  );
}

/** 首次引导：生产新手的实际第一屏（自选只能经同花顺截图镜像同步）。 */
function FirstRun() {
  return (
    <div className="panel panel-pad">
      <h2 className="mb-1 text-[16px] font-bold">还没有自选股</h2>
      <p className="mb-4 text-[13px] leading-relaxed text-muted">
        PIKS 的自选来自你的同花顺自选截图 —— 不用手动维护第二份列表。
      </p>
      <QuickStart />
    </div>
  );
}

/** 页脚：查不在自选里的票（复用既有导流，去黑话）。 */
function FooterFinding() {
  return (
    <p className="mt-5 text-center text-[12px] txt-faint">
      想找一只没在自选里的票？到
      <Link to="/ladder" className="mx-1 no-underline hover:text-accent">
        涨停股
      </Link>
      看当日强势股，或用 ⌘K 搜代码 / 名称。
    </p>
  );
}
